package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
	voteairiskstate "github.com/Wei-Shaw/sub2api/internal/custom/voteai/riskstate"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type contentModerationTestSettingRepo struct {
	values map[string]string
}

func TestContentModerationWeakSignalsDoNotAccumulateOrEscalateByScoreAlone(t *testing.T) {
	weak := &moderationAPIResult{
		Flagged:        false,
		CategoryScores: map[string]float64{"ai_risk": 0.95},
		Signals:        []string{"defensive_context", "ownership_unverified"},
	}
	require.False(t, shouldAccumulateContentModerationRisk(weak, 0.95, 0.7))

	cfg := defaultContentModerationConfig()
	cfg.AIChat.SessionRiskEnabled = true
	cfg.AIChat.ActorRiskEnabled = true
	result, err := (&ContentModerationService{hashCache: &contentModerationTestHashCache{}}).applyAIChatRiskState(
		context.Background(),
		ContentModerationCheckInput{UserID: 10, APIKeyID: 20, SessionID: "weak-only"},
		cfg,
		weak,
	)
	require.NoError(t, err)
	require.Equal(t, voteairiskstate.TierLow, result.Tier)
	require.Equal(t, 0.95, result.CurrentScore)
	require.Zero(t, result.CumulativeScore)

	withCategory := &moderationAPIResult{
		Flagged:        false,
		CategoryScores: map[string]float64{"ai_risk": 0.95, "credential_theft": 0.95},
		Signals:        []string{"ownership_unverified"},
	}
	require.False(t, shouldAccumulateContentModerationRisk(withCategory, 0.95, 0.7))

	flaggedWithCategory := &moderationAPIResult{
		Flagged:        true,
		CategoryScores: map[string]float64{"ai_risk": 0.95, "credential_theft": 0.95},
		Signals:        []string{"defensive_context"},
	}
	require.True(t, shouldAccumulateContentModerationRisk(flaggedWithCategory, 0.95, 0.7))

	withStrongSignal := &moderationAPIResult{
		CategoryScores: map[string]float64{"ai_risk": 0.95},
		Signals:        []string{"ownership_unverified", "auth_bypass"},
	}
	require.True(t, shouldAccumulateContentModerationRisk(withStrongSignal, 0.95, 0.7))
}

func TestContentModerationSessionRiskAccumulatesAndIsolatesIdentity(t *testing.T) {
	cache := &contentModerationTestHashCache{}
	svc := &ContentModerationService{hashCache: cache}
	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.RiskLevelsEnabled = true
	cfg.AIChat.SessionRiskEnabled = true
	cfg.AIChat.ActorRiskEnabled = false
	cfg.normalize()

	input := ContentModerationCheckInput{UserID: 10, APIKeyID: 20, SessionID: "conversation-a"}
	result := &moderationAPIResult{
		CategoryScores: map[string]float64{"ai_risk": 0.45, "credential_theft": 0.45},
		Signals:        []string{"ownership_unverified", "auth_bypass"},
	}
	var got contentModerationTierResult
	for i := 0; i < 4; i++ {
		input.RequestID = fmt.Sprintf("request-%d", i)
		var err error
		got, err = svc.applyAIChatRiskState(context.Background(), input, cfg, result)
		require.NoError(t, err)
	}
	require.Equal(t, voteairiskstate.TierHigh, got.Tier)
	require.GreaterOrEqual(t, got.CumulativeScore, cfg.AIChat.ConfidenceThreshold)

	isolated := input
	isolated.SessionID = "conversation-b"
	isolated.RequestID = "isolated"
	other, err := svc.applyAIChatRiskState(context.Background(), isolated, cfg, result)
	require.NoError(t, err)
	require.Equal(t, voteairiskstate.TierObserve, other.Tier)
	require.InDelta(t, 0.45, other.CumulativeScore, 0.001)
}

func TestContentModerationSessionRisk_DoesNotAccumulateWeakDefensiveSignals(t *testing.T) {
	cache := &contentModerationTestHashCache{}
	svc := &ContentModerationService{hashCache: cache}
	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.RiskLevelsEnabled = true
	cfg.AIChat.SessionRiskEnabled = true
	cfg.AIChat.ActorRiskEnabled = true
	cfg.normalize()
	input := ContentModerationCheckInput{RequestID: "weak-1", UserID: 10, APIKeyID: 20, SessionID: "conversation-a"}
	weak := &moderationAPIResult{
		Flagged:        false,
		CategoryScores: map[string]float64{"ai_risk": 0.45, "credential_theft": 0.45},
		Signals:        []string{"defensive_context", "ownership_unverified"},
	}

	for range 8 {
		_, err := svc.applyAIChatRiskState(context.Background(), input, cfg, weak)
		require.NoError(t, err)
	}
	require.Empty(t, cache.sessionStates, "moderate defensive or ownership-only signals must not create sticky risk")

	strong := &moderationAPIResult{
		CategoryScores: map[string]float64{"ai_risk": 0.45, "cyber_abuse": 0.45},
		Signals:        []string{"ownership_unverified", "auth_bypass"},
	}
	_, err := svc.applyAIChatRiskState(context.Background(), input, cfg, strong)
	require.NoError(t, err)
	require.NotEmpty(t, cache.sessionStates, "a supported strong signal must still accumulate")
}

func TestContentModerationAuditMetadata_IsStructured(t *testing.T) {
	tests := []struct {
		name      string
		action    string
		errText   string
		status    string
		code      string
		retryable bool
	}{
		{name: "success", action: ContentModerationActionAllow, status: ContentModerationAuditStatusSuccess, code: ContentModerationActionAllow},
		{name: "skip", action: ContentModerationActionSkip, errText: "input_extraction_empty_content: empty", status: ContentModerationAuditStatusSkipped, code: "input_extraction_empty_content"},
		{name: "incomplete", action: ContentModerationActionError, errText: "audit_review_incomplete: timed out", status: ContentModerationAuditStatusIncomplete, code: "audit_review_incomplete", retryable: true},
		{name: "error", action: ContentModerationActionError, errText: "audit_invalid_json: malformed", status: ContentModerationAuditStatusError, code: "audit_invalid_json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code, retryable := contentModerationAuditMetadata(tt.action, tt.errText)
			require.Equal(t, tt.status, status)
			require.Equal(t, tt.code, code)
			require.Equal(t, tt.retryable, retryable)
		})
	}
}

func TestContentModerationAuditErrorTextPrefersTimeoutFromJoinedError(t *testing.T) {
	err := errors.Join(
		fmt.Errorf("wrapped transport failure: %w", voteaimoderation.ErrTemporary),
		fmt.Errorf("wrapped deadline: %w", context.DeadlineExceeded),
	)
	require.Equal(t, "audit_timeout: "+err.Error(), contentModerationAuditErrorText(err))
}

func TestContentModerationSessionRiskPrecheckHonorsCooldown(t *testing.T) {
	cache := &contentModerationTestHashCache{}
	svc := &ContentModerationService{hashCache: cache}
	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.RiskLevelsEnabled = true
	cfg.AIChat.SessionRiskEnabled = true
	cfg.normalize()
	input := ContentModerationCheckInput{UserID: 11, APIKeyID: 22, SessionID: "blocked-session", RequestID: "high"}
	sessionKey, _, _ := contentModerationRiskIdentity(input)
	cache.sessionStates = map[string]voteairiskstate.State{
		sessionKey: {
			Version:          1,
			Score:            0.9,
			Tier:             voteairiskstate.TierHigh,
			BlockedUntilUnix: time.Now().Add(10 * time.Minute).Unix(),
		},
	}
	state, blocked, err := svc.getBlockedSessionRisk(context.Background(), input, cfg)
	require.NoError(t, err)
	require.True(t, blocked)
	require.InDelta(t, 0.9, state.Score, 0.001)
}

func (r *contentModerationTestSettingRepo) Get(ctx context.Context, key string) (*Setting, error) {
	if value, ok := r.values[key]; ok {
		return &Setting{Key: key, Value: value}, nil
	}
	return nil, ErrSettingNotFound
}

func (r *contentModerationTestSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (r *contentModerationTestSettingRepo) Set(ctx context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *contentModerationTestSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r *contentModerationTestSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *contentModerationTestSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *contentModerationTestSettingRepo) Delete(ctx context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestContentModerationGetStatusExposesAuditUsageCounters(t *testing.T) {
	svc := &ContentModerationService{
		settingRepo: &contentModerationTestSettingRepo{values: map[string]string{}},
	}
	svc.auditFastCalls.Store(11)
	svc.auditFullCalls.Store(7)
	svc.auditMaxCalls.Store(3)
	svc.auditResultCacheHits.Store(5)
	svc.auditPromptTokens.Store(101)
	svc.auditCachedInputTokens.Store(202)
	svc.auditUncachedInputTokens.Store(303)
	svc.auditOutputTokens.Store(404)
	svc.auditUsageUnknown.Store(2)
	svc.auditInputChars.Store(505)

	status, err := svc.GetStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(11), status.AuditFastCalls)
	require.Equal(t, int64(7), status.AuditFullCalls)
	require.Equal(t, int64(3), status.AuditMaxCalls)
	require.Equal(t, int64(5), status.AuditResultCacheHits)
	require.Equal(t, int64(101), status.AuditPromptTokens)
	require.Equal(t, int64(202), status.AuditCachedInputTokens)
	require.Equal(t, int64(303), status.AuditUncachedInputTokens)
	require.Equal(t, int64(404), status.AuditOutputTokens)
	require.Equal(t, int64(2), status.AuditUsageUnknown)
	require.Equal(t, int64(505), status.AuditInputChars)
}

type contentModerationTestRepo struct {
	mu              sync.Mutex
	logs            []ContentModerationLog
	nextID          int64
	banOutcome      string
	banErr          error
	restoreErr      error
	restoreCalls    int
	moderationState *ContentModerationUserState
}

func (r *contentModerationTestRepo) CreateLog(ctx context.Context, log *ContentModerationLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if log != nil {
		if log.ID <= 0 {
			r.nextID++
			log.ID = r.nextID
		}
		r.logs = append(r.logs, *log)
	}
	return nil
}

func (r *contentModerationTestRepo) UpdateLogEffects(ctx context.Context, logID int64, patch ContentModerationLogEffectsPatch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx := range r.logs {
		if r.logs[idx].ID != logID {
			continue
		}
		r.logs[idx].ViolationCount = patch.ViolationCount
		r.logs[idx].AutoBanned = patch.AutoBanned
		r.logs[idx].EmailSent = patch.EmailSent
		r.logs[idx].SideEffectStatus = patch.SideEffectStatus
		r.logs[idx].NotificationStatus = patch.NotificationStatus
		r.logs[idx].SideEffectError = patch.SideEffectError
		if patch.AuditDetails != nil {
			r.logs[idx].AuditDetails = *patch.AuditDetails
		}
		return nil
	}
	return fmt.Errorf("log %d not found", logID)
}

func (r *contentModerationTestRepo) GetModerationUserState(ctx context.Context, userID int64) (*ContentModerationUserState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.moderationState == nil || r.moderationState.UserID != userID {
		return nil, nil
	}
	state := *r.moderationState
	return &state, nil
}

func (r *contentModerationTestRepo) TryApplyModerationOwnedBan(ctx context.Context, userID, logID int64, disabledAt time.Time) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.banErr != nil {
		return "", r.banErr
	}
	outcome := r.banOutcome
	if outcome == "" {
		outcome = ContentModerationBanOutcomeIneligible
	}
	if outcome == ContentModerationBanOutcomeApplied {
		logIDCopy := logID
		disabledAtCopy := disabledAt
		r.moderationState = &ContentModerationUserState{
			UserID:                  userID,
			ModerationOwnedDisabled: true,
			DisabledLogID:           &logIDCopy,
			DisabledAt:              &disabledAtCopy,
			UpdatedAt:               disabledAt,
		}
	}
	return outcome, nil
}

func (r *contentModerationTestRepo) RestoreModerationOwnedBan(ctx context.Context, userID int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreCalls++
	if r.restoreErr != nil {
		return false, r.restoreErr
	}
	if r.moderationState == nil || r.moderationState.UserID != userID || !r.moderationState.ModerationOwnedDisabled {
		return false, nil
	}
	r.moderationState.ModerationOwnedDisabled = false
	r.moderationState.DisabledLogID = nil
	r.moderationState.DisabledAt = nil
	return true, nil
}

func (r *contentModerationTestRepo) ListLogs(ctx context.Context, filter ContentModerationLogFilter) ([]ContentModerationLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *contentModerationTestRepo) CountFlaggedByUserSince(ctx context.Context, userID int64, since time.Time, excludeCyberPolicy bool) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, log := range r.logs {
		if log.UserID == nil || *log.UserID != userID || !log.Flagged || log.Action == ContentModerationActionHashBlock {
			continue
		}
		if excludeCyberPolicy && log.Action == ContentModerationActionCyberPolicy {
			continue
		}
		if log.CreatedAt.IsZero() || log.CreatedAt.Before(since) {
			continue
		}
		count++
	}
	return count, nil
}

func (r *contentModerationTestRepo) CleanupExpiredLogs(ctx context.Context, hitBefore time.Time, nonHitBefore time.Time) (*ContentModerationCleanupResult, error) {
	return &ContentModerationCleanupResult{}, nil
}

func (r *contentModerationTestRepo) UpdateLogEmailSent(ctx context.Context, id int64, sent bool) error {
	return nil
}

func (r *contentModerationTestRepo) snapshotLogs() []ContentModerationLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ContentModerationLog, len(r.logs))
	copy(out, r.logs)
	return out
}

func requireContentModerationLogCount(t *testing.T, repo *contentModerationTestRepo, want int) []ContentModerationLog {
	t.Helper()
	var logs []ContentModerationLog
	require.Eventually(t, func() bool {
		logs = repo.snapshotLogs()
		return len(logs) == want
	}, time.Second, 10*time.Millisecond)
	return logs
}

func requireRecordedHashCount(t *testing.T, cache *contentModerationTestHashCache, want int) []string {
	t.Helper()
	var hashes []string
	require.Eventually(t, func() bool {
		hashes = cache.snapshotRecorded()
		return len(hashes) == want
	}, time.Second, 10*time.Millisecond)
	return hashes
}

type contentModerationTestHashCache struct {
	mu            sync.Mutex
	hashes        map[string]struct{}
	suppressions  map[string]struct{}
	recorded      []string
	checked       []string
	deleted       []string
	hasResult     bool
	hasResultUsed bool
	results       map[string][]byte
	resultTTLs    map[string]time.Duration
	resultGetErr  error
	resultSetErr  error
	sessionStates map[string]voteairiskstate.State
	sessionGetErr error
	sessionPutErr error
	clearedUsers  []int64
	clearErr      error
	onClear       func(int64)
	epochs        map[int64]int64
}

type contentModerationSuppressionRaceCache struct {
	*contentModerationTestHashCache
}

type contentModerationSuppressionErrorCache struct {
	*contentModerationTestHashCache
	err         error
	recordCalls int
}

func (c *contentModerationSuppressionErrorCache) IsFlaggedInputHashSuppressed(context.Context, string) (bool, error) {
	return false, c.err
}

func (c *contentModerationSuppressionErrorCache) RecordFlaggedInputHashIfAllowed(context.Context, string) (bool, error) {
	c.recordCalls++
	return true, nil
}

func (c *contentModerationSuppressionRaceCache) IsFlaggedInputHashSuppressed(context.Context, string) (bool, error) {
	return false, nil
}

func (c *contentModerationSuppressionRaceCache) RecordFlaggedInputHashIfAllowed(context.Context, string) (bool, error) {
	return false, nil
}

type contentModerationTestUserRepo struct {
	user    *User
	updated []User
}

func (r *contentModerationTestUserRepo) Create(ctx context.Context, user *User) error {
	panic("unexpected Create call")
}

func (r *contentModerationTestUserRepo) CreateWithEmailAliasGuard(ctx context.Context, user *User) error {
	panic("unexpected CreateWithEmailAliasGuard call")
}

func (r *contentModerationTestUserRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	if r.user == nil {
		return nil, ErrUserNotFound
	}
	clone := *r.user
	return &clone, nil
}

func (r *contentModerationTestUserRepo) GetByEmail(ctx context.Context, email string) (*User, error) {
	panic("unexpected GetByEmail call")
}

func (r *contentModerationTestUserRepo) GetFirstAdmin(ctx context.Context) (*User, error) {
	panic("unexpected GetFirstAdmin call")
}

func (r *contentModerationTestUserRepo) Update(ctx context.Context, user *User, fields UserUpdateFields) error {
	if user == nil {
		return nil
	}
	clone := *user
	r.updated = append(r.updated, clone)
	r.user = &clone
	return nil
}

func (r *contentModerationTestUserRepo) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete call")
}

