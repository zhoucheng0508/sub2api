package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestImageTaskStoreRoundTripAndTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	task := &service.ImageTaskRecord{
		ID:        "imgtask_123",
		UserID:    7,
		APIKeyID:  9,
		Status:    service.ImageTaskStatusProcessing,
		CreatedAt: 100,
		ExpiresAt: 200,
	}

	require.NoError(t, store.Save(context.Background(), task, 24*time.Hour))
	got, err := store.Get(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, task, got)
	require.Equal(t, 24*time.Hour, mr.TTL(imageTaskKey(task.ID)))
}

func TestImageTaskStoreMissing(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)

	_, err := store.Get(context.Background(), "imgtask_missing")
	require.ErrorIs(t, err, service.ErrImageTaskNotFound)
}

func TestImageTaskStoreActiveSlotIsAtomicAndOwnerSafe(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()

	acquired, err := store.Acquire(ctx, 9, "imgtask_first", 1, 35*time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	require.InDelta(t, (35 * time.Minute).Seconds(), mr.TTL(imageTaskActiveKey(9)).Seconds(), 1)

	acquired, err = store.Acquire(ctx, 9, "imgtask_second", 1, 35*time.Minute)
	require.NoError(t, err)
	require.False(t, acquired)

	require.NoError(t, store.Release(ctx, 9, "imgtask_wrong"))
	require.True(t, mr.Exists(imageTaskActiveKey(9)))
	require.NoError(t, store.Release(ctx, 9, "imgtask_first"))
	require.False(t, mr.Exists(imageTaskActiveKey(9)))

	acquired, err = store.Acquire(ctx, 9, "imgtask_new", 1, 35*time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, store.Release(ctx, 9, "imgtask_first"))
	members, err := mr.ZMembers(imageTaskActiveKey(9))
	require.NoError(t, err)
	require.Equal(t, []string{"imgtask_new"}, members)
}

func TestImageTaskStoreOnlyOneConcurrentAcquireWins(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()

	const contenders = 32
	winners := make(chan string, contenders)
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			taskID := fmt.Sprintf("imgtask_%d", index)
			acquired, err := store.Acquire(ctx, 99, taskID, 1, 35*time.Minute)
			require.NoError(t, err)
			if acquired {
				winners <- taskID
			}
		}(i)
	}
	wg.Wait()
	close(winners)

	var winnerIDs []string
	for taskID := range winners {
		winnerIDs = append(winnerIDs, taskID)
	}
	require.Len(t, winnerIDs, 1)
	members, err := mr.ZMembers(imageTaskActiveKey(99))
	require.NoError(t, err)
	require.Equal(t, winnerIDs, members)
}

func TestImageTaskStoreAllowsConfiguredSlotsAndReclaimsExpired(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()

	first, err := store.Acquire(ctx, 9, "first", 2, time.Minute)
	require.NoError(t, err)
	require.True(t, first)
	second, err := store.Acquire(ctx, 9, "second", 2, time.Minute)
	require.NoError(t, err)
	require.True(t, second)
	third, err := store.Acquire(ctx, 9, "third", 2, time.Minute)
	require.NoError(t, err)
	require.False(t, third)

	mr.FastForward(61 * time.Second)
	third, err = store.Acquire(ctx, 9, "third", 2, time.Minute)
	require.NoError(t, err)
	require.True(t, third)
}

func TestImageTaskStoreDuplicateTaskDoesNotConsumeAnotherSlot(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()

	acquired, err := store.Acquire(ctx, 9, "same-task", 2, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = store.Acquire(ctx, 9, "same-task", 2, time.Minute)
	require.NoError(t, err)
	require.False(t, acquired)

	acquired, err = store.Acquire(ctx, 9, "other-task", 2, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	members, err := mr.ZMembers(imageTaskActiveKey(9))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"same-task", "other-task"}, members)
}

func TestImageTaskStoreWaitsForLegacyStringLock(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()

	require.NoError(t, rdb.Set(ctx, imageTaskActiveKey(9), "legacy-task", time.Minute).Err())
	acquired, err := store.Acquire(ctx, 9, "new-task", 2, time.Minute)
	require.NoError(t, err)
	require.False(t, acquired)

	require.NoError(t, store.Release(ctx, 9, "different-task"))
	require.True(t, mr.Exists(imageTaskActiveKey(9)))
	require.NoError(t, store.Release(ctx, 9, "legacy-task"))
	require.False(t, mr.Exists(imageTaskActiveKey(9)))

	acquired, err = store.Acquire(ctx, 9, "new-task", 2, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
}
