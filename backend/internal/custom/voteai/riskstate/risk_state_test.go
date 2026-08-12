package riskstate

import (
	"math"
	"testing"
	"time"
)

func TestApplyAccumulatesSuspiciousConversation(t *testing.T) {
	cfg := DefaultConfig()
	now := time.Unix(1700000000, 0).UTC()
	state := State{}
	for i := 0; i < 4; i++ {
		state = Apply(state, Event{
			Score:       0.45,
			Categories:  []string{"credential_theft"},
			Signals:     []string{"ownership_unverified"},
			RequestID:   string(rune('a' + i)),
			SessionHash: "session-a",
			At:          now.Add(time.Duration(i) * time.Second),
		}, cfg)
	}
	if state.Tier != TierHigh || state.Score < cfg.BlockThreshold {
		t.Fatalf("state=%#v", state)
	}
	if state.Strikes != 4 || state.SuspiciousSessionCount != 1 {
		t.Fatalf("unexpected counters: %#v", state)
	}
}

func TestApplyDoesNotAccumulateBenignSecurityAdvice(t *testing.T) {
	cfg := DefaultConfig()
	state := Apply(State{}, Event{
		Score:      0.20,
		Categories: []string{"cyber_abuse"},
		Signals:    []string{"defensive_context"},
		At:         time.Unix(1700000000, 0),
	}, cfg)
	if state.Tier != TierLow || state.Strikes != 0 || state.Score != 0.20 {
		t.Fatalf("state=%#v", state)
	}
}

func TestApplyDeduplicatesRequestID(t *testing.T) {
	cfg := DefaultConfig()
	event := Event{Score: 0.50, Categories: []string{"phishing"}, RequestID: "same", At: time.Unix(1700000000, 0)}
	first := Apply(State{}, event, cfg)
	second := Apply(first, event, cfg)
	if second.Score != first.Score || second.Strikes != first.Strikes {
		t.Fatalf("duplicate changed state: first=%#v second=%#v", first, second)
	}
}

func TestDecayUsesConfiguredHalfLife(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	got := Decay(0.80, now, now.Add(30*time.Minute), 30*time.Minute)
	if math.Abs(got-0.40) > 0.0001 {
		t.Fatalf("got=%f", got)
	}
}

func TestActorBonusRequiresMultipleSuspiciousSessions(t *testing.T) {
	if got := ActorBonus(State{Score: 0.8, SuspiciousSessionCount: 1}); got != 0 {
		t.Fatalf("single session bonus=%f", got)
	}
	if got := ActorBonus(State{Score: 0.8, SuspiciousSessionCount: 3}); got <= 0 || got > 0.15 {
		t.Fatalf("multi-session bonus=%f", got)
	}
}