func (r *contentModerationTestUserRepo) GetUserAvatar(ctx context.Context, userID int64) (*UserAvatar, error) {
	panic("unexpected GetUserAvatar call")
}

func (r *contentModerationTestUserRepo) UpsertUserAvatar(ctx context.Context, userID int64, input UpsertUserAvatarInput) (*UserAvatar, error) {
	panic("unexpected UpsertUserAvatar call")
}

func (r *contentModerationTestUserRepo) DeleteUserAvatar(ctx context.Context, userID int64) error {
	panic("unexpected DeleteUserAvatar call")
}

func (r *contentModerationTestUserRepo) List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (r *contentModerationTestUserRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (r *contentModerationTestUserRepo) GetLatestUsedAtByUserIDs(ctx context.Context, userIDs []int64) (map[int64]*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserIDs call")
}

func (r *contentModerationTestUserRepo) GetLatestUsedAtByUserID(ctx context.Context, userID int64) (*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserID call")
}

func (r *contentModerationTestUserRepo) UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error {
	panic("unexpected UpdateUserLastActiveAt call")
}

func (r *contentModerationTestUserRepo) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	panic("unexpected UpdateBalance call")
}

func (r *contentModerationTestUserRepo) DeductBalance(ctx context.Context, id int64, amount float64) error {
	panic("unexpected DeductBalance call")
}

func (r *contentModerationTestUserRepo) AdjustBalance(ctx context.Context, id int64, delta float64) (BalanceChange, error) {
	panic("unexpected AdjustBalance call")
}

func (r *contentModerationTestUserRepo) SetBalance(ctx context.Context, id int64, value float64) (BalanceChange, error) {
	panic("unexpected SetBalance call")
}

func (r *contentModerationTestUserRepo) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	panic("unexpected UpdateConcurrency call")
}

func (r *contentModerationTestUserRepo) BatchSetConcurrency(ctx context.Context, userIDs []int64, value int) (int, error) {
	panic("unexpected BatchSetConcurrency call")
}

func (r *contentModerationTestUserRepo) BatchAddConcurrency(ctx context.Context, userIDs []int64, delta int) (int, error) {
	panic("unexpected BatchAddConcurrency call")
}
func (r *contentModerationTestUserRepo) BatchUpdateLimits(ctx context.Context, userIDs []int64, concurrency, rpmLimit *int) (int, error) {
	panic("unexpected BatchUpdateLimits call")
}

func (r *contentModerationTestUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	panic("unexpected ExistsByEmail call")
}

func (r *contentModerationTestUserRepo) ExistsByEmailAlias(ctx context.Context, email string) (bool, error) {
	panic("unexpected ExistsByEmailAlias call")
}

func (r *contentModerationTestUserRepo) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected RemoveGroupFromAllowedGroups call")
}

func (r *contentModerationTestUserRepo) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected AddGroupToAllowedGroups call")
}

func (r *contentModerationTestUserRepo) RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected RemoveGroupFromUserAllowedGroups call")
}

func (r *contentModerationTestUserRepo) ListUserAuthIdentities(ctx context.Context, userID int64) ([]UserAuthIdentityRecord, error) {
	panic("unexpected ListUserAuthIdentities call")
}

func (r *contentModerationTestUserRepo) UnbindUserAuthProvider(ctx context.Context, userID int64, provider string) error {
	panic("unexpected UnbindUserAuthProvider call")
}

func (r *contentModerationTestUserRepo) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	panic("unexpected UpdateTotpSecret call")
}

func (r *contentModerationTestUserRepo) EnableTotp(ctx context.Context, userID int64) error {
	panic("unexpected EnableTotp call")
}

func (r *contentModerationTestUserRepo) DisableTotp(ctx context.Context, userID int64) error {
	panic("unexpected DisableTotp call")
}

func (r *contentModerationTestUserRepo) GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error) {
	return r.GetByID(ctx, id)
}

type contentModerationTestAuthCacheInvalidator struct {
	userIDs      []int64
	onInvalidate func(int64)
}

func (i *contentModerationTestAuthCacheInvalidator) InvalidateAuthCacheByKey(ctx context.Context, key string) {
}

func (i *contentModerationTestAuthCacheInvalidator) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	i.userIDs = append(i.userIDs, userID)
	if i.onInvalidate != nil {
		i.onInvalidate(userID)
	}
}

func (i *contentModerationTestAuthCacheInvalidator) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
}

func (c *contentModerationTestHashCache) RecordFlaggedInputHash(ctx context.Context, inputHash string) error {
	_, err := c.RecordFlaggedInputHashIfAllowed(ctx, inputHash)
	return err
}

func (c *contentModerationTestHashCache) RecordFlaggedInputHashIfAllowed(_ context.Context, inputHash string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, suppressed := c.suppressions[inputHash]; suppressed {
		return false, nil
	}
	if c.hashes == nil {
		c.hashes = map[string]struct{}{}
	}
	c.hashes[inputHash] = struct{}{}
	c.recorded = append(c.recorded, inputHash)
	return true, nil
}

func (c *contentModerationTestHashCache) IsFlaggedInputHashSuppressed(_ context.Context, inputHash string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, suppressed := c.suppressions[inputHash]
	return suppressed, nil
}

func (c *contentModerationTestHashCache) HasFlaggedInputHash(ctx context.Context, inputHash string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checked = append(c.checked, inputHash)
	if c.hasResultUsed {
		return c.hasResult, nil
	}
	_, ok := c.hashes[inputHash]
	return ok, nil
}

func (c *contentModerationTestHashCache) DeleteFlaggedInputHash(ctx context.Context, inputHash string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleted = append(c.deleted, inputHash)
	if c.suppressions == nil {
		c.suppressions = map[string]struct{}{}
	}
	c.suppressions[inputHash] = struct{}{}
	if c.hashes == nil {
		return false, nil
	}
	if _, ok := c.hashes[inputHash]; !ok {
		return false, nil
	}
	delete(c.hashes, inputHash)
	return true, nil
}

func (c *contentModerationTestHashCache) ClearFlaggedInputHashes(ctx context.Context) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	deleted := int64(len(c.hashes))
	if c.suppressions == nil {
		c.suppressions = map[string]struct{}{}
	}
	for inputHash := range c.hashes {
		c.suppressions[inputHash] = struct{}{}
	}
	c.hashes = map[string]struct{}{}
	return deleted, nil
}

func (c *contentModerationTestHashCache) CountFlaggedInputHashes(ctx context.Context) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int64(len(c.hashes)), nil
}

func (c *contentModerationTestHashCache) GetContentModerationResult(ctx context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resultGetErr != nil {
		return nil, false, c.resultGetErr
	}
	value, ok := c.results[key]
	return append([]byte(nil), value...), ok, nil
}

func (c *contentModerationTestHashCache) SetContentModerationResult(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resultSetErr != nil {
		return c.resultSetErr
	}
	if c.results == nil {
		c.results = map[string][]byte{}
	}
	if c.resultTTLs == nil {
		c.resultTTLs = map[string]time.Duration{}
	}
	c.results[key] = append([]byte(nil), value...)
	c.resultTTLs[key] = ttl
	return nil
}

func (c *contentModerationTestHashCache) GetContentModerationSessionRisk(ctx context.Context, key string) (voteairiskstate.State, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessionGetErr != nil {
		return voteairiskstate.State{}, false, c.sessionGetErr
	}
	state, ok := c.sessionStates[key]
	return state, ok, nil
}

func (c *contentModerationTestHashCache) UpdateContentModerationSessionRisk(ctx context.Context, key string, event voteairiskstate.Event, cfg voteairiskstate.Config) (voteairiskstate.State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessionPutErr != nil {
		return voteairiskstate.State{}, c.sessionPutErr
	}
	if c.sessionStates == nil {
		c.sessionStates = map[string]voteairiskstate.State{}
	}
	state := voteairiskstate.Apply(c.sessionStates[key], event, cfg)
	c.sessionStates[key] = state
	return state, nil
}

func (c *contentModerationTestHashCache) ClearContentModerationUserState(ctx context.Context, userID int64) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearedUsers = append(c.clearedUsers, userID)
	if c.onClear != nil {
		c.onClear(userID)
	}
	if c.clearErr != nil {
		return 0, c.clearErr
	}
	if c.epochs == nil {
		c.epochs = map[int64]int64{}
	}
	c.epochs[userID]++
	deleted := int64(len(c.sessionStates))
	c.sessionStates = nil
	return deleted, nil
}

func (c *contentModerationTestHashCache) GetContentModerationUserEpoch(ctx context.Context, userID int64) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.epochs[userID], nil
}

func (c *contentModerationTestHashCache) AdvanceContentModerationUserEpoch(ctx context.Context, userID int64) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epochs == nil {
		c.epochs = map[int64]int64{}
	}
	c.epochs[userID]++
	return c.epochs[userID], nil
}

func (c *contentModerationTestHashCache) snapshotRecorded() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.recorded))
	copy(out, c.recorded)
	return out
}

func (c *contentModerationTestHashCache) snapshotChecked() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.checked))
	copy(out, c.checked)
	return out
}

func (c *contentModerationTestHashCache) hasHash(inputHash string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.hashes[inputHash]
	return ok
}

func (c *contentModerationTestHashCache) snapshotDeleted() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.deleted))
	copy(out, c.deleted)
	return out
}

func TestBuildContentModerationLog_RedactsInputExcerpt(t *testing.T) {
	svc := &ContentModerationService{}
	cfg := defaultContentModerationConfig()
	input := ContentModerationCheckInput{
		RequestID: "req-1",
		Endpoint:  "/v1/chat/completions",
		Provider:  "openai",
	}

	testAPIKey := strings.Join([]string{"sk", "proj", "1234567890abcdef"}, "-")
	log := svc.buildLog(input, cfg, ContentModerationActionAllow, true, "sexual", 0.8, map[string]float64{"sexual": 0.8}, "hello "+testAPIKey, nil, nil, "")

	require.NotContains(t, log.InputExcerpt, testAPIKey)
	require.Contains(t, log.InputExcerpt, "[REDACTED_API_KEY]")
}

func TestRedactContentModerationSecrets_LongHexAndTokens(t *testing.T) {
	input := "你哈市多大事cf5bbdc4cd508f3aaf0d2070d529d4a4ac29099f8ecc357f696df28e1df91554 token=abc123456789xyz Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signaturepart https://example.com/private/path?token=abc123"

	out := redactContentModerationSecrets(input)

	require.NotContains(t, out, "cf5bbdc4cd508f3aaf0d2070d529d4a4ac29099f8ecc357f696df28e1df91554")
	require.NotContains(t, out, "abc123456789xyz")
	require.NotContains(t, out, "eyJhbGciOiJIUzI1NiJ9")
	require.Contains(t, out, "https://example.com/private/path?token=[REDACTED]")
	require.NotContains(t, out, "abc123")
	require.Contains(t, out, "[REDACTED")
}

func TestContentModerationConfigNormalize_NonHitRetentionMaxThreeDays(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.NonHitRetentionDays = 30

	cfg.normalize()

	require.Equal(t, 3, cfg.NonHitRetentionDays)
}

func TestContentModerationConfigNormalize_LegacyConfigDefaultsToOpenAIModerations(t *testing.T) {
	var cfg ContentModerationConfig
	require.NoError(t, json.Unmarshal([]byte(`{"enabled":true,"mode":"pre_block","base_url":"https://api.openai.com","model":"omni-moderation-latest"}`), &cfg))

	cfg.normalize()

	require.Equal(t, ContentModerationProviderOpenAIModerations, cfg.AuditProvider)
	require.Equal(t, defaultAIChatBaseURL, cfg.AIChat.BaseURL)
	require.Equal(t, defaultAIChatModel, cfg.AIChat.Model)
}

func TestContentModerationConfigNormalize_UsesDeepSeekOfficialReasoningEfforts(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
	}{
		{input: "adaptive", want: "adaptive"},
		{input: "low", want: "low"},
		{input: "high", want: "high"},
		{input: "max", want: "max"},
		{input: "medium", want: "adaptive"},
		{input: "", want: "adaptive"},
	} {
		cfg := defaultContentModerationConfig()
		cfg.AIChat.ReasoningEffort = tt.input
		cfg.normalize()
		require.Equal(t, tt.want, cfg.AIChat.ReasoningEffort)
	}
}

func TestContentModerationConfigNormalize_EnforcesAIPerformanceBounds(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.AIChat.SynchronousBudgetMS = minAIChatSynchronousBudgetMS - 1
	cfg.AIChat.FastStageBudgetMS = minAIChatFastStageBudgetMS - 1
	cfg.AIChat.MaxInputChars = minAIChatMaxInputChars - 1
	cfg.normalize()
	require.Equal(t, defaultAIChatSynchronousBudgetMS, cfg.AIChat.SynchronousBudgetMS)
	require.Equal(t, defaultAIChatFastStageBudgetMS, cfg.AIChat.FastStageBudgetMS)
	require.Equal(t, defaultAIChatMaxInputChars, cfg.AIChat.MaxInputChars)

	cfg.AIChat.SynchronousBudgetMS = minAIChatSynchronousBudgetMS
	cfg.AIChat.FastStageBudgetMS = maxAIChatFastStageBudgetMS
	cfg.AIChat.MaxInputChars = minAIChatMaxInputChars
	cfg.AIChat.FastInputChars = minAIChatMaxInputChars + 1
	cfg.AIChat.FallbackInputChars = minAIChatMaxInputChars + 1
	cfg.normalize()
	require.Equal(t, minAIChatSynchronousBudgetMS, cfg.AIChat.SynchronousBudgetMS)
	require.Equal(t, minAIChatSynchronousBudgetMS, cfg.AIChat.FastStageBudgetMS)
	require.Equal(t, minAIChatMaxInputChars, cfg.AIChat.MaxInputChars)
	require.Equal(t, minAIChatMaxInputChars, cfg.AIChat.FastInputChars)
	require.Equal(t, minAIChatMaxInputChars, cfg.AIChat.FallbackInputChars)
}

func TestContentModerationFastStageContextUsesConfiguredBudget(t *testing.T) {
	backgroundCtx, cancelBackground := contentModerationFastStageContext(context.Background(), 2200)
	defer cancelBackground()
	backgroundDeadline, ok := backgroundCtx.Deadline()
	require.True(t, ok)
	require.LessOrEqual(t, time.Until(backgroundDeadline), 2200*time.Millisecond)

	parent, cancelParent := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelParent()

	fastCtx, cancelFast := contentModerationFastStageContext(parent, 2200)
	defer cancelFast()
	deadline, ok := fastCtx.Deadline()
	require.True(t, ok)
	remaining := time.Until(deadline)
	require.Greater(t, remaining, 2*time.Second)
	require.LessOrEqual(t, remaining, 2200*time.Millisecond)

	shortParent, cancelShortParent := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancelShortParent()
	shortCtx, cancelShort := contentModerationFastStageContext(shortParent, 2200)
	defer cancelShort()
	shortDeadline, ok := shortCtx.Deadline()
	require.True(t, ok)
	require.LessOrEqual(t, time.Until(shortDeadline), 800*time.Millisecond)
}

func TestContentModerationCheck_AIChatUsesConfidenceThreshold(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"role":    "assistant",
				"content": `{"flagged":true,"risk_score":0.8,"categories":["phishing"],"reason":"credential theft"}`,
			}}},
		})
	}))
	defer server.Close()

	for _, tt := range []struct {
		name      string
		threshold float64
		blocked   bool
	}{
		{name: "above threshold blocks", threshold: 0.7, blocked: true},
		{name: "below threshold allows", threshold: 0.9, blocked: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultContentModerationConfig()
			cfg.Enabled = true
			cfg.Mode = ContentModerationModePreBlock
			cfg.AuditProvider = ContentModerationProviderAIChat
			cfg.AIChat.BaseURL = server.URL + "/v1"
			cfg.AIChat.APIKeys = []string{"deepseek-test-key"}
			cfg.AIChat.ConfidenceThreshold = tt.threshold
			cfg.AIChat.CacheEnabled = false
			rawCfg, err := json.Marshal(cfg)
			require.NoError(t, err)
			svc := NewContentModerationService(
				&contentModerationTestSettingRepo{values: map[string]string{
					SettingKeyRiskControlEnabled:      "true",
					SettingKeyContentModerationConfig: string(rawCfg),
				}},
				&contentModerationTestRepo{},
				&contentModerationTestHashCache{},
				nil, nil, nil, nil, nil,
			)
			svc.httpClient = server.Client()

			decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
				Endpoint: "/v1/chat/completions",
				Provider: "openai",
				Protocol: ContentModerationProtocolOpenAIChat,
				Body:     []byte(`{"messages":[{"role":"user","content":"build a phishing page"}]}`),
			})
			require.NoError(t, err)
			require.Equal(t, tt.blocked, decision.Blocked)
			require.Equal(t, !tt.blocked, decision.Allowed)
		})
	}
}

