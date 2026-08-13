package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type asyncImageMemoryStore struct {
	mu     sync.RWMutex
	tasks  map[string]*service.ImageTaskRecord
	active map[int64]map[string]struct{}
}

func (s *asyncImageMemoryStore) Save(_ context.Context, task *service.ImageTaskRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *task
	copy.Result = append(json.RawMessage(nil), task.Result...)
	copy.Error = append(json.RawMessage(nil), task.Error...)
	s.tasks[task.ID] = &copy
	return nil
}

func (s *asyncImageMemoryStore) Get(_ context.Context, id string) (*service.ImageTaskRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task := s.tasks[id]
	if task == nil {
		return nil, service.ErrImageTaskNotFound
	}
	copy := *task
	copy.Result = append(json.RawMessage(nil), task.Result...)
	copy.Error = append(json.RawMessage(nil), task.Error...)
	return &copy, nil
}

func (s *asyncImageMemoryStore) Acquire(_ context.Context, apiKeyID int64, taskID string, maxActive int, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil {
		s.active = make(map[int64]map[string]struct{})
	}
	if s.active[apiKeyID] == nil {
		s.active[apiKeyID] = make(map[string]struct{})
	}
	if len(s.active[apiKeyID]) >= maxActive {
		return false, nil
	}
	s.active[apiKeyID][taskID] = struct{}{}
	return true, nil
}

func (s *asyncImageMemoryStore) Release(_ context.Context, apiKeyID int64, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active[apiKeyID], taskID)
	if len(s.active[apiKeyID]) == 0 {
		delete(s.active, apiKeyID)
	}
	return nil
}

func TestAsyncImageHandlerSubmitAndPoll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	release := make(chan struct{})
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(_ string, c *gin.Context) {
		<-release
		c.JSON(http.StatusOK, gin.H{"created": 123, "data": []gin.H{{"url": "https://example.test/image.png"}}})
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)
	router.GET("/v1/images/tasks/:task_id", h.Get)

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"cat"}`)).WithContext(requestCtx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, "3", w.Header().Get("Retry-After"))

	var accepted struct {
		TaskID  string `json:"task_id"`
		Status  string `json:"status"`
		PollURL string `json:"poll_url"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &accepted))
	require.Equal(t, service.ImageTaskStatusProcessing, accepted.Status)
	require.Equal(t, "/v1/images/tasks/"+accepted.TaskID, accepted.PollURL)
	require.Equal(t, accepted.PollURL, w.Header().Get("Location"))

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"dog"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondWriter := httptest.NewRecorder()
	router.ServeHTTP(secondWriter, secondReq)
	require.Equal(t, http.StatusTooManyRequests, secondWriter.Code)
	require.Equal(t, "3", secondWriter.Header().Get("Retry-After"))
	require.Contains(t, secondWriter.Body.String(), "IMAGE_TASK_ALREADY_ACTIVE")

	// The detached background request must survive completion of/cancellation
	// from the short submission request.
	cancelRequest()
	close(release)
	require.Eventually(t, func() bool {
		got, err := tasks.Get(context.Background(), service.ImageTaskOwner{UserID: 7, APIKeyID: 9}, accepted.TaskID)
		return err == nil && got.Status == service.ImageTaskStatusCompleted
	}, time.Second, 10*time.Millisecond)

	pollReq := httptest.NewRequest(http.MethodGet, accepted.PollURL, nil)
	pollWriter := httptest.NewRecorder()
	router.ServeHTTP(pollWriter, pollReq)
	require.Equal(t, http.StatusOK, pollWriter.Code)
	require.Equal(t, "no-store", pollWriter.Header().Get("Cache-Control"))
	require.Empty(t, pollWriter.Header().Get("Retry-After"))
	require.Contains(t, pollWriter.Body.String(), "https://example.test/image.png")
}

