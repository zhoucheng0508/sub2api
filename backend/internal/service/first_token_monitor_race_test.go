package service

import (
	"context"
	"errors"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type monitorRaceRepo struct {
	AccountRepository
	guard            sync.Mutex
	mu               sync.Mutex
	account          Account
	guardAttempts    chan struct{}
	afterPauseCommit func()
}

func newMonitorRaceRepo() *monitorRaceRepo {
	return &monitorRaceRepo{account: Account{ID: 42, Platform: PlatformOpenAI, Credentials: map[string]any{"first_token_monitor_enabled": true, "first_token_monitor_generation": int64(1)}}}
}
func (r *monitorRaceRepo) WithFirstTokenMonitorGuard(_ context.Context, _ int64, fn func(AccountRepository) error) error {
	if r.guardAttempts != nil {
		r.guardAttempts <- struct{}{}
	}
	r.guard.Lock()
	defer r.guard.Unlock()
	return fn(r)
}
func (r *monitorRaceRepo) GetByID(context.Context, int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.account
	a.Credentials = maps.Clone(a.Credentials)
	return &a, nil
}
func (r *monitorRaceRepo) SetFirstTokenMonitorPause(_ context.Context, _ int64, generation int64, until time.Time, reason string) (bool, error) {
	r.mu.Lock()
	cfg := firstTokenMonitorConfig(r.account.Credentials)
	if !cfg.Enabled || cfg.Generation != generation {
		r.mu.Unlock()
		return false, nil
	}
	r.account.TempUnschedulableUntil = &until
	r.account.TempUnschedulableReason = reason
	r.mu.Unlock()
	if r.afterPauseCommit != nil {
		r.afterPauseCommit()
	}
	return true, nil
}
func (r *monitorRaceRepo) ClearTempUnschedulable(context.Context, int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.account.TempUnschedulableUntil = nil
	r.account.TempUnschedulableReason = ""
	r.account.Credentials["first_token_monitor_generation"] = firstTokenMonitorConfig(r.account.Credentials).Generation + 1
	return nil
}
func (r *monitorRaceRepo) ClearModelRateLimits(context.Context, int64) error        { return nil }
func (r *monitorRaceRepo) ClearError(context.Context, int64) error                  { return nil }
func (r *monitorRaceRepo) ClearRateLimit(context.Context, int64) error              { return nil }
func (r *monitorRaceRepo) ClearAntigravityQuotaScopes(context.Context, int64) error { return nil }

type monitorRaceCache struct {
	TempUnschedCache
	observed  chan struct{}
	resume    chan struct{}
	mu        sync.Mutex
	paused    bool
	deleted   bool
	deleteErr error
}

func (c *monitorRaceCache) ObserveFirstToken(_ context.Context, _ int64, _ string, _ *int, _ FirstTokenMonitorConfig) (int64, int64, string, error) {
	close(c.observed)
	<-c.resume
	return 3, time.Now().Add(10 * time.Minute).Unix(), "test monitor pause", nil
}
func (c *monitorRaceCache) SetTempUnsched(context.Context, int64, *TempUnschedState) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused = true
	return nil
}
func (c *monitorRaceCache) GetTempUnsched(context.Context, int64) (*TempUnschedState, error) {
	return nil, nil
}
func (c *monitorRaceCache) DeleteTempUnsched(context.Context, int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.deleteErr != nil {
		return c.deleteErr
	}
	c.paused = false
	c.deleted = true
	return nil
}
func (c *monitorRaceCache) ResetAccountMonitoring(ctx context.Context, id int64) error {
	return c.DeleteTempUnsched(ctx, id)
}
func (c *monitorRaceCache) BlockAccountScheduling(*Account, time.Time, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused = true
}
func (c *monitorRaceCache) ClearAccountSchedulingBlock(int64) {}

func TestFirstTokenMonitorLostSessionCannotPublishAfterRecovery(t *testing.T) {
	repo := newMonitorRaceRepo()
	cache := &monitorRaceCache{observed: make(chan struct{}), resume: make(chan struct{})}
	close(cache.resume)
	svc := NewRateLimitService(repo, nil, nil, nil, cache)
	svc.SetAccountRuntimeBlocker(cache)
	repo.afterPauseCommit = func() {
		// Simulate session loss releasing the lock after SQL commit, followed
		// by another session completing recovery before the old call returns.
		require.NoError(t, repo.ClearTempUnschedulable(context.Background(), 42))
		require.NoError(t, cache.DeleteTempUnsched(context.Background(), 42))
	}
	account, _ := repo.GetByID(context.Background(), 42)
	ms := 31000
	svc.MonitorFirstToken(context.Background(), account, &UsageLog{RequestID: "lost-session", FirstTokenMs: &ms})
	current, _ := repo.GetByID(context.Background(), 42)
	require.Nil(t, current.TempUnschedulableUntil)
	require.True(t, cache.deleted)
	require.False(t, cache.paused, "neither cache nor runtime may be republished")
}