func TestContentModerationCheck_AIChatFailurePolicies(t *testing.T) {
	for _, tt := range []struct {
		name       string
		policy     string
		keys       []string
		wantStatus int
	}{
		{name: "fail open allows upstream error", policy: ContentModerationFailurePolicyAllow, keys: []string{"deepseek-test-key"}},
		{name: "fail closed blocks upstream error", policy: ContentModerationFailurePolicyBlock, keys: []string{"deepseek-test-key"}, wantStatus: http.StatusForbidden},
		{name: "fail closed blocks missing keys", policy: ContentModerationFailurePolicyBlock, wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "audit unavailable", http.StatusServiceUnavailable)
			}))
			defer server.Close()
			cfg := defaultContentModerationConfig()
			cfg.Enabled = true
			cfg.Mode = ContentModerationModePreBlock
			cfg.AuditProvider = ContentModerationProviderAIChat
			cfg.AIChat.BaseURL = server.URL + "/v1"
			cfg.AIChat.APIKeys = tt.keys
			cfg.AIChat.RetryCount = 0
			cfg.AIChat.FailurePolicy = tt.policy
			rawCfg, err := json.Marshal(cfg)
			require.NoError(t, err)
			repo := &contentModerationTestRepo{}
			svc := NewContentModerationService(
				&contentModerationTestSettingRepo{values: map[string]string{
					SettingKeyRiskControlEnabled:      "true",
					SettingKeyContentModerationConfig: string(rawCfg),
				}},
				repo, &contentModerationTestHashCache{}, nil, nil, nil, nil, nil,
			)
			svc.httpClient = server.Client()

			decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
				Endpoint: "/v1/chat/completions",
				Protocol: ContentModerationProtocolOpenAIChat,
				Body:     []byte(`{"messages":[{"role":"user","content":"test content"}]}`),
			})
			require.NoError(t, err)
			if tt.wantStatus == 0 {
				require.True(t, decision.Allowed)
				require.False(t, decision.Blocked)
			} else {
				require.False(t, decision.Allowed)
				require.True(t, decision.Blocked)
				require.False(t, decision.Flagged)
				require.Equal(t, tt.wantStatus, decision.StatusCode)
				require.Equal(t, ContentModerationActionUnavailable, decision.Action)
				require.Len(t, repo.snapshotLogs(), 1)
			}
		})
	}
}

func TestContentModerationCheck_AIChatPreBlockHonorsSynchronousBudget(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.BaseURL = server.URL + "/v1"
	cfg.AIChat.APIKeys = []string{"deepseek-test-key"}
	cfg.AIChat.TimeoutMS = 15000
	cfg.AIChat.SynchronousBudgetMS = minAIChatSynchronousBudgetMS
	cfg.AIChat.RetryCount = 5
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyBlock
	cfg.RecordNonHits = true
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo, &contentModerationTestHashCache{}, nil, nil, nil, nil, nil,
	)
	svc.httpClient = server.Client()

	started := time.Now()
	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Endpoint: "/v1/responses",
		Protocol: ContentModerationProtocolOpenAIResponses,
		Body:     []byte(`{"input":"normal request"}`),
	})
	elapsed := time.Since(started)

	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.True(t, decision.Blocked)
	require.Less(t, elapsed, time.Second, "pre-block audit must stop at the synchronous budget")
	require.Equal(t, 1, requestCount, "deadline exhaustion must not start another API-key retry")
	logs := requireContentModerationLogCount(t, repo, 1)
	require.Contains(t, strings.ToLower(logs[0].Error), "deadline")
}

func TestAIChatModerationAttemptBudget(t *testing.T) {
	tests := []struct {
		name       string
		remaining  time.Duration
		backoff    time.Duration
		hasNextKey bool
		want       time.Duration
	}{
		{
			name:       "default budget reserves a full backup window",
			remaining:  4800 * time.Millisecond,
			backoff:    100 * time.Millisecond,
			hasNextKey: true,
			want:       3150 * time.Millisecond,
		},
		{
			name:       "tight viable budget preserves minimum windows",
			remaining:  1300 * time.Millisecond,
			backoff:    100 * time.Millisecond,
			hasNextKey: true,
			want:       500 * time.Millisecond,
		},
		{
			name:       "too little time stays with current key",
			remaining:  1100 * time.Millisecond,
			backoff:    100 * time.Millisecond,
			hasNextKey: true,
			want:       1100 * time.Millisecond,
		},
		{
			name:       "no backup keeps the complete parent budget",
			remaining:  4800 * time.Millisecond,
			backoff:    100 * time.Millisecond,
			hasNextKey: false,
			want:       4800 * time.Millisecond,
		},
		{
			name:       "expired budget remains exhausted",
			remaining:  0,
			backoff:    100 * time.Millisecond,
			hasNextKey: true,
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, aiChatModerationAttemptBudget(tt.remaining, tt.backoff, tt.hasNextKey))
		})
	}
}

func TestContentModerationCallModeration_AIChatTimeoutUsesDistinctBackupKey(t *testing.T) {
	var mu sync.Mutex
	seenKeys := make([]string, 0, 2)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		mu.Lock()
		seenKeys = append(seenKeys, key)
		mu.Unlock()
		if key == "slow-key" {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"choices":[{"message":{"role":"assistant","content":"{\"flagged\":false,\"risk_score\":0.01,\"categories\":[],\"signals\":[],\"reason\":\"benign\"}"}}]}`,
			)),
			Request: r,
		}, nil
	})}

	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.BaseURL = "https://audit.test/v1"
	cfg.AIChat.APIKeys = []string{"slow-key", "healthy-key"}
	cfg.AIChat.RetryCount = 1
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.ReasoningEffort = "high"
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	svc.httpClient = client

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	result, err := svc.callModeration(ctx, cfg, "ordinary request", true)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NoError(t, ctx.Err(), "backup must finish before the global deadline")
	mu.Lock()
	gotKeys := append([]string(nil), seenKeys...)
	mu.Unlock()
	require.Equal(t, []string{"slow-key", "healthy-key"}, gotKeys)

	loads := svc.preBlockAPIKeyLoads(cfg.apiKeys())
	require.Len(t, loads, 2)
	require.Equal(t, int64(0), loads[0].Active)
	require.Equal(t, int64(1), loads[0].Total)
	require.Equal(t, int64(1), loads[0].Errors)
	require.Equal(t, int64(0), loads[1].Active)
	require.Equal(t, int64(1), loads[1].Total)
	require.Equal(t, int64(1), loads[1].Success)
}

func TestContentModerationCallModeration_AIChatTwoKeyTimeoutsStayWithinParentBudget(t *testing.T) {
	var mu sync.Mutex
	seenKeys := make([]string, 0, 3)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		mu.Lock()
		seenKeys = append(seenKeys, key)
		mu.Unlock()
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}

	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.BaseURL = "https://audit.test/v1"
	cfg.AIChat.APIKeys = []string{"slow-one", "slow-two"}
	cfg.AIChat.RetryCount = 1
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.ReasoningEffort = "high"
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	svc.httpClient = client

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := svc.callModeration(ctx, cfg, "ordinary request", true)
	elapsed := time.Since(started)

	require.Nil(t, result)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, elapsed, 2*time.Second)
	mu.Lock()
	gotKeys := append([]string(nil), seenKeys...)
	mu.Unlock()
	require.GreaterOrEqual(t, len(gotKeys), 2)
	require.Equal(t, "slow-one", gotKeys[0])
	for _, key := range gotKeys[1:] {
		require.Equal(t, "slow-two", key, "the second logical attempt must use the distinct backup key")
	}

	loads := svc.preBlockAPIKeyLoads(cfg.apiKeys())
	require.Len(t, loads, 2)
	for _, load := range loads {
		require.Equal(t, int64(0), load.Active)
		require.Equal(t, int64(1), load.Total)
		require.Equal(t, int64(1), load.Errors)
	}
}

func TestContentModerationCallModeration_AIChatParentCancelDoesNotStartBackup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	seenKeys := make([]string, 0, 1)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		mu.Lock()
		seenKeys = append(seenKeys, key)
		mu.Unlock()
		cancel()
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}

	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.BaseURL = "https://audit.test/v1"
	cfg.AIChat.APIKeys = []string{"first-key", "backup-key"}
	cfg.AIChat.RetryCount = 1
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.ReasoningEffort = "high"
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	svc.httpClient = client

	result, err := svc.callModeration(ctx, cfg, "ordinary request", true)

	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
	mu.Lock()
	gotKeys := append([]string(nil), seenKeys...)
	mu.Unlock()
	require.Equal(t, []string{"first-key"}, gotKeys)
	loads := svc.preBlockAPIKeyLoads(cfg.apiKeys())
	require.Equal(t, int64(0), loads[0].Active)
	require.Equal(t, int64(1), loads[0].Errors)
	require.Equal(t, int64(0), loads[1].Total)
}

func TestContentModerationCallModeration_AIChatRetryCountZeroDoesNotStartBackup(t *testing.T) {
	var mu sync.Mutex
	seenKeys := make([]string, 0, 1)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		mu.Lock()
		seenKeys = append(seenKeys, key)
		mu.Unlock()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"not-json"}}]}`)),
			Request:    r,
		}, nil
	})}

	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.BaseURL = "https://audit.test/v1"
	cfg.AIChat.APIKeys = []string{"first-key", "unused-key"}
	cfg.AIChat.RetryCount = 0
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.ReasoningEffort = "high"
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	svc.httpClient = client

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := svc.callModeration(ctx, cfg, "ordinary request", true)

	require.Nil(t, result)
	require.Error(t, err)
	mu.Lock()
	gotKeys := append([]string(nil), seenKeys...)
	mu.Unlock()
	require.Equal(t, []string{"first-key"}, gotKeys)
	loads := svc.preBlockAPIKeyLoads(cfg.apiKeys())
	require.Equal(t, int64(0), loads[0].Active)
	require.Equal(t, int64(1), loads[0].Errors)
	require.Equal(t, int64(0), loads[1].Total)
}

func TestContentModerationCheck_RecordsExtractionFailures(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		failurePolicy string
		wantAllowed   bool
		wantAction    string
		wantErrorCode string
	}{
		{
			name:          "malformed json follows fail open policy",
			body:          `{"input":`,
			failurePolicy: ContentModerationFailurePolicyAllow,
			wantAllowed:   true,
			wantAction:    ContentModerationActionError,
			wantErrorCode: "input_extraction_invalid_json",
		},
		{
			name:          "malformed json follows fail closed policy",
			body:          `{"input":`,
			failurePolicy: ContentModerationFailurePolicyBlock,
			wantAllowed:   false,
			wantAction:    ContentModerationActionError,
			wantErrorCode: "input_extraction_invalid_json",
		},
		{
			name:          "valid empty input is recorded but allowed",
			body:          `{"input":[]}`,
			failurePolicy: ContentModerationFailurePolicyBlock,
			wantAllowed:   true,
			wantAction:    ContentModerationActionSkip,
			wantErrorCode: "input_extraction_empty_content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultContentModerationConfig()
			cfg.Enabled = true
			cfg.Mode = ContentModerationModePreBlock
			cfg.AuditProvider = ContentModerationProviderAIChat
			cfg.AIChat.FailurePolicy = tt.failurePolicy
			rawCfg, err := json.Marshal(cfg)
			require.NoError(t, err)
			repo := &contentModerationTestRepo{}
			svc := NewContentModerationService(
				&contentModerationTestSettingRepo{values: map[string]string{
					SettingKeyRiskControlEnabled:      "true",
					SettingKeyContentModerationConfig: string(rawCfg),
				}},
				repo, &contentModerationTestHashCache{}, nil, nil, nil, nil, nil,
			)

			decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
				Endpoint: "/v1/responses",
				Protocol: ContentModerationProtocolOpenAIResponses,
				Body:     []byte(tt.body),
			})

			require.NoError(t, err)
			require.Equal(t, tt.wantAllowed, decision.Allowed)
			logs := requireContentModerationLogCount(t, repo, 1)
			require.Equal(t, tt.wantAction, logs[0].Action)
			require.Contains(t, logs[0].Error, tt.wantErrorCode)
		})
	}
}

func TestContentModerationCallModeration_AIChatCachesSuccessfulResult(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"role":    "assistant",
				"content": `{"flagged":false,"risk_score":0.1,"categories":[],"reason":"benign"}`,
			}}},
		})
	}))
	defer server.Close()

	cache := &contentModerationTestHashCache{}
	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.BaseURL = server.URL + "/v1"
	cfg.AIChat.APIKeys = []string{"deepseek-test-key"}
	cfg.AIChat.CacheEnabled = true
	cfg.AIChat.CacheTTLSeconds = 123
	cfg.normalize()
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	svc.httpClient = server.Client()

	first, err := svc.callModeration(context.Background(), cfg, "same content")
	require.NoError(t, err)
	second, err := svc.callModeration(context.Background(), cfg, "same content")
	require.NoError(t, err)
	require.False(t, first.ResultCacheHit)
	require.NotEmpty(t, first.AuditKeyHash)
	require.True(t, second.ResultCacheHit)
	require.Empty(t, second.AuditKeyHash)
	require.Zero(t, second.InputChars, "a result-cache hit does not send input to DeepSeek")
	firstComparable := *first
	secondComparable := *second
	firstComparable.AuditKeyHash = ""
	firstComparable.InputChars = 0
	secondComparable.ResultCacheHit = false
	require.Equal(t, firstComparable, secondComparable)
	require.Equal(t, 1, requestCount)
	require.Len(t, cache.results, 1)
	for _, ttl := range cache.resultTTLs {
		require.Equal(t, 123*time.Second, ttl)
	}
}

func TestModerationAPIResultCacheJSONPreservesCategories(t *testing.T) {
	for _, categories := range [][]string{
		{},
		{"cyber_abuse", "policy_evasion"},
	} {
		original := moderationAPIResult{
			Flagged:        len(categories) > 0,
			CategoryScores: map[string]float64{"ai_risk": 0.8},
			Categories:     append([]string{}, categories...),
			Signals:        []string{"progressive_escalation"},
		}
		raw, err := json.Marshal(original)
		require.NoError(t, err)
		var restored moderationAPIResult
		require.NoError(t, json.Unmarshal(raw, &restored))
		require.Equal(t, original.Categories, restored.Categories)
	}

	var upstream moderationAPIResponse
	require.NoError(t, json.Unmarshal([]byte(`{"results":[{"flagged":false,"categories":{"hate":false},"category_scores":{"hate":0.01}}]}`), &upstream))
	require.Len(t, upstream.Results, 1, "the cache-only field must not conflict with OpenAI's object-shaped categories")
}

func TestContentModerationCallModeration_AIChatIncompleteReviewIsNotCached(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{
					"role":    "assistant",
					"content": `{"flagged":false,"risk_score":0.4,"categories":[],"signals":[],"reason":"needs review"}`,
				}}},
			})
			return
		}
		http.Error(w, "temporary audit failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cache := &contentModerationTestHashCache{}
	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.BaseURL = server.URL + "/v1"
	cfg.AIChat.APIKeys = []string{"deepseek-test-key"}
	cfg.AIChat.CacheEnabled = true
	cfg.AIChat.ReasoningEffort = "adaptive"
	cfg.AIChat.RetryCount = 4
	cfg.normalize()
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	svc.httpClient = server.Client()

	result, err := svc.callModeration(context.Background(), cfg, "ambiguous request")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.ReviewIncomplete)
	require.Contains(t, result.ReviewError, "temporary")
	require.Empty(t, cache.results, "a provisional fast-pass result must never become a final cache hit")
	require.Equal(t, 3, requestCount, "fast pass plus one full-review attempt and one bounded fallback")
}

