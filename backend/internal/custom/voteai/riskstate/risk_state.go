package riskstate

import (
	"math"
	"sort"
	"strings"
	"time"
)

const (
	TierLow     = "low"
	TierObserve = "observe"
	TierHigh    = "high"
)

type Config struct {
	ObserveThreshold  float64
	BlockThreshold    float64
	TTL               time.Duration
	HalfLife          time.Duration
	BlockCooldown     time.Duration
	ModerateIncrement float64
	ElevatedIncrement float64
}

type Event struct {
	Score       float64
	Categories  []string
	Signals     []string
	RequestID   string
	SessionHash string
	At          time.Time
}

type State struct {
	Version                int      `json:"version"`
	Score                  float64  `json:"score"`
	Tier                   string   `json:"tier"`
	Strikes                int      `json:"strikes"`
	Signals                []string `json:"signals,omitempty"`
	Categories             []string `json:"categories,omitempty"`
	FirstSeenUnix          int64    `json:"first_seen_unix"`
	LastSeenUnix           int64    `json:"last_seen_unix"`
	BlockedUntilUnix       int64    `json:"blocked_until_unix,omitempty"`
	LastRequestID          string   `json:"last_request_id,omitempty"`
	LastSessionHash        string   `json:"last_session_hash,omitempty"`
	SuspiciousSessionCount int      `json:"suspicious_session_count,omitempty"`
}

var accumulatingCategories = map[string]struct{}{
	"cyber_abuse": {}, "credential_theft": {}, "malware": {}, "phishing": {},
	"fraud": {}, "policy_evasion": {}, "illicit": {},
}

var accumulatingSignals = map[string]struct{}{
	"ownership_unverified": {}, "credential_access": {}, "auth_bypass": {},
	"secret_extraction": {}, "malware_delivery": {}, "policy_evasion": {},
	"progressive_escalation": {},
}

func DefaultConfig() Config {
	return Config{
		ObserveThreshold:  0.35,
		BlockThreshold:    0.70,
		TTL:               2 * time.Hour,
		HalfLife:          30 * time.Minute,
		BlockCooldown:     30 * time.Minute,
		ModerateIncrement: 0.10,
		ElevatedIncrement: 0.20,
	}
}

func NormalizeConfig(cfg Config) Config {
	defaults := DefaultConfig()
	if cfg.ObserveThreshold <= 0 || cfg.ObserveThreshold >= 1 {
		cfg.ObserveThreshold = defaults.ObserveThreshold
	}
	if cfg.BlockThreshold <= cfg.ObserveThreshold || cfg.BlockThreshold > 1 {
		cfg.BlockThreshold = defaults.BlockThreshold
	}
	if cfg.TTL <= 0 {
		cfg.TTL = defaults.TTL
	}
	if cfg.HalfLife <= 0 {
		cfg.HalfLife = defaults.HalfLife
	}
	if cfg.BlockCooldown < 0 {
		cfg.BlockCooldown = 0
	}
	if cfg.ModerateIncrement <= 0 {
		cfg.ModerateIncrement = defaults.ModerateIncrement
	}
	if cfg.ElevatedIncrement <= cfg.ModerateIncrement {
		cfg.ElevatedIncrement = defaults.ElevatedIncrement
	}
	return cfg
}

func Apply(previous State, event Event, cfg Config) State {
	cfg = NormalizeConfig(cfg)
	now := event.At.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if event.RequestID != "" && event.RequestID == previous.LastRequestID {
		return previous
	}

	state := previous
	state.Version = 1
	if state.FirstSeenUnix == 0 {
		state.FirstSeenUnix = now.Unix()
	}
	state.Score = Decay(state.Score, time.Unix(state.LastSeenUnix, 0), now, cfg.HalfLife)
	event.Score = clamp(event.Score)
	risky := event.Score >= cfg.ObserveThreshold && HasAccumulatingSignal(event.Categories, event.Signals)
	if risky {
		increment := cfg.ModerateIncrement
		midpoint := cfg.ObserveThreshold + (cfg.BlockThreshold-cfg.ObserveThreshold)/2
		if event.Score >= midpoint {
			increment = cfg.ElevatedIncrement
		}
		state.Score = math.Max(event.Score, clamp(state.Score+increment))
		state.Strikes++
		state.Categories = mergeValues(state.Categories, event.Categories, 12)
		state.Signals = mergeValues(state.Signals, event.Signals, 12)
		if event.SessionHash != "" && event.SessionHash != state.LastSessionHash {
			state.SuspiciousSessionCount++
			state.LastSessionHash = event.SessionHash
		}
	} else {
		state.Score = math.Max(event.Score, state.Score)
	}

	state.Tier = TierForScore(state.Score, cfg.ObserveThreshold, cfg.BlockThreshold)
	if state.Tier == TierHigh && cfg.BlockCooldown > 0 {
		blockedUntil := now.Add(cfg.BlockCooldown).Unix()
		if blockedUntil > state.BlockedUntilUnix {
			state.BlockedUntilUnix = blockedUntil
		}
	}
	state.LastSeenUnix = now.Unix()
	state.LastRequestID = strings.TrimSpace(event.RequestID)
	return state
}

func TierForScore(score, observeThreshold, blockThreshold float64) string {
	if score >= blockThreshold {
		return TierHigh
	}
	if score >= observeThreshold {
		return TierObserve
	}
	return TierLow
}

func IsBlocked(state State, now time.Time) bool {
	return state.Tier == TierHigh && state.BlockedUntilUnix > now.UTC().Unix()
}

func Decay(score float64, lastSeen, now time.Time, halfLife time.Duration) float64 {
	if score <= 0 || lastSeen.IsZero() || !now.After(lastSeen) || halfLife <= 0 {
		return clamp(score)
	}
	factor := math.Pow(0.5, now.Sub(lastSeen).Seconds()/halfLife.Seconds())
	return clamp(score * factor)
}

func HasAccumulatingSignal(categories, signals []string) bool {
	for _, category := range categories {
		if _, ok := accumulatingCategories[strings.ToLower(strings.TrimSpace(category))]; ok {
			return true
		}
	}
	for _, signal := range signals {
		if _, ok := accumulatingSignals[strings.ToLower(strings.TrimSpace(signal))]; ok {
			return true
		}
	}
	return false
}

func ActorBonus(state State) float64 {
	if state.SuspiciousSessionCount < 2 {
		return 0
	}
	bonus := float64(state.SuspiciousSessionCount-1)*0.03 + math.Max(0, state.Score-0.35)*0.10
	return math.Min(0.15, bonus)
}

func mergeValues(existing, incoming []string, limit int) []string {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	for _, values := range [][]string{existing, incoming} {
		for _, value := range values {
			value = strings.ToLower(strings.TrimSpace(value))
			if value != "" {
				seen[value] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
