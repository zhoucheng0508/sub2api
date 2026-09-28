package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	voteaideterministicrisk "github.com/Wei-Shaw/sub2api/internal/custom/voteai/deterministicrisk"
	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
)

// ContentModerationAuditDetails contains redacted, structured diagnostics for
// one audit. It deliberately excludes complete conversations and credentials.
type ContentModerationAuditDetails struct {
	TotalLatencyMS           *int                                            `json:"total_latency_ms,omitempty"`
	ExtractionLatencyMS      *int                                            `json:"extraction_latency_ms,omitempty"`
	ProvenanceLatencyMS      *int                                            `json:"provenance_latency_ms,omitempty"`
	DeterministicLatencyMS   *int                                            `json:"deterministic_latency_ms,omitempty"`
	VerdictCacheLatencyMS    *int                                            `json:"verdict_cache_latency_ms,omitempty"`
	ContextLoadLatencyMS     *int                                            `json:"context_load_latency_ms,omitempty"`
	FastBuildLatencyMS       *int                                            `json:"fast_build_latency_ms,omitempty"`
	ReviewBuildLatencyMS     *int                                            `json:"review_build_latency_ms,omitempty"`
	ProviderLatencyMS        *int                                            `json:"provider_latency_ms,omitempty"`
	PostprocessLatencyMS     *int                                            `json:"postprocess_latency_ms,omitempty"`
	AuditStage               string                                          `json:"audit_stage,omitempty"`
	EscalationReasons        []string                                        `json:"escalation_reasons,omitempty"`
	SessionSource            string                                          `json:"session_source,omitempty"`
	TurnCount                int                                             `json:"turn_count,omitempty"`
	InputChars               int                                             `json:"input_chars,omitempty"`
	PromptTokens             *int                                            `json:"prompt_tokens,omitempty"`
	CachedInputTokens        *int                                            `json:"cached_input_tokens,omitempty"`
	UncachedInputTokens      *int                                            `json:"uncached_input_tokens,omitempty"`
	OutputTokens             *int                                            `json:"output_tokens,omitempty"`
	ProviderPrefixCacheRatio *float64                                        `json:"provider_prefix_cache_ratio,omitempty"`
	UsageUnknown             bool                                            `json:"usage_unknown,omitempty"`
	ResultCacheHit           bool                                            `json:"result_cache_hit,omitempty"`
	ProviderApplicable       *bool                                           `json:"provider_applicable,omitempty"`
	ResultCacheApplicable    *bool                                           `json:"result_cache_applicable,omitempty"`
	ReviewApplicable         *bool                                           `json:"review_applicable,omitempty"`
	Sub2APIResultCacheHit    *bool                                           `json:"sub2api_result_cache_hit,omitempty"`
	ReviewComplete           *bool                                           `json:"review_complete,omitempty"`
	HasExplicitUserTurn      *bool                                           `json:"has_explicit_user_turn,omitempty"`
	TrustedClient            *bool                                           `json:"trusted_client,omitempty"`
	AuditKeyHash             string                                          `json:"audit_key_hash,omitempty"`
	PrefixEpoch              int                                             `json:"prefix_epoch,omitempty"`
	PrefixContinuity         *bool                                           `json:"prefix_continuity,omitempty"`
	PrefixBaseline           *bool                                           `json:"prefix_baseline,omitempty"`
	PrefixBreakReason        string                                          `json:"prefix_break_reason,omitempty"`
	InputTruncated           *bool                                           `json:"input_truncated,omitempty"`
	AuditTargetKind          string                                          `json:"audit_target_kind,omitempty"`
	AuditTargetSource        string                                          `json:"audit_target_source,omitempty"`
	AuditTargetExcerpt       string                                          `json:"audit_target_excerpt,omitempty"`
	SupportingContextExcerpt string                                          `json:"supporting_context_excerpt,omitempty"`
	TrustedSignals           []string                                        `json:"trusted_signals,omitempty"`
	IgnoredMetadata          []string                                        `json:"ignored_metadata,omitempty"`
	InputHash                string                                          `json:"input_hash,omitempty"`
	HashScope                string                                          `json:"hash_scope,omitempty"`
	HashState                string                                          `json:"hash_state,omitempty"`
	HashPromotionReason      string                                          `json:"hash_promotion_reason,omitempty"`
	PolicyVersion            string                                          `json:"policy_version,omitempty"`
	ReviewIncomplete         bool                                            `json:"review_incomplete,omitempty"`
	ModelReason              string                                          `json:"model_reason,omitempty"`
	ModelSignals             []string                                        `json:"model_signals,omitempty"`
	LocalRuleLevel           string                                          `json:"local_rule_level,omitempty"`
	LocalRuleMatch           *voteaideterministicrisk.DeterministicRiskMatch `json:"local_rule_match,omitempty"`
	Stages                   []ContentModerationAuditStageDetails            `json:"stages,omitempty"`
}

