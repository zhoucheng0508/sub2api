package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const legacyMediaVideoCandidateSelectSQL = `SELECT task.task_id, task.user_id, task.api_key_id, task.hold_amount AS amount
FROM media_video_tasks task
WHERE task.hold_amount > 0
  AND NOT EXISTS (
      SELECT 1 FROM legacy_media_video_cleanup_refunds audit
      WHERE audit.task_id = task.task_id
  )
  AND EXISTS (
      SELECT 1
      FROM (
          SELECT request_id, api_key_id FROM usage_billing_dedup
          UNION ALL
          SELECT request_id, api_key_id FROM usage_billing_dedup_archive
      ) event
      WHERE event.request_id = 'batch_image_hold:' || task.task_id
        AND event.api_key_id = task.api_key_id
  )
  AND NOT EXISTS (
      SELECT 1
      FROM (
          SELECT request_id, api_key_id FROM usage_billing_dedup
          UNION ALL
          SELECT request_id, api_key_id FROM usage_billing_dedup_archive
      ) event
      WHERE event.request_id IN (
                'batch_image_capture:' || task.task_id,
                'batch_image_release:' || task.task_id
            )
        AND event.api_key_id = task.api_key_id
  )`

const legacyMediaVideoCandidateInsertSQL = `
INSERT INTO legacy_media_video_pending_refunds(task_id, user_id, api_key_id, amount)
` + legacyMediaVideoCandidateSelectSQL

type LegacyMediaVideoCleanupResult struct {
	LegacySchemaFound        bool
	RefundedTasks            int64
	RefundedUsers            int64
	RefundedAmount           float64
	PendingBalanceCacheUsers int64
}

type userBalanceCacheInvalidator interface {
	InvalidateUserBalance(context.Context, int64) error
}

func PreviewLegacyMediaVideoCleanup(ctx context.Context, db *sql.DB) (LegacyMediaVideoCleanupResult, error) {
	if db == nil {
		return LegacyMediaVideoCleanupResult{}, errors.New("legacy media-video cleanup database is nil")
	}
	result := LegacyMediaVideoCleanupResult{}
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT user_id)
FROM legacy_media_video_cleanup_refunds
WHERE balance_cache_invalidated_at IS NULL
`).Scan(&result.PendingBalanceCacheUsers); err != nil {
		return result, fmt.Errorf("count pending legacy media-video cache invalidations: %w", err)
	}

	found, err := legacyMediaVideoSchemaExists(ctx, db)
	if err != nil {
		return result, err
	}
	result.LegacySchemaFound = found
	if !found {
		return result, nil
	}

	query := `
WITH candidates AS (` + legacyMediaVideoCandidateSelectSQL + `)
SELECT COUNT(*), COUNT(DISTINCT user_id), COALESCE(SUM(amount), 0)::double precision
FROM candidates`
	if err := db.QueryRowContext(ctx, query).Scan(
		&result.RefundedTasks,
		&result.RefundedUsers,
		&result.RefundedAmount,
	); err != nil {
		return result, fmt.Errorf("preview legacy media-video refunds: %w", err)
	}
	return result, nil
}

func FinalizeLegacyMediaVideoCleanup(ctx context.Context, db *sql.DB) (LegacyMediaVideoCleanupResult, error) {
	if db == nil {
		return LegacyMediaVideoCleanupResult{}, errors.New("legacy media-video cleanup database is nil")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return LegacyMediaVideoCleanupResult{}, fmt.Errorf("begin legacy media-video cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := finalizeLegacyMediaVideoCleanupTx(ctx, tx)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit legacy media-video cleanup: %w", err)
	}
	return result, nil
}

func finalizeLegacyMediaVideoCleanupTx(ctx context.Context, tx *sql.Tx) (LegacyMediaVideoCleanupResult, error) {
	result := LegacyMediaVideoCleanupResult{}
	if _, err := tx.ExecContext(ctx, `
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';
`); err != nil {
		return result, fmt.Errorf("configure legacy media-video cleanup transaction: %w", err)
	}

	found, err := legacyMediaVideoSchemaExists(ctx, tx)
	if err != nil {
		return result, err
	}
	result.LegacySchemaFound = found
	if found {
		if _, err := tx.ExecContext(ctx, "LOCK TABLE media_video_tasks IN ACCESS EXCLUSIVE MODE"); err != nil {
			return result, fmt.Errorf("lock legacy media-video tasks: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
CREATE TEMP TABLE legacy_media_video_pending_refunds (
    task_id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    amount NUMERIC(20,8) NOT NULL
) ON COMMIT DROP
`); err != nil {
			return result, fmt.Errorf("create legacy media-video refund work table: %w", err)
		}
		if _, err := tx.ExecContext(ctx, legacyMediaVideoCandidateInsertSQL); err != nil {
			return result, fmt.Errorf("identify legacy media-video refunds: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*), COUNT(DISTINCT user_id), COALESCE(SUM(amount), 0)::double precision
FROM legacy_media_video_pending_refunds
`).Scan(&result.RefundedTasks, &result.RefundedUsers, &result.RefundedAmount); err != nil {
			return result, fmt.Errorf("summarize legacy media-video refunds: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO legacy_media_video_cleanup_refunds(task_id, user_id, api_key_id, amount)
SELECT task_id, user_id, api_key_id, amount
FROM legacy_media_video_pending_refunds
ON CONFLICT(task_id) DO NOTHING
`); err != nil {
			return result, fmt.Errorf("record legacy media-video refund audit: %w", err)
		}

		var expectedUsers, updatedUsers int64
		if err := tx.QueryRowContext(ctx, `
WITH refunds AS (
    SELECT user_id, SUM(amount) AS amount
    FROM legacy_media_video_pending_refunds
    GROUP BY user_id
), updated AS (
    UPDATE users
       SET balance = users.balance + refunds.amount,
           frozen_balance = users.frozen_balance - refunds.amount,
           updated_at = NOW()
      FROM refunds
     WHERE users.id = refunds.user_id
       AND COALESCE(users.frozen_balance, 0) >= refunds.amount
    RETURNING users.id
)
SELECT (SELECT COUNT(*) FROM refunds), COUNT(*) FROM updated
`).Scan(&expectedUsers, &updatedUsers); err != nil {
			return result, fmt.Errorf("refund legacy media-video holds: %w", err)
		}
		if updatedUsers != expectedUsers {
			return result, fmt.Errorf(
				"legacy media-video refund aborted: updated %d of %d users; a user is missing or frozen balance is insufficient",
				updatedUsers,
				expectedUsers,
			)
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO auth_cache_invalidation_outbox(cache_key)
SELECT DISTINCT encode(sha256(convert_to(api_keys.key, 'UTF8')), 'hex')
FROM api_keys
JOIN legacy_media_video_pending_refunds refund ON refund.user_id = api_keys.user_id
WHERE api_keys.deleted_at IS NULL AND api_keys.key <> ''
`); err != nil {
			return result, fmt.Errorf("enqueue legacy media-video auth cache invalidation: %w", err)
		}

		if _, err := tx.ExecContext(ctx, legacyMediaVideoDropSQL); err != nil {
			return result, fmt.Errorf("drop legacy media-video schema: %w", err)
		}
	}

	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT user_id)
FROM legacy_media_video_cleanup_refunds
WHERE balance_cache_invalidated_at IS NULL
`).Scan(&result.PendingBalanceCacheUsers); err != nil {
		return result, fmt.Errorf("count pending legacy media-video cache invalidations: %w", err)
	}
	return result, nil
}

