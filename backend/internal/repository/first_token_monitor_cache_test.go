package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestFirstTokenMonitorSequenceAndRecovery(t *testing.T) {
	server := miniredis.RunT(t)
	now := time.Now().Truncate(time.Second)
	server.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, ok := NewTempUnschedCache(client).(service.FirstTokenMonitorCache)
	require.True(t, ok)
	cfg := service.FirstTokenMonitorConfig{Enabled: true, ThresholdSeconds: 30, AnomalyCount: 3, CooldownMinutes: 10}
	slow, normal := 30001, 30000
	observe := func(id string, ms *int, count, until int64) {
		n, u, reason, err := store.ObserveFirstToken(context.Background(), 42, id, ms, cfg)
		require.NoError(t, err)
		require.Equal(t, count, n)
		require.Equal(t, until, u)
		if until > 0 {
			require.Contains(t, reason, "30001ms")
		}
	}
	observe("a", &slow, 1, 0)
	observe("a", &slow, 0, 0) // Duplicate cannot advance the streak.
	observe("b", nil, 0, 0)
	observe("c", &slow, 2, 0)   // Missing measurement did not clear it.
	observe("d", &normal, 0, 0) // Exactly 30 seconds is normal.
	// Normal requests do not erase the two anomalies already in this window.
	observe("e", &slow, 3, now.Add(10*time.Minute).Unix())
	observe("f", &slow, 3, now.Add(10*time.Minute).Unix())
	observe("g", &slow, 3, now.Add(10*time.Minute).Unix())
	observe("inflight", &slow, 3, now.Add(10*time.Minute).Unix()) // Cannot extend cooldown.
	server.SetTime(now.Add(10 * time.Minute))
	observe("g", &slow, 0, 0)
	observe("inflight", &slow, 0, 0)
	observe("h", &slow, 1, 0)
	cfg.Enabled = false
	cfg.Generation++
	observe("i", &slow, 0, 0)
	cfg.Enabled = true
	cfg.Generation++
	observe("j", &slow, 1, 0)
	cfg.ThresholdSeconds = 60
	cfg.Generation++
	observe("k", &slow, 0, 0) // Config change resets the old streak.
}

func TestFirstTokenMonitorRollingWindowAndManualRecovery(t *testing.T) {
	server := miniredis.RunT(t)
	now := time.Now().Truncate(time.Second)
	server.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewTempUnschedCache(client)
	store, ok := cache.(service.FirstTokenMonitorCache)
	require.True(t, ok)
	cfg := service.FirstTokenMonitorConfig{Enabled: true, ThresholdSeconds: 30, AnomalyCount: 3, CooldownMinutes: 10}
	ms := 31000
	observe := func(id string, expected int64, tripped bool) {
		count, until, _, err := store.ObserveFirstToken(context.Background(), 42, id, &ms, cfg)
		require.NoError(t, err)
		require.Equal(t, expected, count)
		require.Equal(t, tripped, until > 0)
	}
	observe("a", 1, false)
	server.SetTime(now.Add(5 * time.Minute))
	observe("b", 2, false)
	server.SetTime(now.Add(10 * time.Minute))
	observe("c", 2, false) // a has left the rolling window; this is not a fixed bucket.
	server.SetTime(now.Add(11 * time.Minute))
	observe("d", 3, true)
	require.NoError(t, cache.DeleteTempUnsched(context.Background(), 42))
	observe("after-manual-recovery", 1, false) // Old cooldown and samples are gone.
}

func TestFirstTokenMonitorConcurrentDeduplication(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, ok := NewTempUnschedCache(client).(service.FirstTokenMonitorCache)
	require.True(t, ok)
	cfg := service.FirstTokenMonitorConfig{Enabled: true, ThresholdSeconds: 1, AnomalyCount: 100, CooldownMinutes: 1}
	ms := 2000
	var wg sync.WaitGroup
	errors := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _, err := store.ObserveFirstToken(context.Background(), 42, "same-request", &ms, cfg)
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	count, until, _, err := store.ObserveFirstToken(context.Background(), 42, "next", &ms, cfg)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	require.Zero(t, until)
	count, _, _, err = store.ObserveFirstToken(context.Background(), 43, "same-request", &ms, cfg)
	require.NoError(t, err)
	require.EqualValues(t, 1, count) // Accounts are independent.
}
