package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newContentModerationFlaggedHashTestCache(t *testing.T) *contentModerationHashCache {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &contentModerationHashCache{rdb: client}
}

func TestContentModerationFlaggedHash_DeleteFencesStaleWriter(t *testing.T) {
	cache := newContentModerationFlaggedHashTestCache(t)
	ctx := context.Background()
	const inputHash = "policy=v4:normalizer=v2:sha256:AbC123"

	require.NoError(t, cache.RecordFlaggedInputHash(ctx, inputHash))
	deleted, err := cache.DeleteFlaggedInputHash(ctx, inputHash)
	require.NoError(t, err)
	require.True(t, deleted)

	suppressed, err := cache.rdb.SIsMember(ctx, contentModerationFlaggedHashSuppressionSetKey, inputHash).Result()
	require.NoError(t, err)
	require.True(t, suppressed)
	allowed, err := cache.RecordFlaggedInputHashIfAllowed(ctx, inputHash)
	require.NoError(t, err)
	require.False(t, allowed, "the caller must be told that an administrator suppression vetoed promotion")
	require.NoError(t, cache.RecordFlaggedInputHash(ctx, inputHash), "the compatibility method must still ignore a delayed async task")
	flagged, err := cache.HasFlaggedInputHash(ctx, inputHash)
	require.NoError(t, err)
	require.False(t, flagged)

	// Namespacing remains the caller's responsibility: a newer policy digest is
	// a distinct opaque value and is not suppressed by the old policy tombstone.
	const newPolicyHash = "policy=v5:normalizer=v2:sha256:AbC123"
	require.NoError(t, cache.RecordFlaggedInputHash(ctx, newPolicyHash))
	flagged, err = cache.HasFlaggedInputHash(ctx, newPolicyHash)
	require.NoError(t, err)
	require.True(t, flagged)
}

func TestContentModerationFlaggedHash_DeleteMissingHashStillFencesWriter(t *testing.T) {
	cache := newContentModerationFlaggedHashTestCache(t)
	ctx := context.Background()
	const inputHash = "policy=v4:normalizer=v2:sha256:late-result"

	deleted, err := cache.DeleteFlaggedInputHash(ctx, inputHash)
	require.NoError(t, err)
	require.False(t, deleted, "the return value continues to report whether a confirmed hash existed")
	require.NoError(t, cache.RecordFlaggedInputHash(ctx, inputHash))
	flagged, err := cache.HasFlaggedInputHash(ctx, inputHash)
	require.NoError(t, err)
	require.False(t, flagged)
}

func TestContentModerationFlaggedHash_ClearTombstonesOnlyCurrentMembers(t *testing.T) {
	cache := newContentModerationFlaggedHashTestCache(t)
	ctx := context.Background()
	oldHashes := []string{
		"policy=v4:normalizer=v2:sha256:old-a",
		"policy=v4:normalizer=v2:sha256:old-b",
	}
	for _, inputHash := range oldHashes {
		require.NoError(t, cache.RecordFlaggedInputHash(ctx, inputHash))
	}

	deleted, err := cache.ClearFlaggedInputHashes(ctx)
	require.NoError(t, err)
	require.EqualValues(t, len(oldHashes), deleted)
	for _, inputHash := range oldHashes {
		suppressed, suppressionErr := cache.rdb.SIsMember(ctx, contentModerationFlaggedHashSuppressionSetKey, inputHash).Result()
		require.NoError(t, suppressionErr)
		require.True(t, suppressed)
		require.NoError(t, cache.RecordFlaggedInputHash(ctx, inputHash))
		flagged, flaggedErr := cache.HasFlaggedInputHash(ctx, inputHash)
		require.NoError(t, flaggedErr)
		require.False(t, flagged)
	}

	const novelHash = "policy=v4:normalizer=v2:sha256:novel"
	require.NoError(t, cache.RecordFlaggedInputHash(ctx, novelHash))
	flagged, err := cache.HasFlaggedInputHash(ctx, novelHash)
	require.NoError(t, err)
	require.True(t, flagged, "clear must not suppress hashes that were not present")
	count, err := cache.CountFlaggedInputHashes(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}

func TestContentModerationFlaggedHash_DeleteWinsConcurrentStaleRecords(t *testing.T) {
	cache := newContentModerationFlaggedHashTestCache(t)
	ctx := context.Background()
	const rounds = 32
	const writers = 8

	for round := 0; round < rounds; round++ {
		inputHash := fmt.Sprintf("policy=v4:normalizer=v2:sha256:race-%d", round)
		require.NoError(t, cache.RecordFlaggedInputHash(ctx, inputHash))

		start := make(chan struct{})
		errs := make(chan error, writers+1)
		var wg sync.WaitGroup
		for writer := 0; writer < writers; writer++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs <- cache.RecordFlaggedInputHash(ctx, inputHash)
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, deleteErr := cache.DeleteFlaggedInputHash(ctx, inputHash)
			errs <- deleteErr
		}()
		close(start)
		wg.Wait()
		close(errs)
		for operationErr := range errs {
			require.NoError(t, operationErr)
		}

		flagged, err := cache.HasFlaggedInputHash(ctx, inputHash)
		require.NoError(t, err)
		require.False(t, flagged, "delete and suppression must atomically fence every ordering of stale writers")
	}
}

func TestContentModerationFlaggedHash_ClearWinsConcurrentStaleRecords(t *testing.T) {
	cache := newContentModerationFlaggedHashTestCache(t)
	ctx := context.Background()
	const inputHash = "policy=v4:normalizer=v2:sha256:clear-race"
	const writers = 32
	require.NoError(t, cache.RecordFlaggedInputHash(ctx, inputHash))

	start := make(chan struct{})
	errs := make(chan error, writers+1)
	var wg sync.WaitGroup
	for writer := 0; writer < writers; writer++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- cache.RecordFlaggedInputHash(ctx, inputHash)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, clearErr := cache.ClearFlaggedInputHashes(ctx)
		errs <- clearErr
	}()
	close(start)
	wg.Wait()
	close(errs)
	for operationErr := range errs {
		require.NoError(t, operationErr)
	}

	flagged, err := cache.HasFlaggedInputHash(ctx, inputHash)
	require.NoError(t, err)
	require.False(t, flagged, "clear must atomically transfer the existing member into suppression")
}

func TestContentModerationRequestVerdictClaim_IsAtomicAndOwnerScoped(t *testing.T) {
	cache := newContentModerationFlaggedHashTestCache(t)
	ctx := context.Background()
	const key = "opaque-request-verdict-key"

	acquired, err := cache.TryClaimContentModerationRequestVerdict(ctx, key, "owner-a", 10*time.Second)
	require.NoError(t, err)
	require.True(t, acquired)

	acquired, err = cache.TryClaimContentModerationRequestVerdict(ctx, key, "owner-b", 10*time.Second)
	require.NoError(t, err)
	require.False(t, acquired)

	require.NoError(t, cache.ReleaseContentModerationRequestVerdictClaim(ctx, key, "owner-b"))
	owner, err := cache.rdb.Get(ctx, contentModerationRequestVerdictClaimPrefix+key).Result()
	require.NoError(t, err)
	require.Equal(t, "owner-a", owner, "a non-owner must not release another instance's lease")

	require.NoError(t, cache.ReleaseContentModerationRequestVerdictClaim(ctx, key, "owner-a"))
	acquired, err = cache.TryClaimContentModerationRequestVerdict(ctx, key, "owner-b", 10*time.Second)
	require.NoError(t, err)
	require.True(t, acquired)
}
