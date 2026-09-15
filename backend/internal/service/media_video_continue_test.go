package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMediaVideoReplayConflictRetainsOriginalOrder(t *testing.T) {
	future := time.Now().Add(time.Hour)
	request := MediaVideoCreateRequest{Model: "seedance2.5", Prompt: "original", Duration: 30, Ratio: "16:9", Resolution: "720p"}
	body, _ := json.Marshal(request)
	task := &MediaVideoTask{TaskID: "original", Status: "creating", SubmissionState: "uncertain", ExpiresAt: &future, RequestHash: hashBytes(body)}
	s := &MediaVideoService{repo: &videoContinueRepo{task: task}}
	changed := request
	changed.Prompt = "different"
	if _, status, err := s.Replay(context.Background(), 1, 1, "same-key", changed); status != http.StatusConflict || !errors.Is(err, ErrMediaVideoIdempotencyConflict) {
		t.Fatalf("changed request: status=%d err=%v", status, err)
	}
	if actual, status, err := s.Replay(context.Background(), 1, 1, "same-key", request); err != nil || status != http.StatusOK || actual != task || actual.SubmissionState != "uncertain" {
		t.Fatalf("original request: status=%d err=%v", status, err)
	}
}

type videoContinueRepo struct {
	MediaVideoRepository
	task *MediaVideoTask
}

type videoPreparedRepo struct {
	MediaVideoRepository
	mu      sync.Mutex
	task    MediaVideoTask
	claimed bool
}

func (r *videoPreparedRepo) GetByIdempotency(context.Context, int64, int64, string) (*MediaVideoTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.task
	return &task, nil
}
func (r *videoPreparedRepo) ClaimSubmission(context.Context, string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.claimed {
		return false, nil
	}
	r.claimed = true
	r.task.SubmissionState = "submitting"
	return true, nil
}
func (r *videoPreparedRepo) UpdateUpstream(_ context.Context, _ string, upstream, status string, _ int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.task.UpstreamTaskID = upstream
	r.task.Status = status
	r.task.SubmissionState = "accepted"
	return nil
}

type videoPreparedAccounts struct {
	AccountRepository
	account Account
}

func (r *videoPreparedAccounts) GetByID(context.Context, int64) (*Account, error) {
	return &r.account, nil
}

func TestMediaVideoPreparedConcurrentContinuationClaimsSupplierOnce(t *testing.T) {
	request := MediaVideoCreateRequest{Model: "seedance2.5", Prompt: "test", Duration: 30, Ratio: "16:9", Resolution: "720p"}
	body, _ := json.Marshal(request)
	future := time.Now().Add(time.Hour)
	repo := &videoPreparedRepo{task: MediaVideoTask{TaskID: "original", Status: "creating", SubmissionState: "prepared", BillingStatus: "held", UpstreamAccountID: 1, ExpiresAt: &future, RequestHash: hashBytes(body)}}
	var mu sync.Mutex
	submits := 0
	s := &MediaVideoService{repo: repo, accounts: &videoPreparedAccounts{account: Account{ID: 1, Platform: PlatformLaogou, Credentials: map[string]any{"api_key": "test-only"}}}, http: videoHeaderClient(func(r *http.Request) (*http.Response, error) {
		response := `{"models":[{"id":"seedance2.5","durations":[30],"max_reference_images":1}],"ratios":["16:9"],"resolutions":["720p"],"max_image_bytes":12582912,"max_images_total_bytes":73400320}`
		if r.Method == http.MethodPost {
			mu.Lock()
			submits++
			mu.Unlock()
			response = `{"task_id":"provider-task","status":"queued"}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	var wait sync.WaitGroup
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			task, status, err := s.Create(WithMediaVideoContinueOnly(context.Background()), 1, 1, nil, "original-key", request)
			if err != nil || status < 200 || status >= 300 {
				t.Errorf("continue: status=%d err=%v", status, err)
			}
			if task != nil && (task.SubmissionState == "prepared" || (task.UpstreamTaskID != "" && task.SubmissionState != "accepted")) {
				t.Errorf("continuation returned a stale submission state: %s", task.SubmissionState)
			}
		}()
	}
	wait.Wait()
	if submits != 1 {
		t.Fatalf("supplier submissions=%d", submits)
	}
}

func (r *videoContinueRepo) GetByIdempotency(context.Context, int64, int64, string) (*MediaVideoTask, error) {
	return r.task, nil
}

// Unimplemented repository/account methods deliberately panic if continuation
// ever falls through to inserting or scheduling another order.
func TestMediaVideoContinueOnlyDoesNotCreateMissingOrExpiredOrder(t *testing.T) {
	request := MediaVideoCreateRequest{Model: "seedance2.5", Prompt: "test", Duration: 30, Ratio: "16:9", Resolution: "720p"}
	past := time.Now().Add(-time.Hour)
	for _, task := range []*MediaVideoTask{nil, {Status: "creating", SubmissionState: "prepared", ExpiresAt: &past}} {
		s := &MediaVideoService{repo: &videoContinueRepo{task: task}}
		_, status, err := s.Create(WithMediaVideoContinueOnly(context.Background()), 1, 1, nil, "original-key", request)
		if err == nil || status != http.StatusNotFound {
			t.Fatalf("status=%d error=%v; expected no new order", status, err)
		}
	}
}

func TestMediaVideoContinueOnlyNeverResubmitsAnAcceptedOrUncertainOrder(t *testing.T) {
	request := MediaVideoCreateRequest{Model: "seedance2.5", Prompt: "test", Duration: 30, Ratio: "16:9", Resolution: "720p"}
	body, _ := json.Marshal(request)
	future := time.Now().Add(time.Hour)
	for _, submission := range []string{"submitting", "uncertain", "accepted"} {
		task := &MediaVideoTask{TaskID: "original", Status: "creating", SubmissionState: submission, ExpiresAt: &future, RequestHash: hashBytes(body)}
		if submission == "accepted" {
			task.Status = "running"
			task.UpstreamTaskID = "provider-id"
		}
		s := &MediaVideoService{repo: &videoContinueRepo{task: task}}
		actual, status, err := s.Create(WithMediaVideoContinueOnly(context.Background()), 1, 1, nil, "original-key", request)
		if err != nil || status != http.StatusOK || actual != task {
			t.Fatalf("%s: task=%v status=%d err=%v", submission, actual, status, err)
		}
	}
	if mediaVideoDefiniteRejection(http.StatusConflict) {
		t.Fatal("provider 409 must retain the uncertain order")
	}
}

type videoHeaderClient func(*http.Request) (*http.Response, error)

func (f videoHeaderClient) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestMediaVideoDownloadForwardsIfRange(t *testing.T) {
	s := &MediaVideoService{http: videoHeaderClient(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Range") != "bytes=4-7" || r.Header.Get("If-Range") != `"version-1"` {
			t.Fatal("download validators not forwarded")
		}
		return &http.Response{StatusCode: 206, Header: http.Header{"Etag": {`"version-1"`}}, Body: io.NopCloser(strings.NewReader("data"))}, nil
	})}
	response, _, err := s.callResponse(context.Background(), Account{Credentials: map[string]any{"api_key": "test-only"}}, http.MethodGet, "/v1/media/videos/test/content", nil, "", "bytes=4-7", `"version-1"`)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
}