func TestContentModerationCheckSync_AIChatIncompleteReviewDefersRiskAndSideEffects(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{
					"role":    "assistant",
					"content": `{"flagged":false,"risk_score":0.4,"categories":["cyber_abuse"],"signals":["ownership_unverified"],"reason":"needs full review"}`,
				}}},
			})
			return
		}
		http.Error(w, "temporary audit failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cache := &contentModerationTestHashCache{}
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	svc.repo = repo
	svc.httpClient = server.Client()
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.BaseURL = server.URL + "/v1"
	cfg.AIChat.APIKeys = []string{"deepseek-test-key"}
	cfg.AIChat.CacheEnabled = true
	cfg.AIChat.ReasoningEffort = "adaptive"
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyAllow
	cfg.AIChat.RiskLevelsEnabled = true
	cfg.AIChat.SessionRiskEnabled = true
	cfg.RecordNonHits = false
	cfg.normalize()
	input := ContentModerationCheckInput{
		RequestID: "req-incomplete-1",
		UserID:    7,
		APIKeyID:  9,
		SessionID: "conversation-1",
		Protocol:  ContentModerationProtocolOpenAIResponses,
		Endpoint:  "/v1/responses",
	}
	content := ContentModerationInput{Text: "[USER]\nambiguous request", CurrentText: "ambiguous request"}

	decision := svc.checkSync(context.Background(), input, cfg, content, content.Hash(), nil, true)

	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.Empty(t, cache.sessionStates, "the provisional fast pass must not mutate session or actor risk")
	require.Empty(t, cache.results, "the provisional fast pass must not be cached")
	require.Empty(t, cache.snapshotRecorded(), "the provisional fast pass must not write a flagged hash")
	require.Empty(t, repo.snapshotLogs(), "the provisional fast pass must not create a duplicate audit log")
	require.Equal(t, 1, len(svc.asyncQueue), "only the supplemental review should be queued")
	task := <-svc.asyncQueue
	require.True(t, task.supplemental)
	require.Nil(t, task.log)
	require.NotNil(t, task.config)
	require.True(t, task.config.AIChat.supplementalReview)
	require.Equal(t, "high", task.config.AIChat.ReasoningEffort)
	require.NotEmpty(t, task.config.AIChat.cacheKeyAlias)
	require.Equal(t, int64(1), svc.preBlockErrors.Load())
}

func TestContentModerationCheckSync_AIChatIncompleteReviewQueueFullPersistsEvidence(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{
					"role":    "assistant",
					"content": `{"flagged":false,"risk_score":0.4,"categories":["cyber_abuse"],"signals":["ownership_unverified"],"reason":"needs full review"}`,
				}}},
			})
			return
		}
		http.Error(w, "temporary audit failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cache := &contentModerationTestHashCache{}
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	svc.repo = repo
	svc.httpClient = server.Client()
	// An unbuffered queue makes the non-blocking supplemental send fail while
	// still exercising the real queue-capacity path.
	svc.asyncQueue = make(chan contentModerationTask)
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.BaseURL = server.URL + "/v1"
	cfg.AIChat.APIKeys = []string{"deepseek-test-key"}
	cfg.AIChat.ReasoningEffort = "adaptive"
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyAllow
	cfg.AIChat.RiskLevelsEnabled = true
	cfg.AIChat.SessionRiskEnabled = true
	cfg.normalize()
	input := ContentModerationCheckInput{
		RequestID: "req-incomplete-queue-full",
		UserID:    17,
		APIKeyID:  19,
		SessionID: "conversation-queue-full",
		Protocol:  ContentModerationProtocolOpenAIResponses,
		Endpoint:  "/v1/responses",
	}
	content := ContentModerationInput{Text: "ambiguous request", CurrentText: "ambiguous request"}

	decision := svc.checkSync(context.Background(), input, cfg, content, content.Hash(), nil, true)

	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.Equal(t, int64(1), svc.asyncDropped.Load())
	require.Empty(t, cache.sessionStates)
	require.Empty(t, cache.results)
	require.Empty(t, cache.snapshotRecorded())
	logs := repo.snapshotLogs()
	require.Len(t, logs, 1)
	require.Equal(t, ContentModerationAuditStatusIncomplete, logs[0].AuditStatus)
	require.Equal(t, "audit_review_incomplete", logs[0].AuditCode)
	require.Contains(t, logs[0].Error, "supplemental_queue_unavailable")
	require.False(t, logs[0].Flagged)
	require.Equal(t, ContentModerationSideEffectStatusNotApplicable, logs[0].SideEffectStatus)
	require.Equal(t, ContentModerationNotificationStatusNotRequired, logs[0].NotificationStatus)
}

func TestContentModerationCheckSync_AIChatIncompleteReviewFailurePolicyBlockReturnsPolicyStatus(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{
					"role":    "assistant",
					"content": `{"flagged":false,"risk_score":0.4,"categories":["cyber_abuse"],"signals":["ownership_unverified"],"reason":"needs full review"}`,
				}}},
			})
			return
		}
		http.Error(w, "temporary audit failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cache := &contentModerationTestHashCache{}
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	svc.repo = repo
	svc.httpClient = server.Client()
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.BaseURL = server.URL + "/v1"
	cfg.AIChat.APIKeys = []string{"deepseek-test-key"}
	cfg.AIChat.ReasoningEffort = "adaptive"
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyBlock
	cfg.AIChat.RiskLevelsEnabled = true
	cfg.AIChat.SessionRiskEnabled = true
	cfg.normalize()
	input := ContentModerationCheckInput{
		RequestID: "req-incomplete-fail-closed",
		UserID:    27,
		APIKeyID:  29,
		SessionID: "conversation-fail-closed",
		Protocol:  ContentModerationProtocolOpenAIResponses,
		Endpoint:  "/v1/responses",
	}
	content := ContentModerationInput{Text: "ambiguous request", CurrentText: "ambiguous request"}

	decision := svc.checkSync(context.Background(), input, cfg, content, content.Hash(), nil, true)

	require.False(t, decision.Allowed)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionUnavailable, decision.Action)
	require.False(t, decision.Flagged)
	require.Equal(t, cfg.BlockStatus, decision.StatusCode)
	require.Empty(t, svc.asyncQueue)
	require.Empty(t, cache.sessionStates)
	require.Empty(t, cache.results)
	require.Empty(t, cache.snapshotRecorded())
	logs := repo.snapshotLogs()
	require.Len(t, logs, 1)
	require.Equal(t, ContentModerationAuditStatusIncomplete, logs[0].AuditStatus)
	require.Equal(t, "audit_review_incomplete", logs[0].AuditCode)
	require.False(t, logs[0].Flagged)
	require.Equal(t, ContentModerationSideEffectStatusNotApplicable, logs[0].SideEffectStatus)
	require.Equal(t, ContentModerationNotificationStatusNotRequired, logs[0].NotificationStatus)
}

func TestContentModerationEnqueueAsync_SupplementalTaskIsMemoryBounded(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.QueueSize = defaultContentModerationQueueSize
	cfg.AIChat.MaxInputChars = 350000
	cfg.normalize()

	input := ContentModerationCheckInput{
		RequestID: "large-supplemental",
		UserID:    7,
		APIKeyID:  9,
		Endpoint:  "/v1/responses",
		Body:      bytes.Repeat([]byte("x"), maxContentModerationExtractionBodyBytes),
	}
	content := ContentModerationInput{
		Text:        strings.Repeat("a", cfg.AIChat.MaxInputChars+1000) + " LATEST USER INTENT",
		CurrentText: strings.Repeat("b", maxModerationExcerptRunes+100),
		Images:      []string{"data:image/png;base64," + strings.Repeat("A", 1<<20)},
	}

	svc.enqueueAsync(input, cfg, content, content.Hash(), true)

	require.Len(t, svc.asyncQueue, 1)
	task := <-svc.asyncQueue
	require.True(t, task.supplemental)
	require.Nil(t, task.input.Body, "queued work must not retain the original request body")
	require.LessOrEqual(t, len([]rune(task.content.Text)), cfg.AIChat.MaxInputChars)
	require.Contains(t, task.content.Text, "LATEST USER INTENT")
	require.LessOrEqual(t, len([]rune(task.content.CurrentText)), maxModerationExcerptRunes)
	require.Nil(t, task.content.Images, "AI chat supplemental review is text-only")
	require.Equal(t, int64(1), svc.supplementalPending.Load())
}

func TestContentModerationEnqueueAsync_SupplementalBacklogHasHardLimit(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.QueueSize = defaultContentModerationQueueSize
	cfg.normalize()
	input := ContentModerationCheckInput{UserID: 7, APIKeyID: 9, Endpoint: "/v1/responses", Body: []byte(`{"input":"test"}`)}
	content := ContentModerationInput{Text: "ordinary request", CurrentText: "ordinary request"}

	for range maxContentModerationSupplementalQueueSize + 5 {
		svc.enqueueAsync(input, cfg, content, content.Hash(), true)
	}

	require.Len(t, svc.asyncQueue, maxContentModerationSupplementalQueueSize)
	require.Equal(t, int64(maxContentModerationSupplementalQueueSize), svc.supplementalPending.Load())
	require.Equal(t, int64(5), svc.asyncDropped.Load())
	for range maxContentModerationSupplementalQueueSize {
		task := <-svc.asyncQueue
		require.Nil(t, task.input.Body)
	}
}

func TestContentModerationSupplementalQueueLimit_PreservesLongContextWithinTotalBudget(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.AIChat.MaxInputChars = defaultAIChatMaxInputChars
	require.Equal(t, maxContentModerationSupplementalQueueSize, contentModerationSupplementalQueueLimit(cfg, defaultContentModerationQueueSize))

	cfg.AIChat.MaxInputChars = maxModerationInputRunes
	require.Equal(t, maxContentModerationSupplementalRetainedRunes/maxModerationInputRunes, contentModerationSupplementalQueueLimit(cfg, defaultContentModerationQueueSize))
	require.Equal(t, 3, contentModerationSupplementalQueueLimit(cfg, 3))
}

func TestContentModerationEnqueueAsync_SupplementalCacheAliasUsesOriginalInput(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.CacheEnabled = true
	cfg.AIChat.MaxInputChars = 1000
	cfg.normalize()
	content := ContentModerationInput{Text: strings.Repeat("a", 1500) + " LATEST", CurrentText: "LATEST"}
	originalAlias := contentModerationAIResultCacheKey(cfg, content.AIChatModerationInput())
	compactedAlias := contentModerationAIResultCacheKey(cfg, compactSupplementalModerationContent(content, cfg).AIChatModerationInput())
	require.NotEqual(t, originalAlias, compactedAlias)

	svc.enqueueAsync(ContentModerationCheckInput{UserID: 7}, cfg, content, content.Hash(), true)

	task := <-svc.asyncQueue
	require.Equal(t, originalAlias, task.config.AIChat.cacheKeyAlias)
}

func TestContentModerationEnqueueAsync_SupplementalSendFailureReleasesReservation(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	svc.asyncQueue = make(chan contentModerationTask)
	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.normalize()

	svc.enqueueAsync(ContentModerationCheckInput{UserID: 7}, cfg, ContentModerationInput{Text: "test"}, "hash", true)

	require.Zero(t, svc.supplementalPending.Load())
	require.Equal(t, int64(1), svc.asyncDropped.Load())
}

func TestContentModerationEnqueueAsync_SupplementalConcurrentLimitIsStrict(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.normalize()
	const attempts = maxContentModerationSupplementalQueueSize + 32
	var wg sync.WaitGroup
	wg.Add(attempts)
	for range attempts {
		go func() {
			defer wg.Done()
			svc.enqueueAsync(ContentModerationCheckInput{UserID: 7}, cfg, ContentModerationInput{Text: "test"}, "hash", true)
		}()
	}
	wg.Wait()

	require.Len(t, svc.asyncQueue, maxContentModerationSupplementalQueueSize)
	require.Equal(t, int64(maxContentModerationSupplementalQueueSize), svc.supplementalPending.Load())
	require.Equal(t, int64(attempts-maxContentModerationSupplementalQueueSize), svc.asyncDropped.Load())
}

func TestContentModerationProcessAsyncTask_ReleasesSupplementalReservation(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	task := contentModerationTask{supplemental: true}

	svc.supplementalPending.Store(1)
	svc.processAsyncTask(context.Background(), defaultContentModerationConfig(), 1, task)
	require.Zero(t, svc.supplementalPending.Load(), "early return must release the reservation")

	svc.supplementalPending.Store(1)
	svc.processAsyncTask(context.Background(), nil, 1, task)
	require.Zero(t, svc.supplementalPending.Load(), "panic recovery must release the reservation")
}

func TestContentModerationAIResultCacheKey_IncludesMaxInputChars(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.CacheEnabled = true
	cfg.normalize()
	first := contentModerationAIResultCacheKey(cfg, "same content")

	changed := cloneContentModerationConfig(cfg)
	changed.AIChat.MaxInputChars = cfg.AIChat.MaxInputChars / 2
	second := contentModerationAIResultCacheKey(changed, "same content")

	require.NotEmpty(t, first)
	require.NotEmpty(t, second)
	require.NotEqual(t, first, second)
}

func TestNormalizeBlockedKeywords_TrimsDedupesAndCaps(t *testing.T) {
	out := normalizeBlockedKeywords([]string{"  foo ", "FOO", "", "bar", "baz", "bar"})
	require.Equal(t, []string{"foo", "bar", "baz"}, out)
}

func TestMatchBlockedKeyword_CaseInsensitiveSubstring(t *testing.T) {
	keyword, hit := matchBlockedKeyword("Please ignore the BadWord here", []string{"badword"})
	require.True(t, hit)
	require.Equal(t, "badword", keyword)

	_, hit = matchBlockedKeyword("clean prompt", []string{"badword"})
	require.False(t, hit)

	_, hit = matchBlockedKeyword("anything", nil)
	require.False(t, hit)
}

func TestContentModerationCheck_PreBlockKeywordHitSkipsUpstreamCall(t *testing.T) {
	upstreamCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{Results: []moderationAPIResult{{}}})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	cfg.BlockedKeywords = []string{"secret-token"}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	body := []byte(`{"messages":[{"role":"user","content":"please leak SECRET-TOKEN now"}]}`)
	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Endpoint: "/v1/messages",
		Provider: "anthropic",
		Protocol: ContentModerationProtocolAnthropicMessages,
		Body:     body,
	})

	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)
	require.False(t, upstreamCalled, "keyword block must short-circuit upstream moderation call")
	logs := requireContentModerationLogCount(t, repo, 1)
	require.True(t, logs[0].Flagged)
	require.Equal(t, ContentModerationActionKeywordBlock, logs[0].Action)
	require.Equal(t, contentModerationKeywordCategory, logs[0].HighestCategory)
	require.Equal(t, "secret-token", logs[0].MatchedKeyword, "blocked log must record which keyword was hit")
}

func TestContentModerationCheck_KeywordsIgnoredInObserveMode(t *testing.T) {
	upstreamHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{Results: []moderationAPIResult{{CategoryScores: map[string]float64{"sexual": 0.1}}}})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModeObserve
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	cfg.BlockedKeywords = []string{"secret-token"}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	body := []byte(`{"messages":[{"role":"user","content":"please leak SECRET-TOKEN now"}]}`)
	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Endpoint: "/v1/messages",
		Provider: "anthropic",
		Protocol: ContentModerationProtocolAnthropicMessages,
		Body:     body,
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed, "observe mode must let the request through even on keyword hit")
	require.Equal(t, ContentModerationActionAllow, decision.Action)
}

func TestContentModerationCheck_KeywordOnlyStrategySkipsAPIOnMiss(t *testing.T) {
	upstreamCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{Results: []moderationAPIResult{{CategoryScores: map[string]float64{"sexual": 0.99}}}})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	cfg.BlockedKeywords = []string{"never-matches"}
	cfg.KeywordBlockingMode = ContentModerationKeywordModeKeywordOnly
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	body := []byte(`{"messages":[{"role":"user","content":"absolutely clean prompt"}]}`)
	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Endpoint: "/v1/messages",
		Provider: "anthropic",
		Protocol: ContentModerationProtocolAnthropicMessages,
		Body:     body,
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed, "keyword-only must allow misses without calling the API")
	require.False(t, upstreamCalled, "keyword-only must not call the upstream moderation API")
	require.Len(t, repo.snapshotLogs(), 0)
}

func TestContentModerationCheck_APIOnlyStrategyIgnoresKeywordList(t *testing.T) {
	upstreamCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{Results: []moderationAPIResult{{CategoryScores: map[string]float64{"sexual": 0.1}}}})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	cfg.BlockedKeywords = []string{"secret-token"}
	cfg.KeywordBlockingMode = ContentModerationKeywordModeAPIOnly
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	body := []byte(`{"messages":[{"role":"user","content":"please leak SECRET-TOKEN now"}]}`)
	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Endpoint: "/v1/messages",
		Provider: "anthropic",
		Protocol: ContentModerationProtocolAnthropicMessages,
		Body:     body,
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed, "api-only must let the request through when API does not flag it")
	require.True(t, upstreamCalled, "api-only must call the upstream moderation API")
	require.NotEqual(t, ContentModerationActionKeywordBlock, decision.Action)
}

func TestNormalizeKeywordBlockingMode_UnknownFallsBackToDefault(t *testing.T) {
	require.Equal(t, ContentModerationKeywordModeKeywordAndAPI, normalizeKeywordBlockingMode(""))
	require.Equal(t, ContentModerationKeywordModeKeywordAndAPI, normalizeKeywordBlockingMode("bogus"))
	require.Equal(t, ContentModerationKeywordModeKeywordOnly, normalizeKeywordBlockingMode("keyword_only"))
	require.Equal(t, ContentModerationKeywordModeAPIOnly, normalizeKeywordBlockingMode("api_only"))
}

