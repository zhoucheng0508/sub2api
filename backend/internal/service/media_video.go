package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

const SeedanceLaogouBaseURL = "https://api.laogou.org"

type MediaVideoTask struct {
	TaskID, UpstreamTaskID                                                 string
	UserID, APIKeyID, GroupID, UpstreamAccountID                           int64
	Model, PromptHash, Ratio, Resolution                                   string
	Duration                                                               int
	HasImages                                                              bool
	IdempotencyKeyHash, RequestHash, Status                                string
	Progress                                                               *float64
	Downloadable                                                           bool
	Error                                                                  json.RawMessage
	CreatedAt, UpdatedAt, CompletedAt, ExpiresAt, LastPolledAt, NextPollAt *time.Time
	// BillingStatus tracks the durable hold/capture/release state.
	BillingStatus                 string
	PriceSnapshot, HoldAmount     float64
	ActualAmount                  *float64
	Currency                      string
	HeldAt, SettledAt, ReleasedAt *time.Time
	SettlementAttempts            int
	NextSettlementAt              *time.Time
	LastBillingError              string
	SubmissionState, PollToken    string
	PollFailures                  int
}

type MediaVideoRepository interface {
	Balance(context.Context, int64) (float64, float64, error)
	Insert(context.Context, *MediaVideoTask) (bool, error)
	Get(context.Context, string, int64, int64) (*MediaVideoTask, error)
	GetByIdempotency(context.Context, int64, int64, string) (*MediaVideoTask, error)
	List(context.Context, int64, int64, int, string, string) ([]*MediaVideoTask, bool, string, error)
	ListPollable(context.Context, time.Time, int) ([]*MediaVideoTask, error)
	ClaimPollable(context.Context, string, time.Time) (*MediaVideoTask, error)
	ListBillingRecovery(context.Context, time.Time, int) ([]*MediaVideoTask, error)
	ListExpiredForRelease(context.Context, time.Time, int) ([]*MediaVideoTask, error)
	UpdateUpstream(context.Context, string, string, string, int64) error
	UpdateStatus(context.Context, string, string, *float64, bool, json.RawMessage, time.Time, time.Time) error
	MarkPolled(context.Context, string, time.Time, time.Time) error
	DeleteExpired(context.Context, time.Time, int) error
	Delete(context.Context, string) error
	UpdateBilling(context.Context, string, string, *float64, string, time.Time) error
	ClaimSubmission(context.Context, string) (bool, error)
	RecoverSubmissions(context.Context) error
}

func (s *MediaVideoService) Balance(ctx context.Context, userID int64) (float64, float64, error) {
	return s.repo.Balance(ctx, userID)
}

type MediaVideoHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type MediaVideoService struct {
	repo            MediaVideoRepository
	accounts        AccountRepository
	http            MediaVideoHTTPClient
	billing         UsageBillingRepository
	authCache       APIKeyAuthCacheInvalidator
	billingCache    *BillingCacheService
	downloadLimiter MediaVideoDownloadLimiter
	settings        *SettingService
	capabilities    mediaVideoCapabilityCache
	ctx             context.Context
	cancel          context.CancelFunc
	stop            chan struct{}
	done            chan struct{}
	stopOnce        sync.Once
	httpMu          sync.Mutex
	proxyClients    map[string]*mediaVideoProxyClient
	ownedTransport  *http.Transport
	httpStopped     bool
}

const DefaultMediaVideoPricePerRequest = 2.0

func ResolveMediaVideoPrice(group *Group) float64 {
	if group != nil && group.VideoPricePerRequest != nil && *group.VideoPricePerRequest >= 0 {
		return *group.VideoPricePerRequest
	}
	return DefaultMediaVideoPricePerRequest
}

func NewMediaVideoService(repo MediaVideoRepository, accounts AccountRepository, httpClient MediaVideoHTTPClient) *MediaVideoService {
	s := newMediaVideoService(repo, accounts, httpClient)
	go s.pollLoop()
	return s
}

