package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestContentModerationRepositoryCreateLog_PersistsAll32ArgumentsIncludingAuditDetails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	userID := int64(1001)
	apiKeyID := int64(2002)
	groupID := int64(3003)
	latencyMS := 432
	queueDelayMS := 17
	promptTokens := 789
	createdAt := time.Date(2026, 8, 5, 12, 34, 56, 0, time.UTC)
	log := &service.ContentModerationLog{
		RequestID:          "req-audit-details-create",
		UserID:             &userID,
		UserEmail:          "audit@example.com",
		APIKeyID:           &apiKeyID,
		APIKeyName:         "audit-key",
		GroupID:            &groupID,
		GroupName:          "protected-pro",
		Endpoint:           "/v1/responses",
		Provider:           service.ContentModerationProviderAIChat,
		Model:              "deepseek-v4-flash",
		Mode:               service.ContentModerationModePreBlock,
		Action:             "block",
		Flagged:            true,
		HighestCategory:    "malware",
		HighestScore:       0.91,
		CategoryScores:     map[string]float64{"illicit": 0.81, "malware": 0.91},
		ThresholdSnapshot:  map[string]float64{"illicit": 0.7, "malware": 0.7},
		InputExcerpt:       "redacted excerpt",
		UpstreamLatencyMS:  &latencyMS,
		Error:              "",
		ViolationCount:     2,
		AutoBanned:         false,
		EmailSent:          true,
		QueueDelayMS:       &queueDelayMS,
		MatchedKeyword:     "credential theft",
		AuditStatus:        "success",
		AuditCode:          "semantic_block",
		AuditRetryable:     false,
		SideEffectStatus:   "completed",
		NotificationStatus: "sent",
		SideEffectError:    "",
		AuditDetails: service.ContentModerationAuditDetails{
			AuditStage:         "full",
			EscalationReasons:  []string{"candidate_rule"},
			SessionSource:      "explicit",
			TurnCount:          8,
			PromptTokens:       &promptTokens,
			ResultCacheHit:     true,
			AuditTargetKind:    "user_intent",
			AuditTargetExcerpt: "redacted target",
			PolicyVersion:      "policy-v5",
		},
	}
	categoryScoresJSON, err := json.Marshal(log.CategoryScores)
	require.NoError(t, err)
	thresholdSnapshotJSON, err := json.Marshal(log.ThresholdSnapshot)
	require.NoError(t, err)
	auditDetailsJSON, err := json.Marshal(log.AuditDetails)
	require.NoError(t, err)

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO content_moderation_logs (")).
		WithArgs(
			log.RequestID, userID, log.UserEmail, apiKeyID, log.APIKeyName, groupID, log.GroupName,
			log.Endpoint, log.Provider, log.Model, log.Mode, log.Action, log.Flagged, log.HighestCategory, log.HighestScore,
			string(categoryScoresJSON), string(thresholdSnapshotJSON), log.InputExcerpt, latencyMS, log.Error,
			log.ViolationCount, log.AutoBanned, log.EmailSent, queueDelayMS, log.MatchedKeyword,
			log.AuditStatus, log.AuditCode, log.AuditRetryable, log.SideEffectStatus, log.NotificationStatus,
			log.SideEffectError, string(auditDetailsJSON),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(77), createdAt))

	repo := NewContentModerationRepository(db)
	err = repo.CreateLog(context.Background(), log)

	require.NoError(t, err)
	require.Equal(t, int64(77), log.ID)
	require.Equal(t, createdAt, log.CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositorySumBusinessActualCostSince(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	since := time.Date(2026, 8, 5, 1, 2, 3, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(SUM(actual_cost), 0) FROM usage_logs WHERE created_at >= $1")).
		WithArgs(since).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(12.3456))

	total, err := NewContentModerationRepository(db).SumBusinessActualCostSince(context.Background(), since)
	require.NoError(t, err)
	require.InDelta(t, 12.3456, total, 0.0000001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryListLogs_ScansAuditDetailsInSelectOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	createdAt := time.Date(2026, 8, 5, 13, 45, 0, 0, time.UTC)
	promptTokens := 456
	cachedTokens := 321
	prefixContinuity := true
	auditDetails := service.ContentModerationAuditDetails{
		AuditStage:               "fast",
		EscalationReasons:        []string{"periodic_full_review"},
		SessionSource:            "previous_response_id",
		TurnCount:                12,
		PromptTokens:             &promptTokens,
		CachedInputTokens:        &cachedTokens,
		ResultCacheHit:           true,
		PrefixEpoch:              3,
		PrefixContinuity:         &prefixContinuity,
		AuditTargetKind:          "user_intent",
		AuditTargetSource:        "openai_responses",
		AuditTargetExcerpt:       "safe target excerpt",
		SupportingContextExcerpt: "[openai_responses/tool] redacted output",
		TrustedSignals:           []string{"signed_client"},
		IgnoredMetadata:          []string{"ambient_ui"},
		PolicyVersion:            "policy-v5",
	}
	auditDetailsJSON, err := json.Marshal(auditDetails)
	require.NoError(t, err)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM content_moderation_logs l WHERE l.id IS NOT NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(regexp.QuoteMeta("l.queue_delay_ms, l.matched_keyword, l.audit_details, l.created_at")).
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "request_id", "user_id", "user_email", "api_key_id", "api_key_name", "group_id", "group_name",
			"endpoint", "provider", "model", "mode", "action", "flagged", "highest_category", "highest_score",
			"category_scores", "threshold_snapshot", "input_excerpt", "upstream_latency_ms", "error",
			"audit_status", "audit_code", "audit_retryable", "side_effect_status", "notification_status", "side_effect_error",
			"violation_count", "auto_banned", "email_sent", "user_status", "moderation_owned_disabled",
			"queue_delay_ms", "matched_keyword", "audit_details", "created_at",
		}).AddRow(
			int64(88), "req-audit-details-list", int64(1001), "audit@example.com", int64(2002), "audit-key", int64(3003), "protected-pro",
			"/v1/responses", service.ContentModerationProviderAIChat, "deepseek-v4-flash", service.ContentModerationModeObserve, "allow", false, "illicit", 0.12,
			[]byte(`{"illicit":0.12}`), []byte(`{"illicit":0.7}`), "safe excerpt", int64(250), "",
			"success", "allow", false, "completed", "not_required", "",
			0, false, false, service.StatusActive, false,
			int64(9), "", auditDetailsJSON, createdAt,
		))

	repo := NewContentModerationRepository(db)
	items, page, err := repo.ListLogs(context.Background(), service.ContentModerationLogFilter{})

	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotNil(t, page)
	require.Equal(t, int64(1), page.Total)
	require.Equal(t, 1, page.Page)
	require.Equal(t, 20, page.PageSize)
	item := items[0]
	require.Equal(t, int64(88), item.ID)
	require.Equal(t, int64(1001), *item.UserID)
	require.Equal(t, int64(2002), *item.APIKeyID)
	require.Equal(t, int64(3003), *item.GroupID)
	require.Equal(t, 250, *item.UpstreamLatencyMS)
	require.Equal(t, 9, *item.QueueDelayMS)
	require.Equal(t, map[string]float64{"illicit": 0.12}, item.CategoryScores)
	require.Equal(t, map[string]float64{"illicit": 0.7}, item.ThresholdSnapshot)
	require.Equal(t, auditDetails, item.AuditDetails)
	require.Equal(t, createdAt, item.CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildContentModerationLogWhere_BlockedIncludesAllBlockActions(t *testing.T) {
	where, args := buildContentModerationLogWhere(service.ContentModerationLogFilter{Result: "blocked"})

	require.Empty(t, args)
	sql := strings.Join(where, " AND ")
	require.Contains(t, sql, "l.action IN ('block', 'keyword_block', 'hash_block')")
	require.NotContains(t, sql, "l.action = 'block'")
}

func TestBuildContentModerationLogWhere_ErrorUsesAuditStatusAndExcludesLegacyCyberBody(t *testing.T) {
	where, args := buildContentModerationLogWhere(service.ContentModerationLogFilter{Result: "error"})

	require.Empty(t, args)
	sql := strings.Join(where, " AND ")
	require.Contains(t, sql, "l.audit_status = 'error'")
	require.Contains(t, sql, "l.action <> 'cyber_policy'")
	require.NotEqual(t, "l.id IS NOT NULL AND l.error <> ''", sql)
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesHashBlock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND action <> 'hash_block'")).
		WithArgs(int64(1001), since, false).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 1001, since, false)

	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesCyberPolicyWhenRequested(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND ($3::bool IS FALSE OR action <> 'cyber_policy')")).
		WithArgs(int64(1001), since, true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 1001, since, true)

	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryUpdateLogEffects_PatchesFinalState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	patch := service.ContentModerationLogEffectsPatch{
		ViolationCount:     3,
		AutoBanned:         true,
		EmailSent:          true,
		SideEffectStatus:   "completed",
		NotificationStatus: "sent",
		SideEffectError:    "",
	}
	mock.ExpectExec(regexp.QuoteMeta("UPDATE content_moderation_logs")).
		WithArgs(3, true, true, "completed", "sent", "", nil, int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := NewContentModerationRepository(db)
	err = repo.UpdateLogEffects(context.Background(), 91, patch)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryUpdateLogEffects_PatchesAuditDetailsAsJSON(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	promptTokens := 321
	reviewComplete := true
	auditDetails := &service.ContentModerationAuditDetails{
		AuditStage:          "full",
		EscalationReasons:   []string{"periodic_full_review", "risk_rising"},
		PromptTokens:        &promptTokens,
		ReviewComplete:      &reviewComplete,
		AuditTargetKind:     "user_request",
		AuditTargetExcerpt:  "redacted request",
		HashState:           "candidate",
		HashPromotionReason: "full_review_required",
		PolicyVersion:       "policy-v5",
	}
	encodedAuditDetails, err := json.Marshal(auditDetails)
	require.NoError(t, err)
	patch := service.ContentModerationLogEffectsPatch{
		ViolationCount:     2,
		AutoBanned:         false,
		EmailSent:          false,
		SideEffectStatus:   "completed",
		NotificationStatus: "not_required",
		AuditDetails:       auditDetails,
	}
	mock.ExpectExec(regexp.QuoteMeta("UPDATE content_moderation_logs")).
		WithArgs(2, false, false, "completed", "not_required", "", string(encodedAuditDetails), int64(92)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := NewContentModerationRepository(db)
	err = repo.UpdateLogEffects(context.Background(), 92, patch)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryUpdateLogEffects_RejectsMissingLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE content_moderation_logs")).
		WithArgs(0, false, false, "", "", "", nil, int64(404)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	repo := NewContentModerationRepository(db)
	err = repo.UpdateLogEffects(context.Background(), 404, service.ContentModerationLogEffectsPatch{})

	require.ErrorContains(t, err, "log 404 not found")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryGetModerationUserState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	disabledAt := time.Now().Add(-time.Minute).UTC()
	updatedAt := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, moderation_owned_disabled, disabled_log_id, disabled_at, updated_at")).
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "moderation_owned_disabled", "disabled_log_id", "disabled_at", "updated_at",
		}).AddRow(int64(1001), true, int64(81), disabledAt, updatedAt))

	repo := NewContentModerationRepository(db)
	state, err := repo.GetModerationUserState(context.Background(), 1001)

	require.NoError(t, err)
	require.NotNil(t, state)
	require.True(t, state.ModerationOwnedDisabled)
	require.Equal(t, int64(81), *state.DisabledLogID)
	require.Equal(t, disabledAt, *state.DisabledAt)
	require.Equal(t, updatedAt, state.UpdatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryGetModerationUserState_Missing(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, moderation_owned_disabled, disabled_log_id, disabled_at, updated_at")).
		WithArgs(int64(1001)).
		WillReturnError(sql.ErrNoRows)

	repo := NewContentModerationRepository(db)
	state, err := repo.GetModerationUserState(context.Background(), 1001)

	require.NoError(t, err)
	require.Nil(t, state)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryTryApplyModerationOwnedBan_AppliesAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	disabledAt := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT role, status")).
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"role", "status"}).AddRow(service.RoleUser, service.StatusActive))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT moderation_owned_disabled")).
		WithArgs(int64(1001)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE users")).
		WithArgs(int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO content_moderation_user_state")).
		WithArgs(int64(1001), int64(77), disabledAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := NewContentModerationRepository(db)
	outcome, err := repo.TryApplyModerationOwnedBan(context.Background(), 1001, 77, disabledAt)

	require.NoError(t, err)
	require.Equal(t, "applied", outcome)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryTryApplyModerationOwnedBan_RollsBackStatusWhenOwnershipWriteFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	disabledAt := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT role, status")).
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"role", "status"}).AddRow(service.RoleUser, service.StatusActive))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT moderation_owned_disabled")).
		WithArgs(int64(1001)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE users")).
		WithArgs(int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO content_moderation_user_state")).
		WithArgs(int64(1001), int64(77), disabledAt).
		WillReturnError(errors.New("state write failed"))
	mock.ExpectRollback()

	repo := NewContentModerationRepository(db)
	outcome, err := repo.TryApplyModerationOwnedBan(context.Background(), 1001, 77, disabledAt)

	require.Empty(t, outcome)
	require.ErrorContains(t, err, "record content moderation ban ownership")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryTryApplyModerationOwnedBan_AlreadyOwned(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT role, status")).
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"role", "status"}).AddRow(service.RoleUser, service.StatusDisabled))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT moderation_owned_disabled")).
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"moderation_owned_disabled"}).AddRow(true))
	mock.ExpectCommit()

	repo := NewContentModerationRepository(db)
	outcome, err := repo.TryApplyModerationOwnedBan(context.Background(), 1001, 88, time.Now())

	require.NoError(t, err)
	require.Equal(t, "already_owned", outcome)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryTryApplyModerationOwnedBan_RejectsAdmin(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT role, status")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"role", "status"}).AddRow(service.RoleAdmin, service.StatusActive))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT moderation_owned_disabled")).
		WithArgs(int64(1)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectCommit()

	repo := NewContentModerationRepository(db)
	outcome, err := repo.TryApplyModerationOwnedBan(context.Background(), 1, 88, time.Now())

	require.NoError(t, err)
	require.Equal(t, "ineligible", outcome)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryTryApplyModerationOwnedBan_PreservesManualDisable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT role, status")).
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"role", "status"}).AddRow(service.RoleUser, service.StatusDisabled))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT moderation_owned_disabled")).
		WithArgs(int64(1001)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectCommit()

	repo := NewContentModerationRepository(db)
	outcome, err := repo.TryApplyModerationOwnedBan(context.Background(), 1001, 88, time.Now())

	require.NoError(t, err)
	require.Equal(t, "ineligible", outcome)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryRestoreModerationOwnedBan_RestoresOwnedDisable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status")).
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(service.StatusDisabled))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT moderation_owned_disabled")).
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"moderation_owned_disabled"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE users")).
		WithArgs(int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE content_moderation_user_state")).
		WithArgs(int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := NewContentModerationRepository(db)
	restored, err := repo.RestoreModerationOwnedBan(context.Background(), 1001)

	require.NoError(t, err)
	require.True(t, restored)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryRestoreModerationOwnedBan_PreservesManualDisable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status")).
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(service.StatusDisabled))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT moderation_owned_disabled")).
		WithArgs(int64(1001)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectCommit()

	repo := NewContentModerationRepository(db)
	restored, err := repo.RestoreModerationOwnedBan(context.Background(), 1001)

	require.NoError(t, err)
	require.False(t, restored)
	require.NoError(t, mock.ExpectationsWereMet())
}
