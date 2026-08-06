package service

import (
	"fmt"
	"strings"

	voteaiinputprovenance "github.com/Wei-Shaw/sub2api/internal/custom/voteai/inputprovenance"
	openaipkg "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// CUSTOM(VOTE-AI-AUDIT-CONTEXT): normalize provenance before keyword, hash,
// deterministic-risk, or semantic checks. Text labels never establish trust.
func normalizeContentModerationProvenance(input ContentModerationCheckInput, snapshot *contentModerationRuntimeSnapshot, content *ContentModerationInput) {
	if content == nil || snapshot == nil || snapshot.config == nil {
		return
	}
	if !snapshot.config.AIChat.InputProvenanceV2Enabled {
		content.AuditTargetText = strings.TrimSpace(content.CurrentText)
		if content.AuditTargetText == "" {
			content.AuditTargetText = strings.TrimSpace(content.Text)
		}
		content.AuditTargetKind = string(voteaiinputprovenance.TargetUserRequest)
		content.HasExplicitUser = content.AuditTargetText != ""
		return
	}

	headers := input.ClientHeaders
	strictUA := openaipkg.IsCodexOfficialClientRequestStrict(headers.Get("User-Agent"))
	officialOriginator := openaipkg.IsCodexOfficialClientOriginator(headers.Get("Originator"))
	signals := snapshot.engineFingerprintSignals
	hasRequiredFingerprint := contentModerationHasRequiredEngineFingerprintSignal(signals)
	engineFingerprint := hasRequiredFingerprint && openaipkg.EvaluateEngineFingerprint(headers, input.Body, signals)
	structured := len(content.Turns) > 0

	trustSignals := make([]voteaiinputprovenance.TrustSignal, 0, 5)
	if input.TrustedMetadataProvenance {
		trustSignals = append(trustSignals, voteaiinputprovenance.TrustInternalProvenance)
	}
	if strictUA {
		trustSignals = append(trustSignals, voteaiinputprovenance.TrustStrictUserAgent)
	}
	if officialOriginator {
		trustSignals = append(trustSignals, voteaiinputprovenance.TrustOfficialOriginator)
	}
	if engineFingerprint {
		trustSignals = append(trustSignals, voteaiinputprovenance.TrustEngineFingerprint)
	}
	if structured {
		trustSignals = append(trustSignals, voteaiinputprovenance.TrustStructuredMessage)
	}
	trust := voteaiinputprovenance.TrustDecision{
		Verified: input.TrustedMetadataProvenance && strictUA && officialOriginator && structured && hasRequiredFingerprint && engineFingerprint,
		Signals:  trustSignals,
	}

	turns := make([]voteaiinputprovenance.Turn, 0, len(content.Turns))
	for _, turn := range content.Turns {
		turns = append(turns, voteaiinputprovenance.Turn{
			Role:               voteaiinputprovenance.Role(turn.Role),
			Text:               turn.Text,
			ToolCall:           turn.ToolCall,
			Truncated:          turn.Truncated,
			Current:            turn.Current,
			LinkedToUserIntent: turn.LinkedToUserIntent,
			MetadataEnvelope:   turn.MetadataEnvelope,
			MetadataHint:       voteaiinputprovenance.MetadataKind(turn.MetadataHint),
		})
	}
	result := voteaiinputprovenance.NormalizeAndSelect(turns, trust)
	content.AuditTargetText = strings.TrimSpace(result.Target.Text)
	content.AuditTargetKind = string(result.Target.Kind)
	content.HasExplicitUser = result.HasExplicitUser
	content.TrustedClient = result.TrustedClient
	content.TrustedSignals = make([]string, 0, len(trustSignals))
	for _, signal := range trustSignals {
		content.TrustedSignals = append(content.TrustedSignals, string(signal))
	}
	content.IgnoredMetadata = make([]string, 0, len(result.IgnoredMetadata))
	for _, kind := range result.IgnoredMetadata {
		content.IgnoredMetadata = append(content.IgnoredMetadata, string(kind))
	}
	content.Turns = make([]ContentModerationTurn, 0, len(result.Turns))
	for _, turn := range result.Turns {
		content.Turns = append(content.Turns, ContentModerationTurn{
			Role:               string(turn.Role),
			Source:             string(turn.Source),
			Purpose:            string(turn.Purpose),
			MetadataKind:       string(turn.MetadataKind),
			Text:               turn.Text,
			ToolCall:           turn.ToolCall,
			Truncated:          turn.Truncated,
			Current:            turn.Current,
			LinkedToUserIntent: turn.LinkedToUserIntent,
		})
	}
	content.CurrentText = content.AuditTargetText
	content.Text = renderNormalizedContentModerationTurns(content.Turns)
	content.Normalize()
}

func contentModerationHasRequiredEngineFingerprintSignal(signals []openaipkg.EngineFingerprintSignal) bool {
	for _, signal := range signals {
		if signal.Required {
			return true
		}
	}
	return false
}

func renderNormalizedContentModerationTurns(turns []ContentModerationTurn) string {
	parts := make([]string, 0, len(turns))
	for _, turn := range turns {
		if turn.Source == string(voteaiinputprovenance.SourceTrustedMetadata) {
			continue
		}
		text := strings.TrimSpace(turn.Text)
		if text == "" {
			continue
		}
		label := strings.ToUpper(strings.TrimSpace(turn.Role))
		if turn.Source == string(voteaiinputprovenance.SourceClientInstruction) {
			if label == "" {
				label = "INSTRUCTION"
			}
			label = "CLIENT_" + label
		} else if label == "" {
			label = "CLIENT_INSTRUCTION"
		}
		parts = append(parts, fmt.Sprintf("[%s]\n%s", label, text))
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func contentModerationHasAuditTarget(content ContentModerationInput) bool {
	return strings.TrimSpace(content.AuditTargetText) != "" &&
		strings.TrimSpace(content.AuditTargetKind) != string(voteaiinputprovenance.TargetNoNewUserIntent)
}
