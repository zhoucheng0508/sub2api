package auditcontext

import (
	"reflect"
	"testing"
	"time"
)

func TestDecideFullReviewReasons(t *testing.T) {
	t.Parallel()
	now := time.Unix(5000, 0).UTC()
	state := State{TurnCount: 8, CurrentScore: 0.22, UpdatedAt: now.Unix()}
	decision := DecideFullReview(state, ReviewInput{
		FastScore: 0.45, Signals: []string{"auth_bypass"}, LatestUserText: "继续写成脚本",
		StableSession: true, At: now,
	}, DefaultConfig())
	want := []string{ReviewReasonScoreThreshold, ReviewReasonStrongSignal, ReviewReasonRiskRise, ReviewReasonProgressiveLanguage}
	if !decision.Required || !reflect.DeepEqual(decision.Reasons, want) {
		t.Fatalf("decision=%#v want=%v", decision, want)
	}
}

func TestDecideFullReviewWeakSignalsAloneDoNotEscalate(t *testing.T) {
	t.Parallel()
	decision := DecideFullReview(State{}, ReviewInput{
		FastScore: 0.55, Signals: []string{"ownership_unverified", "credential_access"},
		LatestUserText: "查找我本地保存的测试管理员密码",
	}, DefaultConfig())
	if decision.Required {
		t.Fatalf("weak credential signals caused review: %#v", decision)
	}
}

func TestDecideFullReviewPeriodicRequiresContext(t *testing.T) {
	t.Parallel()
	state := State{TurnCount: 9}
	withoutContext := DecideFullReview(state, ReviewInput{}, DefaultConfig())
	if withoutContext.Required {
		t.Fatalf("periodic review ran without stable session/history: %#v", withoutContext)
	}
	withSession := DecideFullReview(state, ReviewInput{StableSession: true}, DefaultConfig())
	if !reflect.DeepEqual(withSession.Reasons, []string{ReviewReasonPeriodic}) {
		t.Fatalf("decision=%#v", withSession)
	}
}

func TestDecideFullReviewPeriodicRestartsAfterFullReview(t *testing.T) {
	t.Parallel()
	state := State{TurnCount: 9, LastFullReviewTurn: 8}
	decision := DecideFullReview(state, ReviewInput{StableSession: true}, DefaultConfig())
	if decision.Required {
		t.Fatalf("recent full review did not reset periodic interval: %#v", decision)
	}
}

func TestDecideFullReviewTruncatedDefensiveInputDoesNotEscalate(t *testing.T) {
	t.Parallel()
	weak := DecideFullReview(State{}, ReviewInput{InputTruncated: true, Signals: []string{"defensive_context"}}, DefaultConfig())
	if weak.Required {
		t.Fatalf("defensive truncation caused review: %#v", weak)
	}
	risky := DecideFullReview(State{}, ReviewInput{InputTruncated: true, Categories: []string{"malware"}}, DefaultConfig())
	if !reflect.DeepEqual(risky.Reasons, []string{ReviewReasonTruncatedRisk}) {
		t.Fatalf("decision=%#v", risky)
	}
}

func TestDecideFullReviewUsesDecayedRisk(t *testing.T) {
	t.Parallel()
	now := time.Unix(6000, 0).UTC()
	cfg := DefaultConfig()
	cfg.RiskHalfLife = time.Minute
	state := State{CurrentScore: 0.60, UpdatedAt: now.Add(-10 * time.Minute).Unix()}
	decision := DecideFullReview(state, ReviewInput{FastScore: 0.10, At: now}, cfg)
	if decision.Required {
		t.Fatalf("stale risk should have decayed: %#v", decision)
	}
}

func TestDecideFullReviewDoesNotTreatFirstLowScoreAsRiskRise(t *testing.T) {
	t.Parallel()
	decision := DecideFullReview(State{}, ReviewInput{FastScore: 0.20}, DefaultConfig())
	if decision.Required {
		t.Fatalf("first low-score request caused review: %#v", decision)
	}
}
