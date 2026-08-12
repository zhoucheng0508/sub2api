package repository

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	voteaiauditcontext "github.com/Wei-Shaw/sub2api/internal/custom/voteai/auditcontext"
	"github.com/Wei-Shaw/sub2api/internal/custom/voteai/riskstate"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newContentModerationDedupeTestCache(t *testing.T) (*contentModerationHashCache, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &contentModerationHashCache{rdb: client}, server
}

func tryReserveContentModerationEmailForTest(ctx context.Context, cache *contentModerationHashCache, userID int64, scopeHash string, riskRank int, ttl time.Duration) (bool, error) {
	result, err := cache.TryReserveContentModerationEmail(ctx, userID, []service.ContentModerationEmailDedupeScope{{Hash: scopeHash, TTL: ttl}}, riskRank)
	return result.Acquired, err
}

func TestContentModerationEmailDedupe_AtomicSingleWinner(t *testing.T) {
	cache, _ := newContentModerationDedupeTestCache(t)
	ctx := context.Background()
	const workers = 20
	var winners atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			acquired, err := tryReserveContentModerationEmailForTest(ctx, cache, 7, "scope-a", 2, 10*time.Minute)
			if err != nil {
				errs <- err
				return
			}
			if acquired {
				winners.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, winners.Load())
}

func TestContentModerationEmailDedupe_ReservesAllScopesOrNone(t *testing.T) {
	cache, server := newContentModerationDedupeTestCache(t)
	ctx := context.Background()
	inputScope := service.ContentModerationEmailDedupeScope{Hash: "input-a", TTL: 10 * time.Minute}
	sessionScope := service.ContentModerationEmailDedupeScope{Hash: "session-b", TTL: 30 * time.Minute}

	initial, err := cache.TryReserveContentModerationEmail(ctx, 7, []service.ContentModerationEmailDedupeScope{inputScope}, 2)
	require.NoError(t, err)
	require.True(t, initial.Acquired)

	conflict, err := cache.TryReserveContentModerationEmail(ctx, 7, []service.ContentModerationEmailDedupeScope{sessionScope, inputScope}, 2)
	require.NoError(t, err)
	require.False(t, conflict.Acquired)
	require.Equal(t, 1, conflict.ConflictScopeIndex)
	for rank := 0; rank <= 2; rank++ {
		require.False(t, server.Exists(contentModerationEmailDedupeMarkerKey(7, sessionScope.Hash, rank)))
	}

	novel, err := cache.TryReserveContentModerationEmail(ctx, 7, []service.ContentModerationEmailDedupeScope{
		sessionScope,
		{Hash: "input-b", TTL: 10 * time.Minute},
	}, 2)
	require.NoError(t, err)
	require.True(t, novel.Acquired)
}

func TestContentModerationEmailDedupe_ReleaseAllowsRetryAfterSendFailure(t *testing.T) {
	cache, _ := newContentModerationDedupeTestCache(t)
	ctx := context.Background()
	scopes := []service.ContentModerationEmailDedupeScope{
		{Hash: "session-a", TTL: 30 * time.Minute},
		{Hash: "input-a", TTL: 10 * time.Minute},
	}

	first, err := cache.TryReserveContentModerationEmail(ctx, 7, scopes, 2)
	require.NoError(t, err)
	require.True(t, first.Acquired)
	require.NotNil(t, first.Lease)
	deleted, err := cache.ReleaseContentModerationEmail(ctx, *first.Lease)
	require.NoError(t, err)
	require.EqualValues(t, 6, deleted)

	retry, err := cache.TryReserveContentModerationEmail(ctx, 7, scopes, 2)
	require.NoError(t, err)
	require.True(t, retry.Acquired)
}

