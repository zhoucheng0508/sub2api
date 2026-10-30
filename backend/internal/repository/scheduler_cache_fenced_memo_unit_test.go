//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A warmed v0.2.19 snapshot must observe a production monitoring pause,
// while a delayed snapshot from before that pause cannot clear it.
func TestSchedulerCacheFencedWriteInvalidatesWarmSnapshot(t *testing.T) {
	ctx := context.Background()
	cache, _, _ := newSchedulerCacheMemoUnit(t, 2)
	bucket := memoTestBucket(719)
	old := memoTestAccount(719, "before-pause")
	old.Credentials["first_token_monitor_generation"] = 4
	old.Credentials["scheduler_state_version"] = "9007199254740992"
	memoPublish(t, cache, bucket, old)
	require.Nil(t, memoRead(t, cache, bucket)[0].TempUnschedulableUntil)

	paused := memoTestAccount(old.ID, "paused")
	paused.Credentials["first_token_monitor_generation"] = 4
	paused.Credentials["scheduler_state_version"] = "9007199254740993"
	until := time.Now().Add(time.Hour)
	paused.TempUnschedulableUntil = &until
	require.NoError(t, cache.SetAccount(ctx, &paused))
	require.NotNil(t, memoRead(t, cache, bucket)[0].TempUnschedulableUntil)

	memoPublish(t, cache, bucket, old)
	got := memoRead(t, cache, bucket)
	require.Len(t, got, 1)
	require.Equal(t, "paused", got[0].Name)
	require.NotNil(t, got[0].TempUnschedulableUntil)

	recovered := memoTestAccount(old.ID, "recovered")
	recovered.Credentials["first_token_monitor_generation"] = 5
	recovered.Credentials["scheduler_state_version"] = "9007199254740994"
	require.NoError(t, cache.SetAccount(ctx, &recovered))
	require.NoError(t, cache.SetAccount(ctx, &paused))
	got = memoRead(t, cache, bucket)
	require.Equal(t, "recovered", got[0].Name)
	require.Nil(t, got[0].TempUnschedulableUntil)
}
