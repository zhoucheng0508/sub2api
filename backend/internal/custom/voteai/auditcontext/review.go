package auditcontext

import (
	"strings"
	"time"
)

const (
	ReviewReasonForced              = "forced"
	ReviewReasonScoreThreshold      = "score_threshold"
	ReviewReasonStrongSignal        = "strong_signal"
	ReviewReasonRiskRise            = "risk_rise"
	ReviewReasonCumulativeRisk      = "cumulative_risk"
	ReviewReasonProgressiveLanguage = "progressive_language"
	ReviewReasonPeriodic            = "periodic"
	ReviewReasonTruncatedRisk       = "truncated_risk"
)

var strongSignals = map[string]struct{}{
	"auth_bypass":            {},
	"secret_extraction":      {},
	"malware_delivery":       {},
	"policy_evasion":         {},
	"progressive_escalation": {},
}

var weakSignals = map[string]struct{}{
	"defensive_context":    {},
	"ownership_unverified": {},
	"credential_access":    {},
}

func DecideFullReview(state State, input ReviewInput, cfg Config) ReviewDecision {
	cfg = NormalizeConfig(cfg)
	onlyWeakSignals := hasOnlyWeakSignals(input.Categories, input.Signals)
	reasons := make([]string, 0, 8)
	add := func(reason string) {
		for _, existing := range reasons {
			if existing == reason {
				return
			}
		}
		reasons = append(reasons, reason)
	}

	if input.Force {
		add(ReviewReasonForced)
	}
	if !onlyWeakSignals && clampScore(input.FastScore) >= cfg.FullReviewThreshold {
		add(ReviewReasonScoreThreshold)
	}
	if HasStrongSignal(input.Signals) {
		add(ReviewReasonStrongSignal)
	}

	now := input.At.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	previousScore := decayedScore(state, now, cfg.RiskHalfLife)
	if state.UpdatedAt > 0 && !onlyWeakSignals && clampScore(input.FastScore)-previousScore >= cfg.RiskRiseThreshold {
		add(ReviewReasonRiskRise)
	}
	if previousScore >= cfg.CumulativeReviewThreshold {
		add(ReviewReasonCumulativeRisk)
	}
	if NeedsPreviousContext(input.LatestUserText) && previousScore >= cfg.HistoryRiskThreshold {
		add(ReviewReasonProgressiveLanguage)
	}

	nextTurn := state.TurnCount + 1
	periodicEligible := input.StableSession || input.FullHistoryAvailable
	turnsSinceReview := nextTurn - state.LastFullReviewTurn
	if periodicEligible && cfg.PeriodicFullReviewTurns > 0 && turnsSinceReview >= cfg.PeriodicFullReviewTurns {
		add(ReviewReasonPeriodic)
	}
	if input.InputTruncated && hasNonDefensiveRisk(input.Categories, input.Signals) {
		add(ReviewReasonTruncatedRisk)
	}
	return ReviewDecision{Required: len(reasons) > 0, Reasons: reasons}
}

func hasOnlyWeakSignals(categories, signals []string) bool {
	if len(categories) > 0 || len(signals) == 0 {
		return false
	}
	for _, signal := range signals {
		if _, weak := weakSignals[strings.ToLower(strings.TrimSpace(signal))]; !weak {
			return false
		}
	}
	return true
}

func HasStrongSignal(signals []string) bool {
	for _, signal := range signals {
		if _, ok := strongSignals[strings.ToLower(strings.TrimSpace(signal))]; ok {
			return true
		}
	}
	return false
}

func hasNonDefensiveRisk(categories, signals []string) bool {
	for _, category := range categories {
		if strings.TrimSpace(category) != "" {
			return true
		}
	}
	for _, signal := range signals {
		signal = strings.ToLower(strings.TrimSpace(signal))
		if signal == "" {
			continue
		}
		if _, weak := weakSignals[signal]; !weak {
			return true
		}
	}
	return false
}
