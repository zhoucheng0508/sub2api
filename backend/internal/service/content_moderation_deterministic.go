package service

import (
	"fmt"
	"strings"

	voteaiauditcontext "github.com/Wei-Shaw/sub2api/internal/custom/voteai/auditcontext"
	voteaideterministicrisk "github.com/Wei-Shaw/sub2api/internal/custom/voteai/deterministicrisk"
	voteaiinputprovenance "github.com/Wei-Shaw/sub2api/internal/custom/voteai/inputprovenance"
	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
)

// CUSTOM(VOTE-AI-AUDIT-DETERMINISTIC-V2): adapt only the normalized target to
// the custom detector. Raw envelopes and trusted metadata never enter it.
func evaluateContentModerationDeterministicRisk(content ContentModerationInput) voteaideterministicrisk.Result {
	target := voteaideterministicrisk.AuditTarget{
		Kind: voteaiinputprovenance.TargetKind(strings.TrimSpace(content.AuditTargetKind)),
		Text: strings.TrimSpace(content.AuditTargetText),
	}
	targetIndex := -1
	for index, turn := range content.Turns {
		if turn.Purpose != string(voteaiinputprovenance.PurposeAuditTarget) {
			continue
		}
		targetIndex = index
		target.Source = voteaiinputprovenance.Source(turn.Source)
		target.MetadataKind = voteaiinputprovenance.MetadataKind(turn.MetadataKind)
		target.LinkedToUserIntent = turn.LinkedToUserIntent
		break
	}
	if target.Source == "" {
		switch target.Kind {
		case voteaiinputprovenance.TargetUserRequest:
			target.Source = voteaiinputprovenance.SourceEndUser
		case voteaiinputprovenance.TargetToolContinuation:
			target.Source = voteaiinputprovenance.SourceToolOutput
		case voteaiinputprovenance.TargetClientInstruction:
			target.Source = voteaiinputprovenance.SourceClientInstruction
		}
	}

	directIndex := -1
	if voteaiauditcontext.NeedsPreviousContext(target.Text) && targetIndex > 0 {
		for index := targetIndex - 1; index >= 0; index-- {
			turn := content.Turns[index]
			if turn.Purpose != string(voteaiinputprovenance.PurposeSupportingContext) ||
				voteaiinputprovenance.Source(turn.Source) != target.Source {
				continue
			}
			directIndex = index
			break
		}
	}

	supporting := make([]voteaideterministicrisk.SupportingContext, 0, len(content.Turns))
	for index, turn := range content.Turns {
		if turn.Purpose != string(voteaiinputprovenance.PurposeSupportingContext) ||
			turn.Source == string(voteaiinputprovenance.SourceTrustedMetadata) {
			continue
		}
		role := voteaiinputprovenance.Role(turn.Role)
		if turn.Source == string(voteaiinputprovenance.SourceClientInstruction) {
			role = voteaiinputprovenance.RoleDeveloper
		}
		supporting = append(supporting, voteaideterministicrisk.SupportingContext{
			Role:           role,
			Source:         voteaiinputprovenance.Source(turn.Source),
			Purpose:        voteaiinputprovenance.Purpose(turn.Purpose),
			MetadataKind:   voteaiinputprovenance.MetadataKind(turn.MetadataKind),
			Text:           turn.Text,
			DirectlyLinked: index == directIndex,
		})
	}

	return voteaideterministicrisk.Detect(voteaideterministicrisk.Input{
		Target:            target,
		SupportingContext: supporting,
		MetadataExcluded:  append([]string(nil), content.IgnoredMetadata...),
	})
}

func moderationAPIResultFromDeterministicRisk(result voteaideterministicrisk.Result) *moderationAPIResult {
	if result.Level != voteaideterministicrisk.LevelConfirmed || result.SuggestedRiskScore == nil {
		return nil
	}
	score := *result.SuggestedRiskScore
	reason := "本地确定性规则确认存在未经授权的凭据或认证绕过请求"
	if result.Match != nil {
		reason = fmt.Sprintf("本地规则 %s/%s 确认高风险：%s", result.Match.RuleID, result.Match.RuleVersion, result.Match.MatchedExcerpt)
	}
	return &moderationAPIResult{
		Flagged:               true,
		CategoryScores:        map[string]float64{"ai_risk": score, "credential_theft": score, "cyber_abuse": score},
		Categories:            []string{"credential_theft", "cyber_abuse"},
		Signals:               []string{"credential_access", "auth_bypass"},
		Reason:                reason,
		Stage:                 voteaimoderation.ReviewStage("local"),
		LocalDecision:         true,
		HashPromotionEligible: true,
		HashPromotionReason:   "confirmed_deterministic_v2",
		LocalRuleLevel:        string(result.Level),
		LocalRuleMatch:        result.Match,
	}
}

func annotateModerationResultWithDeterministicRisk(result *moderationAPIResult, local voteaideterministicrisk.Result) {
	if result == nil || local.Level == voteaideterministicrisk.LevelNone {
		return
	}
	result.LocalRuleLevel = string(local.Level)
	result.LocalRuleMatch = local.Match
	if local.Level == voteaideterministicrisk.LevelCandidate {
		result.HashPromotionEligible = false
		if !result.ResultCacheHit && (result.Stage == voteaimoderation.StageFull || result.Stage == voteaimoderation.StageMax) {
			// The local rule only requested semantic confirmation. A fresh full
			// review may resolve that ambiguity; the normal strong-signal and score
			// gates still decide whether the hash is promoted.
			result.HashPromotionVeto = false
			result.HashPromotionReason = "candidate_resolved_by_semantic_full_review"
		} else {
			result.HashPromotionVeto = true
			result.HashPromotionReason = "candidate_requires_semantic_review"
		}
	}
}