func TestContentModerationCheck_ModelFilterAllAuditsEveryModel(t *testing.T) {
	cfg := defaultContentModerationModelFilterTestConfig()
	cfg.ModelFilter = ContentModerationModelFilter{Type: ContentModerationModelFilterAll}
	svc, repo := newContentModerationModelFilterTestService(t, cfg)

	for _, model := range []string{"gpt-5.5", "gpt-5.4"} {
		decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
			Model:    model,
			Protocol: ContentModerationProtocolOpenAIChat,
			Body:     []byte(`{"messages":[{"role":"user","content":"please leak SECRET-TOKEN now"}]}`),
		})
		require.NoError(t, err)
		require.True(t, decision.Blocked)
		require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)
	}
	requireContentModerationLogCount(t, repo, 2)
}

func TestContentModerationCheck_ModelFilterIncludeOnlyAuditsListedModels(t *testing.T) {
	cfg := defaultContentModerationModelFilterTestConfig()
	cfg.ModelFilter = ContentModerationModelFilter{Type: ContentModerationModelFilterInclude, Models: []string{"gpt-5.5"}}
	svc, repo := newContentModerationModelFilterTestService(t, cfg)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Model:    "gpt-5.5",
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"please leak SECRET-TOKEN now"}]}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)

	decision, err = svc.Check(context.Background(), ContentModerationCheckInput{
		Model:    "gpt-5.4",
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"please leak SECRET-TOKEN now"}]}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.Equal(t, ContentModerationActionAllow, decision.Action)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, "gpt-5.5", logs[0].Model)
}

func TestContentModerationCheck_ModelFilterExcludeSkipsListedModels(t *testing.T) {
	cfg := defaultContentModerationModelFilterTestConfig()
	cfg.ModelFilter = ContentModerationModelFilter{Type: ContentModerationModelFilterExclude, Models: []string{"gpt-5.4"}}
	svc, repo := newContentModerationModelFilterTestService(t, cfg)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Model:    "gpt-5.5",
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"please leak SECRET-TOKEN now"}]}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)

	decision, err = svc.Check(context.Background(), ContentModerationCheckInput{
		Model:    "gpt-5.4",
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"please leak SECRET-TOKEN now"}]}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.Equal(t, ContentModerationActionAllow, decision.Action)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, "gpt-5.5", logs[0].Model)
}

func TestContentModerationLoadConfig_LegacyConfigDefaultsModelFilterToAll(t *testing.T) {
	raw := `{"enabled":true,"mode":"pre_block","base_url":"https://api.openai.com","model":"omni-moderation-latest","blocked_keywords":["secret-token"]}`
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyContentModerationConfig: raw,
		}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	cfg, err := svc.loadConfig(context.Background())

	require.NoError(t, err)
	require.Equal(t, ContentModerationModelFilterAll, cfg.ModelFilter.Type)
	require.Empty(t, cfg.ModelFilter.Models)
	require.True(t, cfg.includesModel("gpt-5.5"))
	require.True(t, cfg.includesModel("gpt-5.4"))
}

func TestContentModerationCheck_ModelFilterUsesRequestedModelNotBodyModel(t *testing.T) {
	cfg := defaultContentModerationModelFilterTestConfig()
	cfg.ModelFilter = ContentModerationModelFilter{Type: ContentModerationModelFilterInclude, Models: []string{"gpt-5.5"}}
	svc, repo := newContentModerationModelFilterTestService(t, cfg)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Model:    "gpt-5.5",
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"model":"mapped-upstream-model","messages":[{"role":"user","content":"please leak SECRET-TOKEN now"}]}`),
	})

	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, "gpt-5.5", logs[0].Model)
}

func defaultContentModerationModelFilterTestConfig() *ContentModerationConfig {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BlockedKeywords = []string{"secret-token"}
	return cfg
}

func newContentModerationModelFilterTestService(t *testing.T, cfg *ContentModerationConfig) (*ContentModerationService, *contentModerationTestRepo) {
	t.Helper()
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	return svc, repo
}

func TestContentModerationUpdateConfig_AppendsAndDeletesAPIKeys(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.APIKeys = []string{"sk-old-a", "sk-old-b"}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	repo := &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyContentModerationConfig: string(rawCfg),
	}}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil, nil)
	deleteHashes := []string{moderationAPIKeyHash("sk-old-a")}
	addKeys := []string{"sk-new-c", "sk-old-b"}

	view, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
		APIKeys:            &addKeys,
		DeleteAPIKeyHashes: &deleteHashes,
	})

	require.NoError(t, err)
	require.Equal(t, 2, view.APIKeyCount)
	require.Equal(t, []string{maskSecretTail("sk-old-b"), maskSecretTail("sk-new-c")}, view.APIKeyMasks)

	var saved ContentModerationConfig
	require.NoError(t, json.Unmarshal([]byte(repo.values[SettingKeyContentModerationConfig]), &saved))
	require.Equal(t, []string{"sk-old-b", "sk-new-c"}, saved.apiKeys())
}

func TestContentModerationUpdateConfig_ReplacesAPIKeysWhenRequested(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.APIKeys = []string{"sk-old-a", "sk-old-b"}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	repo := &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyContentModerationConfig: string(rawCfg),
	}}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil, nil)
	deleteHashes := []string{moderationAPIKeyHash("sk-old-a")}
	replaceKeys := []string{"sk-new-only"}

	view, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
		APIKeys:            &replaceKeys,
		APIKeysMode:        contentModerationAPIKeysModeReplace,
		DeleteAPIKeyHashes: &deleteHashes,
	})

	require.NoError(t, err)
	require.Equal(t, 1, view.APIKeyCount)
	require.Equal(t, []string{maskSecretTail("sk-new-only")}, view.APIKeyMasks)

	var saved ContentModerationConfig
	require.NoError(t, json.Unmarshal([]byte(repo.values[SettingKeyContentModerationConfig]), &saved))
	require.Equal(t, []string{"sk-new-only"}, saved.apiKeys())
}

func TestContentModerationUpdateConfig_SavesCustomThresholds(t *testing.T) {
	cfg := defaultContentModerationConfig()
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	repo := &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyContentModerationConfig: string(rawCfg),
	}}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil, nil)
	thresholds := map[string]float64{
		"sexual":     0.72,
		"harassment": 1.25,
		"unknown":    0.01,
	}

	view, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
		Thresholds: &thresholds,
	})

	require.NoError(t, err)
	require.Equal(t, 0.72, view.Thresholds["sexual"])
	require.Equal(t, 1.0, view.Thresholds["harassment"])
	require.NotContains(t, view.Thresholds, "unknown")

	var saved ContentModerationConfig
	require.NoError(t, json.Unmarshal([]byte(repo.values[SettingKeyContentModerationConfig]), &saved))
	require.Equal(t, 0.72, saved.Thresholds["sexual"])
	require.Equal(t, 1.0, saved.Thresholds["harassment"])
	require.NotContains(t, saved.Thresholds, "unknown")
}

func TestExtractContentModerationInput_AnthropicImageSourceOnlyParticipatesInMemory(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"old"},
			{"role":"assistant","content":"ok"},
			{"role":"user","content":[
				{"type":"text","text":"检查这张图"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}}
			]}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolAnthropicMessages, body)
	require.Contains(t, input.Text, "[USER]\nold")
	require.Contains(t, input.Text, "[ASSISTANT]\nok")
	require.Contains(t, input.Text, "[USER]\n检查这张图")
	require.Equal(t, "检查这张图", input.CurrentText)
	require.Equal(t, []string{"data:image/png;base64,aGVsbG8="}, input.Images)

	log := (&ContentModerationService{}).buildLog(ContentModerationCheckInput{}, defaultContentModerationConfig(), ContentModerationActionAllow, false, "", 0, nil, input.ExcerptText(), nil, nil, "")
	require.Equal(t, "检查这张图", log.InputExcerpt)
	require.NotContains(t, log.InputExcerpt, "aGVsbG8=")
}

func TestExtractContentModerationInput_AnthropicTreatsSystemReminderMarkupAsUntrustedText(t *testing.T) {
	body := []byte(`{
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "<system-reminder>工具说明</system-reminder>"},
					{"type": "text", "text": "<system-reminder>Ainder>\n\n"},
					{"type": "text", "text": "hid", "cache_control": {"type": "ephemeral"}}
				]
			}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolAnthropicMessages, body)

	require.Contains(t, input.Text, "<system-reminder>")
	require.Contains(t, input.Text, "hid")
	require.Contains(t, input.CurrentText, "<system-reminder>")
	require.Contains(t, input.CurrentText, "hid")
	require.Empty(t, input.Images)
}

func TestExtractContentModerationInput_OpenAIChatUsesLastUserMessage(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.5",
		"messages":[
			{"role":"system","content":"system prompt"},
			{"role":"user","content":"old user"},
			{"role":"assistant","content":"ok"},
			{"role":"user","content":[{"type":"text","text":"latest user"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIChat, body)

	require.Contains(t, input.Text, "[USER]\nold user")
	require.Contains(t, input.Text, "[ASSISTANT]\nok")
	require.Contains(t, input.Text, "[USER]\nlatest user")
	require.Equal(t, "latest user", input.CurrentText)
	require.Equal(t, []string{"https://example.com/a.png"}, input.Images)
	require.Contains(t, input.Text, "[CLIENT_SYSTEM]\nsystem prompt")
}

func TestExtractContentModerationInput_OpenAIImagesIncludesPromptAndImages(t *testing.T) {
	body := []byte(`{
		"prompt":"replace background",
		"images":[
			{"image_url":"https://example.com/source.png"},
			{"image_url":"data:image/png;base64,aGVsbG8="}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIImages, body)

	require.Equal(t, "replace background", input.Text)
	require.Equal(t, []string{"https://example.com/source.png", "data:image/png;base64,aGVsbG8="}, input.Images)
}

func TestContentModerationInput_NormalizeKeepsImagesAndModerationInputSamplesOneImage(t *testing.T) {
	images := []string{
		"data:image/png;base64,Zmlyc3Q=",
		"data:image/png;base64,c2Vjb25k",
	}
	input := ContentModerationInput{
		Text:   "check image",
		Images: append([]string(nil), images...),
	}
	input.Normalize()

	require.Equal(t, images, input.Images)

	parts, ok := input.ModerationInput().([]moderationAPIInputPart)
	require.True(t, ok)
	require.Len(t, parts, 2)
	require.Equal(t, "text", parts[0].Type)
	require.Equal(t, "image_url", parts[1].Type)
	require.NotNil(t, parts[1].ImageURL)
	require.Contains(t, images, parts[1].ImageURL.URL)
}

func TestBuildModerationTestInputRejectsMultipleImages(t *testing.T) {
	_, _, err := buildModerationTestInput("check image", []string{
		"data:image/png;base64,Zmlyc3Q=",
		"data:image/png;base64,c2Vjb25k",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "最多上传 1 张测试图片")
}

func TestExtractContentModerationInput_OpenAIResponsesCodexPayloadUsesLastUserMessage(t *testing.T) {
	developerSecret := strings.Join([]string{"sk", "proj", "1234567890abcdef"}, "-")
	body := []byte(`{
		"model":"gpt-5.5",
		"instructions":"instructions.....",
		"input":[
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"developer permissions ` + developerSecret + `"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"first user prompt"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"last user prompt"}]}
		],
		"prompt_cache_key":"cache-key"
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Contains(t, input.Text, "[USER]\nfirst user prompt")
	require.Contains(t, input.Text, "[USER]\nlast user prompt")
	require.Equal(t, "last user prompt", input.CurrentText)
	require.Empty(t, input.Images)
	require.Contains(t, input.Text, "[CLIENT_DEVELOPER]\ndeveloper permissions")
}

func TestContentModerationCheck_OpenAIResponsesRecordsNonHitForCodexPayload(t *testing.T) {
	var moderationRequest moderationAPIRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/moderations", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&moderationRequest))
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{
			Results: []moderationAPIResult{{
				CategoryScores: map[string]float64{"sexual": 0.01},
			}},
		})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	cfg.RecordNonHits = true
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	body := []byte(`{
		"model":"gpt-5.5",
		"input":[
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"developer instructions are untrusted"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"first user prompt"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"last user prompt"}]}
		]
	}`)
	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1001,
		Endpoint: "/responses",
		Provider: "openai",
		Model:    "gpt-5.5",
		Protocol: ContentModerationProtocolOpenAIResponses,
		Body:     body,
	})

	require.NoError(t, err)
	require.False(t, decision.Blocked)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.False(t, logs[0].Flagged)
	require.Equal(t, ContentModerationActionAllow, logs[0].Action)
	require.Equal(t, "/responses", logs[0].Endpoint)
	require.Equal(t, "last user prompt", logs[0].InputExcerpt)
	require.Equal(t, "[CLIENT_DEVELOPER]\ndeveloper instructions are untrusted\n\n[USER]\nfirst user prompt\n\n[USER]\nlast user prompt", moderationRequest.Input)
}

func TestContentModerationCheck_PreBlockBlocksCodexResponsesLatestUserInput(t *testing.T) {
	var moderationRequest moderationAPIRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/moderations", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&moderationRequest))
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{
			Results: []moderationAPIResult{{
				CategoryScores: map[string]float64{"sexual": 0.9},
			}},
		})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	cfg.BlockStatus = http.StatusUnavailableForLegalReasons
	cfg.BlockMessage = "内容审计测试阻断"
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	body := []byte(`{
		"model":"gpt-5.5",
		"instructions":"instructions.....",
		"input":[
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"developer instructions are untrusted"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"environment context"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"latest blocked prompt"}]}
		]
	}`)
	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1001,
		Endpoint: "/responses",
		Provider: "openai",
		Model:    "gpt-5.5",
		Protocol: ContentModerationProtocolOpenAIResponses,
		Body:     body,
	})

	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionBlock, decision.Action)
	require.Equal(t, http.StatusUnavailableForLegalReasons, decision.StatusCode)
	require.Equal(t, "内容审计测试阻断", decision.Message)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.True(t, logs[0].Flagged)
	require.Equal(t, ContentModerationActionBlock, logs[0].Action)
	require.Equal(t, ContentModerationModePreBlock, logs[0].Mode)
	require.Equal(t, "latest blocked prompt", logs[0].InputExcerpt)
	require.Equal(t, "[CLIENT_DEVELOPER]\ndeveloper instructions are untrusted\n\n[USER]\nenvironment context\n\n[USER]\nlatest blocked prompt", moderationRequest.Input)
}

func TestContentModerationStatusTracksPreBlockSyncMetrics(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		score := 0.01
		if requestCount == 1 {
			score = 0.9
		}
		time.Sleep(5 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{
			Results: []moderationAPIResult{{
				CategoryScores: map[string]float64{"sexual": score},
			}},
		})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		&contentModerationTestRepo{},
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	for _, prompt := range []string{"blocked prompt", "clean prompt"} {
		_, err := svc.Check(context.Background(), ContentModerationCheckInput{
			UserID:   1001,
			Protocol: ContentModerationProtocolOpenAIChat,
			Body:     []byte(fmt.Sprintf(`{"messages":[{"role":"user","content":%q}]}`, prompt)),
		})
		require.NoError(t, err)
	}

	status, err := svc.GetStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(2), status.PreBlockChecked)
	require.Equal(t, int64(1), status.PreBlockAllowed)
	require.Equal(t, int64(1), status.PreBlockBlocked)
	require.Equal(t, int64(0), status.PreBlockErrors)
	require.Equal(t, 0, status.PreBlockActive)
	require.GreaterOrEqual(t, status.PreBlockAvgLatencyMS, int64(1))
}

func TestContentModerationStatusTracksPreBlockAPIKeyLoad(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{
			Results: []moderationAPIResult{{
				CategoryScores: map[string]float64{"sexual": 0.01},
			}},
		})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-one", "sk-two"}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		&contentModerationTestRepo{},
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	for idx := 0; idx < 4; idx++ {
		_, err := svc.Check(context.Background(), ContentModerationCheckInput{
			UserID:   1001,
			Protocol: ContentModerationProtocolOpenAIChat,
			Body:     []byte(fmt.Sprintf(`{"messages":[{"role":"user","content":"prompt %d"}]}`, idx)),
		})
		require.NoError(t, err)
	}

	status, err := svc.GetStatus(context.Background())
	require.NoError(t, err)
	require.Len(t, status.PreBlockAPIKeyLoads, 2)
	require.Equal(t, int64(4), status.PreBlockAPIKeyTotalCalls)
	require.Equal(t, int64(2), status.PreBlockAPIKeyAvailableCount)
	require.Equal(t, int64(0), status.PreBlockAPIKeyActive)
	require.Equal(t, int64(0), status.PreBlockAPIKeyLoads[0].Active)
	require.Equal(t, int64(2), status.PreBlockAPIKeyLoads[0].Total)
	require.Equal(t, int64(2), status.PreBlockAPIKeyLoads[0].Success)
	require.Equal(t, int64(0), status.PreBlockAPIKeyLoads[0].Errors)
	require.Equal(t, int64(2), status.PreBlockAPIKeyLoads[1].Total)
	require.Equal(t, int64(2), status.PreBlockAPIKeyLoads[1].Success)
}