func newMediaVideoService(repo MediaVideoRepository, accounts AccountRepository, httpClient MediaVideoHTTPClient) *MediaVideoService {
	var transport *http.Transport
	if httpClient == nil {
		transport = newMediaVideoTransport()
		httpClient = newMediaVideoHTTPClient(transport)
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	s := &MediaVideoService{repo: repo, accounts: accounts, http: httpClient, ctx: workerCtx, cancel: cancel, stop: make(chan struct{}), done: make(chan struct{})}
	s.ownedTransport = transport
	return s
}

func ProvideMediaVideoService(repo MediaVideoRepository, accounts AccountRepository, billing UsageBillingRepository, authCache APIKeyAuthCacheInvalidator, httpClient MediaVideoHTTPClient, billingCache *BillingCacheService, downloadLimiter MediaVideoDownloadLimiter, settings *SettingService) *MediaVideoService {
	s := newMediaVideoService(repo, accounts, httpClient)
	s.billing, s.authCache = billing, authCache
	s.billingCache = billingCache
	s.downloadLimiter, s.settings = downloadLimiter, settings
	go s.pollLoop()
	return s
}

func mediaVideoBalanceCommand(task *MediaVideoTask, action string, actual float64) *BatchImageBalanceHoldCommand {
	requestID := BatchImageHoldRequestID(task.TaskID)
	if action == "capture" {
		requestID = BatchImageCaptureRequestID(task.TaskID)
	}
	if action == "release" {
		requestID = BatchImageReleaseRequestID(task.TaskID)
	}
	return &BatchImageBalanceHoldCommand{RequestID: requestID, APIKeyID: task.APIKeyID, UserID: task.UserID, BatchID: task.TaskID, HoldAmount: task.HoldAmount, ActualAmount: actual, RequestPayloadHash: task.RequestHash}
}

func (s *MediaVideoService) ensureBalanceHeld(ctx context.Context, task *MediaVideoTask) error {
	if task == nil {
		return errors.New("media video task is required")
	}
	if task.BillingStatus == "held" || task.BillingStatus == "settled" {
		return nil
	}
	if s.billing == nil {
		return errors.New("media video billing repository is unavailable")
	}
	_, err := s.billing.ReserveBatchImageBalance(ctx, mediaVideoBalanceCommand(task, "hold", 0))
	if err != nil {
		return err
	}
	if err := s.repo.UpdateBilling(ctx, task.TaskID, "held", nil, "", time.Now()); err != nil {
		return err
	}
	task.BillingStatus = "held"
	if s.billingCache != nil {
		_ = s.billingCache.InvalidateUserBalance(ctx, task.UserID)
		_ = s.billingCache.InvalidateAPIKeyRateLimit(ctx, task.APIKeyID)
	}
	if s.authCache != nil {
		s.authCache.InvalidateAuthCacheByUserID(ctx, task.UserID)
	}
	return nil
}

func (s *MediaVideoService) captureBalance(ctx context.Context, task *MediaVideoTask) error {
	if task == nil || task.BillingStatus == "settled" {
		return nil
	}
	if s.billing == nil {
		return errors.New("media video billing repository is unavailable")
	}
	_, err := s.billing.CaptureBatchImageBalance(ctx, mediaVideoBalanceCommand(task, "capture", task.PriceSnapshot))
	if err != nil {
		if !errors.Is(err, ErrMediaVideoStateConflict) {
			_ = s.repo.UpdateBilling(ctx, task.TaskID, "settlement_pending", nil, err.Error(), time.Now())
		}
		return err
	}
	actual := task.PriceSnapshot
	if err := s.repo.UpdateBilling(ctx, task.TaskID, "settled", &actual, "", time.Now()); err != nil {
		return err
	}
	task.BillingStatus, task.ActualAmount = "settled", &actual
	if s.billingCache != nil {
		_ = s.billingCache.InvalidateUserBalance(ctx, task.UserID)
		_ = s.billingCache.InvalidateAPIKeyRateLimit(ctx, task.APIKeyID)
	}
	if s.authCache != nil {
		s.authCache.InvalidateAuthCacheByUserID(ctx, task.UserID)
	}
	return nil
}

func (s *MediaVideoService) releaseBalance(ctx context.Context, task *MediaVideoTask) error {
	if task == nil || task.BillingStatus == "released" || task.BillingStatus == "settled" || task.BillingStatus == "not_billed" {
		return nil
	}
	if s.billing == nil {
		return errors.New("media video billing repository is unavailable")
	}
	_, err := s.billing.ReleaseBatchImageBalance(ctx, mediaVideoBalanceCommand(task, "release", 0))
	if err != nil {
		if !errors.Is(err, ErrMediaVideoStateConflict) {
			_ = s.repo.UpdateBilling(ctx, task.TaskID, "release_pending", nil, err.Error(), time.Now())
		}
		return err
	}
	if err := s.repo.UpdateBilling(ctx, task.TaskID, "released", nil, "", time.Now()); err != nil {
		return err
	}
	task.BillingStatus = "released"
	if s.billingCache != nil {
		_ = s.billingCache.InvalidateUserBalance(ctx, task.UserID)
		_ = s.billingCache.InvalidateAPIKeyRateLimit(ctx, task.APIKeyID)
	}
	if s.authCache != nil {
		s.authCache.InvalidateAuthCacheByUserID(ctx, task.UserID)
	}
	return nil
}

func mediaVideoBillingErrorStatus(err error) int {
	if errors.Is(err, ErrMediaVideoKeyLimit) {
		return http.StatusTooManyRequests
	}
	if errors.Is(err, ErrBatchImageInsufficientBalance) {
		return http.StatusPaymentRequired
	}
	return http.StatusInternalServerError
}
func (s *MediaVideoService) Stop() {
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.stop != nil {
			close(s.stop)
		}
		s.closeHTTPConnections()
	})
	if s.done != nil {
		<-s.done
	}
}