// contentModerationLatencyBreakdown is request-local and never persisted as a
// whole. Only bounded millisecond counters are copied into audit_details.
type contentModerationLatencyBreakdown struct {
	startedAt              time.Time
	postprocessStartedAt   time.Time
	extractionLatencyMS    *int
	provenanceLatencyMS    *int
	deterministicLatencyMS *int
	verdictCacheLatencyMS  *int
	contextLoadLatencyMS   *int
	fastBuildLatencyMS     *int
	reviewBuildLatencyMS   *int
	providerLatencyMS      *int
}

func newContentModerationLatencyBreakdown() *contentModerationLatencyBreakdown {
	return &contentModerationLatencyBreakdown{startedAt: time.Now()}
}

func moderationElapsedMS(startedAt time.Time) *int {
	if startedAt.IsZero() {
		return nil
	}
	return auditIntPtr(max(0, int(time.Since(startedAt).Milliseconds())))
}

func addModerationElapsedMS(target **int, startedAt time.Time) {
	if target == nil || startedAt.IsZero() {
		return
	}
	elapsed := max(0, int(time.Since(startedAt).Milliseconds()))
	if *target == nil {
		*target = auditIntPtr(elapsed)
		return
	}
	**target += elapsed
}

func contentModerationProviderLatencyMS(result *moderationAPIResult) *int {
	if result == nil {
		return nil
	}
	total := 0
	observed := false
	for _, stage := range result.StageDetails {
		if !stage.ProviderCalled || stage.LatencyMS == nil {
			continue
		}
		total += max(0, *stage.LatencyMS)
		observed = true
	}
	if !observed {
		return nil
	}
	return auditIntPtr(total)
}

// ContentModerationAuditStageDetails separates provider work from Sub2API
// result-cache hits. This prevents a partially cached two-stage review from
// being reported as a zero-cost audit.
type ContentModerationAuditStageDetails struct {
	Stage               string `json:"stage"`
	ProviderCalled      bool   `json:"provider_called"`
	ResultCacheHit      bool   `json:"result_cache_hit"`
	UsageKnown          bool   `json:"usage_known"`
	Failed              bool   `json:"failed"`
	InputChars          *int   `json:"input_chars,omitempty"`
	LatencyMS           *int   `json:"latency_ms,omitempty"`
	PromptTokens        *int   `json:"prompt_tokens,omitempty"`
	CachedInputTokens   *int   `json:"cached_input_tokens,omitempty"`
	UncachedInputTokens *int   `json:"uncached_input_tokens,omitempty"`
	OutputTokens        *int   `json:"output_tokens,omitempty"`
}

