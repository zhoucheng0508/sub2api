package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestFinalizeLegacyMediaVideoCleanupRollsBackOnFrozenBalanceDeficit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)SET LOCAL lock_timeout.*SET LOCAL statement_timeout`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT to_regclass\('media_video_tasks'\)`).
		WillReturnRows(sqlmock.NewRows([]string{"to_regclass"}).AddRow("media_video_tasks"))
	mock.ExpectExec(`LOCK TABLE media_video_tasks IN ACCESS EXCLUSIVE MODE`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE TEMP TABLE legacy_media_video_pending_refunds`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)INSERT INTO legacy_media_video_pending_refunds.*batch_image_hold:`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\), COUNT\(DISTINCT user_id\).*legacy_media_video_pending_refunds`).
		WillReturnRows(sqlmock.NewRows([]string{"tasks", "users", "amount"}).AddRow(1, 1, 2.0))
	mock.ExpectExec(`(?s)INSERT INTO legacy_media_video_cleanup_refunds.*ON CONFLICT\(task_id\) DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)WITH refunds AS.*COALESCE\(users.frozen_balance, 0\) >= refunds.amount`).
		WillReturnRows(sqlmock.NewRows([]string{"expected", "updated"}).AddRow(1, 0))
	mock.ExpectRollback()

	_, err = FinalizeLegacyMediaVideoCleanup(context.Background(), db)
	require.ErrorContains(t, err, "updated 0 of 1 users")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInvalidateLegacyMediaVideoBalanceCachesLeavesFailedUserPending(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(`(?s)SELECT DISTINCT user_id.*balance_cache_invalidated_at IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
	cacheErr := errors.New("redis unavailable")
	cache := &legacyMediaVideoCacheInvalidatorStub{err: cacheErr}

	invalidated, err := InvalidateLegacyMediaVideoBalanceCaches(context.Background(), db, cache)
	require.Zero(t, invalidated)
	require.ErrorIs(t, err, cacheErr)
	require.Equal(t, []int64{42}, cache.userIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInvalidateLegacyMediaVideoBalanceCachesMarksSuccessfulUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(`(?s)SELECT DISTINCT user_id.*balance_cache_invalidated_at IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
	mock.ExpectExec(`(?s)UPDATE legacy_media_video_cleanup_refunds.*balance_cache_invalidated_at = NOW\(\)`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	cache := &legacyMediaVideoCacheInvalidatorStub{}

	invalidated, err := InvalidateLegacyMediaVideoBalanceCaches(context.Background(), db, cache)
	require.NoError(t, err)
	require.EqualValues(t, 1, invalidated)
	require.Equal(t, []int64{42}, cache.userIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

type legacyMediaVideoCacheInvalidatorStub struct {
	userIDs []int64
	err     error
}

func (s *legacyMediaVideoCacheInvalidatorStub) InvalidateUserBalance(_ context.Context, userID int64) error {
	s.userIDs = append(s.userIDs, userID)
	return s.err
}