type MediaVideoCreateRequest struct {
	Model      string   `json:"model"`
	Prompt     string   `json:"prompt"`
	Duration   int      `json:"duration"`
	Ratio      string   `json:"ratio"`
	Resolution string   `json:"resolution"`
	Images     []string `json:"images,omitempty"`
}

// BeforeSubmit runs after resolving the durable account binding and before
// claiming a submission. The HTTP handler supplies account audit and capacity checks.
type MediaVideoBeforeSubmit func(*Account) (func(), error)

func (s *MediaVideoService) Create(ctx context.Context, userID, apiKeyID int64, group *Group, idem string, req MediaVideoCreateRequest, beforeSubmit ...MediaVideoBeforeSubmit) (*MediaVideoTask, int, error) {
	var groupID *int64
	if group != nil {
		groupID = &group.ID
	}
	if strings.TrimSpace(idem) == "" {
		return nil, http.StatusBadRequest, errors.New("Idempotency-Key is required")
	}
	imageSizes, validationErr := validateMediaVideoRequestBasics(req)
	if validationErr != nil {
		return nil, http.StatusBadRequest, validationErr
	}
	requestBytes, _ := json.Marshal(req)
	requestHash := hashBytes(requestBytes)
	idemHash := hashString(idem)
	var existingTask *MediaVideoTask
	if old, err := s.repo.GetByIdempotency(ctx, userID, apiKeyID, idemHash); err != nil {
		return nil, 500, err
	} else if old != nil && old.ExpiresAt != nil && old.ExpiresAt.After(time.Now()) {
		if old.RequestHash != requestHash {
			return nil, http.StatusConflict, errors.New("Idempotency-Key was reused with a different request")
		}
		if old.Status != "creating" || old.UpstreamTaskID != "" || old.SubmissionState == "submitting" || old.SubmissionState == "uncertain" {
			return old, http.StatusOK, nil
		}
		existingTask = old
	}
	var accounts []Account
	var err error
	if existingTask != nil {
		bound, getErr := s.accounts.GetByID(ctx, existingTask.UpstreamAccountID)
		if getErr != nil || bound == nil || bound.Platform != PlatformLaogou {
			return existingTask, http.StatusServiceUnavailable, errors.New("bound Laogou account is unavailable")
		}
		accounts = []Account{*bound}
	} else if groupID != nil {
		accounts, err = s.accounts.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformLaogou)
	} else {
		accounts, err = s.accounts.ListSchedulableByPlatform(ctx, PlatformLaogou)
	}
	if err != nil {
		return nil, 500, err
	}
	if len(accounts) == 0 {
		return nil, http.StatusServiceUnavailable, errors.New("no available Laogou account")
	}
	a, validationStatus, validationErr := s.selectVideoAccount(ctx, accounts, req, imageSizes)
	if validationErr != nil {
		return existingTask, validationStatus, validationErr
	}
	now := time.Now()
	exp := now.Add(24 * time.Hour)
	task := existingTask
	if task == nil {
		price := ResolveMediaVideoPrice(group)
		task = &MediaVideoTask{TaskID: "vid_" + strings.ReplaceAll(uuid.NewString(), "-", ""), UserID: userID, APIKeyID: apiKeyID, Model: req.Model, PromptHash: hashString(req.Prompt), Duration: req.Duration, Ratio: req.Ratio, Resolution: req.Resolution, HasImages: len(req.Images) > 0, IdempotencyKeyHash: idemHash, RequestHash: requestHash, Status: "creating", Downloadable: false, CreatedAt: &now, UpdatedAt: &now, ExpiresAt: &exp, NextPollAt: &now, BillingStatus: "hold_pending", PriceSnapshot: price, HoldAmount: price, Currency: "internal"}
	}
	if groupID != nil {
		task.GroupID = *groupID
	}
	task.UpstreamAccountID = a.ID
	created := existingTask != nil
	if existingTask == nil {
		created, err = s.repo.Insert(ctx, task)
	}
	if err != nil {
		return nil, infraerrors.Code(err), err
	}
	if !created {
		old, getErr := s.repo.GetByIdempotency(ctx, userID, apiKeyID, idemHash)
		if getErr != nil {
			return nil, http.StatusInternalServerError, getErr
		}
		if old != nil {
			if old.RequestHash != requestHash {
				return nil, http.StatusConflict, errors.New("Idempotency-Key was reused with a different request")
			}
			if old.Status != "creating" || old.UpstreamTaskID != "" || old.SubmissionState == "submitting" || old.SubmissionState == "uncertain" {
				return old, http.StatusOK, nil
			}
			existingTask = old
			task = old
			if bound, boundErr := s.accounts.GetByID(ctx, old.UpstreamAccountID); boundErr == nil && bound != nil && bound.Platform == PlatformLaogou {
				a, validationStatus, validationErr = s.selectVideoAccount(ctx, []Account{*bound}, req, imageSizes)
				if validationErr != nil {
					return old, validationStatus, validationErr
				}
			} else {
				return old, http.StatusServiceUnavailable, errors.New("bound Laogou account is unavailable")
			}
		}
		if existingTask == nil {
			return nil, http.StatusInternalServerError, errors.New("video task idempotency record was not persisted")
		}
	}
	if task.SubmissionState == "submitting" || task.SubmissionState == "uncertain" {
		return task, http.StatusAccepted, nil
	}
	if len(beforeSubmit) > 0 && beforeSubmit[0] != nil {
		release, checkErr := beforeSubmit[0](&a)
		if checkErr != nil {
			return task, http.StatusTooManyRequests, checkErr
		}
		if release != nil {
			defer release()
		}
	}
	if err := s.ensureBalanceHeld(ctx, task); err != nil {
		return task, mediaVideoBillingErrorStatus(err), err
	}
	claimed, claimErr := s.repo.ClaimSubmission(ctx, task.TaskID)
	if claimErr != nil {
		return task, http.StatusServiceUnavailable, claimErr
	}
	if !claimed {
		return task, http.StatusAccepted, nil
	}
	task.SubmissionState = "submitting"
	body, _ := json.Marshal(req)
	createCtx, cancelCreate := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelCreate()
	upstreamIdem := mediaVideoUpstreamIdempotencyKey(userID, apiKeyID, task.TaskID, idem)
	upstream, status, err := s.callCreateOnce(createCtx, a, body, upstreamIdem)
	if err != nil {
		if !mediaVideoDefiniteRejection(status) {
			return task, status, err
		}
		upstreamErr := extractMediaVideoError(upstream)
		persistCtx, cancelPersist := context.WithTimeout(context.Background(), 10*time.Second)
		if persistErr := s.repo.UpdateStatus(persistCtx, task.TaskID, "failed", nil, false, upstreamErr, time.Now(), time.Now()); persistErr != nil {
			slog.Error("media_video.failure_status_persist_failed", "task_id", task.TaskID, "error", persistErr)
		}
		cancelPersist()
		if releaseErr := s.releaseBalance(context.Background(), task); releaseErr != nil {
			slog.Error("media_video.create_failure_release_failed", "task_id", task.TaskID, "error", releaseErr)
		}
		return task, status, err
	}
	var envelope map[string]any
	_ = json.Unmarshal(upstream, &envelope)
	upID := laogouStringValue(envelope, "task_id")
	if upID == "" {
		upID = laogouStringValue(envelope, "id")
	}
	if upID == "" {
		return task, http.StatusBadGateway, errors.New("upstream response missing task id")
	}
	st := laogouStringValue(envelope, "status")
	if st == "" {
		st = "queued"
	}
	st = normalizeMediaVideoStatus(st)
	persistStatus := st
	if st == "succeeded" || st == "failed" {
		persistStatus = "queued"
	}
	persistCtx, cancelPersist := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelPersist()
	var mappingErr error
	for attempt := 0; attempt < 3; attempt++ {
		mappingErr = s.repo.UpdateUpstream(persistCtx, task.TaskID, upID, persistStatus, a.ID)
		if mappingErr == nil {
			break
		}
		select {
		case <-persistCtx.Done():
			break
		case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
		}
	}
	if mappingErr != nil {
		slog.Error("media_video.upstream_mapping_failed", "task_id", task.TaskID, "upstream_task_id", upID, "error", mappingErr)
		return task, http.StatusInternalServerError, errors.New("video task persistence failed")
	}
	task.UpstreamTaskID = upID
	task.Status = persistStatus
	task.UpdatedAt = &now
	if st == "succeeded" || st == "failed" {
		// Creation uses the same lease as polling/recovery before processing a
		// terminal provider response. A competing worker or refund may win it.
		owned, claimErr := s.repo.ClaimPollable(persistCtx, task.TaskID, time.Now())
		if claimErr != nil {
			return task, http.StatusInternalServerError, claimErr
		}
		if owned == nil {
			current, getErr := s.repo.Get(persistCtx, task.TaskID, userID, apiKeyID)
			if getErr != nil {
				return task, http.StatusInternalServerError, getErr
			}
			if current != nil {
				task = current
			}
			return task, http.StatusAccepted, nil
		}
		task = owned
		persistCtx = WithMediaVideoPollToken(persistCtx, task.PollToken)
		task.Downloadable = false
		if st == "failed" {
			task.Error = extractMediaVideoError(upstream)
			task.CompletedAt = &now
			if releaseErr := s.releaseBalance(persistCtx, task); releaseErr != nil {
				return task, http.StatusInternalServerError, releaseErr
			}
		} else {
			if completeErr := s.completeSucceeded(persistCtx, task, &a); completeErr != nil {
				if errors.Is(completeErr, ErrMediaVideoStateConflict) {
					current, getErr := s.repo.Get(persistCtx, task.TaskID, userID, apiKeyID)
					if getErr == nil && current != nil {
						return current, http.StatusAccepted, nil
					}
					return task, http.StatusConflict, completeErr
				}
				s.deferCompletion(persistCtx, task, completeErr)
				task.Status = "running"
				return task, http.StatusAccepted, nil
			}
			return task, http.StatusAccepted, nil
		}
		if updateErr := s.repo.UpdateStatus(persistCtx, task.TaskID, st, nil, task.Downloadable, task.Error, now, now); updateErr != nil {
			slog.Error("media_video.terminal_status_persist_failed", "task_id", task.TaskID, "error", updateErr)
			return task, http.StatusInternalServerError, errors.New("video task persistence failed")
		}
	}
	return task, http.StatusAccepted, nil
}