func InvalidateLegacyMediaVideoBalanceCaches(
	ctx context.Context,
	db *sql.DB,
	cache userBalanceCacheInvalidator,
) (int64, error) {
	if db == nil {
		return 0, errors.New("legacy media-video cleanup database is nil")
	}
	return invalidateLegacyMediaVideoBalanceCaches(ctx, db, cache)
}

func invalidateLegacyMediaVideoBalanceCaches(
	ctx context.Context,
	db interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	cache userBalanceCacheInvalidator,
) (int64, error) {
	if cache == nil {
		return 0, errors.New("legacy media-video billing cache is nil")
	}
	rows, err := db.QueryContext(ctx, `
SELECT DISTINCT user_id
FROM legacy_media_video_cleanup_refunds
WHERE balance_cache_invalidated_at IS NULL
ORDER BY user_id
`)
	if err != nil {
		return 0, fmt.Errorf("list legacy media-video cache invalidations: %w", err)
	}
	var userIDs []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan legacy media-video cache invalidation: %w", err)
		}
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterate legacy media-video cache invalidations: %w", err)
	}
	_ = rows.Close()

	var invalidated int64
	for _, userID := range userIDs {
		if err := cache.InvalidateUserBalance(ctx, userID); err != nil {
			return invalidated, fmt.Errorf("invalidate legacy media-video balance cache for user %d: %w", userID, err)
		}
		if _, err := db.ExecContext(ctx, `
UPDATE legacy_media_video_cleanup_refunds
SET balance_cache_invalidated_at = NOW()
WHERE user_id = $1 AND balance_cache_invalidated_at IS NULL
`, userID); err != nil {
			return invalidated, fmt.Errorf("mark legacy media-video balance cache invalidated for user %d: %w", userID, err)
		}
		invalidated++
	}
	return invalidated, nil
}

func legacyMediaVideoSchemaExists(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) (bool, error) {
	var relation sql.NullString
	if err := queryer.QueryRowContext(ctx, "SELECT to_regclass('media_video_tasks')").Scan(&relation); err != nil {
		return false, fmt.Errorf("detect legacy media-video schema: %w", err)
	}
	return relation.Valid, nil
}

const legacyMediaVideoDropSQL = `
DROP TRIGGER IF EXISTS media_video_guard_account_soft_delete ON accounts;
DROP TRIGGER IF EXISTS media_video_guard_account_delete ON accounts;
DROP TRIGGER IF EXISTS media_video_guard_user_soft_delete ON users;
DROP TRIGGER IF EXISTS media_video_guard_user_delete ON users;

DROP TABLE IF EXISTS media_video_tasks CASCADE;

DROP FUNCTION IF EXISTS guard_media_video_parent_deletion();
DROP FUNCTION IF EXISTS guard_media_video_parent_admission();
DROP FUNCTION IF EXISTS reconcile_media_video_submission(TEXT, BIGINT, TEXT, BOOLEAN, TEXT);
DROP FUNCTION IF EXISTS refund_media_video_dispute(TEXT, BIGINT, TEXT);
DROP FUNCTION IF EXISTS archive_media_video_order();

DROP TABLE IF EXISTS media_video_ledger;
DROP TABLE IF EXISTS media_video_order_archive;

ALTER TABLE groups DROP COLUMN IF EXISTS video_price_per_request;
`
