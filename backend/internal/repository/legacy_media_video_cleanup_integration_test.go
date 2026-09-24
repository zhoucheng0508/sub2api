//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

const legacyMediaVideoPreparationMigration = "238_prepare_legacy_media_video_cleanup.sql"

func TestLegacyMediaVideoPreparationMigrationPreservesOldSchema(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	createLegacyMediaVideoSchema(ctx, t, tx)

	migrationSQL, err := dbmigrations.FS.ReadFile(legacyMediaVideoPreparationMigration)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	requireRelationExists(ctx, t, tx, "media_video_tasks", true)
	var legacyPriceColumnCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.columns
WHERE table_name = 'groups' AND column_name = 'video_price_per_request'
`).Scan(&legacyPriceColumnCount))
	require.Equal(t, 1, legacyPriceColumnCount)
}

func TestFinalizeLegacyMediaVideoCleanupRefundsOnlyOutstandingHolds(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	userID, apiKeyID, rawKey := insertLegacyMediaVideoOwner(ctx, t, tx, 90, 11)
	createLegacyMediaVideoSchema(ctx, t, tx)

	for taskID, amount := range map[string]float64{
		"vid_outstanding_hot":     4,
		"vid_outstanding_archive": 1,
		"vid_captured":            3,
		"vid_released":            2,
		"vid_unclaimed":           1,
	} {
		_, err := tx.ExecContext(ctx, `
INSERT INTO media_video_tasks(task_id, user_id, api_key_id, hold_amount)
VALUES ($1, $2, $3, $4)
`, taskID, userID, apiKeyID, amount)
		require.NoError(t, err)
	}

	insertLegacyMediaBillingClaim(ctx, t, tx, "usage_billing_dedup", apiKeyID, "batch_image_hold:vid_outstanding_hot")
	insertLegacyMediaBillingClaim(ctx, t, tx, "usage_billing_dedup_archive", apiKeyID, "batch_image_hold:vid_outstanding_archive")
	insertLegacyMediaBillingClaim(ctx, t, tx, "usage_billing_dedup", apiKeyID, "batch_image_hold:vid_captured")
	insertLegacyMediaBillingClaim(ctx, t, tx, "usage_billing_dedup_archive", apiKeyID, "batch_image_capture:vid_captured")
	insertLegacyMediaBillingClaim(ctx, t, tx, "usage_billing_dedup", apiKeyID, "batch_image_hold:vid_released")
	insertLegacyMediaBillingClaim(ctx, t, tx, "usage_billing_dedup", apiKeyID, "batch_image_release:vid_released")

	result, err := finalizeLegacyMediaVideoCleanupTx(ctx, tx)
	require.NoError(t, err)
	require.True(t, result.LegacySchemaFound)
	require.EqualValues(t, 2, result.RefundedTasks)
	require.EqualValues(t, 1, result.RefundedUsers)
	require.InDelta(t, 5, result.RefundedAmount, 0.00000001)
	require.EqualValues(t, 1, result.PendingBalanceCacheUsers)

	var balance, frozen float64
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT balance, frozen_balance FROM users WHERE id = $1", userID).Scan(&balance, &frozen))
	require.InDelta(t, 95, balance, 0.00000001)
	require.InDelta(t, 6, frozen, 0.00000001)

	var refundRows, authInvalidations int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM legacy_media_video_cleanup_refunds WHERE user_id = $1
`, userID).Scan(&refundRows))
	require.Equal(t, 2, refundRows)
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_cache_invalidation_outbox
WHERE cache_key = encode(sha256(convert_to($1, 'UTF8')), 'hex')
`, rawKey).Scan(&authInvalidations))
	require.GreaterOrEqual(t, authInvalidations, 1)

	for _, relation := range []string{"media_video_tasks", "media_video_ledger", "media_video_order_archive"} {
		requireRelationExists(ctx, t, tx, relation, false)
	}
	var legacyPriceColumnCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.columns
WHERE table_name = 'groups' AND column_name = 'video_price_per_request'
`).Scan(&legacyPriceColumnCount))
	require.Zero(t, legacyPriceColumnCount)

	cache := NewBillingCache(integrationRedis)
	require.NoError(t, cache.SetUserBalance(ctx, userID, 90))
	invalidated, err := invalidateLegacyMediaVideoBalanceCaches(ctx, tx, cache)
	require.NoError(t, err)
	require.EqualValues(t, 1, invalidated)
	_, err = cache.GetUserBalance(ctx, userID)
	require.ErrorIs(t, err, redis.Nil)

	var pendingCacheRows int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM legacy_media_video_cleanup_refunds