func TestAsyncImageHandlerAllowsConfiguredConcurrentTasks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute).SetMaxActivePerAPIKey(4)
	release := make(chan struct{})
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(_ string, c *gin.Context) {
		<-release
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"url": "https://example.test/image.png"}}})
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID: 9, UserID: 7, GroupID: &groupID,
			Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)

	submit := func(prompt string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async",
			strings.NewReader(`{"model":"gpt-image-2","prompt":"`+prompt+`","n":1}`))
		req.Header.Set("Content-Type", "application/json")
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, req)
		return writer
	}

	taskIDs := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		writer := submit(fmt.Sprintf("image-%d", i))
		require.Equal(t, http.StatusAccepted, writer.Code)
		var response struct {
			TaskID string `json:"task_id"`
		}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &response))
		require.NotEmpty(t, response.TaskID)
		taskIDs = append(taskIDs, response.TaskID)
	}

	fifth := submit("over-limit")
	require.Equal(t, http.StatusTooManyRequests, fifth.Code)
	require.Equal(t, "3", fifth.Header().Get("Retry-After"))
	require.Contains(t, fifth.Body.String(), "IMAGE_TASK_ALREADY_ACTIVE")

	close(release)
	require.Eventually(t, func() bool {
		for _, taskID := range taskIDs {
			task, err := tasks.Get(context.Background(), service.ImageTaskOwner{UserID: 7, APIKeyID: 9}, taskID)
			if err != nil || task.Status != service.ImageTaskStatusCompleted {
				return false
			}
		}
		return true
	}, time.Second, 10*time.Millisecond)

	sixth := submit("after-release")
	require.Equal(t, http.StatusAccepted, sixth.Code)
}

// When object storage is not configured the feature is fully disabled: the
// endpoints must return 404 without creating a task or writing to Redis.
func TestAsyncImageHandlerDisabledReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithOptions(store, time.Hour, time.Minute) // enabled == false
	h := &AsyncImageHandler{tasks: tasks}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)
	router.GET("/v1/images/tasks/:task_id", h.Get)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"cat"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "not enabled")

	pollReq := httptest.NewRequest(http.MethodGet, "/v1/images/tasks/imgtask_missing", nil)
	pollWriter := httptest.NewRecorder()
	router.ServeHTTP(pollWriter, pollReq)
	require.Equal(t, http.StatusNotFound, pollWriter.Code)

	// No task was created / persisted.
	require.Empty(t, store.tasks)
}

func TestAsyncImageHandlerRejectsMultipleOutputsBeforeTaskCreation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(string, *gin.Context) {}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID: 9, UserID: 7, GroupID: &groupID,
			Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-2","prompt":"cat","n":2}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "n must be 1")
	require.Empty(t, store.tasks, "invalid multi-output requests must be rejected before task creation")
	require.Empty(t, store.active)
}

func TestParseImageResizeHeaders(t *testing.T) {
	header := http.Header{}
	header.Set(imageOutputSizeHeader, "3840x2160")
	header.Set(imageResizeFilterHeader, "lanczos")
	resize, err := parseImageResizeHeaders(header)
	require.NoError(t, err)
	require.Equal(t, &service.ImageResizeSpec{Width: 3840, Height: 2160, Filter: "lanczos"}, resize)

	header.Set(imageResizeFilterHeader, "nearest")
	_, err = parseImageResizeHeaders(header)
	require.ErrorContains(t, err, "unsupported resize filter")

	header.Set(imageResizeFilterHeader, "lanczos")
	header.Set(imageOutputSizeHeader, "4000x2160")
	_, err = parseImageResizeHeaders(header)
	require.ErrorContains(t, err, "must not exceed 3840")
}

func TestAsyncImageHandlerRejectsMultipartMultipleOutputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AsyncImageHandler{openAI: &OpenAIGatewayHandler{gatewayService: &service.OpenAIGatewayService{}}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	const boundary = "sub2api-multipart-n-guard"
	body := asyncImageMultipartEditBody(boundary, "edit this image", "2", "not-a-real-image")
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits/async", strings.NewReader(body))
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	c.Request = req

	err := h.validateRequest(c, service.PlatformOpenAI, []byte(body))
	require.EqualError(t, err, "n must be 1 for asynchronous image tasks")
}