func TestContentModerationStatusTracksPreBlockLocalBlocks(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.KeywordBlockingMode = ContentModerationKeywordModeKeywordOnly
	cfg.BlockedKeywords = []string{"blocked"}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		&contentModerationTestRepo{},
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	for _, prompt := range []string{"blocked prompt", "clean prompt"} {
		_, err := svc.Check(context.Background(), ContentModerationCheckInput{
			UserID:   1001,
			Protocol: ContentModerationProtocolOpenAIChat,
			Body:     []byte(fmt.Sprintf(`{"messages":[{"role":"user","content":%q}]}`, prompt)),
		})
		require.NoError(t, err)
	}

	status, err := svc.GetStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(2), status.PreBlockChecked)
	require.Equal(t, int64(1), status.PreBlockAllowed)
	require.Equal(t, int64(1), status.PreBlockBlocked)
	require.Equal(t, int64(0), status.PreBlockErrors)
}

func TestBuildContentModerationTestAuditResult_UsesConfiguredThresholdsOnly(t *testing.T) {
	result := buildContentModerationTestAuditResult(&moderationAPIResult{
		Flagged: true,
		CategoryScores: map[string]float64{
			"harassment": 0.65,
		},
	}, nil)

	require.NotNil(t, result)
	require.False(t, result.Flagged)
	require.Equal(t, "harassment", result.HighestCategory)
	require.Equal(t, 0.65, result.HighestScore)
	require.Equal(t, 0.65, result.CompositeScore)
	require.Equal(t, 0.98, result.Thresholds["harassment"])
	require.Equal(t, 0.65, result.RiskScore)
	require.Empty(t, result.RiskTier)
	require.Empty(t, result.Categories)
	require.Empty(t, result.Signals)
}

func TestBuildContentModerationTestAuditResult_ExposesAIReviewDetails(t *testing.T) {
	result := buildContentModerationTestAuditResult(&moderationAPIResult{
		Flagged:          true,
		CategoryScores:   map[string]float64{"ai_risk": 0.82, "credential_theft": 0.82},
		Categories:       []string{"credential_theft"},
		Signals:          []string{"ownership_unverified", "auth_bypass"},
		Reason:           "请求绕过官方找回流程",
		ReviewIncomplete: true,
		ReviewError:      "supplemental review timeout",
	}, map[string]float64{"ai_risk": 0.7}, 0.35, 0.7)

	require.NotNil(t, result)
	require.True(t, result.Flagged)
	require.Equal(t, 0.82, result.RiskScore)
	require.Equal(t, "high", result.RiskTier)
	require.Equal(t, []string{"credential_theft"}, result.Categories)
	require.Equal(t, []string{"ownership_unverified", "auth_bypass"}, result.Signals)
	require.Equal(t, "请求绕过官方找回流程", result.Reason)
	require.True(t, result.ReviewIncomplete)
	require.Equal(t, "supplemental review timeout", result.ReviewError)
}

func TestBuildContentModerationTestAuditResult_WeakSignalsRemainLowTierAtHighRawScore(t *testing.T) {
	result := buildContentModerationTestAuditResult(&moderationAPIResult{
		Flagged:        false,
		CategoryScores: map[string]float64{"ai_risk": 0.95},
		Signals:        []string{"ownership_unverified"},
		Reason:         "仅所有权尚未核实",
	}, map[string]float64{"ai_risk": 0.7}, 0.35, 0.7)

	require.NotNil(t, result)
	require.False(t, result.Flagged)
	require.Equal(t, 0.95, result.RiskScore)
	require.Equal(t, "low", result.RiskTier)
}

func TestContentModerationConfigViewReportsPromptMetadataWithoutReplacingCustomPrompt(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	cfg := defaultContentModerationConfig()

	recommendedView := svc.configView(cfg)
	require.NotEmpty(t, recommendedView.AIChat.RecommendedSystemPrompt)
	require.Equal(t, voteaimoderation.RecommendedSystemPromptVersion, recommendedView.AIChat.RecommendedPromptVersion)
	require.Equal(t, recommendedView.AIChat.RecommendedPromptVersion, recommendedView.AIChat.SystemPromptVersion)
	require.True(t, recommendedView.AIChat.UsesRecommendedSystemPrompt)

	cfg.AIChat.SystemPrompt = "管理员自定义审核提示词"
	cfg.normalize()
	customView := svc.configView(cfg)
	require.Equal(t, "管理员自定义审核提示词", customView.AIChat.SystemPrompt)
	require.Equal(t, "custom", customView.AIChat.SystemPromptVersion)
	require.False(t, customView.AIChat.UsesRecommendedSystemPrompt)
}

func TestContentModerationCallModeration_400DoesNotFreezeAPIKey(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Number of images (5) exceeds maximum of 1","type":"invalid_request_error","param":"input","code":"too_many_images"}}`))
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	cfg.RetryCount = 5
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)

	_, err := svc.callModeration(context.Background(), cfg, "hello")

	require.Error(t, err)
	require.Equal(t, 1, requestCount)
	status := svc.apiKeyStatusForHash(0, moderationAPIKeyHash("sk-test"), maskSecretTail("sk-test"), true)
	require.Equal(t, "error", status.Status)
	require.Equal(t, http.StatusBadRequest, status.LastHTTPStatus)
	require.Zero(t, status.FailureCount)
	require.Nil(t, status.FrozenUntil)
}

func TestContentModerationCallModeration_FreezesByHTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		minFreeze  time.Duration
		maxFreeze  time.Duration
	}{
		{name: "401 freezes ten minutes", statusCode: http.StatusUnauthorized, minFreeze: 9*time.Minute + 55*time.Second, maxFreeze: 10*time.Minute + time.Second},
		{name: "403 freezes ten minutes", statusCode: http.StatusForbidden, minFreeze: 9*time.Minute + 55*time.Second, maxFreeze: 10*time.Minute + time.Second},
		{name: "429 freezes one minute", statusCode: http.StatusTooManyRequests, minFreeze: 55 * time.Second, maxFreeze: time.Minute + time.Second},
		{name: "529 freezes one minute", statusCode: 529, minFreeze: 55 * time.Second, maxFreeze: time.Minute + time.Second},
		{name: "500 freezes ten seconds", statusCode: http.StatusInternalServerError, minFreeze: 5 * time.Second, maxFreeze: 11 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"error":{"message":"upstream error"}}`))
			}))
			defer server.Close()

			cfg := defaultContentModerationConfig()
			cfg.BaseURL = server.URL
			cfg.APIKeys = []string{"sk-test"}
			cfg.RetryCount = 0
			svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)

			_, err := svc.callModeration(context.Background(), cfg, "hello")

			require.Error(t, err)
			status := svc.apiKeyStatusForHash(0, moderationAPIKeyHash("sk-test"), maskSecretTail("sk-test"), true)
			require.Equal(t, "frozen", status.Status)
			require.Equal(t, tt.statusCode, status.LastHTTPStatus)
			require.Equal(t, 1, status.FailureCount)
			require.NotNil(t, status.FrozenUntil)
			remaining := time.Until(*status.FrozenUntil)
			require.GreaterOrEqual(t, remaining, tt.minFreeze)
			require.LessOrEqual(t, remaining, tt.maxFreeze)
		})
	}
}

func TestContentModerationMarkAPIKeyError_HTTP200DoesNotFreezeAPIKey(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)

	svc.markAPIKeyError("sk-test", "AI audit API returned empty content", 25, http.StatusOK)

	status := svc.apiKeyStatusForHash(0, moderationAPIKeyHash("sk-test"), maskSecretTail("sk-test"), true)
	require.Equal(t, "error", status.Status)
	require.Equal(t, http.StatusOK, status.LastHTTPStatus)
	require.Zero(t, status.FailureCount)
	require.Nil(t, status.FrozenUntil)
}

func TestContentModerationTestAPIKeys_400DoesNotFreezeAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid moderation request"}}`))
	}))
	defer server.Close()

	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	result, err := svc.TestAPIKeys(context.Background(), TestContentModerationAPIKeysInput{
		APIKeys: []string{"sk-test"},
		BaseURL: server.URL,
		Prompt:  "hello",
	})

	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "error", result.Items[0].Status)
	require.Equal(t, http.StatusBadRequest, result.Items[0].LastHTTPStatus)
	require.Zero(t, result.Items[0].FailureCount)
	require.Nil(t, result.Items[0].FrozenUntil)
	require.Nil(t, result.AuditResult)
	require.NotNil(t, result.AuditError)
	require.Equal(t, "audit_request_failed", result.AuditError.Code)
	require.Equal(t, http.StatusBadRequest, result.AuditError.HTTPStatus)
}

func TestContentModerationTestAPIKeys_HTTP200WithoutValidAuditResultReturnsStructuredError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"role":    "assistant",
				"content": "not-json",
			}}},
		})
	}))
	defer server.Close()

	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{}},
		nil, nil, nil, nil, nil, nil, nil,
	)
	result, err := svc.TestAPIKeys(context.Background(), TestContentModerationAPIKeysInput{
		APIKeys:       []string{"deepseek-test-key"},
		AuditProvider: ContentModerationProviderAIChat,
		BaseURL:       server.URL + "/v1",
		Model:         "deepseek-v4-flash",
		Prompt:        "test prompt",
	})

	require.NoError(t, err)
	require.Nil(t, result.AuditResult)
	require.NotNil(t, result.AuditError)
	require.Equal(t, "audit_request_failed", result.AuditError.Code)
	require.Equal(t, http.StatusOK, result.AuditError.HTTPStatus)
	require.Contains(t, result.AuditError.Message, "JSON")
	require.Len(t, result.Items, 1)
	require.Equal(t, "error", result.Items[0].Status)
	require.Equal(t, http.StatusOK, result.Items[0].LastHTTPStatus)
}

func TestContentModerationTestAPIKeys_UsesDraftRiskLevelSettingsAndLabelsRequestScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"role":    "assistant",
				"content": `{"flagged":false,"risk_score":0.5,"categories":["other"],"signals":[],"reason":"manual review"}`,
			}}},
		})
	}))
	defer server.Close()

	for _, tt := range []struct {
		name         string
		riskLevels   bool
		wantRiskTier string
	}{
		{name: "draft levels enabled", riskLevels: true, wantRiskTier: "observe"},
		{name: "draft levels disabled", riskLevels: false, wantRiskTier: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			riskLevels := tt.riskLevels
			svc := NewContentModerationService(
				&contentModerationTestSettingRepo{values: map[string]string{}},
				nil, nil, nil, nil, nil, nil, nil,
			)
			result, err := svc.TestAPIKeys(context.Background(), TestContentModerationAPIKeysInput{
				APIKeys:               []string{"deepseek-test-key"},
				AuditProvider:         ContentModerationProviderAIChat,
				BaseURL:               server.URL + "/v1",
				Model:                 "deepseek-v4-flash",
				Prompt:                "test prompt",
				AIConfidenceThreshold: 0.7,
				AIRiskLevelsEnabled:   &riskLevels,
				AIObserveThreshold:    0.45,
			})

			require.NoError(t, err)
			require.Nil(t, result.AuditError)
			require.NotNil(t, result.AuditResult)
			require.Equal(t, tt.wantRiskTier, result.AuditResult.RiskTier)
			require.Equal(t, "request", result.AuditResult.Scope)
		})
	}
}

func TestContentModerationCheck_PreHashUsesRedisHashCache(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.PreHashCheckEnabled = true
	cfg.APIKeys = []string{"sk-test"}
	cfg.BlockStatus = http.StatusConflict
	cfg.BlockMessage = "命中历史风险输入"
	cfg.AutoBanEnabled = true
	cfg.BanThreshold = 1
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	hashCache := &contentModerationTestHashCache{hashes: map[string]struct{}{}}
	content := ContentModerationInput{
		Text:            "[USER]\nblocked prompt",
		CurrentText:     "blocked prompt",
		AuditTargetText: "blocked prompt",
		AuditTargetKind: "user_request",
	}
	content.Normalize()
	hashText := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))
	hashCache.hashes[hashText] = struct{}{}

	repo := &contentModerationTestRepo{}
	userRepo := &contentModerationTestUserRepo{user: &User{ID: 1001, Status: StatusActive}}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		hashCache,
		nil,
		userRepo,
		nil,
		nil,
		nil,
	)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1001,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"blocked prompt"}]}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionHashBlock, decision.Action)
	require.Equal(t, http.StatusConflict, decision.StatusCode)
	require.Equal(t, hashText, decision.InputHash)
	require.Contains(t, decision.Message, "命中历史风险输入")
	require.Contains(t, decision.Message, hashText)
	require.Len(t, hashCache.snapshotChecked(), 1)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.True(t, logs[0].Flagged)
	require.Equal(t, ContentModerationActionHashBlock, logs[0].Action)
	require.Equal(t, 1.0, logs[0].CategoryScores["hash"])
	require.Equal(t, ContentModerationModePreBlock, logs[0].Mode)
	require.Zero(t, logs[0].ViolationCount)
	require.False(t, logs[0].AutoBanned)
	require.Empty(t, userRepo.updated)
}