// Replay never creates, freezes, or submits, even if the requested key is absent.
func (s *MediaVideoService) Replay(ctx context.Context, userID, apiKeyID int64, idem string, req MediaVideoCreateRequest) (*MediaVideoTask, int, error) {
	if strings.TrimSpace(idem) == "" {
		return nil, 400, errors.New("Idempotency-Key is required")
	}
	task, err := s.repo.GetByIdempotency(ctx, userID, apiKeyID, hashString(idem))
	if err != nil {
		return nil, 500, err
	}
	if task == nil || task.ExpiresAt == nil || !task.ExpiresAt.After(time.Now()) {
		return nil, 404, sqlErrNotFound
	}
	body, _ := json.Marshal(req)
	if task.RequestHash != hashBytes(body) {
		return nil, 409, errors.New("Idempotency-Key was reused with a different request")
	}
	return task, 200, nil
}

func (s *MediaVideoService) completeSucceeded(ctx context.Context, task *MediaVideoTask, account *Account) error {
	if task == nil || account == nil {
		return errors.New("media video completion context is incomplete")
	}
	if MediaVideoPollToken(ctx) == "" {
		return ErrMediaVideoStateConflict
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, _, err := s.callResponse(probeCtx, *account, http.MethodGet, "/v1/media/videos/"+task.UpstreamTaskID+"/content", nil, "", "bytes=0-0")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return &mediaVideoProbeError{status: resp.StatusCode, retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if contentType != "video/mp4" {
		return fmt.Errorf("unexpected video content type %q", contentType)
	}
	var one [1]byte
	if _, readErr := io.ReadFull(resp.Body, one[:]); readErr != nil {
		return fmt.Errorf("video content probe is empty or incomplete: %w", readErr)
	}
	if err := s.captureBalance(ctx, task); err != nil {
		return err
	}
	now := time.Now()
	downloadable := task.ExpiresAt == nil || task.ExpiresAt.After(now)
	if err := s.repo.UpdateStatus(ctx, task.TaskID, "succeeded", task.Progress, downloadable, nil, now, now); err != nil {
		return err
	}
	task.Status, task.Downloadable, task.CompletedAt = "succeeded", downloadable, &now
	return nil
}

type mediaVideoProbeError struct {
	status     int
	retryAfter time.Duration
}

func (e *mediaVideoProbeError) Error() string {
	return fmt.Sprintf("video content probe status %d", e.status)
}
func mediaVideoCompletionDelay(task *MediaVideoTask, err error) time.Duration {
	var probe *mediaVideoProbeError
	var after time.Duration
	if errors.As(err, &probe) {
		after = probe.retryAfter
	}
	return mediaVideoPollBackoff(task.PollFailures, after)
}
func (s *MediaVideoService) deferCompletion(ctx context.Context, task *MediaVideoTask, err error) {
	if errors.Is(err, ErrMediaVideoStateConflict) {
		return
	}
	now := time.Now()
	if updateErr := s.repo.MarkPolled(ctx, task.TaskID, now, now.Add(mediaVideoCompletionDelay(task, err))); updateErr != nil {
		slog.Error("media_video.completion_retry_schedule_failed", "task_id", task.TaskID, "error", updateErr)
	}
}

func mediaVideoUpstreamIdempotencyKey(userID, apiKeyID int64, taskID, clientKey string) string {
	return "sub2api-laogou-" + hashString(fmt.Sprintf("%d:%d:%s:%s", userID, apiKeyID, taskID, clientKey))
}

func extractMediaVideoError(body []byte) json.RawMessage {
	if len(body) == 0 {
		return json.RawMessage(`{"type":"upstream_error","message":"empty upstream error"}`)
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) == nil {
		if raw, ok := envelope["error"]; ok && len(raw) > 0 {
			return raw
		}
		return json.RawMessage(body)
	}
	message := strings.TrimSpace(string(body))
	if len(message) > 4096 {
		message = message[:4096]
	}
	wrapped, _ := json.Marshal(map[string]string{"type": "upstream_error", "message": message})
	return wrapped
}

func (s *MediaVideoService) callCreateOnce(ctx context.Context, account Account, body []byte, idem string) ([]byte, int, error) {
	// Provider idempotency is not a confirmed contract. Never replay a POST
	// after a transport or server failure, including a partially read response.
	resp, status, err := s.callResponse(ctx, account, http.MethodPost, "/v1/media/videos", body, idem)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	defer resp.Body.Close()
	b, readErr := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if readErr != nil || len(b) > 2<<20 {
		return nil, http.StatusBadGateway, errors.New("incomplete upstream create response; reconciliation required")
	}
	if status < 200 || status >= 300 {
		return b, status, fmt.Errorf("upstream status %d", status)
	}
	return b, status, nil
}

func mediaVideoDefiniteRejection(status int) bool {
	switch status {
	case 400, 401, 403, 404, 405, 413, 415, 422:
		return true
	default:
		return false
	}
}

func normalizeMediaVideoStatus(status string) string {
	switch status {
	case "queued", "running", "succeeded", "failed":
		return status
	default:
		return "running"
	}
}

type mediaVideoPollContextKey struct{}

func WithMediaVideoPollToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, mediaVideoPollContextKey{}, token)
}
func MediaVideoPollToken(ctx context.Context) string {
	token, _ := ctx.Value(mediaVideoPollContextKey{}).(string)
	return token
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	seconds, err := strconv.Atoi(value)
	if err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}