func TestContentModerationEmailDedupe_ReleaseDoesNotDeleteNewLease(t *testing.T) {
	cache, server := newContentModerationDedupeTestCache(t)
	ctx := context.Background()
	scopes := []service.ContentModerationEmailDedupeScope{{Hash: "input-a", TTL: time.Minute}}

	oldReservation, err := cache.TryReserveContentModerationEmail(ctx, 7, scopes, 2)
	require.NoError(t, err)
	require.True(t, oldReservation.Acquired)
	server.FastForward(time.Minute + time.Second)
	newReservation, err := cache.TryReserveContentModerationEmail(ctx, 7, scopes, 2)
	require.NoError(t, err)
	require.True(t, newReservation.Acquired)
	require.NotEqual(t, oldReservation.Lease.Token, newReservation.Lease.Token)

	deleted, err := cache.ReleaseContentModerationEmail(ctx, *oldReservation.Lease)
	require.NoError(t, err)
	require.Zero(t, deleted)
	marker := contentModerationEmailDedupeMarkerKey(7, scopes[0].Hash, 2)
	markerValue, err := server.Get(marker)
	require.NoError(t, err)
	require.Equal(t, newReservation.Lease.Token, markerValue)

	duplicate, err := cache.TryReserveContentModerationEmail(ctx, 7, scopes, 2)
	require.NoError(t, err)
	require.False(t, duplicate.Acquired)
}

func TestContentModerationEmailDedupe_ExpiresInputAndSessionScopes(t *testing.T) {
	cache, server := newContentModerationDedupeTestCache(t)
	ctx := context.Background()

	inputAcquired, err := tryReserveContentModerationEmailForTest(ctx, cache, 7, "input", 2, 10*time.Minute)
	require.NoError(t, err)
	require.True(t, inputAcquired)
	inputMarker := contentModerationEmailDedupeMarkerKey(7, "input", 2)
	require.Equal(t, 10*time.Minute, server.TTL(inputMarker))

	sessionAcquired, err := tryReserveContentModerationEmailForTest(ctx, cache, 7, "session", 2, 30*time.Minute)
	require.NoError(t, err)
	require.True(t, sessionAcquired)
	sessionMarker := contentModerationEmailDedupeMarkerKey(7, "session", 2)
	require.Equal(t, 30*time.Minute, server.TTL(sessionMarker))

	server.FastForward(10*time.Minute + time.Second)
	inputAcquired, err = tryReserveContentModerationEmailForTest(ctx, cache, 7, "input", 2, 10*time.Minute)
	require.NoError(t, err)
	require.True(t, inputAcquired)
	sessionAcquired, err = tryReserveContentModerationEmailForTest(ctx, cache, 7, "session", 2, 30*time.Minute)
	require.NoError(t, err)
	require.False(t, sessionAcquired)

	server.FastForward(20 * time.Minute)
	sessionAcquired, err = tryReserveContentModerationEmailForTest(ctx, cache, 7, "session", 2, 30*time.Minute)
	require.NoError(t, err)
	require.True(t, sessionAcquired)
}

func TestContentModerationEmailDedupe_AllowsEscalationAndSuppressesDowngrade(t *testing.T) {
	cache, _ := newContentModerationDedupeTestCache(t)
	ctx := context.Background()

	high, err := tryReserveContentModerationEmailForTest(ctx, cache, 7, "scope-a", 2, 10*time.Minute)
	require.NoError(t, err)
	critical, err := tryReserveContentModerationEmailForTest(ctx, cache, 7, "scope-a", 3, 10*time.Minute)
	require.NoError(t, err)
	downgrade, err := tryReserveContentModerationEmailForTest(ctx, cache, 7, "scope-a", 2, 10*time.Minute)
	require.NoError(t, err)

	require.True(t, high)
	require.True(t, critical)
	require.False(t, downgrade)
}

func TestContentModerationEmailDedupe_IsolatesUsers(t *testing.T) {
	cache, _ := newContentModerationDedupeTestCache(t)
	ctx := context.Background()

	userOne, err := tryReserveContentModerationEmailForTest(ctx, cache, 7, "shared-scope", 2, 10*time.Minute)
	require.NoError(t, err)
	userTwo, err := tryReserveContentModerationEmailForTest(ctx, cache, 8, "shared-scope", 2, 10*time.Minute)
	require.NoError(t, err)

	require.True(t, userOne)
	require.True(t, userTwo)
}