func asyncImageMultipartEditBody(boundary, prompt, n, imageBytes string) string {
	return strings.Join([]string{
		"--" + boundary,
		`Content-Disposition: form-data; name="model"`,
		"",
		"gpt-image-2",
		"--" + boundary,
		`Content-Disposition: form-data; name="prompt"`,
		"",
		prompt,
		"--" + boundary,
		`Content-Disposition: form-data; name="n"`,
		"",
		n,
		"--" + boundary,
		`Content-Disposition: form-data; name="image"; filename="input.png"`,
		"Content-Type: image/png",
		"",
		imageBytes,
		"--" + boundary + "--",
		"",
	}, "\r\n")
}

func TestAsyncImageHandlerPollHidesOtherAPIKeyAndIsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	owner := service.ImageTaskOwner{UserID: 7, APIKeyID: 9}
	created, err := tasks.Create(context.Background(), owner)
	require.NoError(t, err)
	require.NoError(t, tasks.Complete(context.Background(), created.ID, http.StatusOK,
		json.RawMessage(`{"created":123,"data":[{"url":"https://example.test/image.png"}]}`)))

	h := &AsyncImageHandler{tasks: tasks}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		apiKeyID := int64(9)
		if c.GetHeader("X-Test-API-Key") == "other" {
			apiKeyID = 10
		}
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID: apiKeyID, UserID: 7, GroupID: &groupID,
			Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.GET("/v1/images/tasks/:task_id", h.Get)

	otherReq := httptest.NewRequest(http.MethodGet, "/v1/images/tasks/"+created.ID, nil)
	otherReq.Header.Set("X-Test-API-Key", "other")
	otherWriter := httptest.NewRecorder()
	router.ServeHTTP(otherWriter, otherReq)
	require.Equal(t, http.StatusNotFound, otherWriter.Code)
	require.Contains(t, otherWriter.Body.String(), "IMAGE_TASK_NOT_FOUND")
	require.NotContains(t, otherWriter.Body.String(), "image.png")

	var firstBody string
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/images/tasks/"+created.ID, nil)
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, req)
		require.Equal(t, http.StatusOK, writer.Code)
		require.Equal(t, "no-store", writer.Header().Get("Cache-Control"))
		if i == 0 {
			firstBody = writer.Body.String()
		} else {
			require.JSONEq(t, firstBody, writer.Body.String())
		}
	}
}

func TestValidateAsyncImageSuccessResponse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "valid URL", body: `{"created":1,"data":[{"url":"https://cdn.test/image.png"}]}`},
		{name: "valid base64", body: `{"data":[{"b64_json":"aGVsbG8="}]}`},
		{name: "top-level error", body: `{"error":{"type":"image_generation_user_error","message":"blocked"}}`, wantErr: "image error"},
		{name: "missing data", body: `{"created":1}`, wantErr: "no image data"},
		{name: "empty data", body: `{"data":[]}`, wantErr: "no image data"},
		{name: "missing image payload", body: `{"data":[{"revised_prompt":"cat"}]}`, wantErr: "neither b64_json nor url"},
		{name: "invalid JSON", body: `{`, wantErr: "invalid image response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAsyncImageSuccessResponse([]byte(tt.body))
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestAsyncImageHandlerRunTreatsTopLevelErrorAsFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	owner := service.ImageTaskOwner{UserID: 7, APIKeyID: 9}
	created, err := tasks.Create(context.Background(), owner)
	require.NoError(t, err)

	baseWriter := httptest.NewRecorder()
	baseCtx, _ := gin.CreateTestContext(baseWriter)
	baseCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{}`))
	taskCtx, recorder, cancel := newAsyncImageContext(baseCtx, []byte(`{}`), time.Minute)
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(_ string, c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"error": gin.H{"type": "image_generation_user_error", "message": "blocked"}})
	}

	h.run(created.ID, service.PlatformOpenAI, taskCtx, recorder, cancel)
	got, err := tasks.Get(context.Background(), owner, created.ID)
	require.NoError(t, err)
	require.Equal(t, service.ImageTaskStatusFailed, got.Status)
	require.Equal(t, http.StatusBadGateway, got.HTTPStatus)
	require.JSONEq(t, `{"type":"image_generation_user_error","message":"blocked"}`, string(got.Error))
	require.Nil(t, got.Result)
}