func (s *MediaVideoService) Get(ctx context.Context, taskID string, userID, apiKeyID int64) (*MediaVideoTask, error) {
	t, e := s.repo.Get(ctx, taskID, userID, apiKeyID)
	if e != nil {
		return nil, e
	}
	if t == nil || t.ExpiresAt == nil || !t.ExpiresAt.After(time.Now()) {
		// Cleanup must first settle or release outstanding funds. Reads never
		// delete the durable record needed by billing recovery.
		return nil, sqlErrNotFound
	}
	return t, nil
}

var sqlErrNotFound = errors.New("video task not found")
var ErrMediaVideoKeyLimit = errors.New("video request exceeds API key quota or spending limit")
var ErrMediaVideoStateConflict = errors.New("video task ownership or state changed")
var ErrLaogouInvalidCursor = errors.New("invalid video cursor")

func (s *MediaVideoService) List(ctx context.Context, userID, apiKeyID int64, limit int, status, cursor string) ([]*MediaVideoTask, bool, string, error) {
	return s.repo.List(ctx, userID, apiKeyID, limit, status, cursor)
}
func (s *MediaVideoService) Content(ctx context.Context, t *MediaVideoTask, w http.ResponseWriter, r *http.Request) (int, error) {
	return s.streamContent(ctx, t, w, r)
}