func TestContentModerationCheck_HashBlockLogsDoNotIncreaseNextViolationCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{
			Results: []moderationAPIResult{{
				CategoryScores: map[string]float64{"sexual": 0.9},
			}},
		})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	cfg.AutoBanEnabled = false
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	userID := int64(1001)
	repo := &contentModerationTestRepo{}
	hashLog := &ContentModerationLog{
		UserID:          &userID,
		Action:          ContentModerationActionHashBlock,
		Flagged:         true,
		HighestCategory: "hash",
		HighestScore:    1,
		CreatedAt:       time.Now(),
	}
	require.NoError(t, repo.CreateLog(context.Background(), hashLog))

	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   userID,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"new blocked prompt"}]}`),
	})

	require.NoError(t, err)
	require.True(t, decision.Blocked)
	var logs []ContentModerationLog
	require.Eventually(t, func() bool {
		logs = repo.snapshotLogs()
		return len(logs) == 2 && logs[1].SideEffectStatus != ContentModerationSideEffectStatusPending
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, ContentModerationActionHashBlock, logs[0].Action)
	require.Equal(t, ContentModerationActionBlock, logs[1].Action)
	require.Equal(t, 1, logs[1].ViolationCount)
}

func TestContentModerationAutoBanLeavesIneligibleAccountUnchanged(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.BanThreshold = 2
	cfg.ViolationWindowHours = 24

	userID := int64(1001)
	repo := &contentModerationTestRepo{banOutcome: ContentModerationBanOutcomeIneligible}
	require.NoError(t, repo.CreateLog(context.Background(), newContentModerationFlaggedLog(userID)))
	invalidator := &contentModerationTestAuthCacheInvalidator{}
	svc := NewContentModerationService(nil, repo, nil, nil, nil, nil, invalidator, nil)

	svc.persistContentModerationLog(context.Background(), cfg, newContentModerationFlaggedLog(userID), "", false, true)

	logs := requireContentModerationLogCount(t, repo, 2)
	require.Equal(t, 2, logs[1].ViolationCount)
	require.False(t, logs[1].AutoBanned)
	require.Empty(t, invalidator.userIDs)
}

func TestContentModerationAutoBanDisablesRegularUserAtThreshold(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.BanThreshold = 2
	cfg.ViolationWindowHours = 24

	userID := int64(1001)
	repo := &contentModerationTestRepo{banOutcome: ContentModerationBanOutcomeApplied}
	require.NoError(t, repo.CreateLog(context.Background(), newContentModerationFlaggedLog(userID)))
	invalidator := &contentModerationTestAuthCacheInvalidator{}
	svc := NewContentModerationService(nil, repo, nil, nil, nil, nil, invalidator, nil)

	svc.persistContentModerationLog(context.Background(), cfg, newContentModerationFlaggedLog(userID), "", false, true)

	logs := requireContentModerationLogCount(t, repo, 2)
	require.Equal(t, 2, logs[1].ViolationCount)
	require.True(t, logs[1].AutoBanned)
	require.NotNil(t, repo.moderationState)
	require.True(t, repo.moderationState.ModerationOwnedDisabled)
	require.Equal(t, []int64{userID}, invalidator.userIDs)
}

func TestContentModerationBanSideEffectReturnsExplicitOutcome(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.BanThreshold = 1
	cfg.ViolationWindowHours = 0
	userID := int64(1001)

	for _, expected := range []string{
		ContentModerationBanOutcomeApplied,
		ContentModerationBanOutcomeAlreadyOwned,
		ContentModerationBanOutcomeIneligible,
	} {
		t.Run(expected, func(t *testing.T) {
			repo := &contentModerationTestRepo{banOutcome: expected}
			invalidator := &contentModerationTestAuthCacheInvalidator{}
			svc := NewContentModerationService(nil, repo, nil, nil, nil, nil, invalidator, nil)
			log := newContentModerationFlaggedLog(userID)
			log.ID = 42

			outcome, err := svc.applyFlaggedAccountSideEffects(context.Background(), cfg, log)

			require.NoError(t, err)
			require.Equal(t, expected, outcome)
			require.Equal(t, expected == ContentModerationBanOutcomeApplied, log.AutoBanned)
			if expected == ContentModerationBanOutcomeApplied {
				require.Equal(t, []int64{userID}, invalidator.userIDs)
			} else {
				require.Empty(t, invalidator.userIDs)
			}
		})
	}
}

func TestContentModerationAlreadyOwnedBanSuppressesOrdinaryNotification(t *testing.T) {
	userID := int64(1001)
	svc := &ContentModerationService{emailService: &EmailService{}}
	cfg := defaultContentModerationConfig()
	log := newContentModerationFlaggedLog(userID)
	log.UserEmail = "user@example.com"

	status, sent, err := svc.sendFlaggedNotificationSideEffectsForBanOutcome(
		context.Background(), cfg, log, ContentModerationBanOutcomeAlreadyOwned,
	)

	require.NoError(t, err)
	require.False(t, sent)
	require.Equal(t, ContentModerationNotificationStatusNotRequired, status)
}

func TestContentModerationNotificationFailureReleasesDedupeLease(t *testing.T) {
	dedupe := &contentModerationEmailDedupeTestStore{}
	cache := &contentModerationEmailIntegrationCache{
		contentModerationTestHashCache:        &contentModerationTestHashCache{},
		contentModerationEmailDedupeTestStore: dedupe,
	}
	emailService := NewEmailService(&contentModerationTestSettingRepo{values: map[string]string{}}, nil)
	svc := &ContentModerationService{hashCache: cache, emailService: emailService}
	userID, apiKeyID := int64(1001), int64(2002)
	log := newContentModerationFlaggedLog(userID)
	log.UserEmail = "user@example.com"
	log.APIKeyID = &apiKeyID
	log.SessionID = "conversation-a"
	log.InputHash = "input-a"
	cfg := defaultContentModerationConfig()

	status, sent, err := svc.sendFlaggedNotificationSideEffectsForBanOutcome(
		context.Background(), cfg, log, ContentModerationBanOutcomeIneligible,
	)

	require.Error(t, err)
	require.False(t, sent)
	require.Equal(t, ContentModerationNotificationStatusFailed, status)
	retry := svc.reserveContentModerationEmailForLog(context.Background(), log)
	require.True(t, retry.ShouldSend, "a failed email send must release its dedupe lease")
}

func TestRecordCyberPolicyEvent_AlreadyOwnedBanSuppressesCyberNotification(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.BanThreshold = 1
	cfg.ViolationWindowHours = 0
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{banOutcome: ContentModerationBanOutcomeAlreadyOwned}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo, &contentModerationTestHashCache{}, nil, nil, nil, nil, &EmailService{},
	)

	svc.RecordCyberPolicyEvent(context.Background(), CyberPolicyRecordInput{
		UserID: 1001, UserEmail: "user@example.com", APIKeyID: 2002,
		SessionID: "conversation-a", InputHash: "input-a", UpstreamMessage: "cyber policy",
	})

	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, ContentModerationNotificationStatusNotRequired, logs[0].NotificationStatus)
	require.False(t, logs[0].EmailSent)
	require.False(t, logs[0].AutoBanned)
	require.Equal(t, ContentModerationSideEffectStatusCompleted, logs[0].SideEffectStatus)
}

func TestContentModerationSideEffectStatusUsesOperationCounts(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.BanThreshold = 1
	cfg.ViolationWindowHours = 0
	userID := int64(1001)

	t.Run("completed when all attempted operations succeed", func(t *testing.T) {
		repo := &contentModerationTestRepo{banOutcome: ContentModerationBanOutcomeIneligible}
		svc := NewContentModerationService(nil, repo, nil, nil, nil, nil, nil, nil)
		svc.persistContentModerationLog(context.Background(), cfg, newContentModerationFlaggedLog(userID), "", false, true)

		logs := requireContentModerationLogCount(t, repo, 1)
		require.Equal(t, ContentModerationSideEffectStatusCompleted, logs[0].SideEffectStatus)
		require.Empty(t, logs[0].SideEffectError)
	})

	t.Run("failed when every attempted operation fails", func(t *testing.T) {
		repo := &contentModerationTestRepo{banErr: errors.New("ban store unavailable")}
		svc := NewContentModerationService(nil, repo, nil, nil, nil, nil, nil, nil)
		svc.persistContentModerationLog(context.Background(), cfg, newContentModerationFlaggedLog(userID), "", false, true)

		logs := requireContentModerationLogCount(t, repo, 1)
		require.Equal(t, ContentModerationSideEffectStatusFailed, logs[0].SideEffectStatus)
		require.Contains(t, logs[0].SideEffectError, "ban store unavailable")
	})

	t.Run("partial when successful hash recording precedes a failed ban", func(t *testing.T) {
		repo := &contentModerationTestRepo{banErr: errors.New("ban store unavailable")}
		cache := &contentModerationTestHashCache{}
		svc := NewContentModerationService(nil, repo, cache, nil, nil, nil, nil, nil)
		svc.persistContentModerationLog(context.Background(), cfg, newContentModerationFlaggedLog(userID), strings.Repeat("a", 64), true, true)

		logs := requireContentModerationLogCount(t, repo, 1)
		require.Equal(t, ContentModerationSideEffectStatusPartial, logs[0].SideEffectStatus)
		require.Contains(t, logs[0].SideEffectError, "ban store unavailable")
	})
}

func TestContentModerationAdminBelowBanThresholdRecordsViolationOnly(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.BanThreshold = 2
	cfg.ViolationWindowHours = 24

	userID := int64(1001)
	repo := &contentModerationTestRepo{}
	invalidator := &contentModerationTestAuthCacheInvalidator{}
	svc := NewContentModerationService(nil, repo, nil, nil, nil, nil, invalidator, nil)

	svc.persistContentModerationLog(context.Background(), cfg, newContentModerationFlaggedLog(userID), "", false, true)

	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, 1, logs[0].ViolationCount)
	require.False(t, logs[0].AutoBanned)
	require.Empty(t, invalidator.userIDs)
}

func newContentModerationFlaggedLog(userID int64) *ContentModerationLog {
	return &ContentModerationLog{
		UserID:          &userID,
		Action:          ContentModerationActionBlock,
		Flagged:         true,
		HighestCategory: "sexual",
		HighestScore:    0.9,
		CreatedAt:       time.Now(),
	}
}

func TestContentModerationCheck_PreBlockFlaggedWritesRedisHashCache(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{
			Results: []moderationAPIResult{{
				CategoryScores: map[string]float64{"sexual": 0.9},
			}},
		})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.PreHashCheckEnabled = true
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	cfg.BlockStatus = http.StatusConflict
	cfg.BlockMessage = "命中风险输入"
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	repo := &contentModerationTestRepo{}
	hashCache := &contentModerationTestHashCache{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		hashCache,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	body := []byte(`{"messages":[{"role":"user","content":"repeat blocked prompt"}]}`)
	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     body,
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionBlock, decision.Action)
	require.Equal(t, 1, requestCount)
	recorded := requireRecordedHashCount(t, hashCache, 1)
	requireContentModerationLogCount(t, repo, 1)

	decision, err = svc.Check(context.Background(), ContentModerationCheckInput{
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     body,
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionHashBlock, decision.Action)
	require.Equal(t, recorded[0], decision.InputHash)
	require.Equal(t, 1, requestCount)
	logs := requireContentModerationLogCount(t, repo, 2)
	require.Equal(t, ContentModerationActionBlock, logs[0].Action)
	require.Equal(t, ContentModerationActionHashBlock, logs[1].Action)
}

func TestContentModerationDeleteFlaggedInputHash_NormalizesAndDeletes(t *testing.T) {
	existingHash := strings.Repeat("a", 64)
	hashCache := &contentModerationTestHashCache{hashes: map[string]struct{}{
		existingHash: {},
	}}
	svc := &ContentModerationService{hashCache: hashCache}

	result, err := svc.DeleteFlaggedInputHash(context.Background(), strings.ToUpper(existingHash))

	require.NoError(t, err)
	require.Equal(t, existingHash, result.InputHash)
	require.True(t, result.Deleted)
	require.False(t, hashCache.hasHash(existingHash))
	require.Equal(t, []string{existingHash}, hashCache.snapshotDeleted())

	result, err = svc.DeleteFlaggedInputHash(context.Background(), existingHash)

	require.NoError(t, err)
	require.Equal(t, existingHash, result.InputHash)
	require.False(t, result.Deleted)
}

func TestContentModerationSuppressedHashBypassesStaleAIResultCache(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "fresh provider allow")
	}))
	defer server.Close()

	inputHash := strings.Repeat("b", 64)
	cache := &contentModerationTestHashCache{suppressions: map[string]struct{}{inputHash: {}}}
	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.CacheEnabled = true
	cfg.AIChat.CacheTTLSeconds = 3600
	cfg.AIChat.inputHash = inputHash
	cfg.AIChat.auditStage = string(voteaimoderation.StageFast)
	input := "same false-positive input"
	cacheKey := contentModerationAIResultCacheKey(cfg, input)
	require.NotEmpty(t, cacheKey)
	stale, err := json.Marshal(moderationAPIResult{
		Flagged: true, CategoryScores: map[string]float64{"ai_risk": 0.95},
		Stage: voteaimoderation.StageFast,
	})
	require.NoError(t, err)
	require.NoError(t, cache.SetContentModerationResult(context.Background(), cacheKey, stale, time.Hour))

	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	svc.httpClient = server.Client()
	first, err := svc.callModeration(context.Background(), cfg, input)
	require.NoError(t, err)
	require.False(t, first.Flagged)
	require.False(t, first.ResultCacheHit)
	second, err := svc.callModeration(context.Background(), cfg, input)
	require.NoError(t, err)
	require.False(t, second.Flagged)
	require.Equal(t, 2, requestCount, "suppression must fence both reads and writes of the stale result cache")
}

func TestContentModerationSuppressedAsyncRequestBypassesStaleCaches(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "fresh provider allow")
	}))
	defer server.Close()

	inputHash := strings.Repeat("d", 64)
	cache := &contentModerationTestHashCache{suppressions: map[string]struct{}{inputHash: {}}}
	cfg := contentModerationGuardConfig(server.URL)
	cfg.Mode = ContentModerationModeObserve
	cfg.AIChat.CacheEnabled = true
	cfg.AIChat.CacheTTLSeconds = 3600
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.inputHash = ""
	content := contentModerationGuardInput("same false-positive async input")
	cacheKey := contentModerationAIResultCacheKey(cfg, content.AIChatModerationInput())
	require.NotEmpty(t, cacheKey)
	stale, err := json.Marshal(moderationAPIResult{
		Flagged: true, CategoryScores: map[string]float64{"ai_risk": 0.95},
	})
	require.NoError(t, err)
	require.NoError(t, cache.SetContentModerationResult(context.Background(), cacheKey, stale, time.Hour))

	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	input := ContentModerationCheckInput{
		RequestID: "suppressed-async-retry", UserID: 17, APIKeyID: 27,
		SessionID: "suppressed-async", ModerationEpochSet: true,
	}
	queueDelay := 5
	for range 2 {
		decision := svc.checkSyncIdempotent(context.Background(), input, cfg, content, inputHash, &queueDelay, false)
		require.NotNil(t, decision)
		require.True(t, decision.Allowed)
		require.False(t, decision.Flagged)
	}

	require.Equal(t, 2, requestCount, "a suppressed async target must bypass both the ordinary result cache and request ledger")
	verdictKey := contentModerationRequestVerdictCacheKey(input, cfg, content, inputHash, "async_observe")
	require.NotEmpty(t, verdictKey)
	cache.mu.Lock()
	_, verdictCached := cache.results[verdictKey]
	cache.mu.Unlock()
	require.False(t, verdictCached, "suppression must prevent a replacement request verdict from being cached")
}

func TestPersistContentModerationLog_SuppressionVetoIsVisibleAndPreventsPromotion(t *testing.T) {
	inputHash := strings.Repeat("c", 64)
	cache := &contentModerationTestHashCache{suppressions: map[string]struct{}{inputHash: {}}}
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(nil, repo, cache, nil, nil, nil, nil, nil)
	log := &ContentModerationLog{
		Action: ContentModerationActionBlock, Flagged: true,
		AuditDetails: ContentModerationAuditDetails{HashState: "confirmed", HashPromotionReason: "semantic_full_review_strong_signal"},
	}

	svc.persistContentModerationLog(context.Background(), defaultContentModerationConfig(), log, inputHash, true, false)

	logs := repo.snapshotLogs()
	require.Len(t, logs, 1)
	require.Equal(t, "suppressed", logs[0].AuditDetails.HashState)
	require.Equal(t, "administrator_suppression", logs[0].AuditDetails.HashPromotionReason)
	require.Empty(t, cache.snapshotRecorded())
	require.False(t, cache.hasHash(inputHash))
}

func TestPersistContentModerationLog_SuppressionVetoCancelsLateSideEffects(t *testing.T) {
	inputHash := strings.Repeat("e", 64)
	tests := []struct {
		name  string
		cache ContentModerationHashCache
		base  *contentModerationTestHashCache
	}{
		{
			name: "suppressed before persistence",
			base: &contentModerationTestHashCache{suppressions: map[string]struct{}{inputHash: {}}},
		},
		{
			name: "suppressed during atomic promotion",
			base: &contentModerationTestHashCache{},
		},
	}
	tests[0].cache = tests[0].base
	tests[1].cache = &contentModerationSuppressionRaceCache{contentModerationTestHashCache: tests[1].base}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			userID := int64(71)
			repo := &contentModerationTestRepo{banOutcome: ContentModerationBanOutcomeApplied}
			svc := NewContentModerationService(nil, repo, test.cache, nil, nil, nil, nil, nil)
			cfg := defaultContentModerationConfig()
			cfg.AutoBanEnabled = true
			cfg.BanThreshold = 1
			cfg.EmailOnHit = true
			log := &ContentModerationLog{
				UserID: &userID, Action: ContentModerationActionBlock, Flagged: true,
				AuditDetails: ContentModerationAuditDetails{
					HashState: "confirmed", HashPromotionReason: "semantic_full_review_strong_signal",
				},
			}

			svc.persistContentModerationLog(context.Background(), cfg, log, inputHash, true, true)

			logs := repo.snapshotLogs()
			require.Len(t, logs, 1)
			require.Equal(t, "suppressed", logs[0].AuditDetails.HashState)
			require.Equal(t, "administrator_suppression", logs[0].AuditDetails.HashPromotionReason)
			require.Equal(t, ContentModerationSideEffectStatusNotApplicable, logs[0].SideEffectStatus)
			require.Equal(t, ContentModerationNotificationStatusNotRequired, logs[0].NotificationStatus)
			require.Zero(t, logs[0].ViolationCount)
			require.False(t, logs[0].AutoBanned)
			state, stateErr := repo.GetModerationUserState(context.Background(), userID)
			require.NoError(t, stateErr)
			require.Nil(t, state, "a suppressed late task must not re-ban the user")
			require.Empty(t, test.base.snapshotRecorded())
		})
	}
}

func TestPersistContentModerationLog_SuppressionLookupFailureCancelsAllSideEffects(t *testing.T) {
	inputHash := strings.Repeat("f", 64)
	userID := int64(72)
	cache := &contentModerationSuppressionErrorCache{
		contentModerationTestHashCache: &contentModerationTestHashCache{},
		err:                            errors.New("redis unavailable"),
	}
	repo := &contentModerationTestRepo{banOutcome: ContentModerationBanOutcomeApplied}
	svc := NewContentModerationService(nil, repo, cache, nil, nil, nil, nil, nil)
	cfg := defaultContentModerationConfig()
	cfg.AutoBanEnabled = true
	cfg.BanThreshold = 1
	cfg.EmailOnHit = true
	log := &ContentModerationLog{
		UserID: &userID, Action: ContentModerationActionBlock, Flagged: true,
		AuditDetails: ContentModerationAuditDetails{
			HashState: "confirmed", HashPromotionReason: "semantic_full_review_strong_signal",
		},
	}

	svc.persistContentModerationLog(context.Background(), cfg, log, inputHash, true, true)

	logs := repo.snapshotLogs()
	require.Len(t, logs, 1)
	require.Equal(t, "candidate", logs[0].AuditDetails.HashState)
	require.Equal(t, "promotion_state_unavailable", logs[0].AuditDetails.HashPromotionReason)
	require.Equal(t, ContentModerationSideEffectStatusNotApplicable, logs[0].SideEffectStatus)
	require.Equal(t, ContentModerationNotificationStatusNotRequired, logs[0].NotificationStatus)
	require.Zero(t, logs[0].ViolationCount)
	require.False(t, logs[0].AutoBanned)
	require.Zero(t, cache.recordCalls)
	state, stateErr := repo.GetModerationUserState(context.Background(), userID)
	require.NoError(t, stateErr)
	require.Nil(t, state)
}

func TestContentModerationClearFlaggedInputHashesAndStatusCount(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	hashCache := &contentModerationTestHashCache{hashes: map[string]struct{}{
		strings.Repeat("a", 64): {},
		strings.Repeat("b", 64): {},
	}}
	svc := &ContentModerationService{
		settingRepo: &contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		hashCache: hashCache,
		keyHealth: make(map[string]*contentModerationKeyHealth),
	}

	status, err := svc.GetStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(2), status.FlaggedHashCount)

	result, err := svc.ClearFlaggedInputHashes(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(2), result.Deleted)

	status, err = svc.GetStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(0), status.FlaggedHashCount)
}

func TestContentModerationCheck_AsyncFlaggedWritesRedisHashCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{
			Results: []moderationAPIResult{{
				CategoryScores: map[string]float64{"sexual": 0.9},
			}},
		})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModeObserve
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	repo := &contentModerationTestRepo{}
	hashCache := &contentModerationTestHashCache{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		hashCache,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	decision := svc.checkSync(context.Background(), ContentModerationCheckInput{
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"bad prompt"}]}`),
	}, cfg, ContentModerationInput{Text: "bad prompt"}, strings.Repeat("b", 64), contentModerationIntPtr(25), false)

	require.False(t, decision.Blocked)
	requireRecordedHashCount(t, hashCache, 1)
	requireContentModerationLogCount(t, repo, 1)
}