WHERE user_id = $1 AND balance_cache_invalidated_at IS NULL
`, userID).Scan(&pendingCacheRows))
	require.Zero(t, pendingCacheRows)
}

func TestFinalizeLegacyMediaVideoCleanupRejectsFrozenBalanceDeficit(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	userID, apiKeyID, _ := insertLegacyMediaVideoOwner(ctx, t, tx, 90, 1)
	createLegacyMediaVideoSchema(ctx, t, tx)
	_, err := tx.ExecContext(ctx, `
INSERT INTO media_video_tasks(task_id, user_id, api_key_id, hold_amount)
VALUES ('vid_deficit', $1, $2, 2)
`, userID, apiKeyID)
	require.NoError(t, err)
	insertLegacyMediaBillingClaim(ctx, t, tx, "usage_billing_dedup", apiKeyID, "batch_image_hold:vid_deficit")

	_, err = finalizeLegacyMediaVideoCleanupTx(ctx, tx)
	require.ErrorContains(t, err, "updated 0 of 1 users")

	var balance, frozen float64
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT balance, frozen_balance FROM users WHERE id = $1", userID).Scan(&balance, &frozen))
	require.InDelta(t, 90, balance, 0.00000001)
	require.InDelta(t, 1, frozen, 0.00000001)
	requireRelationExists(ctx, t, tx, "media_video_tasks", true)
}

func insertLegacyMediaVideoOwner(
	ctx context.Context,
	t *testing.T,
	tx *sql.Tx,
	balance float64,
	frozen float64,
) (int64, int64, string) {
	t.Helper()
	suffix := uuid.NewString()
	rawKey := "sk-legacy-media-" + suffix
	var userID, apiKeyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, balance, frozen_balance)
VALUES ($1, 'hash', $2, $3)
RETURNING id
`, "legacy-media-"+suffix+"@example.com", balance, frozen).Scan(&userID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO api_keys (user_id, key, name)
VALUES ($1, $2, 'legacy media migration')
RETURNING id
`, userID, rawKey).Scan(&apiKeyID))
	return userID, apiKeyID, rawKey
}

func createLegacyMediaVideoSchema(ctx context.Context, t *testing.T, tx *sql.Tx) {
	t.Helper()
	_, err := tx.ExecContext(ctx, `
CREATE TABLE media_video_tasks (
    task_id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id),
    hold_amount NUMERIC(20,8) NOT NULL
);
CREATE TABLE media_video_ledger (task_id TEXT PRIMARY KEY);
CREATE TABLE media_video_order_archive (task_id TEXT PRIMARY KEY);
ALTER TABLE groups ADD COLUMN video_price_per_request NUMERIC(20,8);

CREATE FUNCTION guard_media_video_parent_deletion() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'legacy deletion guard';
END;
$$ LANGUAGE plpgsql;
CREATE FUNCTION guard_media_video_parent_admission() RETURNS trigger AS $$
BEGIN
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER media_video_guard_user_delete
    BEFORE DELETE ON users FOR EACH ROW EXECUTE FUNCTION guard_media_video_parent_deletion();
CREATE TRIGGER media_video_guard_account_delete
    BEFORE DELETE ON accounts FOR EACH ROW EXECUTE FUNCTION guard_media_video_parent_deletion();
CREATE TRIGGER media_video_guard_parent_admission
    BEFORE INSERT ON media_video_tasks FOR EACH ROW EXECUTE FUNCTION guard_media_video_parent_admission();
`)
	require.NoError(t, err)
}

func insertLegacyMediaBillingClaim(
	ctx context.Context,
	t *testing.T,
	tx *sql.Tx,
	table string,
	apiKeyID int64,
	requestID string,
) {
	t.Helper()
	query := `
INSERT INTO ` + table + `(request_id, api_key_id, request_fingerprint, created_at)
VALUES ($1, $2, $3, NOW())
`
	_, err := tx.ExecContext(ctx, query, requestID, apiKeyID, requestID)
	require.NoError(t, err)
}

func requireRelationExists(ctx context.Context, t *testing.T, tx *sql.Tx, relation string, want bool) {
	t.Helper()
	var regclass sql.NullString
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT to_regclass($1)", relation).Scan(&regclass))
	require.Equal(t, want, regclass.Valid, relation)
}