func (s *MediaVideoService) callResponse(ctx context.Context, a Account, method, path string, body []byte, idem string, rangeHeader ...string) (*http.Response, int, error) {
	apiKey := strings.TrimSpace(a.GetCredential("api_key"))
	if apiKey == "" {
		return nil, http.StatusServiceUnavailable, errors.New("Laogou account is missing api_key")
	}
	var rd io.Reader
	if body != nil {
		rd = strings.NewReader(string(body))
	}
	req, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(SeedanceLaogouBaseURL, "/")+path, rd)
	if e != nil {
		return nil, 500, e
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if method == http.MethodPost {
		// Idempotency-Key makes Transport consider POST replayable. Disable
		// body rewind because supplier-side idempotency is not confirmed.
		req.GetBody = nil
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	if len(rangeHeader) > 0 && strings.TrimSpace(rangeHeader[0]) != "" {
		req.Header.Set("Range", rangeHeader[0])
	}
	client := s.http
	if a.Proxy != nil {
		client, e = s.proxyHTTPClient(a.Proxy.URL())
		if e != nil {
			return nil, http.StatusServiceUnavailable, e
		}
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, http.StatusGatewayTimeout, e
	}
	return resp, resp.StatusCode, nil
}
func (s *MediaVideoService) pollLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	defer close(s.done)
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			if err := s.repo.RecoverSubmissions(s.ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("media_video.submission_recovery_failed", "error", err)
			}
			if err := s.repo.DeleteExpired(s.ctx, time.Now(), 100); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("media_video.expiry_cleanup_failed", "error", err)
			}
			s.recoverBillingOnce(s.ctx)
			s.pollOnce(s.ctx)
		}
	}
}

