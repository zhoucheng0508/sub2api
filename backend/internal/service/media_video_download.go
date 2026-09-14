package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

var ErrMediaVideoDownloadLimited = errors.New("video download limit reached; retry later")
var ErrMediaVideoDownloadUnavailable = errors.New("video download protection is temporarily unavailable")

type MediaVideoDownloadLimiter interface {
	Acquire(context.Context, int64, MediaVideoDownloadSettings) (MediaVideoDownloadPermit, error)
}

type MediaVideoDownloadPermit interface {
	Context() context.Context
	WaitN(context.Context, int) error
	Release()
}

func (s *MediaVideoService) streamContent(ctx context.Context, task *MediaVideoTask, w http.ResponseWriter, r *http.Request) (statusCode int, resultErr error) {
	if task == nil || !task.Downloadable || task.UpstreamTaskID == "" || task.ExpiresAt == nil || !task.ExpiresAt.After(time.Now()) {
		return http.StatusNotFound, errMediaVideoTaskNotFound
	}
	started := time.Now()
	var transferred int64
	defer func() {
		slog.Info("media_video.download_finished", "task_id", task.TaskID, "user_id", task.UserID,
			"http_status", statusCode, "bytes_sent", transferred, "duration_ms", time.Since(started).Milliseconds(), "interrupted", resultErr != nil)
	}()
	if s.downloadLimiter == nil || s.settings == nil {
		return 503, ErrMediaVideoDownloadUnavailable
	}
	limits, err := s.settings.GetMediaVideoDownloadSettingsCached(ctx)
	if err != nil {
		return 503, ErrMediaVideoDownloadUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(limits.MaxDurationSeconds)*time.Second)
	defer cancel()
	if s.ctx != nil {
		stop := context.AfterFunc(s.ctx, cancel)
		defer stop()
	}
	permit, err := s.downloadLimiter.Acquire(ctx, task.UserID, limits)
	if err != nil {
		w.Header().Set("Retry-After", "5")
		if errors.Is(err, ErrMediaVideoDownloadLimited) {
			return 429, err
		}
		return 503, ErrMediaVideoDownloadUnavailable
	}
	defer permit.Release()
	ctx = permit.Context()
	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil {
		return 503, ErrMediaVideoDownloadUnavailable
	}
	// Context cancellation closes the upstream body, but cannot interrupt a
	// blocked downstream Write by itself. A write deadline also handles that case.
	var deadlineMu sync.Mutex
	interrupted := make(chan struct{})
	stopInterrupt := context.AfterFunc(ctx, func() {
		deadlineMu.Lock()
		_ = controller.SetWriteDeadline(time.Now())
		deadlineMu.Unlock()
		close(interrupted)
	})
	defer func() {
		if !stopInterrupt() {
			<-interrupted
		}
		_ = controller.SetWriteDeadline(time.Time{})
	}()
	account, err := s.accounts.GetByID(ctx, task.UpstreamAccountID)
	if err != nil || account == nil {
		return 503, errors.New("upstream account unavailable")
	}
	resp, status, err := s.callResponse(ctx, *account, http.MethodGet, "/v1/media/videos/"+task.UpstreamTaskID+"/content", nil, "", r.Header.Get("Range"))
	if err != nil {
		return status, err
	}
	defer func() { _ = resp.Body.Close() }()
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Content-Disposition", "Accept-Ranges", "Retry-After"} {
		if value := resp.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(status)
	// Do not use io.Copy's ReaderFrom/WriterTo fast paths: every chunk, including
	// Range responses, must pass through both shared byte budgets.
	buffer := make([]byte, 32768)
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if err := permit.WaitN(ctx, n); err != nil {
				return status, err
			}
			deadlineMu.Lock()
			writeErr := ctx.Err()
			if writeErr == nil {
				deadline := time.Now().Add(time.Duration(limits.WriteTimeoutSeconds) * time.Second)
				if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
					deadline = end
				}
				writeErr = controller.SetWriteDeadline(deadline)
			}
			deadlineMu.Unlock()
			if writeErr != nil {
				return status, writeErr
			}
			written, writeErr := w.Write(buffer[:n])
			transferred += int64(written)
			if writeErr != nil {
				return status, writeErr
			}
			if written != n {
				return status, io.ErrShortWrite
			}
			if err := controller.Flush(); err != nil {
				return status, err
			}
			// Deliberate bandwidth throttling is not a stalled client write.
			deadlineMu.Lock()
			if ctx.Err() == nil {
				writeErr = controller.SetWriteDeadline(time.Time{})
			}
			deadlineMu.Unlock()
			if writeErr != nil {
				return status, writeErr
			}
		}
		if errors.Is(readErr, io.EOF) {
			return status, nil
		}
		if readErr != nil {
			return status, readErr
		}
		if err := ctx.Err(); err != nil {
			return status, err
		}
	}
}
