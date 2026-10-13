package repository

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestFirstTokenMonitorRealRedisTwoClients(t *testing.T) {
	addr := os.Getenv("FIRST_TOKEN_MONITOR_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set FIRST_TOKEN_MONITOR_TEST_REDIS_ADDR for the local Redis test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	opts := &redis.Options{Addr: addr, Password: os.Getenv("FIRST_TOKEN_MONITOR_TEST_REDIS_PASSWORD")}
	clients := []*redis.Client{redis.NewClient(opts), redis.NewClient(opts)}
	defer func() { _ = clients[0].Close() }()
	defer func() { _ = clients[1].Close() }()
	id := time.Now().UnixNano() / 1000
	key := fmt.Sprintf("first_token_monitor:{%d}", id)
	keys := []string{key, key + ":samples", fmt.Sprintf("sched:acc:%d", id), fmt.Sprintf("sched:meta:%d", id), fmt.Sprintf("sched:monitor_generation:%d", id), fmt.Sprintf("sched:state_version:%d", id)}
	for i := 0; i < 10; i++ {
		keys = append(keys, fmt.Sprintf("%s:request:parallel-%d", key, i))
	}
	for _, request := range []string{"duplicate", "new-a", "stale", "new-b", "trigger", "recovered"} {
		keys = append(keys, key+":request:"+request)
	}
	defer func() { require.NoError(t, clients[0].Del(context.Background(), keys...).Err()) }()
	first, ok := NewTempUnschedCache(clients[0]).(service.FirstTokenMonitorCache)
	require.True(t, ok)
	second, ok := NewTempUnschedCache(clients[1]).(service.FirstTokenMonitorCache)
	require.True(t, ok)
	stores := []service.FirstTokenMonitorCache{first, second}
	cfg := service.FirstTokenMonitorConfig{Enabled: true, ThresholdSeconds: 30, AnomalyCount: 100, CooldownMinutes: 10, Generation: 1}
	ms := 31000
	errors := make(chan error, 10)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, _, err := stores[i%2].ObserveFirstToken(ctx, id, fmt.Sprintf("parallel-%d", i), &ms, cfg)
			errors <- err
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	count, _, _, err := stores[0].ObserveFirstToken(ctx, id, "duplicate", &ms, cfg)
	require.NoError(t, err)
	require.EqualValues(t, 11, count)
	count, _, _, err = stores[1].ObserveFirstToken(ctx, id, "duplicate", &ms, cfg)
	require.NoError(t, err)
	require.Zero(t, count)
	old := cfg
	cfg.Generation = 2
	cfg.AnomalyCount = 3
	count, _, _, err = stores[0].ObserveFirstToken(ctx, id, "new-a", &ms, cfg)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	count, until, _, err := stores[1].ObserveFirstToken(ctx, id, "stale", &ms, old)
	require.NoError(t, err)
	require.Zero(t, count)
	require.Zero(t, until)
	count, _, _, err = stores[1].ObserveFirstToken(ctx, id, "new-b", &ms, cfg)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	count, until, _, err = stores[0].ObserveFirstToken(ctx, id, "trigger", &ms, cfg)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)
	require.Greater(t, until, time.Now().Unix())
	require.NoError(t, NewTempUnschedCache(clients[1]).DeleteTempUnsched(ctx, id))
	cfg.Generation = 3
	count, until, _, err = stores[0].ObserveFirstToken(ctx, id, "recovered", &ms, cfg)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.Zero(t, until)
	// Cross-client delayed snapshot publication after a recovery barrier.
	deadline := time.Now().Add(time.Hour)
	stale := service.Account{ID: id, Credentials: map[string]any{"first_token_monitor_generation": 2}, TempUnschedulableUntil: &deadline}
	recovered := stale
	recovered.Credentials = map[string]any{"first_token_monitor_generation": 3}
	recovered.TempUnschedulableUntil = nil
	require.NoError(t, NewSchedulerCache(clients[1]).SetAccount(ctx, &recovered))
	require.NoError(t, NewSchedulerCache(clients[0]).SetAccount(ctx, &stale))
	snapshot, err := NewSchedulerCache(clients[1]).GetAccount(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Nil(t, snapshot.TempUnschedulableUntil)
	// Same monitor generation: a delayed pre-pause row must not undo a pause.
	beforePause := recovered
	beforePause.Credentials = map[string]any{"first_token_monitor_generation": 3, "scheduler_state_version": "10"}
	paused := beforePause
	paused.Credentials = map[string]any{"first_token_monitor_generation": 3, "scheduler_state_version": "11"}
	paused.TempUnschedulableUntil = &deadline
	require.NoError(t, NewSchedulerCache(clients[1]).SetAccount(ctx, &paused))
	require.NoError(t, NewSchedulerCache(clients[0]).SetAccount(ctx, &beforePause))
	snapshot, err = NewSchedulerCache(clients[1]).GetAccount(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, snapshot.TempUnschedulableUntil)
	require.Equal(t, deadline.Unix(), snapshot.TempUnschedulableUntil.Unix())
}