func populateContentModerationAuditDetails(
	log *ContentModerationLog,
	cfg *ContentModerationConfig,
	content ContentModerationInput,
	result *moderationAPIResult,
	plan *contentModerationIncrementalPlan,
	recordHash bool,
) {
	if log == nil || cfg == nil || cfg.AuditProvider != ContentModerationProviderAIChat {
		return
	}
	details := ContentModerationAuditDetails{
		SessionSource:            contentModerationSessionSource(plan, log.SessionSource, log.SessionID),
		HasExplicitUserTurn:      auditBoolPtr(content.HasExplicitUser),
		TrustedClient:            auditBoolPtr(content.TrustedClient),
		AuditTargetKind:          strings.TrimSpace(content.AuditTargetKind),
		AuditTargetSource:        contentModerationAuditTargetSource(content),
		AuditTargetExcerpt:       trimRunes(redactContentModerationSecrets(content.AuditTargetText), 1200),
		SupportingContextExcerpt: contentModerationSupportingContextExcerpt(content),
		TrustedSignals:           append([]string(nil), content.TrustedSignals...),
		IgnoredMetadata:          append([]string(nil), content.IgnoredMetadata...),
		HashScope:                "audit_target_v1",
		PolicyVersion:            contentModerationAuditPolicyVersion(cfg),
	}
	if timings := content.auditTimings; timings != nil {
		details.TotalLatencyMS = moderationElapsedMS(timings.startedAt)
		details.ExtractionLatencyMS = cloneIntPtr(timings.extractionLatencyMS)
		details.ProvenanceLatencyMS = cloneIntPtr(timings.provenanceLatencyMS)
		details.DeterministicLatencyMS = cloneIntPtr(timings.deterministicLatencyMS)
		details.VerdictCacheLatencyMS = cloneIntPtr(timings.verdictCacheLatencyMS)
		details.ContextLoadLatencyMS = cloneIntPtr(timings.contextLoadLatencyMS)
		details.FastBuildLatencyMS = cloneIntPtr(timings.fastBuildLatencyMS)
		details.ReviewBuildLatencyMS = cloneIntPtr(timings.reviewBuildLatencyMS)
		details.ProviderLatencyMS = cloneIntPtr(timings.providerLatencyMS)
		details.PostprocessLatencyMS = moderationElapsedMS(timings.postprocessStartedAt)
	}
	providerApplicable := (result != nil && !result.LocalDecision) || log.Action == ContentModerationActionError
	resultCacheApplicable := providerApplicable && cfg.AIChat.CacheEnabled
	reviewApplicable := providerApplicable
	details.ProviderApplicable = auditBoolPtr(providerApplicable)
	details.ResultCacheApplicable = auditBoolPtr(resultCacheApplicable)
	details.ReviewApplicable = auditBoolPtr(reviewApplicable)
	if resultCacheApplicable {
		cacheHit := result != nil && result.ResultCacheHit
		details.Sub2APIResultCacheHit = auditBoolPtr(cacheHit)
	}
	if reviewApplicable {
		reviewComplete := result != nil && !result.ReviewIncomplete && log.Action != ContentModerationActionError
		details.ReviewComplete = auditBoolPtr(reviewComplete)
	}
	if plan != nil {
		details.EscalationReasons = append([]string(nil), plan.escalationReasons...)
		details.TurnCount = plan.state.TurnCount
		details.PrefixEpoch = plan.state.PrefixEpoch
		details.PrefixContinuity = auditBoolPtr(plan.state.PrefixContinuity)
		details.PrefixBaseline = auditBoolPtr(plan.state.PrefixBaseline)
		details.PrefixBreakReason = plan.state.PrefixBreakReason
		details.InputTruncated = auditBoolPtr(plan.inputTruncated)
		details.InputChars = plan.inputChars
	}
	if result != nil {
		if providerLatency := contentModerationProviderLatencyMS(result); providerLatency != nil {
			details.ProviderLatencyMS = providerLatency
		}
		details.AuditStage = string(result.Stage)
		if details.InputChars == 0 {
			details.InputChars = contentModerationResultInputChars(result)
		}
		details.ResultCacheHit = result.ResultCacheHit
		details.ReviewIncomplete = result.ReviewIncomplete
		details.ModelReason = trimRunes(redactContentModerationSecrets(result.Reason), 500)
		details.ModelSignals = append([]string(nil), result.Signals...)
		details.LocalRuleLevel = result.LocalRuleLevel
		details.LocalRuleMatch = result.LocalRuleMatch
		details.AuditKeyHash = strings.TrimSpace(result.AuditKeyHash)
		details.HashPromotionReason = result.HashPromotionReason
		details.Stages = cloneContentModerationStageDetails(result.StageDetails)
		if result.Usage != nil {
			details.PromptTokens = cloneIntPtr(result.Usage.PromptTokens)
			details.CachedInputTokens = cloneIntPtr(result.Usage.CachedPromptTokens)
			details.UncachedInputTokens = cloneIntPtr(result.Usage.UncachedPromptTokens)
			details.OutputTokens = cloneIntPtr(result.Usage.CompletionTokens)
			details.ProviderPrefixCacheRatio = contentModerationProviderPrefixCacheRatio(result.Usage)
		}
		details.UsageUnknown = contentModerationResultUsageUnknown(result)
		if result.HashPromotionVeto || result.LocalRuleLevel == string(voteaideterministicrisk.LevelCandidate) {
			details.HashState = "candidate"
			if details.HashPromotionReason == "" {
				details.HashPromotionReason = "candidate_resolved_by_semantic_full_review"
			}
		}
	}
	if recordHash {
		details.HashState = "confirmed"
		if details.HashPromotionReason == "" {
			details.HashPromotionReason = "semantic_full_review_strong_signal"
		}
	}
	log.AuditDetails = details
}

