package auditcontext

import "testing"

func TestUpdatePrefixEstablishesBaselineAndAppendContinuity(t *testing.T) {
	t.Parallel()
	state := UpdatePrefix(State{}, PrefixObservation{
		CanonicalPrefixHash: "hash-1", PrefixChars: 100, PrefixTokens: 20,
		PolicyVersion: "p1", Model: "deepseek-v4-flash", AuditKeyHash: "key-a",
	})
	if state.PrefixEpoch != 1 || state.PrefixContinuity || !state.PrefixBaseline || state.PrefixBreakReason != "" {
		t.Fatalf("baseline=%#v", state)
	}
	state = UpdatePrefix(state, PrefixObservation{
		CanonicalPrefixHash: "hash-2", PreviousPrefixHash: "hash-1", PrefixChars: 150, PrefixTokens: 30,
		PolicyVersion: "p1", Model: "deepseek-v4-flash", AuditKeyHash: "key-a",
	})
	if state.PrefixEpoch != 1 || !state.PrefixContinuity || state.PrefixBaseline || state.PrefixBreakReason != "" || state.CanonicalPrefixHash != "hash-2" {
		t.Fatalf("append=%#v", state)
	}
}

func TestUpdatePrefixClassifiesBreakAndStartsNewEpoch(t *testing.T) {
	t.Parallel()
	base := State{
		Version: StateVersion, PrefixEpoch: 3, CanonicalPrefixHash: "old", PolicyVersion: "p1",
		PrefixModel: "deepseek-v4-flash", AuditKeyHash: "key-a", TurnCount: 8,
		CurrentScore: 0.88, Categories: []string{"cyber_abuse"}, Signals: []string{"auth_bypass"},
	}
	tests := []struct {
		name   string
		input  PrefixObservation
		reason string
	}{
		{"policy", PrefixObservation{CanonicalPrefixHash: "new", PolicyVersion: "p2", Model: "deepseek-v4-flash", AuditKeyHash: "key-a"}, PrefixBreakPolicyChanged},
		{"model", PrefixObservation{CanonicalPrefixHash: "new", PolicyVersion: "p1", Model: "other", AuditKeyHash: "key-a"}, PrefixBreakModelChanged},
		{"key", PrefixObservation{CanonicalPrefixHash: "new", PolicyVersion: "p1", Model: "deepseek-v4-flash", AuditKeyHash: "key-b"}, PrefixBreakAuditKeyChanged},
		{"compaction", PrefixObservation{CanonicalPrefixHash: "new", PolicyVersion: "p1", Model: "deepseek-v4-flash", AuditKeyHash: "key-a", Compacted: true}, PrefixBreakCompactionEpoch},
		{"truncated", PrefixObservation{CanonicalPrefixHash: "new", PolicyVersion: "p1", Model: "deepseek-v4-flash", AuditKeyHash: "key-a", HistoryTruncated: true}, PrefixBreakHistoryTruncated},
		{"rewritten-before-compaction", PrefixObservation{CanonicalPrefixHash: "new", PolicyVersion: "p1", Model: "deepseek-v4-flash", AuditKeyHash: "key-a", HistoryRewritten: true, Compacted: true}, PrefixBreakHistoryRewritten},
		{"rewrite", PrefixObservation{CanonicalPrefixHash: "new", PreviousPrefixHash: "different", PolicyVersion: "p1", Model: "deepseek-v4-flash", AuditKeyHash: "key-a"}, PrefixBreakHistoryRewritten},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := UpdatePrefix(base, test.input)
			if got.PrefixEpoch != 4 || got.PrefixContinuity || got.PrefixBreakReason != test.reason {
				t.Fatalf("state=%#v", got)
			}
			if test.reason == PrefixBreakPolicyChanged && (got.TurnCount != 0 || got.CurrentScore != 0 || len(got.Categories) != 0 || len(got.Signals) != 0) {
				t.Fatalf("policy change retained stale risk state: %#v", got)
			}
		})
	}
}

func TestUpdatePrefixNeverStoresRawAuditKey(t *testing.T) {
	t.Parallel()
	state := UpdatePrefix(State{}, PrefixObservation{CanonicalPrefixHash: "hash", AuditKeyHash: "opaque-hash"})
	if state.AuditKeyHash != "opaque-hash" {
		t.Fatalf("audit key fingerprint was not retained: %#v", state)
	}
}