func TestContentModerationUserStateClear_IsTargeted(t *testing.T) {
	cache, server := newContentModerationDedupeTestCache(t)
	ctx := context.Background()
	cfg := riskstate.DefaultConfig()
	event := riskstate.Event{Score: 0.8, Categories: []string{"cyber_abuse"}, At: time.Now().UTC()}
	auditCfg := voteaiauditcontext.DefaultConfig()
	auditEvent := voteaiauditcontext.AuditEvent{
		RiskScore:     0.42,
		Categories:    []string{"cyber_abuse"},
		Reason:        "redacted audit state",
		RequestID:     "request-7",
		PolicyVersion: "policy-v5",
		TurnIncrement: 1,
		At:            time.Now().UTC(),
	}

	for _, key := range []string{"user-7-session", "user-7-actor"} {
		_, err := cache.UpdateContentModerationSessionRiskForUser(ctx, 7, key, event, cfg)
		require.NoError(t, err)
	}
	_, err := cache.UpdateContentModerationSessionRiskForUser(ctx, 8, "user-8-session", event, cfg)
	require.NoError(t, err)
	_, err = cache.UpdateContentModerationAuditContextForUser(ctx, 7, "user-7-audit", auditEvent, auditCfg, 10*time.Minute)
	require.NoError(t, err)
	auditEvent.RequestID = "request-8"
	_, err = cache.UpdateContentModerationAuditContextForUser(ctx, 8, "user-8-audit", auditEvent, auditCfg, 10*time.Minute)
	require.NoError(t, err)
	userSevenEmail, err := tryReserveContentModerationEmailForTest(ctx, cache, 7, "email-scope", 2, 10*time.Minute)
	require.NoError(t, err)
	require.True(t, userSevenEmail)
	userEightEmail, err := tryReserveContentModerationEmailForTest(ctx, cache, 8, "email-scope", 2, 10*time.Minute)
	require.NoError(t, err)
	require.True(t, userEightEmail)
	require.NoError(t, cache.RecordFlaggedInputHash(ctx, "global-flagged-hash"))
	epoch, err := cache.GetContentModerationUserEpoch(ctx, 7)
	require.NoError(t, err)
	require.Zero(t, epoch)
	require.True(t, server.Exists(contentModerationAuditContextPrefix+"user-7-audit"))
	require.True(t, server.Exists(contentModerationAuditContextIndexKey(7)))

	cleared, err := cache.ClearContentModerationUserState(ctx, 7)
	require.NoError(t, err)
	require.EqualValues(t, 6, cleared)
	require.False(t, server.Exists(contentModerationSessionRiskIndexKey(7)))
	require.False(t, server.Exists(contentModerationEmailDedupeIndexKey(7)))
	require.False(t, server.Exists(contentModerationAuditContextPrefix+"user-7-audit"))
	require.False(t, server.Exists(contentModerationAuditContextIndexKey(7)))
	require.True(t, server.Exists(contentModerationSessionRiskIndexKey(8)))
	require.True(t, server.Exists(contentModerationEmailDedupeIndexKey(8)))
	require.True(t, server.Exists(contentModerationAuditContextPrefix+"user-8-audit"))
	require.True(t, server.Exists(contentModerationAuditContextIndexKey(8)))
	epoch, err = cache.GetContentModerationUserEpoch(ctx, 7)
	require.NoError(t, err)
	require.EqualValues(t, 1, epoch)
	otherEpoch, err := cache.GetContentModerationUserEpoch(ctx, 8)
	require.NoError(t, err)
	require.Zero(t, otherEpoch)

	for _, key := range []string{"user-7-session", "user-7-actor"} {
		_, found, getErr := cache.GetContentModerationSessionRisk(ctx, key)
		require.NoError(t, getErr)
		require.False(t, found)
	}
	_, found, err := cache.GetContentModerationSessionRisk(ctx, "user-8-session")
	require.NoError(t, err)
	require.True(t, found)
	_, found, err = cache.GetContentModerationAuditContext(ctx, "user-7-audit")
	require.NoError(t, err)
	require.False(t, found)
	_, found, err = cache.GetContentModerationAuditContext(ctx, "user-8-audit")
	require.NoError(t, err)
	require.True(t, found)
	userSevenEmail, err = tryReserveContentModerationEmailForTest(ctx, cache, 7, "email-scope", 2, 10*time.Minute)
	require.NoError(t, err)
	require.True(t, userSevenEmail)
	userEightEmail, err = tryReserveContentModerationEmailForTest(ctx, cache, 8, "email-scope", 2, 10*time.Minute)
	require.NoError(t, err)
	require.False(t, userEightEmail)
	flagged, err := cache.HasFlaggedInputHash(ctx, "global-flagged-hash")
	require.NoError(t, err)
	require.True(t, flagged)
}

