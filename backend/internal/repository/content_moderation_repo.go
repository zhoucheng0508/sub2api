package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type contentModerationRepository struct {
	db *sql.DB
}

var _ service.ContentModerationRepository = (*contentModerationRepository)(nil)
var _ service.ContentModerationLifecycleRepository = (*contentModerationRepository)(nil)

func NewContentModerationRepository(db *sql.DB) *contentModerationRepository {
	return &contentModerationRepository{db: db}
}

func (r *contentModerationRepository) CreateLog(ctx context.Context, log *service.ContentModerationLog) error {
	if log == nil {
		return nil
	}
	categoryScores, err := json.Marshal(log.CategoryScores)
	if err != nil {
		return fmt.Errorf("marshal moderation category scores: %w", err)
	}
	thresholdSnapshot, err := json.Marshal(log.ThresholdSnapshot)
	if err != nil {
		return fmt.Errorf("marshal moderation thresholds: %w", err)
	}
	var userID any
	if log.UserID != nil {
		userID = *log.UserID
	}
	var apiKeyID any
	if log.APIKeyID != nil {
		apiKeyID = *log.APIKeyID
	}
	var groupID any
	if log.GroupID != nil {
		groupID = *log.GroupID
	}
	var latency any
	if log.UpstreamLatencyMS != nil {
		latency = *log.UpstreamLatencyMS
	}
	err = r.db.QueryRowContext(ctx, `
INSERT INTO content_moderation_logs (
    request_id, user_id, user_email, api_key_id, api_key_name, group_id, group_name,
    endpoint, provider, model, mode, action, flagged, highest_category, highest_score,
    category_scores, threshold_snapshot, input_excerpt, upstream_latency_ms, error,
    violation_count, auto_banned, email_sent, queue_delay_ms, matched_keyword,
    audit_status, audit_code, audit_retryable, side_effect_status, notification_status,
    side_effect_error
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14, $15,
    $16::jsonb, $17::jsonb, $18, $19, $20,
    $21, $22, $23, $24, $25,
    $26, $27, $28, $29, $30, $31
) RETURNING id, created_at`,
		log.RequestID, userID, log.UserEmail, apiKeyID, log.APIKeyName, groupID, log.GroupName,
		log.Endpoint, log.Provider, log.Model, log.Mode, log.Action, log.Flagged, log.HighestCategory, log.HighestScore,
		string(categoryScores), string(thresholdSnapshot), log.InputExcerpt, latency, log.Error,
		log.ViolationCount, log.AutoBanned, log.EmailSent, nullableIntPtr(log.QueueDelayMS), log.MatchedKeyword,
		log.AuditStatus, log.AuditCode, log.AuditRetryable, log.SideEffectStatus, log.NotificationStatus,
		log.SideEffectError,
	).Scan(&log.ID, &log.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert content moderation log: %w", err)
	}
	return nil
}

