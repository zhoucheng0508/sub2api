package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type imageTaskMemoryStore struct {
	task         *ImageTaskRecord
	ttl          time.Duration
	saveErr      error
	getErr       error
	releaseErr   error
	releaseCalls int
	active       map[int64]string
}

func (s *imageTaskMemoryStore) Save(_ context.Context, task *ImageTaskRecord, ttl time.Duration) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	copy := *task
	s.task = &copy
	s.ttl = ttl
	return nil
}

func (s *imageTaskMemoryStore) Get(_ context.Context, _ string) (*ImageTaskRecord, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.task == nil {
		return nil, ErrImageTaskNotFound
	}
	copy := *s.task
	return &copy, nil
}

func (s *imageTaskMemoryStore) Acquire(_ context.Context, apiKeyID int64, taskID string, _ time.Duration) (bool, error) {
	if s.active == nil {
		s.active = make(map[int64]string)
	}
	if s.active[apiKeyID] != "" {
		return false, nil
	}
	s.active[apiKeyID] = taskID
	return true, nil
}

func (s *imageTaskMemoryStore) Release(_ context.Context, apiKeyID int64, taskID string) error {
	s.releaseCalls++
	if s.releaseErr != nil {
		return s.releaseErr
	}
	if s.active[apiKeyID] == taskID {
		delete(s.active, apiKeyID)
	}
	return nil
}

func TestImageTaskServiceLifecycleAndOwnership(t *testing.T) {
	store := &imageTaskMemoryStore{}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, 10*time.Minute)
	owner := ImageTaskOwner{UserID: 7, APIKeyID: 9}

	created, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusProcessing, created.Status)
	require.Equal(t, created.ID, created.TaskID)
	require.Equal(t, "image.generation.task", created.Object)
	require.Equal(t, time.Hour, store.ttl)
	require.Equal(t, owner.UserID, store.task.UserID)
	require.Equal(t, owner.APIKeyID, store.task.APIKeyID)

	_, err = svc.Get(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 10}, created.ID)
	require.ErrorIs(t, err, ErrImageTaskNotFound)

	result := json.RawMessage(`{"created":123,"data":[{"url":"https://example.test/image.png"}]}`)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, result))

	completed, err := svc.Get(context.Background(), owner, created.ID)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusCompleted, completed.Status)
	require.Equal(t, http.StatusOK, completed.HTTPStatus)
	require.Equal(t, "https://example.test/image.png", completed.ImageURL)
	require.JSONEq(t, string(result), string(completed.Result))
	require.NotNil(t, completed.CompletedAt)

	next, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)
	require.NotEqual(t, created.ID, next.ID)
}

func TestImageTaskServiceRejectsSecondActiveTaskForSameAPIKey(t *testing.T) {
	store := &imageTaskMemoryStore{}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, 10*time.Minute)
	owner := ImageTaskOwner{UserID: 7, APIKeyID: 9}

	_, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), owner)
	require.ErrorIs(t, err, ErrImageTaskActive)

	_, err = svc.Create(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 10})
	require.NoError(t, err)
}

func TestImageTaskServiceInvalidResultBecomesFailed(t *testing.T) {
	store := &imageTaskMemoryStore{}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, time.Minute)
	created, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 1, APIKeyID: 2})
	require.NoError(t, err)

	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, json.RawMessage(`not-json`)))
	got, err := svc.Get(context.Background(), ImageTaskOwner{UserID: 1, APIKeyID: 2}, created.ID)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusFailed, got.Status)
	require.Equal(t, http.StatusBadGateway, got.HTTPStatus)
	require.Contains(t, string(got.Error), "non-JSON")
}

func TestImageTaskServiceMapsStoreFailures(t *testing.T) {
	store := &imageTaskMemoryStore{saveErr: errors.New("redis down")}
	svc := NewImageTaskService(store)

	_, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 1, APIKeyID: 2})
	require.ErrorIs(t, err, ErrImageTaskUnavailable)
	require.Empty(t, store.active)
}

func TestImageTaskServiceFailedTaskReleasesActiveSlot(t *testing.T) {
	store := &imageTaskMemoryStore{}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, time.Minute)
	owner := ImageTaskOwner{UserID: 7, APIKeyID: 9}

	created, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)
	require.NoError(t, svc.Fail(context.Background(), created.ID, http.StatusBadGateway,
		json.RawMessage(`{"type":"api_error","message":"upstream failed"}`)))
	require.Empty(t, store.active)

	next, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)
	require.NotEqual(t, created.ID, next.ID)
}

func TestImageTaskServiceTerminalSaveFailureStillAttemptsSlotRelease(t *testing.T) {
	store := &imageTaskMemoryStore{}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, time.Minute)
	owner := ImageTaskOwner{UserID: 7, APIKeyID: 9}

	created, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)
	store.saveErr = errors.New("redis write failed")

	err = svc.Fail(context.Background(), created.ID, http.StatusBadGateway,
		json.RawMessage(`{"type":"api_error","message":"upstream failed"}`))
	require.ErrorIs(t, err, ErrImageTaskUnavailable)
	require.Equal(t, 1, store.releaseCalls)
	require.Empty(t, store.active, "terminal persistence failure must not strand the per-key active slot")
	require.Equal(t, ImageTaskStatusProcessing, store.task.Status, "failed terminal persistence must not pretend the task was stored")
}
