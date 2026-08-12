package auditcontext

import (
	"testing"
	"time"
)

func TestApplyNumericRiskOnlyDropsConversationDerivedState(t *testing.T) {
	t.Parallel()
	now := time.Unix(9000, 0).UTC()
	previous := State{
		Version:             StateVersion,
		TurnCount:           7,
		CurrentScore:        0.60,
		MaxScore:            0.85,
		Trend:               TrendRising,
		Tier:                TierObserve,
		Categories:          []string{"cyber_abuse"},
		Signals:             []string{"auth_bypass"},
		RecentReasons:       []string{"conversation-one-canary"},
		LastFullReviewTurn:  6,
		LastFullReviewAt:    now.Add(-time.Minute).Unix(),
		LastRequestID:       "req-previous",
		RecentRequestIDs:    []string{"req-previous"},
		PolicyVersion:       "p2",
		UpdatedAt:           now.Add(-time.Minute).Unix(),
		PrefixEpoch:         2,
		CanonicalPrefixHash: "conversation-prefix",
		PrefixModel:         "audit-model",
		AuditKeyHash:        "audit-key-hash",
	}

	state := Apply(previous, AuditEvent{
		RiskScore:       0.40,
		Categories:      []string{"fraud"},
		Signals:         []string{"credential_access"},
		Reason:          "conversation-two-canary",
		RequestID:       "req-current",
		PolicyVersion:   "p2",
		FullReview:      true,
		At:              now,
		NumericRiskOnly: true,
	}, DefaultConfig())

	if state.CurrentScore != 0.40 || state.MaxScore != 0.85 || state.Tier != TierObserve {
		t.Fatalf("numeric actor risk was not retained: %#v", state)
	}
	if state.TurnCount != 0 || len(state.Categories) != 0 || len(state.Signals) != 0 || len(state.RecentReasons) != 0 {
		t.Fatalf("conversation-derived state survived numeric-only update: %#v", state)
	}
	if state.LastFullReviewTurn != 0 || state.LastFullReviewAt != 0 || state.CanonicalPrefixHash != "" || state.PrefixModel != "" || state.AuditKeyHash != "" {
		t.Fatalf("session metadata survived numeric-only update: %#v", state)
	}
	if state.LastRequestID != "req-current" || len(state.RecentRequestIDs) != 2 {
		t.Fatalf("actor idempotency metadata was not retained: %#v", state)
	}
}