func cloneContentModerationStageDetails(items []ContentModerationAuditStageDetails) []ContentModerationAuditStageDetails {
	if len(items) == 0 {
		return nil
	}
	out := make([]ContentModerationAuditStageDetails, len(items))
	copy(out, items)
	for index := range out {
		out[index].InputChars = cloneIntPtr(items[index].InputChars)
		out[index].LatencyMS = cloneIntPtr(items[index].LatencyMS)
		out[index].PromptTokens = cloneIntPtr(items[index].PromptTokens)
		out[index].CachedInputTokens = cloneIntPtr(items[index].CachedInputTokens)
		out[index].UncachedInputTokens = cloneIntPtr(items[index].UncachedInputTokens)
		out[index].OutputTokens = cloneIntPtr(items[index].OutputTokens)
	}
	return out
}

func contentModerationResultUsageUnknown(result *moderationAPIResult) bool {
	if result == nil || result.LocalDecision {
		return false
	}
	if len(result.StageDetails) > 0 {
		for _, stage := range result.StageDetails {
			if stage.ProviderCalled && !stage.UsageKnown {
				return true
			}
		}
		return false
	}
	return !result.ResultCacheHit && !contentModerationUsageComplete(result.Usage)
}

func contentModerationSessionSource(plan *contentModerationIncrementalPlan, source, sessionID string) string {
	if plan != nil && strings.TrimSpace(plan.sessionSource) != "" {
		return strings.TrimSpace(plan.sessionSource)
	}
	return normalizeContentModerationSessionSource(source, sessionID)
}

func normalizeContentModerationSessionSource(source, sessionID string) string {
	switch strings.TrimSpace(source) {
	case ContentModerationSessionSourceHeader:
		if strings.TrimSpace(sessionID) != "" {
			return ContentModerationSessionSourceHeader
		}
	case ContentModerationSessionSourcePromptCacheKey:
		if strings.TrimSpace(sessionID) != "" {
			return ContentModerationSessionSourcePromptCacheKey
		}
	case ContentModerationSessionSourceNone:
		return ContentModerationSessionSourceNone
	}
	// Preserve compatibility for internal callers/tests that predate the source
	// field while never inventing a body-derived identity.
	if strings.TrimSpace(sessionID) != "" {
		return ContentModerationSessionSourceHeader
	}
	return ContentModerationSessionSourceNone
}