func (r *contentModerationRepository) ListLogs(ctx context.Context, filter service.ContentModerationLogFilter) ([]service.ContentModerationLog, *pagination.PaginationResult, error) {
	where, args := buildContentModerationLogWhere(filter)
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM content_moderation_logs l "+whereSQL, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count content moderation logs: %w", err)
	}

	params := filter.Pagination
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	rows, err := r.db.QueryContext(ctx, `
SELECT
    l.id, l.request_id, l.user_id, l.user_email, l.api_key_id, l.api_key_name, l.group_id, l.group_name,
    l.endpoint, l.provider, l.model, l.mode, l.action, l.flagged, l.highest_category, l.highest_score,
    l.category_scores, l.threshold_snapshot, l.input_excerpt, l.upstream_latency_ms, l.error,
    l.audit_status, l.audit_code, l.audit_retryable,
    l.side_effect_status, l.notification_status, l.side_effect_error,
    l.violation_count, l.auto_banned, l.email_sent, COALESCE(u.status, ''),
    COALESCE(mus.moderation_owned_disabled, FALSE),
    l.queue_delay_ms, l.matched_keyword, l.created_at
FROM content_moderation_logs l
LEFT JOIN users u ON u.id = l.user_id
LEFT JOIN content_moderation_user_state mus ON mus.user_id = l.user_id `+whereSQL+`
ORDER BY l.created_at DESC, l.id DESC
LIMIT $`+fmt.Sprint(len(queryArgs)-1)+` OFFSET $`+fmt.Sprint(len(queryArgs)),
		queryArgs...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("list content moderation logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.ContentModerationLog, 0)
	for rows.Next() {
		var item service.ContentModerationLog
		var userID, apiKeyID, groupID, latency, queueDelay sql.NullInt64
		var scoresRaw, thresholdsRaw []byte
		if err := rows.Scan(
			&item.ID,
			&item.RequestID,
			&userID,
			&item.UserEmail,
			&apiKeyID,
			&item.APIKeyName,
			&groupID,
			&item.GroupName,
			&item.Endpoint,
			&item.Provider,
			&item.Model,
			&item.Mode,
			&item.Action,
			&item.Flagged,
			&item.HighestCategory,
			&item.HighestScore,
			&scoresRaw,
			&thresholdsRaw,
			&item.InputExcerpt,
			&latency,
			&item.Error,
			&item.AuditStatus,
			&item.AuditCode,
			&item.AuditRetryable,
			&item.SideEffectStatus,
			&item.NotificationStatus,
			&item.SideEffectError,
			&item.ViolationCount,
			&item.AutoBanned,
			&item.EmailSent,
			&item.UserStatus,
			&item.ModerationBanActive,
			&queueDelay,
			&item.MatchedKeyword,
			&item.CreatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan content moderation log: %w", err)
		}
		if userID.Valid {
			v := userID.Int64
			item.UserID = &v
		}
		if apiKeyID.Valid {
			v := apiKeyID.Int64
			item.APIKeyID = &v
		}
		if groupID.Valid {
			v := groupID.Int64
			item.GroupID = &v
		}
		if latency.Valid {
			v := int(latency.Int64)
			item.UpstreamLatencyMS = &v
		}
		if queueDelay.Valid {
			v := int(queueDelay.Int64)
			item.QueueDelayMS = &v
		}
		item.CategoryScores = map[string]float64{}
		_ = json.Unmarshal(scoresRaw, &item.CategoryScores)
		item.ThresholdSnapshot = map[string]float64{}
		_ = json.Unmarshal(thresholdsRaw, &item.ThresholdSnapshot)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate content moderation logs: %w", err)
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *contentModerationRepository) CountFlaggedByUserSince(ctx context.Context, userID int64, since time.Time, excludeCyberPolicy bool) (int, error) {
	if userID <= 0 {
		return 0, nil
	}
	// SQL 中的 'cyber_policy' 字面量须与 service.ContentModerationActionCyberPolicy 保持一致。
	var count int
	err := r.db.QueryRowContext(ctx, `
WITH last_auto_ban AS (
    SELECT MAX(created_at) AS at
    FROM content_moderation_logs
    WHERE user_id = $1 AND auto_banned = TRUE
)
SELECT COUNT(*)
FROM content_moderation_logs
WHERE user_id = $1
  AND flagged = TRUE
  AND action <> 'hash_block'
  AND ($3::bool IS FALSE OR action <> 'cyber_policy')
  AND created_at >= $2
  AND created_at > COALESCE((SELECT at FROM last_auto_ban), '-infinity'::timestamptz)
`, userID, since, excludeCyberPolicy).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count user content moderation flagged logs: %w", err)
	}
	return count, nil
}

func (r *contentModerationRepository) UpdateLogEmailSent(ctx context.Context, id int64, sent bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE content_moderation_logs SET email_sent = $1 WHERE id = $2`, sent, id)
	if err != nil {
		return fmt.Errorf("update content moderation log email_sent: %w", err)
	}
	return nil
}

func (r *contentModerationRepository) UpdateLogEffects(ctx context.Context, id int64, patch service.ContentModerationLogEffectsPatch) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE content_moderation_logs
SET violation_count = $1,
    auto_banned = $2,
    email_sent = $3,
    side_effect_status = $4,
    notification_status = $5,
    side_effect_error = $6
WHERE id = $7
`, patch.ViolationCount, patch.AutoBanned, patch.EmailSent, patch.SideEffectStatus, patch.NotificationStatus, patch.SideEffectError, id)
	if err != nil {
		return fmt.Errorf("update content moderation log effects: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read content moderation log effects update result: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("update content moderation log effects: log %d not found", id)
	}
	return nil
}

func (r *contentModerationRepository) GetModerationUserState(ctx context.Context, userID int64) (*service.ContentModerationUserState, error) {
	if userID <= 0 {
		return nil, nil
	}
	var (
		state         service.ContentModerationUserState
		disabledLogID sql.NullInt64
		disabledAt    sql.NullTime
	)
	err := r.db.QueryRowContext(ctx, `
SELECT user_id, moderation_owned_disabled, disabled_log_id, disabled_at, updated_at
FROM content_moderation_user_state
WHERE user_id = $1
`, userID).Scan(
		&state.UserID,
		&state.ModerationOwnedDisabled,
		&disabledLogID,
		&disabledAt,
		&state.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get content moderation user state: %w", err)
	}
	if disabledLogID.Valid {
		value := disabledLogID.Int64
		state.DisabledLogID = &value
	}
	if disabledAt.Valid {
		value := disabledAt.Time
		state.DisabledAt = &value
	}
	return &state, nil
}

// TryApplyModerationOwnedBan serializes all moderation ban decisions on the
// user row and records ownership in the same transaction as the status change.
func (r *contentModerationRepository) TryApplyModerationOwnedBan(ctx context.Context, userID, logID int64, disabledAt time.Time) (string, error) {
	if userID <= 0 || logID <= 0 {
		return "ineligible", nil
	}
	if disabledAt.IsZero() {
		disabledAt = time.Now()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin content moderation ban transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var role, status string
	err = tx.QueryRowContext(ctx, `
SELECT role, status
FROM users
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE
`, userID).Scan(&role, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "ineligible", nil
	}
	if err != nil {
		return "", fmt.Errorf("lock user for content moderation ban: %w", err)
	}

	var owned bool
	err = tx.QueryRowContext(ctx, `
SELECT moderation_owned_disabled
FROM content_moderation_user_state
WHERE user_id = $1
FOR UPDATE
`, userID).Scan(&owned)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("lock content moderation user state: %w", err)
	}
	if owned {
		if err := tx.Commit(); err != nil {
			return "", fmt.Errorf("commit existing content moderation ban: %w", err)
		}
		return "already_owned", nil
	}
	if role == service.RoleAdmin || status != service.StatusActive {
		if err := tx.Commit(); err != nil {
			return "", fmt.Errorf("commit ineligible content moderation ban: %w", err)
		}
		return "ineligible", nil
	}

	result, err := tx.ExecContext(ctx, `
UPDATE users
SET status = 'disabled', updated_at = NOW()
WHERE id = $1 AND status = 'active' AND deleted_at IS NULL
`, userID)
	if err != nil {
		return "", fmt.Errorf("disable user for content moderation: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("read content moderation user disable result: %w", err)
	}
	if affected != 1 {
		return "", fmt.Errorf("disable user for content moderation: locked user %d changed unexpectedly", userID)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO content_moderation_user_state (
    user_id, moderation_owned_disabled, disabled_log_id, disabled_at, updated_at
) VALUES ($1, TRUE, $2, $3, NOW())
ON CONFLICT (user_id) DO UPDATE SET
    moderation_owned_disabled = TRUE,
    disabled_log_id = EXCLUDED.disabled_log_id,
    disabled_at = EXCLUDED.disabled_at,
    updated_at = NOW()
`, userID, logID, disabledAt)
	if err != nil {
		return "", fmt.Errorf("record content moderation ban ownership: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit content moderation ban: %w", err)
	}
	return "applied", nil
}

// RestoreModerationOwnedBan restores only users whose current disable is
// explicitly owned by content moderation. Manual disables are left untouched.
func (r *contentModerationRepository) RestoreModerationOwnedBan(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin content moderation restore transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	err = tx.QueryRowContext(ctx, `
SELECT status
FROM users
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE
`, userID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock user for content moderation restore: %w", err)
	}

	var owned bool
	err = tx.QueryRowContext(ctx, `
SELECT moderation_owned_disabled
FROM content_moderation_user_state
WHERE user_id = $1
FOR UPDATE
`, userID).Scan(&owned)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit unowned content moderation restore: %w", err)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock content moderation state for restore: %w", err)
	}
	if !owned {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit unowned content moderation restore: %w", err)
		}
		return false, nil
	}

	restored := status == service.StatusDisabled
	if restored {
		result, err := tx.ExecContext(ctx, `
UPDATE users
SET status = 'active', updated_at = NOW()
WHERE id = $1 AND status = 'disabled' AND deleted_at IS NULL
`, userID)
		if err != nil {
			return false, fmt.Errorf("restore content moderation disabled user: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return false, fmt.Errorf("read content moderation user restore result: %w", err)
		}
		if affected != 1 {
			return false, fmt.Errorf("restore content moderation disabled user: locked user %d changed unexpectedly", userID)
		}
	}
	_, err = tx.ExecContext(ctx, `
UPDATE content_moderation_user_state
SET moderation_owned_disabled = FALSE,
    disabled_log_id = NULL,
    disabled_at = NULL,
    updated_at = NOW()
WHERE user_id = $1 AND moderation_owned_disabled = TRUE
`, userID)
	if err != nil {
		return false, fmt.Errorf("clear content moderation ban ownership after restore: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit content moderation restore: %w", err)
	}
	return restored, nil
}

func (r *contentModerationRepository) CleanupExpiredLogs(ctx context.Context, hitBefore time.Time, nonHitBefore time.Time) (*service.ContentModerationCleanupResult, error) {
	result := &service.ContentModerationCleanupResult{FinishedAt: time.Now()}
	if r == nil || r.db == nil {
		return result, nil
	}
	hitExec, err := r.db.ExecContext(ctx, `
DELETE FROM content_moderation_logs
WHERE flagged = TRUE AND created_at < $1
`, hitBefore)
	if err != nil {
		return nil, fmt.Errorf("delete expired hit content moderation logs: %w", err)
	}
	result.DeletedHit, _ = hitExec.RowsAffected()

	nonHitExec, err := r.db.ExecContext(ctx, `
DELETE FROM content_moderation_logs
WHERE flagged = FALSE AND created_at < $1
`, nonHitBefore)
	if err != nil {
		return nil, fmt.Errorf("delete expired non-hit content moderation logs: %w", err)
	}
	result.DeletedNonHit, _ = nonHitExec.RowsAffected()

	result.FinishedAt = time.Now()
	return result, nil
}

func nullableIntPtr(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func buildContentModerationLogWhere(filter service.ContentModerationLogFilter) ([]string, []any) {
	where := []string{"l.id IS NOT NULL"}
	args := make([]any, 0)
	add := func(expr string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(expr, len(args)))
	}
	switch strings.ToLower(strings.TrimSpace(filter.Result)) {
	case "hit", "flagged":
		where = append(where, "l.flagged = TRUE")
	case "blocked", "block":
		where = append(where, "l.action IN ('block', 'keyword_block', 'hash_block')")
	case "pass", "allow":
		where = append(where, "l.flagged = FALSE AND (l.audit_status = 'success' OR (l.audit_status = '' AND l.error = ''))")
	case "error":
		where = append(where, "(l.audit_status = 'error' OR (l.audit_status = '' AND l.action <> 'cyber_policy' AND l.error <> ''))")
	}
	if filter.GroupID != nil {
		add("l.group_id = $%d", *filter.GroupID)
	}
	if endpoint := strings.TrimSpace(filter.Endpoint); endpoint != "" {
		add("l.endpoint = $%d", endpoint)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		like := "%" + search + "%"
		args = append(args, like, like, like, like, like)
		idx := len(args) - 4
		where = append(where, fmt.Sprintf("(l.request_id ILIKE $%d OR l.user_email ILIKE $%d OR l.api_key_name ILIKE $%d OR l.model ILIKE $%d OR l.input_excerpt ILIKE $%d)", idx, idx+1, idx+2, idx+3, idx+4))
	}
	if filter.From != nil && !filter.From.IsZero() {
		add("l.created_at >= $%d", *filter.From)
	}
	if filter.To != nil && !filter.To.IsZero() {
		add("l.created_at <= $%d", *filter.To)
	}
	return where, args
}