func TestBuildContentModerationAccountDisabledEmailBody_ContainsBanDetails(t *testing.T) {
	userID := int64(1001)
	cfg := defaultContentModerationConfig()
	cfg.BanThreshold = 10
	body := buildContentModerationAccountDisabledEmailBody("Sub2API <Admin>", &ContentModerationLog{
		UserID:          &userID,
		UserEmail:       "user@example.com",
		GroupName:       "vip_2",
		HighestCategory: "sexual",
		HighestScore:    0.926,
		ViolationCount:  10,
	}, cfg)

	require.Contains(t, body, "账户已被自动禁用")
	require.Contains(t, body, "封禁详情")
	require.Contains(t, body, "账户当前处于封禁状态，所有 API 请求将被拒绝")
	require.Contains(t, body, "10 次（阈值 10）")
	require.Contains(t, body, "sexual / 0.926")
	require.Contains(t, body, "Sub2API &lt;Admin&gt;")
}

func TestContentModerationUnbanUser_RestoresOwnedBanAndClearsRiskState(t *testing.T) {
	disabledAt := time.Now().Add(-time.Hour)
	disabledLogID := int64(42)
	var events []string
	invalidator := &contentModerationTestAuthCacheInvalidator{onInvalidate: func(int64) {
		events = append(events, "invalidate")
	}}
	repo := &contentModerationTestRepo{moderationState: &ContentModerationUserState{
		UserID:                  1001,
		ModerationOwnedDisabled: true,
		DisabledLogID:           &disabledLogID,
		DisabledAt:              &disabledAt,
	}}
	cache := &contentModerationTestHashCache{
		sessionStates: map[string]voteairiskstate.State{"session": {Score: 0.8}},
		onClear: func(int64) {
			events = append(events, "clear")
		},
	}
	svc := NewContentModerationService(nil, repo, cache, nil, nil, nil, invalidator, nil)

	result, err := svc.UnbanUser(context.Background(), 1001, ContentModerationUnbanModeRestoreAndClearRisk)

	require.NoError(t, err)
	require.Equal(t, int64(1001), result.UserID)
	require.Equal(t, StatusActive, result.Status)
	require.True(t, result.Restored)
	require.True(t, result.RiskStateCleared)
	require.Equal(t, ContentModerationUnbanModeRestoreAndClearRisk, result.Mode)
	require.False(t, repo.moderationState.ModerationOwnedDisabled)
	require.Equal(t, []int64{1001}, cache.clearedUsers)
	require.Equal(t, []int64{1001}, invalidator.userIDs)
	require.Equal(t, []string{"clear", "invalidate"}, events)
	epoch, epochErr := cache.GetContentModerationUserEpoch(context.Background(), 1001)
	require.NoError(t, epochErr)
	require.EqualValues(t, 1, epoch)
}

func TestContentModerationUnbanUser_FencesQueuedRecordAndSupplementalTasks(t *testing.T) {
	userID := int64(1001)
	disabledAt := time.Now().Add(-time.Hour)
	disabledLogID := int64(42)
	repo := &contentModerationTestRepo{
		banOutcome: ContentModerationBanOutcomeApplied,
		moderationState: &ContentModerationUserState{
			UserID:                  userID,
			ModerationOwnedDisabled: true,
			DisabledLogID:           &disabledLogID,
			DisabledAt:              &disabledAt,
		},
	}
	baseCache := &contentModerationTestHashCache{sessionStates: map[string]voteairiskstate.State{
		"old-session": {Score: 0.9},
	}}
	dedupe := &contentModerationEmailDedupeTestStore{}
	cache := &contentModerationEmailIntegrationCache{
		contentModerationTestHashCache:        baseCache,
		contentModerationEmailDedupeTestStore: dedupe,
	}
	svc := NewContentModerationService(nil, repo, cache, nil, nil, nil, nil, &EmailService{})
	cfg := defaultContentModerationConfig()
	cfg.AutoBanEnabled = true
	cfg.BanThreshold = 1
	cfg.EmailOnHit = true

	oldInput := ContentModerationCheckInput{RequestID: "old-request", UserID: userID, APIKeyID: 7, SessionID: "old-session"}
	svc.captureContentModerationEpoch(context.Background(), &oldInput)
	require.True(t, oldInput.ModerationEpochSet)
	require.Zero(t, oldInput.ModerationEpoch)
	oldRecord := contentModerationTask{
		input:            oldInput,
		inputHash:        strings.Repeat("a", 64),
		log:              svc.buildLog(oldInput, cfg, ContentModerationActionBlock, true, "credential_theft", 0.95, map[string]float64{"credential_theft": 0.95}, "old blocked prompt", nil, nil, ""),
		config:           cloneContentModerationConfig(cfg),
		recordHash:       true,
		applySideEffects: true,
		enqueuedAt:       time.Now(),
	}
	oldSupplemental := contentModerationTask{
		input:        oldInput,
		content:      ContentModerationInput{Text: "old supplemental prompt"},
		inputHash:    strings.Repeat("b", 64),
		config:       cloneContentModerationConfig(cfg),
		supplemental: true,
		enqueuedAt:   time.Now(),
	}

	result, err := svc.UnbanUser(context.Background(), userID, ContentModerationUnbanModeRestoreAndClearRisk)
	require.NoError(t, err)
	require.True(t, result.Restored)
	require.True(t, result.RiskStateCleared)

	svc.processAsyncTask(context.Background(), cfg, 0, oldRecord)
	svc.supplementalPending.Store(1)
	svc.processAsyncTask(context.Background(), cfg, 0, oldSupplemental)
	require.Empty(t, repo.logs, "pre-unban record task must not persist after unban")
	require.Empty(t, cache.snapshotRecorded(), "pre-unban task must not restore the flagged hash")
	require.Empty(t, cache.sessionStates, "pre-unban supplemental task must not restore session risk")
	require.False(t, repo.moderationState.ModerationOwnedDisabled, "pre-unban task must not re-ban the user")
	require.Empty(t, dedupe.snapshotCalls(), "pre-unban task must not reserve or send a notification")
	require.Zero(t, svc.supplementalPending.Load())

	cfg.EmailOnHit = false
	newInput := ContentModerationCheckInput{RequestID: "new-request", UserID: userID, APIKeyID: 7, SessionID: "new-session"}
	svc.captureContentModerationEpoch(context.Background(), &newInput)
	require.EqualValues(t, 1, newInput.ModerationEpoch)
	newRecord := contentModerationTask{
		input:            newInput,
		inputHash:        strings.Repeat("c", 64),
		log:              svc.buildLog(newInput, cfg, ContentModerationActionBlock, true, "credential_theft", 0.95, map[string]float64{"credential_theft": 0.95}, "new blocked prompt", nil, nil, ""),
		config:           cloneContentModerationConfig(cfg),
		recordHash:       true,
		applySideEffects: true,
		enqueuedAt:       time.Now(),
	}
	svc.processAsyncTask(context.Background(), cfg, 0, newRecord)
	require.Len(t, repo.logs, 1, "post-unban task must remain eligible")
	require.Len(t, cache.snapshotRecorded(), 1)
	require.True(t, repo.moderationState.ModerationOwnedDisabled, "post-unban violation may apply a new moderation-owned ban")
}

func TestContentModerationUnbanUser_RestoreOnlyAdvancesEpochWithoutClearingRisk(t *testing.T) {
	userID := int64(1001)
	disabledAt := time.Now().Add(-time.Hour)
	disabledLogID := int64(42)
	repo := &contentModerationTestRepo{moderationState: &ContentModerationUserState{
		UserID:                  userID,
		ModerationOwnedDisabled: true,
		DisabledLogID:           &disabledLogID,
		DisabledAt:              &disabledAt,
	}}
	cache := &contentModerationTestHashCache{sessionStates: map[string]voteairiskstate.State{"session": {Score: 0.9}}}
	svc := NewContentModerationService(nil, repo, cache, nil, nil, nil, nil, nil)

	result, err := svc.UnbanUser(context.Background(), userID, ContentModerationUnbanModeRestoreOnly)

	require.NoError(t, err)
	require.True(t, result.Restored)
	require.False(t, result.RiskStateCleared)
	require.NotEmpty(t, cache.sessionStates)
	epoch, epochErr := cache.GetContentModerationUserEpoch(context.Background(), userID)
	require.NoError(t, epochErr)
	require.EqualValues(t, 1, epoch)
}

func TestContentModerationUnbanUser_RetriesFailedCleanupWithClearRiskOnly(t *testing.T) {
	disabledAt := time.Now().Add(-time.Hour)
	disabledLogID := int64(42)
	var events []string
	repo := &contentModerationTestRepo{moderationState: &ContentModerationUserState{
		UserID:                  1001,
		ModerationOwnedDisabled: true,
		DisabledLogID:           &disabledLogID,
		DisabledAt:              &disabledAt,
	}}
	cache := &contentModerationTestHashCache{
		clearErr: errors.New("redis unavailable"),
		onClear: func(int64) {
			events = append(events, "clear")
		},
	}
	invalidator := &contentModerationTestAuthCacheInvalidator{onInvalidate: func(int64) {
		events = append(events, "invalidate")
	}}
	svc := NewContentModerationService(nil, repo, cache, nil, nil, nil, invalidator, nil)

	restored, err := svc.UnbanUser(context.Background(), 1001, ContentModerationUnbanModeRestoreAndClearRisk)

	require.NoError(t, err)
	require.True(t, restored.Restored)
	require.False(t, restored.RiskStateCleared)
	require.NotEmpty(t, restored.Warning)
	require.Equal(t, []string{"clear", "invalidate"}, events)
	require.False(t, repo.moderationState.ModerationOwnedDisabled)
	require.Equal(t, 1, repo.restoreCalls)

	cache.clearErr = nil
	retried, err := svc.UnbanUser(context.Background(), 1001, ContentModerationUnbanModeClearRiskOnly)

	require.NoError(t, err)
	require.False(t, retried.Restored)
	require.True(t, retried.RiskStateCleared)
	require.Equal(t, ContentModerationUnbanModeClearRiskOnly, retried.Mode)
	require.Equal(t, 1, repo.restoreCalls, "clear_risk_only must not restore the database user again")
	require.Equal(t, []string{"clear", "invalidate", "clear"}, events)
	require.Equal(t, []int64{1001, 1001}, cache.clearedUsers)
	require.Equal(t, []int64{1001}, invalidator.userIDs)
}

func TestContentModerationUnbanUser_ClearRiskOnlyValidatesLifecycleAndUserID(t *testing.T) {
	cache := &contentModerationTestHashCache{}
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(nil, repo, cache, nil, nil, nil, nil, nil)

	result, err := svc.UnbanUser(context.Background(), 0, ContentModerationUnbanModeClearRiskOnly)
	require.Error(t, err)
	require.ErrorContains(t, err, "INVALID_USER_ID")
	require.Nil(t, result)
	require.Empty(t, cache.clearedUsers)

	result, err = svc.UnbanUser(context.Background(), 1001, ContentModerationUnbanModeClearRiskOnly)
	require.Error(t, err)
	require.ErrorContains(t, err, "CONTENT_MODERATION_RISK_CLEAR_NOT_ELIGIBLE")
	require.Nil(t, result)
	require.Empty(t, cache.clearedUsers)

	repo.moderationState = &ContentModerationUserState{UserID: 1001, ModerationOwnedDisabled: true}
	result, err = svc.UnbanUser(context.Background(), 1001, ContentModerationUnbanModeClearRiskOnly)
	require.Error(t, err)
	require.ErrorContains(t, err, "CONTENT_MODERATION_BAN_STILL_ACTIVE")
	require.Nil(t, result)
	require.Empty(t, cache.clearedUsers)
}

func TestContentModerationUnbanUser_ClearRiskOnlyReportsCurrentManualDisable(t *testing.T) {
	cache := &contentModerationTestHashCache{}
	repo := &contentModerationTestRepo{moderationState: &ContentModerationUserState{
		UserID:                  1001,
		ModerationOwnedDisabled: false,
	}}
	userRepo := &contentModerationTestUserRepo{user: &User{ID: 1001, Status: StatusDisabled}}
	svc := NewContentModerationService(nil, repo, cache, nil, userRepo, nil, nil, nil)

	result, err := svc.UnbanUser(context.Background(), 1001, ContentModerationUnbanModeClearRiskOnly)

	require.NoError(t, err)
	require.Equal(t, StatusDisabled, result.Status)
	require.False(t, result.Restored)
	require.True(t, result.RiskStateCleared)
	require.Equal(t, []int64{1001}, cache.clearedUsers)
}

func TestContentModerationUnbanUser_RejectsNonModerationDisable(t *testing.T) {
	invalidator := &contentModerationTestAuthCacheInvalidator{}
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(nil, repo, &contentModerationTestHashCache{}, nil, nil, nil, invalidator, nil)

	result, err := svc.UnbanUser(context.Background(), 1001, ContentModerationUnbanModeRestoreOnly)

	require.Error(t, err)
	require.ErrorContains(t, err, "CONTENT_MODERATION_BAN_NOT_OWNED")
	require.Nil(t, result)
	require.Empty(t, invalidator.userIDs)
}

func contentModerationIntPtr(v int) *int {
	return &v
}

func TestContentModerationUpdateConfig_CyberPolicyExcludeFromBanCount(t *testing.T) {
	settingRepo := &contentModerationTestSettingRepo{values: map[string]string{}}
	svc := NewContentModerationService(settingRepo, nil, nil, nil, nil, nil, nil, nil)

	// 默认值必须是 false（计入，保持现状）
	view, err := svc.GetConfig(context.Background())
	require.NoError(t, err)
	require.False(t, view.CyberPolicyExcludeFromBanCount, "默认必须计入封号计数")

	// 指针式部分更新为 true
	exclude := true
	view, err = svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
		CyberPolicyExcludeFromBanCount: &exclude,
	})
	require.NoError(t, err)
	require.True(t, view.CyberPolicyExcludeFromBanCount)

	// 持久化 JSON 含字段
	var saved ContentModerationConfig
	require.NoError(t, json.Unmarshal([]byte(settingRepo.values[SettingKeyContentModerationConfig]), &saved))
	require.True(t, saved.CyberPolicyExcludeFromBanCount)

	// 二次读取（从持久化 JSON 反序列化）roundtrip
	view, err = svc.GetConfig(context.Background())
	require.NoError(t, err)
	require.True(t, view.CyberPolicyExcludeFromBanCount)

	// 不传该字段的更新不得改动它（指针 nil = 保留）
	view, err = svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{})
	require.NoError(t, err)
	require.True(t, view.CyberPolicyExcludeFromBanCount)

	// 主动回拨 false 必须生效（防止未来误加 if val 保护逻辑）
	revert := false
	view, err = svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
		CyberPolicyExcludeFromBanCount: &revert,
	})
	require.NoError(t, err)
	require.False(t, view.CyberPolicyExcludeFromBanCount)
}
