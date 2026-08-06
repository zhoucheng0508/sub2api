package auditcontext

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestApplyComputesTrendTierAndDeduplicatesRequest(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0).UTC()
	previous := State{CurrentScore: 0.10, MaxScore: 0.20, TurnCount: 3, PolicyVersion: "p2", UpdatedAt: now.Add(-time.Minute).Unix()}
	event := AuditEvent{
		RiskScore: 0.55, Categories: []string{"Cyber_Abuse"}, Signals: []string{"auth_bypass"},
		Reason: "risk increased", RequestID: "req-1", PolicyVersion: "p2", At: now,
	}
	state := Apply(previous, event, DefaultConfig())
	if state.TurnCount != 4 || state.Trend != TrendRising || state.Tier != TierObserve {
		t.Fatalf("state=%#v", state)
	}
	if state.CurrentScore != 0.55 || state.MaxScore != 0.55 || state.PolicyVersion != "p2" {
		t.Fatalf("state=%#v", state)
	}
	if !reflect.DeepEqual(state.RecentRequestIDs, []string{"req-1"}) {
		t.Fatalf("recent request IDs=%v", state.RecentRequestIDs)
	}
	if got := Apply(state, event, DefaultConfig()); got.TurnCount != state.TurnCount {
		t.Fatalf("duplicate request advanced state: before=%#v after=%#v", state, got)
	}
}

func TestApplyPolicyChangeResetsAccumulatedStateBeforeApplyingEvent(t *testing.T) {
	t.Parallel()
	now := time.Unix(1500, 0).UTC()
	previous := State{
		Version: StateVersion, PolicyVersion: "p1", TurnCount: 12,
		CurrentScore: 0.91, MaxScore: 0.99, Tier: TierHigh,
		Categories: []string{"cyber_abuse"}, Signals: []string{"auth_bypass"},
		RecentReasons: []string{"old policy reason"}, LastFullReviewTurn: 10,
		RecentRequestIDs: []string{"old-1", "old-2"},
		PrefixEpoch:      4, CanonicalPrefixHash: "old-prefix", UpdatedAt: now.Add(-time.Minute).Unix(),
	}

	state := Apply(previous, AuditEvent{
		RiskScore: 0.10, Reason: "new policy request", RequestID: "req-new-policy",
		PolicyVersion: "p2", At: now,
	}, DefaultConfig())

	if state.PolicyVersion != "p2" || state.TurnCount != 1 || state.CurrentScore != 0.10 || state.MaxScore != 0.10 {
		t.Fatalf("state=%#v", state)
	}
	if len(state.Categories) != 0 || len(state.Signals) != 0 || state.LastFullReviewTurn != 0 || state.CanonicalPrefixHash != "" {
		t.Fatalf("stale policy state survived reset: %#v", state)
	}
	if !reflect.DeepEqual(state.RecentRequestIDs, []string{"req-new-policy"}) {
		t.Fatalf("old policy request IDs survived reset: %v", state.RecentRequestIDs)
	}
}

func TestApplyInOrderEventsAdvanceCurrentState(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	firstAt := time.Unix(5000, 100).UTC()
	secondAt := firstAt.Add(time.Nanosecond)
	state := Apply(State{}, AuditEvent{
		RiskScore: 0.65, Categories: []string{"cyber_abuse"}, Signals: []string{"auth_bypass"},
		Reason: "first", RequestID: "req-first", PolicyVersion: "p2", At: firstAt,
	}, cfg)
	state = Apply(state, AuditEvent{
		RiskScore: 0.20, Categories: []string{"fraud"}, Signals: []string{"defensive_context"},
		Reason: "second", RequestID: "req-second", PolicyVersion: "p2", At: secondAt,
	}, cfg)

	if state.TurnCount != 2 || state.CurrentScore != 0.20 || state.MaxScore != 0.65 {
		t.Fatalf("state=%#v", state)
	}
	if state.UpdatedAtUnixNano != secondAt.UnixNano() || state.LastRequestID != "req-second" {
		t.Fatalf("latest event not retained: %#v", state)
	}
	if !reflect.DeepEqual(state.Categories, []string{"cyber_abuse", "fraud"}) ||
		!reflect.DeepEqual(state.Signals, []string{"auth_bypass", "defensive_context"}) {
		t.Fatalf("evidence not merged: %#v", state)
	}
	if !reflect.DeepEqual(state.RecentRequestIDs, []string{"req-first", "req-second"}) {
		t.Fatalf("recent request IDs=%v", state.RecentRequestIDs)
	}
}

