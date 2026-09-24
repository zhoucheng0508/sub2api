//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const legacyMediaVideoDecommissionMigration = "238_decommission_legacy_media_video.sql"

func TestLegacyMediaVideoDecommissionRefundsOnlyOutstandingHolds(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	userID, apiKeyID := insertLegacyMediaVideoOwner(ctx, t, tx, 90, 10)
	createLegacyMediaVideoSchema(ctx, t, tx)

	for taskID, amount := range map[string]float64{
		"vid_outstanding": 4,
		"vid_captured":    3,
		"vid_released":    2,
		"vid_unclaimed":   1,
	} {
		_, err := tx.ExecContext(ctx, `
INSERT INTO media_video_tasks(task_id, user_id, api_key_id, hold_amount)
VALUES ($1, $2, $3, $4)
`, taskID, userID, apiKeyID, amount)
		require.NoError(t, err)
	}

	insertLegacyMediaBillingClaim(ctx, t, tx, apiKeyID, "batch_image_hold:vid_outstanding")
	insertLegacyMediaBillingClaim(ctx, t, tx, apiKeyID, "batch_image_hold:vid_captured")
	insertLegacyMediaBillingClaim(ctx, t, tx, apiKeyID, "batch_image_capture:vid_captured")
	insertLegacyMediaBillingClaim(ctx, t, tx, apiKeyID, "batch_image_hold:vid_released")
	insertLegacyMediaBillingClaim(ctx, t, tx, apiKeyID, "batch_image_release:vid_released")

	applyLegacyMediaVideoDecommission(ctx, t, tx)

	var balance, frozen float64
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT balance, frozen_balance FROM users WHERE id = $1", userID).Scan(&balance, &frozen))
	require.InDelta(t, 94, balance, 0.00000001)
	require.InDelta(t, 6, frozen, 0.00000001)

	for _, relation := range []string{"media_video_tasks", "media_video_ledger", "media_video_order_archive"} {
		var regclass sql.NullString
		require.NoError(t, tx.QueryRowContext(ctx, "SELECT to_regclass($1)", relation).Scan(&regclass))
		require.False(t, regclass.Valid, relation)
	}

	var legacyPriceColumnCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.columns
WHERE table_name = 'groups' AND column_name = 'video_price_per_request'
`).Scan(&legacyPriceColumnCount))
	require.Zero(t, legacyPriceColumnCount)

	for _, trigger := range []string{
		"media_video_guard_user_delete",
		"media_video_guard_account_delete",
	} {
		var count int
		require.NoError(t, tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM pg_trigger WHERE tgname = $1", trigger).Scan(&count))
		require.Zero(t, count, trigger)
	}
}

func TestLegacyMediaVideoDecommissionRejectsFrozenBalanceDeficit(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	userID, apiKeyID := insertLegacyMediaVideoOwner(ctx, t, tx, 90, 1)
	createLegacyMediaVideoSchema(ctx, t, tx)
	_, err := tx.ExecContext(ctx, `
INSERT INTO media_video_tasks(task_id, user_id, api_key_id, hold_amount)
VALUES ('vid_deficit', $1, $2, 2)
`, userID, apiKeyID)
	require.NoError(t, err)
	insertLegacyMediaBillingClaim(ctx, t, tx, apiKeyID, "batch_image_hold:vid_deficit")

	migrationSQL, err := dbmigrations.FS.ReadFile(legacyMediaVideoDecommissionMigration)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.ErrorContains(t, err, "legacy media-video holds exceed frozen balance")
}

func insertLegacyMediaVideoOwner(
	ctx context.Context,
	t *testing.T,
	tx *sql.Tx,
	balance float64,
	frozen float64,
) (int64, int64) {
	t.Helper()
	suffix := uuid.NewString()
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
`, userID, "sk-legacy-media-"+suffix).Scan(&apiKeyID))
	return userID, apiKeyID
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
	apiKeyID int64,
	requestID string,
) {
	t.Helper()
	_, err := tx.ExecContext(ctx, `
INSERT INTO usage_billing_dedup(request_id, api_key_id, request_fingerprint)
VALUES ($1, $2, $3)
`, requestID, apiKeyID, requestID)
	require.NoError(t, err)
}

func applyLegacyMediaVideoDecommission(ctx context.Context, t *testing.T, tx *sql.Tx) {
	t.Helper()
	migrationSQL, err := dbmigrations.FS.ReadFile(legacyMediaVideoDecommissionMigration)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
}
