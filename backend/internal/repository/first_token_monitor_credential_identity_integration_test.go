//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestFirstTokenMonitorSchedulerVersionDoesNotChangeCredentialIdentity(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "scheduler-version-credential-cas", Platform: service.PlatformGrok,
		Type: service.AccountTypeOAuth, Credentials: map[string]any{"access_token": "original"},
	})
	observed, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "UPDATE accounts SET name = 'renamed' WHERE id = $1", account.ID)
	require.NoError(t, err)
	current, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotEqual(t, observed.Credentials["scheduler_state_version"], current.Credentials["scheduler_state_version"])
	// A non-credential update must not invalidate the original token snapshot.
	applied, err := repo.SetGrokOAuthErrorIfCredentialsUnchanged(ctx, account.ID, observed.Credentials, "missing refresh token")
	require.NoError(t, err)
	require.True(t, applied)
	_, err = tx.ExecContext(ctx, "UPDATE accounts SET status = 'active', credentials = jsonb_set(credentials, '{access_token}', '\"rotated\"') WHERE id = $1", account.ID)
	require.NoError(t, err)
	// Excluding the fence must never allow an actually replaced token to match.
	applied, err = repo.SetGrokOAuthErrorIfCredentialsUnchanged(ctx, account.ID, observed.Credentials, "stale token error")
	require.NoError(t, err)
	require.False(t, applied)
}
