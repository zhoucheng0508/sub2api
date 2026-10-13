package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestFirstTokenMonitorRejectsStaleConfigurationGeneration(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, ok := NewTempUnschedCache(client).(service.FirstTokenMonitorCache)
	require.True(t, ok)
	ctx := context.Background()
	ms := 31000
	cfg := service.FirstTokenMonitorConfig{Enabled: true, ThresholdSeconds: 30, AnomalyCount: 3, CooldownMinutes: 10, Generation: 2}
	count, _, _, err := store.ObserveFirstToken(ctx, 42, "new-a", &ms, cfg)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	old := cfg
	old.Generation = 1
	old.ThresholdSeconds = 1
	count, until, _, err := store.ObserveFirstToken(ctx, 42, "old", &ms, old)
	require.NoError(t, err)
	require.Zero(t, count)
	require.Zero(t, until)
	count, _, _, err = store.ObserveFirstToken(ctx, 42, "new-b", &ms, cfg)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	cfg.Generation = 3
	cfg.Enabled = false
	_, _, _, err = store.ObserveFirstToken(ctx, 42, "disabled", &ms, cfg)
	require.NoError(t, err)
	old.Generation = 2
	count, until, _, err = store.ObserveFirstToken(ctx, 42, "old-after-disable", &ms, old)
	require.NoError(t, err)
	require.Zero(t, count)
	require.Zero(t, until)
	cfg.Generation = 4
	cfg.Enabled = true
	count, _, _, err = store.ObserveFirstToken(ctx, 42, "reenabled", &ms, cfg)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}
