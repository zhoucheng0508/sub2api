package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

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
		WithArgs(3, true, true, "completed", "sent", "", int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := NewContentModerationRepository(db)
	err = repo.UpdateLogEffects(context.Background(), 91, patch)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryUpdateLogEffects_RejectsMissingLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE content_moderation_logs")).
		WithArgs(0, false, false, "", "", "", int64(404)).
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
