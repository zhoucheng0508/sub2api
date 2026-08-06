//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration196AddsContentModerationAuditDetailsIdempotently(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	migrationSQL, err := dbmigrations.FS.ReadFile("196_vote_ai_content_moderation_audit_details.sql")
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `ALTER TABLE content_moderation_logs DROP COLUMN IF EXISTS audit_details`)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err, "migration 196 must be safe to apply more than once")

	var (
		dataType      string
		isNullable    string
		columnDefault sql.NullString
		comment       sql.NullString
	)
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT data_type, is_nullable, column_default
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = 'content_moderation_logs'
  AND column_name = 'audit_details'
`).Scan(&dataType, &isNullable, &columnDefault))
	require.Equal(t, "jsonb", dataType)
	require.Equal(t, "NO", isNullable)
	require.True(t, columnDefault.Valid)
	require.Contains(t, columnDefault.String, "'{}'::jsonb")

	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT col_description('content_moderation_logs'::regclass, attnum)
FROM pg_attribute
WHERE attrelid = 'content_moderation_logs'::regclass
  AND attname = 'audit_details'
  AND NOT attisdropped
`).Scan(&comment))
	require.True(t, comment.Valid)
	require.Contains(t, comment.String, "Redacted audit target")
}