func TestApplyOutOfOrderEventMergesEvidenceWithoutRegressingCurrentState(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	newerAt := time.Unix(6000, 900).UTC()
	olderAt := newerAt.Add(-time.Nanosecond)
	state := Apply(State{}, AuditEvent{
		RiskScore: 0.25, Categories: []string{"fraud"}, Signals: []string{"defensive_context"},
		Reason: "newer", RequestID: "req-newer", PolicyVersion: "p2", FullReview: true, At: newerAt,
	}, cfg)
	wantTrend, wantTier := state.Trend, state.Tier
	wantUpdatedAt, wantUpdatedAtNanos := state.UpdatedAt, state.UpdatedAtUnixNano
	wantFullReviewTurn, wantFullReviewAt := state.LastFullReviewTurn, state.LastFullReviewAt

	state = Apply(state, AuditEvent{
		RiskScore: 0.95, Categories: []string{"cyber_abuse"}, Signals: []string{"auth_bypass"},
		Reason: "older must not become recent", RequestID: "req-older", PolicyVersion: "p2", FullReview: true, At: olderAt,
	}, cfg)

	if state.TurnCount != 2 || state.CurrentScore != 0.25 || state.MaxScore != 0.95 {
		t.Fatalf("state=%#v", state)
	}
	if state.Trend != wantTrend || state.Tier != wantTier || state.UpdatedAt != wantUpdatedAt || state.UpdatedAtUnixNano != wantUpdatedAtNanos {
		t.Fatalf("stale event regressed current state: %#v", state)
	}
	if state.LastRequestID != "req-newer" {
		t.Fatalf("stale event replaced latest request marker: %#v", state)
	}
	if state.LastFullReviewTurn != wantFullReviewTurn || state.LastFullReviewAt != wantFullReviewAt {
		t.Fatalf("stale full review changed cadence: %#v", state)
	}
	if !reflect.DeepEqual(state.Categories, []string{"cyber_abuse", "fraud"}) ||
		!reflect.DeepEqual(state.Signals, []string{"auth_bypass", "defensive_context"}) {
		t.Fatalf("stale evidence was not safely merged: %#v", state)
	}
	if strings.Contains(strings.Join(state.RecentReasons, " "), "older") {
		t.Fatalf("stale reason displaced current reasons: %v", state.RecentReasons)
	}
}

func TestApplyReplayedRecentRequestIDIsFullyIdempotent(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	base := time.Unix(7000, 0).UTC()
	first := AuditEvent{RiskScore: 0.30, RequestID: "req-first", PolicyVersion: "p2", At: base}
	state := Apply(State{}, first, cfg)
	state = Apply(state, AuditEvent{RiskScore: 0.40, RequestID: "req-second", PolicyVersion: "p2", At: base.Add(time.Second)}, cfg)

	replayed := first
	replayed.RiskScore = 0.99
	replayed.Categories = []string{"must_not_merge"}
	got := Apply(state, replayed, cfg)
	if !reflect.DeepEqual(got, state) {
		t.Fatalf("replayed request mutated state:\nbefore=%#v\nafter=%#v", state, got)
	}
}

func TestApplyRecentRequestIDsAreBounded(t *testing.T) {
	t.Parallel()
	state := State{}
	base := time.Unix(8000, 0).UTC()
	for i := 0; i < RecentRequestIDLimit+5; i++ {
		state = Apply(state, AuditEvent{
			RequestID: "req-" + string(rune('A'+i)), PolicyVersion: "p2", At: base.Add(time.Duration(i) * time.Second),
		}, DefaultConfig())
	}
	if len(state.RecentRequestIDs) != RecentRequestIDLimit {
		t.Fatalf("recent request IDs len=%d want=%d", len(state.RecentRequestIDs), RecentRequestIDLimit)
	}
	if state.RecentRequestIDs[len(state.RecentRequestIDs)-1] != state.LastRequestID {
		t.Fatalf("last request not retained: %#v", state)
	}
}

func TestApplyTracksFallingRiskWithoutStrongSignal(t *testing.T) {
	t.Parallel()
	now := time.Unix(2000, 0).UTC()
	previous := State{CurrentScore: 0.70, MaxScore: 0.70, UpdatedAt: now.Unix()}
	state := Apply(previous, AuditEvent{RiskScore: 0.30, Signals: []string{"defensive_context"}, At: now}, DefaultConfig())
	if state.Trend != TrendFalling || state.Tier != TierLow {
		t.Fatalf("state=%#v", state)
	}
}

func TestApplyStoresOnlyLimitedRedactedReasons(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.ReasonMaxChars = 60
	cfg.RecentReasonLimit = 3
	state := State{}
	for i, reason := range []string{
		"first", "second", "password=super-secret-password", "Bearer abcdefghijklmnopqrstuvwxyz", "last " + strings.Repeat("x", 100),
	} {
		state = Apply(state, AuditEvent{Reason: reason, RequestID: string(rune('a' + i)), At: time.Unix(int64(3000+i), 0)}, cfg)
	}
	if len(state.RecentReasons) != 3 {
		t.Fatalf("reasons=%v", state.RecentReasons)
	}
	joined := strings.Join(state.RecentReasons, " ")
	if strings.Contains(joined, "super-secret") || strings.Contains(joined, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("secret persisted in reasons: %q", joined)
	}
	for _, reason := range state.RecentReasons {
		if runeLen(reason) > cfg.ReasonMaxChars {
			t.Fatalf("reason exceeds cap: %q", reason)
		}
	}
}

func TestDecayedScore(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.RiskHalfLife = 10 * time.Minute
	now := time.Unix(4000, 0).UTC()
	state := State{CurrentScore: 0.8, UpdatedAt: now.Add(-10 * time.Minute).Unix()}
	got := DecayedScore(state, now, cfg)
	if got < 0.399 || got > 0.401 {
		t.Fatalf("decayed score=%f", got)
	}
}