func contentModerationProviderPrefixCacheRatio(usage *voteaimoderation.Usage) *float64 {
	if usage == nil || usage.Incomplete || usage.PromptTokens == nil || usage.CachedPromptTokens == nil || usage.UncachedPromptTokens == nil {
		return nil
	}
	prompt := *usage.PromptTokens
	cached := *usage.CachedPromptTokens
	uncached := *usage.UncachedPromptTokens
	if prompt <= 0 || cached < 0 || uncached < 0 || prompt != cached+uncached {
		return nil
	}
	ratio := float64(cached) / float64(prompt)
	return &ratio
}

func contentModerationUsageComplete(usage *voteaimoderation.Usage) bool {
	if usage == nil || usage.Incomplete || usage.CompletionTokens == nil || *usage.CompletionTokens < 0 {
		return false
	}
	return contentModerationProviderPrefixCacheRatio(usage) != nil
}

func contentModerationAuditTargetSource(content ContentModerationInput) string {
	for _, turn := range content.Turns {
		if turn.Purpose == "audit_target" {
			return strings.TrimSpace(turn.Source)
		}
	}
	return ""
}

func contentModerationSupportingContextExcerpt(content ContentModerationInput) string {
	const (
		maxSupportingTurns = 12
		maxTurnRunes       = 1200
		maxToolRunes       = 1600
		maxExcerptRunes    = 8000
	)
	type candidate struct {
		turn  ContentModerationTurn
		index int
		score int
	}
	candidates := make([]candidate, 0, len(content.Turns))
	for index, turn := range content.Turns {
		if turn.Purpose != "supporting_context" {
			continue
		}
		// Recent user/assistant turns are the most useful evidence when
		// explaining a block. Client/developer metadata remains available, but
		// is deliberately lower priority so it cannot crowd out the conversation.
		score := 2
		if turn.Role == "tool" {
			score = 1
		}
		if turn.Source == "client_instruction" || turn.Source == "trusted_metadata" || turn.Role == "system" || turn.Role == "developer" {
			score = 0
		}
		candidates = append(candidates, candidate{turn: turn, index: index, score: score})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].index > candidates[j].index
	})
	if len(candidates) > maxSupportingTurns {
		candidates = candidates[:maxSupportingTurns]
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].index < candidates[j].index })
	partsNewestFirst := make([]string, 0, len(candidates))
	remainingRunes := maxExcerptRunes
	const omittedMarker = "[CONTEXT OMITTED]\n"
	for index := len(candidates) - 1; index >= 0 && remainingRunes > 0; index-- {
		candidate := candidates[index]
		turn := candidate.turn
		text := strings.TrimSpace(turn.Text)
		if turn.Role == "tool" {
			text = sanitizeContentModerationToolOutput(text, maxToolRunes)
		} else {
			text = trimRunes(redactContentModerationSecrets(text), maxTurnRunes)
		}
		if text == "" {
			continue
		}
		part := fmt.Sprintf("[%s/%s] %s", turn.Source, turn.Role, text)
		partRunes := []rune(part)
		separatorRunes := 0
		if len(partsNewestFirst) > 0 {
			separatorRunes = 1
		}
		if len(partRunes)+separatorRunes > remainingRunes {
			available := remainingRunes - separatorRunes - len([]rune(omittedMarker))
			if available <= 0 {
				break
			}
			partRunes = append([]rune(omittedMarker), partRunes[len(partRunes)-available:]...)
		}
		partsNewestFirst = append(partsNewestFirst, string(partRunes))
		remainingRunes -= len(partRunes) + separatorRunes
	}
	parts := make([]string, len(partsNewestFirst))
	for index := range partsNewestFirst {
		parts[len(partsNewestFirst)-1-index] = partsNewestFirst[index]
	}
	return strings.Join(parts, "\n")
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func auditBoolPtr(value bool) *bool {
	return &value
}

func auditIntPtr(value int) *int {
	return &value
}