func TestContentModerationSessionRiskV2IgnoresLegacyV1State(t *testing.T) {
	cache, server := newContentModerationDedupeTestCache(t)
	ctx := context.Background()
	const key = "legacy-session"
	legacyKey := "content_moderation:session_risk:v1:" + key
	require.NoError(t, server.Set(legacyKey, `{"score":0.99}`))

	state, found, err := cache.GetContentModerationSessionRisk(ctx, key)

	require.NoError(t, err)
	require.False(t, found)
	require.Zero(t, state.Score)
	require.True(t, server.Exists(legacyKey), "legacy state should expire naturally without being read")
	require.Equal(t, "content_moderation:session_risk:v2:", contentModerationSessionRiskPrefix)
	require.Equal(t, "content_moderation:session_risk_index:v2:", contentModerationSessionRiskIndexPrefix)
}

func TestContentModerationUserEpochAdvancesIndependently(t *testing.T) {
	cache, _ := newContentModerationDedupeTestCache(t)
	ctx := context.Background()

	first, err := cache.AdvanceContentModerationUserEpoch(ctx, 7)
	require.NoError(t, err)
	second, err := cache.AdvanceContentModerationUserEpoch(ctx, 7)
	require.NoError(t, err)
	other, err := cache.GetContentModerationUserEpoch(ctx, 8)
	require.NoError(t, err)

	require.EqualValues(t, 1, first)
	require.EqualValues(t, 2, second)
	require.Zero(t, other)
	require.Equal(t, fmt.Sprintf("%s{%d}", contentModerationUserEpochPrefix, 7), contentModerationUserEpochKey(7))
}

func TestContentModerationSessionRiskIndexTTLDoesNotShrink(t *testing.T) {
	cache, server := newContentModerationDedupeTestCache(t)
	ctx := context.Background()
	event := riskstate.Event{Score: 0.8, Categories: []string{"cyber_abuse"}, At: time.Now().UTC()}
	longCfg := riskstate.DefaultConfig()
	longCfg.TTL = 24 * time.Hour
	shortCfg := riskstate.DefaultConfig()
	shortCfg.TTL = 2 * time.Hour

	_, err := cache.UpdateContentModerationSessionRiskForUser(ctx, 7, "actor", event, longCfg)
	require.NoError(t, err)
	_, err = cache.UpdateContentModerationSessionRiskForUser(ctx, 7, "session", event, shortCfg)
	require.NoError(t, err)

	ttl := server.TTL(contentModerationSessionRiskIndexKey(7))
	require.GreaterOrEqual(t, ttl, 24*time.Hour-time.Second)
	require.LessOrEqual(t, ttl, 24*time.Hour)
	require.Equal(t, fmt.Sprintf("%s{%d}", contentModerationSessionRiskIndexPrefix, 7), contentModerationSessionRiskIndexKey(7))
}

func TestContentModerationSessionRiskConcurrentActorUpdatesRetryConflicts(t *testing.T) {
	cache, _ := newContentModerationDedupeTestCache(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const workers = 20
	const actorKey = "shared-actor-risk-key"
	cfg := riskstate.DefaultConfig()
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			_, err := cache.UpdateContentModerationSessionRiskForUser(ctx, 7, actorKey, riskstate.Event{
				Score:      0.4,
				Categories: []string{"cyber_abuse"},
				RequestID:  fmt.Sprintf("request-%d", worker),
				At:         time.Now().UTC(),
			}, cfg)
			if err != nil {
				errs <- err
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	state, found, err := cache.GetContentModerationSessionRisk(ctx, actorKey)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, workers, state.Strikes)
}