func (s *MediaVideoService) recoverBillingOnce(ctx context.Context) {
	expired, expiredErr := s.repo.ListExpiredForRelease(ctx, time.Now(), 50)
	if expiredErr != nil {
		slog.Error("media_video.expired_hold_query_failed", "error", expiredErr)
	} else {
		for _, task := range expired {
			if err := s.releaseBalance(ctx, task); err != nil {
				slog.Warn("media_video.expired_hold_release_failed", "task_id", task.TaskID, "error", err)
			}
		}
	}
	tasks, err := s.repo.ListBillingRecovery(ctx, time.Now(), 1)
	if err != nil {
		slog.Error("media_video.billing_recovery_query_failed", "error", err)
		return
	}
	for _, task := range tasks {
		ctx := WithMediaVideoPollToken(ctx, task.PollToken)
		switch task.BillingStatus {
		case "release_pending":
			if err := s.releaseBalance(ctx, task); err != nil {
				slog.Warn("media_video.release_retry_failed", "task_id", task.TaskID, "error", err)
			}
		case "settlement_pending":
			account, accountErr := s.accounts.GetByID(ctx, task.UpstreamAccountID)
			if accountErr != nil || account == nil {
				continue
			}
			if err := s.completeSucceeded(ctx, task, account); err != nil {
				slog.Warn("media_video.settlement_retry_failed", "task_id", task.TaskID, "error", err)
				s.deferCompletion(ctx, task, err)
			}
		}
	}
}
func (s *MediaVideoService) pollOnce(ctx context.Context) {
	started := time.Now()
	for claimed := 0; claimed < 50 && ctx.Err() == nil && time.Since(started) < time.Minute; claimed++ {
		now := time.Now()
		tasks, e := s.repo.ListPollable(ctx, now, 1)
		if e != nil {
			slog.Error("media_video.pollable_task_query_failed", "error", e)
			return
		}
		if len(tasks) == 0 {
			return
		}
		for _, t := range tasks {
			ctx := WithMediaVideoPollToken(ctx, t.PollToken)
			if t.UpstreamTaskID == "" || t.UpstreamAccountID == 0 {
				continue
			}
			a, e := s.accounts.GetByID(ctx, t.UpstreamAccountID)
			if e != nil || a == nil {
				continue
			}
			pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			resp, status, e := s.callResponse(pollCtx, *a, http.MethodGet, "/v1/media/videos/"+t.UpstreamTaskID, nil, "")
			var b []byte
			var retryAfter time.Duration
			if resp != nil {
				retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
				b, e = io.ReadAll(io.LimitReader(resp.Body, 2<<20))
				_ = resp.Body.Close()
				if e == nil && (status < 200 || status >= 300) {
					e = fmt.Errorf("upstream status %d", status)
				}
			}
			cancel()
			var v map[string]any
			if e == nil {
				e = json.Unmarshal(b, &v)
			}
			if e != nil {
				delay := mediaVideoPollBackoff(t.PollFailures, retryAfter)
				if updateErr := s.repo.MarkPolled(ctx, t.TaskID, now, time.Now().Add(delay)); updateErr != nil {
					slog.Error("media_video.poll_lease_update_failed", "task_id", t.TaskID, "error", updateErr)
				}
				continue
			}
			st := normalizeMediaVideoStatus(laogouStringValue(v, "status"))
			var er json.RawMessage
			if x, ok := v["error"]; ok {
				er, _ = json.Marshal(x)
			}
			var progress *float64
			if value, ok := v["progress"].(float64); ok {
				progress = &value
			}
			t.Progress = progress
			if st == "succeeded" {
				if completeErr := s.completeSucceeded(ctx, t, a); completeErr != nil {
					slog.Warn("media_video.completion_pending", "task_id", t.TaskID, "error", completeErr)
					s.deferCompletion(ctx, t, completeErr)
				}
			} else if st == "failed" {
				if releaseErr := s.releaseBalance(ctx, t); releaseErr != nil {
					slog.Error("media_video.failure_release_failed", "task_id", t.TaskID, "error", releaseErr)
				}
				if updateErr := s.repo.UpdateStatus(ctx, t.TaskID, st, progress, false, er, now, now); updateErr != nil {
					slog.Error("media_video.status_update_failed", "task_id", t.TaskID, "error", updateErr)
				}
			} else {
				if updateErr := s.repo.UpdateStatus(ctx, t.TaskID, st, progress, false, er, now, time.Now().Add(5*time.Second)); updateErr != nil {
					slog.Error("media_video.status_update_failed", "task_id", t.TaskID, "error", updateErr)
				}
			}
		}
	}
}
func mediaVideoPollBackoff(failures int, retryAfter time.Duration) time.Duration {
	if failures < 0 {
		failures = 0
	}
	if failures > 6 {
		failures = 6
	}
	delay := time.Duration(1<<failures) * 5 * time.Second
	if retryAfter > delay {
		delay = retryAfter
	}
	return delay
}
func hashBytes(b []byte) string  { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func hashString(s string) string { return hashBytes([]byte(s)) }
func laogouStringValue(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}
