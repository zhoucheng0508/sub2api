package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// FirstTokenMonitorConfig is explicitly opted into by each account, across all groups.
type FirstTokenMonitorConfig struct {
	Enabled          bool
	ThresholdSeconds int
	AnomalyCount     int
	CooldownMinutes  int
	Revision         string
	Generation       int64
}

func firstTokenMonitorConfig(credentials map[string]any) FirstTokenMonitorConfig {
	c := FirstTokenMonitorConfig{ThresholdSeconds: 30, AnomalyCount: 3, CooldownMinutes: 10}
	c.Enabled, _ = credentials["first_token_monitor_enabled"].(bool)
	for key, dst := range map[string]*int{
		"first_token_monitor_threshold_seconds": &c.ThresholdSeconds,
		"first_token_monitor_anomaly_count":     &c.AnomalyCount,
		"first_token_monitor_cooldown_minutes":  &c.CooldownMinutes,
	} {
		if v, ok := monitorNumber(credentials[key]); ok && v > 0 {
			*dst = v
		}
	}
	c.Revision, _ = credentials["first_token_monitor_revision"].(string)
	switch value := credentials["first_token_monitor_generation"].(type) {
	case float64:
		c.Generation = int64(value)
	case int64:
		c.Generation = value
	case int:
		c.Generation = int64(value)
	}
	return c
}

func monitorNumber(raw any) (int, bool) {
	var n float64
	switch v := raw.(type) {
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	case float64:
		n = v
	default:
		return 0, false
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) || n < 1 || n > 86400 {
		return 0, false
	}
	return int(n), true
}

func ValidateFirstTokenMonitorCredentials(credentials map[string]any) error {
	if raw, exists := credentials["first_token_monitor_enabled"]; exists {
		if _, ok := raw.(bool); !ok {
			return infraerrors.BadRequest("FIRST_TOKEN_MONITOR_INVALID", "first_token_monitor_enabled must be a boolean")
		}
	}
	for key, max := range map[string]int{"first_token_monitor_threshold_seconds": 3600, "first_token_monitor_anomaly_count": 100, "first_token_monitor_cooldown_minutes": 1440} {
		if raw, exists := credentials[key]; exists {
			if n, ok := monitorNumber(raw); !ok || n > max {
				return infraerrors.BadRequest("FIRST_TOKEN_MONITOR_INVALID", fmt.Sprintf("%s must be an integer between 1 and %d", key, max))
			}
		}
	}
	return nil
}

// Optional extension of TempUnschedCache; production Redis implements atomic counting
// and request deduplication shared by every gateway instance.
type FirstTokenMonitorCache interface {
	ObserveFirstToken(context.Context, int64, string, *int, FirstTokenMonitorConfig) (count int64, until int64, reason string, err error)
}

type FirstTokenMonitorGuard interface {
	WithFirstTokenMonitorGuard(context.Context, int64, func(AccountRepository) error) error
	SetFirstTokenMonitorPause(context.Context, int64, int64, time.Time, string) (bool, error)
}

func withFirstTokenMonitorGuard(ctx context.Context, repo AccountRepository, id int64, fn func(AccountRepository) error) error {
	if guard, ok := repo.(FirstTokenMonitorGuard); ok {
		return guard.WithFirstTokenMonitorGuard(ctx, id, fn)
	}
	return fn(repo) // Legacy test doubles; production repository implements the guard.
}

type AccountMonitoringResetter interface {
	ResetAccountMonitoring(context.Context, int64) error
}

func (s *RateLimitService) monitoringScope(repo AccountRepository) *RateLimitService {
	return &RateLimitService{accountRepo: repo, tempUnschedCache: s.tempUnschedCache, runtimeBlocker: s.runtimeBlocker, openAI403CounterCache: s.openAI403CounterCache}
}

func (s *RateLimitService) MonitorFirstToken(ctx context.Context, account *Account, usage *UsageLog) {
	if s == nil || s.accountRepo == nil || account == nil || usage == nil {
		return
	}
	// Existing accounts without this feature never add work to the request path.
	if !firstTokenMonitorConfig(account.Credentials).Enabled {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := withFirstTokenMonitorGuard(ctx, s.accountRepo, account.ID, func(repo AccountRepository) error {
		copy := s.monitoringScope(repo)
		copy.monitorFirstTokenLocked(ctx, account, usage)
		return nil
	}); err != nil {
		slog.Warn("first_token_monitor_guard_failed", "account_id", account.ID, "error", err)
	}
}

func (s *RateLimitService) monitorFirstTokenLocked(ctx context.Context, account *Account, usage *UsageLog) {
	cache, ok := s.tempUnschedCache.(FirstTokenMonitorCache)
	guard, guarded := s.accountRepo.(FirstTokenMonitorGuard)
	if !ok || !guarded {
		return
	}
	// Re-read current settings: disabling monitoring must also affect in-flight requests.
	current, err := s.accountRepo.GetByID(ctx, account.ID)
	if err != nil {
		slog.Warn("first_token_monitor_settings_failed", "account_id", account.ID, "error", err)
		return
	}
	cfg := firstTokenMonitorConfig(current.Credentials)
	if cfg.Enabled && usage.FirstTokenMs != nil && *usage.FirstTokenMs > cfg.ThresholdSeconds*1000 {
		slog.Warn("first_token_monitor_anomaly", "account_id", account.ID, "model", usage.Model, "first_token_ms", *usage.FirstTokenMs, "threshold_seconds", cfg.ThresholdSeconds, "request_id", usage.RequestID)
	}
	// In-flight completions cannot start another streak while any global pause
	// is active, including when the operator changes monitor settings mid-pause.
	if current.TempUnschedulableUntil != nil && current.TempUnschedulableUntil.After(time.Now()) {
		return
	}
	_, untilUnix, reason, err := cache.ObserveFirstToken(ctx, account.ID, usage.RequestID, usage.FirstTokenMs, cfg)
	if err != nil {
		slog.Warn("first_token_monitor_counter_failed", "account_id", account.ID, "error", err)
		return
	}
	if untilUnix <= time.Now().Unix() {
		return
	}
	until := time.Unix(untilUnix, 0)
	applied, err := guard.SetFirstTokenMonitorPause(ctx, account.ID, cfg.Generation, until, reason)
	if err != nil {
		slog.Warn("first_token_monitor_pause_failed", "account_id", account.ID, "error", err)
		return
	}
	if !applied {
		return
	}
	// No post-commit external publication: a lost session may have allowed
	// recovery to finish already. The transactional outbox propagates current
	// database state instead of publishing this potentially stale observation.
	slog.Warn("first_token_monitor_paused", "account_id", account.ID, "until", until, "reason", reason)
}
