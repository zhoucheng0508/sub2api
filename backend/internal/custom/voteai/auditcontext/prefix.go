package auditcontext

import "strings"

const (
	PrefixBreakPolicyChanged    = "policy_changed"
	PrefixBreakModelChanged     = "model_changed"
	PrefixBreakHistoryRewritten = "history_rewritten"
	PrefixBreakHistoryTruncated = "history_truncated"
	PrefixBreakCompactionEpoch  = "compaction_epoch"
	PrefixBreakSessionChanged   = "session_identity_changed"
	PrefixBreakAuditKeyChanged  = "audit_key_changed"
	PrefixBreakUnknown          = "unknown"
)

// UpdatePrefix records cache-prefix continuity without storing conversation
// text. PreviousPrefixHash must be the hash of the new request's prefix cut at
// the prior LastPrefixChars boundary; this permits append-only verification.
func UpdatePrefix(previous State, observation PrefixObservation) State {
	if observation.AtUnixNano > 0 {
		latest := previous.PrefixUpdatedAtUnixNano
		if previous.UpdatedAtUnixNano > latest {
			latest = previous.UpdatedAtUnixNano
		}
		if latest > observation.AtUnixNano {
			return previous
		}
	}
	reason := prefixBreakReason(previous, observation)
	state := previous
	if reason == PrefixBreakPolicyChanged {
		// A new policy invalidates both prefix continuity and every accumulated
		// risk field. Preserve only the monotonic epoch and the break diagnosis.
		state = State{
			PrefixEpoch:       max(1, previous.PrefixEpoch+1),
			PrefixContinuity:  false,
			PrefixBreakReason: PrefixBreakPolicyChanged,
		}
	}
	state.Version = StateVersion
	if state.PrefixEpoch <= 0 {
		state.PrefixEpoch = 1
	}

	baseline := strings.TrimSpace(previous.CanonicalPrefixHash) == ""
	state.PrefixBaseline = false
	if reason == PrefixBreakPolicyChanged {
		// The reset above already records the break and starts a new epoch.
	} else if baseline {
		// The first provider-observed prefix establishes a baseline. It is neither
		// a cache-continuous request nor a break and must not inflate either metric.
		state.PrefixContinuity = false
		state.PrefixBaseline = true
		state.PrefixBreakReason = ""
	} else if reason == "" {
		state.PrefixContinuity = true
		state.PrefixBreakReason = ""
	} else {
		state.PrefixEpoch++
		state.PrefixContinuity = false
		state.PrefixBreakReason = reason
	}

	state.CanonicalPrefixHash = strings.TrimSpace(observation.CanonicalPrefixHash)
	if observation.PrefixChars >= 0 {
		state.LastPrefixChars = observation.PrefixChars
	}
	if observation.PrefixTokens >= 0 {
		state.LastPrefixTokens = observation.PrefixTokens
	}
	if value := strings.TrimSpace(observation.PolicyVersion); value != "" {
		state.PolicyVersion = value
	}
	if value := strings.TrimSpace(observation.Model); value != "" {
		state.PrefixModel = value
	}
	if value := strings.TrimSpace(observation.AuditKeyHash); value != "" {
		state.AuditKeyHash = value
	}
	if observation.AtUnixNano > 0 {
		state.PrefixUpdatedAtUnixNano = observation.AtUnixNano
	}
	return state
}

func prefixBreakReason(previous State, current PrefixObservation) string {
	currentPolicy := strings.TrimSpace(current.PolicyVersion)
	if currentPolicy != "" && hasStoredAuditContext(previous) && strings.TrimSpace(previous.PolicyVersion) != currentPolicy {
		return PrefixBreakPolicyChanged
	}
	if strings.TrimSpace(previous.CanonicalPrefixHash) == "" {
		return ""
	}
	if current.SessionChanged {
		return PrefixBreakSessionChanged
	}
	if previous.PrefixModel != "" && current.Model != "" && previous.PrefixModel != current.Model {
		return PrefixBreakModelChanged
	}
	if previous.AuditKeyHash != "" && current.AuditKeyHash != "" && previous.AuditKeyHash != current.AuditKeyHash {
		return PrefixBreakAuditKeyChanged
	}
	canonical := strings.TrimSpace(current.CanonicalPrefixHash)
	priorPrefix := strings.TrimSpace(current.PreviousPrefixHash)
	// Truncation is not itself a cache break when the new compacted history
	// still begins with the exact previous canonical prefix. This is what lets
	// a newly established compaction epoch accumulate cache hits on append-only
	// follow-up reviews.
	if canonical == strings.TrimSpace(previous.CanonicalPrefixHash) || priorPrefix == strings.TrimSpace(previous.CanonicalPrefixHash) {
		return ""
	}
	if current.HistoryRewritten {
		return PrefixBreakHistoryRewritten
	}
	if current.Compacted {
		return PrefixBreakCompactionEpoch
	}
	if current.HistoryTruncated {
		return PrefixBreakHistoryTruncated
	}
	if canonical == "" {
		return PrefixBreakUnknown
	}
	return PrefixBreakHistoryRewritten
}

func hasStoredAuditContext(state State) bool {
	return state.Version != 0 || state.TurnCount != 0 || state.UpdatedAt != 0 ||
		strings.TrimSpace(state.PolicyVersion) != "" || strings.TrimSpace(state.CanonicalPrefixHash) != ""
}
