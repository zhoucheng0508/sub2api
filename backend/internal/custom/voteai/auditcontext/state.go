package auditcontext

import (
	"math"
	"sort"
	"strings"
	"time"
)

func Apply(previous State, event AuditEvent, cfg Config) State {
	cfg = NormalizeConfig(cfg)
	policyVersion := strings.TrimSpace(event.PolicyVersion)
	if policyVersion != "" && strings.TrimSpace(previous.PolicyVersion) != policyVersion {
		// Scores, tiers, reasons, review cadence, and prompt-prefix state are all
		// policy-derived. Never merge them across policy versions.
		previous = State{}
	}
	if event.NumericRiskOnly {
		previous = NumericRiskOnlyState(previous)
		event.Categories = nil
		event.Signals = nil
		event.Reason = ""
		event.FullReview = false
	}
	requestID := strings.TrimSpace(event.RequestID)
	if requestID != "" && hasRecentRequestID(previous, requestID) {
		return previous
	}

	now := event.At.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	previousScore := decayedScore(previous, now, cfg.RiskHalfLife)
	score := clampScore(event.RiskScore)
	strong := HasStrongSignal(event.Signals)
	stale := eventPrecedesState(now, previous)

	state := previous
	state.Version = StateVersion
	increment := event.TurnIncrement
	if increment <= 0 {
		increment = 1
	}
	state.TurnCount += increment
	state.MaxScore = math.Max(clampScore(previous.MaxScore), score)
	state.Categories = mergeValues(previous.Categories, event.Categories, 16)
	state.Signals = mergeValues(previous.Signals, event.Signals, 16)
	recentRequestIDs := appendRecentRequestID(previous.RecentRequestIDs, previous.LastRequestID)
	state.RecentRequestIDs = appendRecentRequestID(recentRequestIDs, requestID)
	if policyVersion != "" {
		state.PolicyVersion = policyVersion
	}
	if !stale {
		state.LastRequestID = requestID
		state.CurrentScore = score
		state.Trend = ComputeTrend(previousScore, score, strong, cfg)
		state.Tier = tierForScore(score, cfg)
		state.RecentReasons = appendReason(previous.RecentReasons, event.Reason, cfg)
		state.UpdatedAt = now.Unix()
		state.UpdatedAtUnixNano = now.UnixNano()
		if event.FullReview {
			state.LastFullReviewTurn = state.TurnCount
			state.LastFullReviewAt = now.Unix()
		}
	}
	if event.NumericRiskOnly {
		state = NumericRiskOnlyState(state)
	}
	return state
}

// NumericRiskOnlyState removes state that describes or fingerprints one
// conversation while retaining the actor-level score, tier and trend. This is
// the only state shape that may be reused when no stable session identity is
// available.
func NumericRiskOnlyState(state State) State {
	state.TurnCount = 0
	state.Categories = nil
	state.Signals = nil
	state.RecentReasons = nil
	state.LastFullReviewTurn = 0
	state.LastFullReviewAt = 0
	state.PrefixEpoch = 0
	state.CanonicalPrefixHash = ""
	state.LastPrefixChars = 0
	state.LastPrefixTokens = 0
	state.PrefixContinuity = false
	state.PrefixBaseline = false
	state.PrefixBreakReason = ""
	state.PrefixModel = ""
	state.AuditKeyHash = ""
	state.PrefixUpdatedAtUnixNano = 0
	return state
}

func hasRecentRequestID(state State, requestID string) bool {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return false
	}
	if strings.TrimSpace(state.LastRequestID) == requestID {
		return true
	}
	for _, existing := range state.RecentRequestIDs {
		if strings.TrimSpace(existing) == requestID {
			return true
		}
	}
	return false
}

func appendRecentRequestID(existing []string, requestID string) []string {
	requestID = strings.TrimSpace(requestID)
	out := make([]string, 0, min(RecentRequestIDLimit, len(existing)+1))
	seen := make(map[string]struct{}, len(existing)+1)
	for _, value := range existing {
		value = strings.TrimSpace(value)
		if value == "" || value == requestID {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if requestID != "" {
		out = append(out, requestID)
	}
	if len(out) > RecentRequestIDLimit {
		out = append([]string(nil), out[len(out)-RecentRequestIDLimit:]...)
	}
	return out
}

func eventPrecedesState(at time.Time, state State) bool {
	if at.IsZero() {
		return false
	}
	if state.UpdatedAtUnixNano > 0 {
		return at.UnixNano() < state.UpdatedAtUnixNano
	}
	if state.UpdatedAt > 0 {
		return at.Unix() < state.UpdatedAt
	}
	return false
}

func ComputeTrend(previousScore, currentScore float64, strongSignal bool, cfg Config) string {
	cfg = NormalizeConfig(cfg)
	delta := clampScore(currentScore) - clampScore(previousScore)
	if delta >= cfg.RiskRiseThreshold {
		return TrendRising
	}
	if delta <= -cfg.RiskFallThreshold && !strongSignal {
		return TrendFalling
	}
	return TrendStable
}

func DecayedScore(state State, at time.Time, cfg Config) float64 {
	cfg = NormalizeConfig(cfg)
	return decayedScore(state, at.UTC(), cfg.RiskHalfLife)
}

func decayedScore(state State, at time.Time, halfLife time.Duration) float64 {
	score := clampScore(state.CurrentScore)
	if score == 0 || state.UpdatedAt <= 0 || at.IsZero() || halfLife <= 0 {
		return score
	}
	updated := time.Unix(state.UpdatedAt, 0).UTC()
	if !at.After(updated) {
		return score
	}
	return clampScore(score * math.Pow(0.5, at.Sub(updated).Seconds()/halfLife.Seconds()))
}

func tierForScore(score float64, cfg Config) string {
	if score >= cfg.BlockThreshold {
		return TierHigh
	}
	if score >= cfg.ObserveThreshold {
		return TierObserve
	}
	return TierLow
}

func appendReason(existing []string, reason string, cfg Config) []string {
	out := make([]string, 0, cfg.RecentReasonLimit)
	start := len(existing) - cfg.RecentReasonLimit + 1
	if start < 0 {
		start = 0
	}
	for _, value := range existing[start:] {
		if cleaned := SanitizeReason(value, cfg.ReasonMaxChars); cleaned != "" {
			out = append(out, cleaned)
		}
	}
	if cleaned := SanitizeReason(reason, cfg.ReasonMaxChars); cleaned != "" {
		out = append(out, cleaned)
	}
	if len(out) > cfg.RecentReasonLimit {
		out = out[len(out)-cfg.RecentReasonLimit:]
	}
	return out
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

func clampScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