func TestFirstTokenMonitorStatusDoesNotBackfillPauseCache(t *testing.T) {
	repo := newMonitorRaceRepo()
	until := time.Now().Add(time.Hour)
	repo.account.TempUnschedulableUntil = &until
	repo.account.TempUnschedulableReason = "First token monitor: 3 slow requests"
	cache := &monitorRaceCache{}
	svc := NewRateLimitService(repo, nil, nil, nil, cache)
	state, err := svc.GetTempUnschedStatus(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.False(t, cache.paused)
}

func TestFirstTokenMonitorOAuthRefreshPreservesUnrelatedPauses(t *testing.T) {
	for _, reason := range []string{"First token monitor: 3 slow requests", "first_token_monitor", `{"reason":"first_token_monitor"}`, "upstream overload", "OAuth 401: expired", "token refresh retry exhausted: timeout"} {
		t.Run(reason, func(t *testing.T) {
			repo := newMonitorRaceRepo()
			until := time.Now().Add(time.Hour)
			repo.account.TempUnschedulableUntil = &until
			repo.account.TempUnschedulableReason = reason
			cache := &monitorRaceCache{}
			svc := &TokenRefreshService{accountRepo: repo, tempUnschedCache: cache}
			require.NoError(t, svc.clearCredentialPauseAfterRefresh(context.Background(), 42))
			current, _ := repo.GetByID(context.Background(), 42)
			if credentialRefreshRepairsPause(reason) {
				require.Nil(t, current.TempUnschedulableUntil)
				require.True(t, cache.deleted)
			} else {
				require.Equal(t, &until, current.TempUnschedulableUntil)
				require.False(t, cache.deleted)
			}
		})
	}
}

func TestFirstTokenMonitorPostRefreshRereadsPauseReason(t *testing.T) {
	repo := newMonitorRaceRepo()
	until := time.Now().Add(time.Hour)
	repo.account.TempUnschedulableUntil = &until
	repo.account.TempUnschedulableReason = "OAuth 401: expired"
	stale, _ := repo.GetByID(context.Background(), 42)
	// A monitor pause replaces the credential pause while refresh is in flight.
	repo.account.TempUnschedulableReason = "First token monitor: 3 slow requests"
	cache := &monitorRaceCache{}
	svc := &TokenRefreshService{accountRepo: repo, tempUnschedCache: cache}
	svc.postRefreshActions(context.Background(), stale)
	current, _ := repo.GetByID(context.Background(), 42)
	require.Equal(t, &until, current.TempUnschedulableUntil)
	require.Equal(t, "First token monitor: 3 slow requests", current.TempUnschedulableReason)
	require.False(t, cache.deleted)
}

func TestFirstTokenMonitorRecoveryWaitsForInFlightPause(t *testing.T) {
	repo := newMonitorRaceRepo()
	repo.guardAttempts = make(chan struct{}, 2)
	cache := &monitorRaceCache{observed: make(chan struct{}), resume: make(chan struct{})}
	svc := NewRateLimitService(repo, nil, nil, nil, cache)
	account, _ := repo.GetByID(context.Background(), 42)
	ms := 31000
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		svc.MonitorFirstToken(context.Background(), account, &UsageLog{RequestID: "slow", FirstTokenMs: &ms})
	}()
	<-repo.guardAttempts
	<-cache.observed
	recoveryDone := make(chan error, 1)
	go func() { recoveryDone <- svc.ClearTempUnschedulable(context.Background(), 42) }()
	<-repo.guardAttempts // Recovery has attempted to enter the same guard.
	select {
	case err := <-recoveryDone:
		t.Fatalf("recovery finished before old pause: %v", err)
	default:
	}
	close(cache.resume)
	<-monitorDone
	require.NoError(t, <-recoveryDone)
	current, _ := repo.GetByID(context.Background(), 42)
	require.Nil(t, current.TempUnschedulableUntil)
	require.EqualValues(t, 2, firstTokenMonitorConfig(current.Credentials).Generation)
	require.True(t, cache.deleted)
	require.False(t, cache.paused)
}

func TestFirstTokenMonitorConfigurationChangeRejectsOldPause(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "changed", false: "disabled"}[enabled], func(t *testing.T) {
			repo := newMonitorRaceRepo()
			cache := &monitorRaceCache{observed: make(chan struct{}), resume: make(chan struct{})}
			svc := NewRateLimitService(repo, nil, nil, nil, cache)
			account, _ := repo.GetByID(context.Background(), 42)
			ms := 31000
			done := make(chan struct{})
			go func() {
				defer close(done)
				svc.MonitorFirstToken(context.Background(), account, &UsageLog{FirstTokenMs: &ms})
			}()
			<-cache.observed
			repo.mu.Lock()
			repo.account.Credentials["first_token_monitor_generation"] = int64(2)
			repo.account.Credentials["first_token_monitor_enabled"] = enabled
			repo.mu.Unlock()
			close(cache.resume)
			<-done
			current, _ := repo.GetByID(context.Background(), 42)
			require.Nil(t, current.TempUnschedulableUntil)
			require.False(t, cache.paused)
		})
	}
}

func TestFirstTokenMonitorClearErrorResetsCacheAndReportsFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{true: "cache_failure", false: "success"}[fail], func(t *testing.T) {
			repo := newMonitorRaceRepo()
			until := time.Now().Add(time.Hour)
			repo.account.TempUnschedulableUntil = &until
			cache := &monitorRaceCache{paused: true}
			if fail {
				cache.deleteErr = errors.New("redis unavailable")
			}
			admin := &adminServiceImpl{accountRepo: repo, runtimeBlocker: cache}
			_, err := admin.ClearAccountError(context.Background(), 42)
			if fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.True(t, cache.deleted)
				require.False(t, cache.paused)
			}
		})
	}
}
