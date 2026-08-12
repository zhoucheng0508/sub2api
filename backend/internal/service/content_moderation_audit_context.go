package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	voteaiauditcontext "github.com/Wei-Shaw/sub2api/internal/custom/voteai/auditcontext"
	voteaideterministicrisk "github.com/Wei-Shaw/sub2api/internal/custom/voteai/deterministicrisk"
	voteaiinputprovenance "github.com/Wei-Shaw/sub2api/internal/custom/voteai/inputprovenance"
	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
)

// CUSTOM(VOTE-AI-AUDIT-COST/CONTEXT): this file contains the incremental
// orchestration so the official moderation service keeps only a narrow hook.

const (
	contentModerationPinnedSystemChars = 2000
	contentModerationPinnedUserChars   = 2000
	periodicReviewUserTurns            = 10
	periodicReviewTargetChars          = 512
	periodicReviewUserChars            = 64
	periodicReviewAssistantChars       = 32
	periodicReviewSystemChars          = 128
)

func contentModerationFastStageContext(ctx context.Context, budgetMS int) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return ctx, func() {}
	}
	budget := time.Duration(max(1, budgetMS)) * time.Millisecond
	deadline, ok := ctx.Deadline()
	if ok && time.Until(deadline) <= budget {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, budget)
}

type contentModerationIncrementalPlan struct {
	state                voteaiauditcontext.State
	stateKey             string
	sessionSource        string
	fastInput            voteaiauditcontext.FastInput
	fullInput            string
	canonicalFullPrefix  string
	periodicInput        string
	canonicalPeriodic    string
	reviewSourceTurns    []ContentModerationTurn
	reviewTargetKind     string
	reviewTargetText     string
	reviewInputBuilt     bool
	latestUserText       string
	stableSession        bool
	fullHistoryAvailable bool
	inputTruncated       bool
	fullHistoryTruncated bool
	prefixCompacted      bool
	prefixHistoryRewrite bool
	inputChars           int
	policyVersion        string
	escalationReasons    []string
	eventAt              time.Time
}

func (cfg *ContentModerationConfig) auditContextConfig() voteaiauditcontext.Config {
	if cfg == nil {
		return voteaiauditcontext.DefaultConfig()
	}
	return voteaiauditcontext.NormalizeConfig(voteaiauditcontext.Config{
		FastInputChars:            cfg.AIChat.FastInputChars,
		SummaryMaxChars:           cfg.AIChat.SummaryMaxChars,
		ToolTurnMaxChars:          min(1000, cfg.AIChat.FastInputChars/3),
		RecentUserTurns:           cfg.AIChat.RecentUserTurns,
		ReasonMaxChars:            200,
		RecentReasonLimit:         3,
		FullReviewThreshold:       cfg.AIChat.FullReviewThreshold,
		CumulativeReviewThreshold: min(cfg.AIChat.FullReviewThreshold, 0.30),
		RiskRiseThreshold:         cfg.AIChat.FullReviewRiskDelta,
		RiskFallThreshold:         0.10,
		HistoryRiskThreshold:      0.20,
		ObserveThreshold:          cfg.AIChat.ObserveThreshold,
		BlockThreshold:            cfg.AIChat.ConfidenceThreshold,
		PeriodicFullReviewTurns:   cfg.AIChat.PeriodicFullReviewTurns,
		RiskHalfLife:              time.Duration(cfg.AIChat.SessionRiskHalfLifeMinutes) * time.Minute,
	})
}

func (s *ContentModerationService) prepareIncrementalAudit(
	ctx context.Context,
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	content ContentModerationInput,
) (*contentModerationIncrementalPlan, error) {
	prepareStarted := time.Now()
	incrementalContextLoadMS := 0
	turns := make([]voteaiauditcontext.Turn, 0, len(content.Turns))
	userTurns := 0
	targetIndex := -1
	rawAuditTargetText := strings.TrimSpace(content.AuditTargetText)
	fullAuditTargetText := voteaiauditcontext.RedactSecrets(rawAuditTargetText)
	fastAuditTargetText := fullAuditTargetText
	fastAuditTargetTruncated := false
	if content.AuditTargetKind == string(voteaiinputprovenance.TargetToolContinuation) {
		// Tool continuations are auditable current content, but unlike an explicit
		// user turn they may contain megabytes of logs or command output. Fast and
		// full review use separate deterministic budgets. The local deterministic
		// detector has already scanned the complete normalized audit target before
		// either provider sample is built.
		fullAuditTargetText = normalizeContentModerationToolOutput(fullAuditTargetText)
		fastAuditTargetText = sampleContentModerationHeadMiddleTail(fullAuditTargetText, cfg.auditContextConfig().ToolTurnMaxChars)
		fastAuditTargetTruncated = fastAuditTargetText != fullAuditTargetText
	}
	latestUserText := fullAuditTargetText
	inputTruncated := false
	for _, turn := range content.Turns {
		if turn.Source == "trusted_metadata" || turn.Purpose == "ignored" {
			continue
		}
		text := strings.TrimSpace(turn.Text)
		if turn.Purpose == "audit_target" && fastAuditTargetText != "" {
			// Keep the lightweight fast view bounded. Historical redaction and
			// full-review rendering are deferred until escalation is confirmed.
			text = fastAuditTargetText
		}
		converted := voteaiauditcontext.Turn{
			Role:      voteaiauditcontext.Role(strings.ToLower(strings.TrimSpace(turn.Role))),
			Text:      text,
			ToolCall:  turn.ToolCall,
			Truncated: turn.Truncated,
		}
		if converted.Text == "" {
			continue
		}
		if converted.Role == voteaiauditcontext.RoleUser && strings.TrimSpace(converted.Text) != "" {
			userTurns++
		}
		if turn.Purpose == "audit_target" {
			targetIndex = len(turns)
		}
		inputTruncated = inputTruncated || converted.Truncated
		turns = append(turns, converted)
	}
	if targetIndex < 0 && latestUserText != "" {
		turns = append(turns, voteaiauditcontext.Turn{Role: voteaiauditcontext.RoleUser, Text: latestUserText})
		targetIndex = len(turns) - 1
		if content.AuditTargetKind == "user_request" {
			userTurns++
		}
	}

	plan := &contentModerationIncrementalPlan{
		latestUserText:       latestUserText,
		fullHistoryAvailable: len(turns) > 1,
		inputTruncated:       inputTruncated || fastAuditTargetTruncated,
		policyVersion:        contentModerationAuditPolicyVersion(cfg),
		eventAt:              time.Now().UTC(),
		reviewSourceTurns:    append([]ContentModerationTurn(nil), content.Turns...),
		reviewTargetKind:     content.AuditTargetKind,
		reviewTargetText:     fullAuditTargetText,
	}
	sessionKey, actorKey, _ := contentModerationRiskIdentity(input)
	plan.stateKey = sessionKey
	plan.sessionSource = normalizeContentModerationSessionSource(input.SessionSource, input.SessionID)
	plan.stableSession = sessionKey != ""
	if !plan.stableSession {
		plan.stateKey = actorKey
		plan.sessionSource = ContentModerationSessionSourceNone
	}
	if plan.stateKey != "" {
		if store, ok := s.hashCache.(ContentModerationAuditContextStore); ok {
			contextLoadStarted := time.Now()
			state, found, err := store.GetContentModerationAuditContext(ctx, plan.stateKey)
			incrementalContextLoadMS = max(0, int(time.Since(contextLoadStarted).Milliseconds()))
			if content.auditTimings != nil {
				addModerationElapsedMS(&content.auditTimings.contextLoadLatencyMS, contextLoadStarted)
			}
			if err != nil {
				slog.Warn("content_moderation.audit_context_get_failed", "error", err)
			} else if found {
				if strings.TrimSpace(state.PolicyVersion) != plan.policyVersion {
					slog.Info("content_moderation.audit_context_policy_reset",
						"previous_policy_version", state.PolicyVersion,
						"policy_version", plan.policyVersion,
						"session_source", plan.sessionSource)
					state = voteaiauditcontext.State{}
				}
				plan.state = state
				if !plan.stableSession {
					plan.state = lowWeightContentModerationActorState(state)
				}
			}
		}
	}
	if !plan.stableSession && userTurns > 1 {
		// Stateless clients that submit their full history can still receive the
		// periodic review without creating a cross-request identity.
		plan.state.TurnCount = userTurns - 1
		plan.state.LastFullReviewTurn = 0
	}

	fastTurns := turns
	if targetIndex >= 0 && fastAuditTargetTruncated {
		fastTurns = append([]voteaiauditcontext.Turn(nil), turns...)
		fastTurns[targetIndex].Truncated = true
	}
	fast, err := voteaiauditcontext.BuildFastAuditInputForTarget(fastTurns, voteaiauditcontext.AuditTarget{
		Kind:          content.AuditTargetKind,
		Text:          fastAuditTargetText,
		OriginalIndex: targetIndex,
	}, plan.state, cfg.auditContextConfig())
	if err != nil {
		return nil, err
	}
	if content.auditTimings != nil {
		fastBuildMS := max(0, int(time.Since(prepareStarted).Milliseconds())-incrementalContextLoadMS)
		content.auditTimings.fastBuildLatencyMS = auditIntPtr(fastBuildMS)
	}
	plan.fastInput = fast
	plan.inputTruncated = plan.inputTruncated || fast.Truncated
	return plan, nil
}

func (plan *contentModerationIncrementalPlan) ensureReviewInput(
	cfg *ContentModerationConfig,
	periodic bool,
	timings *contentModerationLatencyBreakdown,
) {
	if plan == nil || plan.reviewInputBuilt {
		return
	}
	startedAt := time.Now()
	turns, targetIndex, sourceTruncated := buildContentModerationReviewTurns(plan, cfg)
	if periodic {
		plan.canonicalPeriodic, plan.periodicInput = buildContentModerationPeriodicReviewInputForTurns(
			turns, targetIndex, plan.reviewTargetKind, plan.reviewTargetText, plan.state,
		)
		plan.fullInput = plan.periodicInput
		plan.canonicalFullPrefix = plan.canonicalPeriodic
		plan.fullHistoryTruncated = true
		plan.prefixCompacted = true
		plan.prefixHistoryRewrite = false
	} else {
		var fullTruncated bool
		plan.canonicalFullPrefix, plan.fullInput, fullTruncated, plan.prefixCompacted, plan.prefixHistoryRewrite = buildContentModerationFullReviewInputForTurns(
			turns, targetIndex, plan.reviewTargetKind, plan.reviewTargetText, plan.state, cfg,
		)
		plan.fullHistoryTruncated = sourceTruncated || fullTruncated
	}
	plan.inputTruncated = plan.inputTruncated || sourceTruncated || plan.fullHistoryTruncated
	plan.reviewInputBuilt = true
	if timings != nil {
		addModerationElapsedMS(&timings.reviewBuildLatencyMS, startedAt)
	}
}

func buildContentModerationReviewTurns(
	plan *contentModerationIncrementalPlan,
	cfg *ContentModerationConfig,
) (turns []voteaiauditcontext.Turn, targetIndex int, truncated bool) {
	if plan == nil {
		return nil, -1, false
	}
	turns = make([]voteaiauditcontext.Turn, 0, len(plan.reviewSourceTurns)+1)
	targetIndex = -1
	toolLimit := voteaiauditcontext.DefaultConfig().ToolTurnMaxChars
	if cfg != nil {
		toolLimit = cfg.auditContextConfig().ToolTurnMaxChars
	}
	for _, turn := range plan.reviewSourceTurns {
		if turn.Source == "trusted_metadata" || turn.Purpose == "ignored" {
			continue
		}
		text := strings.TrimSpace(turn.Text)
		if turn.Purpose == "audit_target" && plan.reviewTargetText != "" {
			text = plan.reviewTargetText
		} else if turn.Role == "tool" {
			text = sanitizeContentModerationToolOutput(text, toolLimit)
		} else {
			text = voteaiauditcontext.RedactSecrets(text)
		}
		converted := voteaiauditcontext.Turn{
			Role:      voteaiauditcontext.Role(strings.ToLower(strings.TrimSpace(turn.Role))),
			Text:      text,
			ToolCall:  turn.ToolCall,
			Truncated: turn.Truncated,
		}
		if converted.Text == "" {
			continue
		}
		if turn.Purpose == "audit_target" {
			targetIndex = len(turns)
		}
		truncated = truncated || converted.Truncated
		turns = append(turns, converted)
	}
	if targetIndex < 0 && plan.reviewTargetText != "" {
		turns = append(turns, voteaiauditcontext.Turn{Role: voteaiauditcontext.RoleUser, Text: plan.reviewTargetText})
		targetIndex = len(turns) - 1
	}
	return turns, targetIndex, truncated
}

// buildContentModerationPeriodicReviewInputForTurns provides a compact
// trajectory view only for clean sessions whose sole escalation reason is the
// periodic guard. Risk-driven and forced reviews always retain fullInput.
func buildContentModerationPeriodicReviewInputForTurns(
	turns []voteaiauditcontext.Turn,
	targetIndex int,
	targetKind, targetText string,
	state voteaiauditcontext.State,
) (canonical, input string) {
	targetKind = defaultContentModerationString(strings.TrimSpace(targetKind), "user_request")
	targetText = voteaiauditcontext.RedactSecrets(strings.TrimSpace(targetText))
	if targetIndex < 0 || targetIndex >= len(turns) {
		turns = append(turns, voteaiauditcontext.Turn{Role: voteaiauditcontext.RoleUser, Text: targetText})
		targetIndex = len(turns) - 1
	}

	start := targetIndex
	userTurns := 0
	for index := targetIndex; index >= 0; index-- {
		if turns[index].Role == voteaiauditcontext.RoleUser {
			userTurns++
			if userTurns > periodicReviewUserTurns {
				break
			}
		}
		start = index
	}

	selected := append([]voteaiauditcontext.Turn(nil), turns[start:targetIndex+1]...)
	visibleTarget := targetIndex - start
	for index := range selected {
		text := voteaiauditcontext.RedactSecrets(strings.TrimSpace(selected[index].Text))
		limit := periodicReviewAssistantChars
		switch selected[index].Role {
		case voteaiauditcontext.RoleSystem:
			limit = periodicReviewSystemChars
		case voteaiauditcontext.RoleUser:
			limit = periodicReviewUserChars
		}
		if index == visibleTarget {
			text = targetText
			limit = periodicReviewTargetChars
		}
		selected[index].Text = sampleContentModerationHeadMiddleTail(text, limit)
		selected[index].Truncated = len([]rune(text)) > len([]rune(selected[index].Text))
	}

	targetRole := strings.ToUpper(strings.TrimSpace(string(selected[visibleTarget].Role)))
	if targetRole == "" {
		targetRole = "USER"
	}
	conversation := renderContentModerationAuditTurns(selected)
	canonical = "[PERIODIC-RISK-TRAJECTORY last_user_turns=10]\n" + strings.TrimSpace(conversation)
	input = canonical + "\n\n" + contentModerationAuditTargetLocator(targetKind, visibleTarget+1, targetRole)
	if summary := contentModerationAuditStateSummary(state); summary != "" {
		input += "\n\n" + summary
	}
	return canonical, input
}

func contentModerationUsesPeriodicTrajectory(reasons []string) bool {
	return len(reasons) == 1 && reasons[0] == voteaiauditcontext.ReviewReasonPeriodic
}

func lowWeightContentModerationActorState(state voteaiauditcontext.State) voteaiauditcontext.State {
	state = voteaiauditcontext.NumericRiskOnlyState(state)
	state.CurrentScore = math.Min(0.20, state.CurrentScore*0.25)
	state.MaxScore = math.Min(0.30, state.MaxScore*0.25)
	state.Tier = voteaiauditcontext.TierLow
	return state
}

// buildContentModerationFullReviewInputForTurns keeps an append-stable history
// anchor after compaction. It reconstructs the previous anchor from the opaque
// prefix hash, so Redis never needs to store raw conversation text.
func buildContentModerationFullReviewInputForTurns(
	turns []voteaiauditcontext.Turn,
	targetIndex int,
	targetKind, targetText string,
	state voteaiauditcontext.State,
	cfg *ContentModerationConfig,
) (canonical, input string, truncated, compacted, rewritten bool) {
	limit := defaultAIChatFullReviewMaxInputChars
	if cfg != nil && cfg.AIChat.FullReviewMaxInputChars > 0 {
		limit = cfg.AIChat.FullReviewMaxInputChars
	}
	summary := contentModerationAuditStateSummary(state)
	targetKind = defaultContentModerationString(strings.TrimSpace(targetKind), "user_request")
	targetText = voteaiauditcontext.RedactSecrets(strings.TrimSpace(targetText))
	if targetIndex < 0 || targetIndex >= len(turns) {
		turns = append(turns, voteaiauditcontext.Turn{Role: voteaiauditcontext.RoleUser, Text: targetText})
		targetIndex = len(turns) - 1
	}
	turns = append([]voteaiauditcontext.Turn(nil), turns...)
	turns[targetIndex].Text = targetText
	targetRole := strings.ToUpper(strings.TrimSpace(string(turns[targetIndex].Role)))
	if targetRole == "" {
		targetRole = "USER"
	}
	targetLocator := contentModerationAuditTargetLocator(targetKind, targetIndex+1, targetRole)
	fixedChars := len([]rune("[CONVERSATION-HISTORY]\n")) +
		len([]rune("["+targetRole+"]\n")) +
		len([]rune("\n\n"+targetLocator))
	if summary != "" {
		fixedChars += len([]rune("\n\n" + summary))
	}
	targetBudget := max(1, limit-fixedChars)
	if len([]rune(targetText)) > targetBudget {
		targetText = sampleContentModerationHeadMiddleTail(targetText, targetBudget)
		turns[targetIndex].Text = targetText
		turns[targetIndex].Truncated = true
		truncated = true
	}
	targetConversationBlock := renderContentModerationAuditTurns(turns[targetIndex : targetIndex+1])

	conversationLimit := max(0, limit-len([]rune(summary))-len([]rune(targetLocator))-32)
	anchor, anchorMatched := findContentModerationPrefixAnchor(turns, state)
	hadPreviousAnchor := strings.TrimSpace(state.CanonicalPrefixHash) != "" && state.LastPrefixChars > 0
	if anchor > targetIndex {
		anchor = 0
	}
	start := anchor
	conversation, visibleTurns := renderContentModerationAuditConversation(turns, start)
	if len([]rune(conversation)) > conversationLimit {
		// Rebase to roughly 70% of the available history budget so subsequent
		// append-only reviews can reuse this epoch instead of dropping one turn
		// and rotating the prefix on every request.
		rebaseLimit := max(len([]rune(targetConversationBlock)), conversationLimit*70/100)
		for len([]rune(conversation)) > rebaseLimit && start < targetIndex {
			start++
			conversation, visibleTurns = renderContentModerationAuditConversation(turns, start)
		}
	}
	for len([]rune(conversation)) > conversationLimit && start < targetIndex {
		start++
		conversation, visibleTurns = renderContentModerationAuditConversation(turns, start)
	}
	if len([]rune(conversation)) > conversationLimit {
		// Supporting turns after the target are lower priority than the target
		// itself. Retain the complete target rather than trimming its middle.
		conversation = targetConversationBlock
		start = targetIndex
		visibleTurns = map[int]int{targetIndex: 1}
		truncated = true
	}
	if start > 0 {
		truncated = true
	}
	compacted = start > anchor
	// A prior provider-observed prefix that cannot be reconstructed from the
	// normalized history is a rewrite, even when the new history also needs
	// compaction. Reporting it as a compaction would hide the actual cache break.
	if hadPreviousAnchor && !anchorMatched {
		compacted = false
		rewritten = true
	}
	targetVisibleIndex := visibleTurns[targetIndex]
	if targetVisibleIndex <= 0 {
		targetVisibleIndex = 1
	}
	targetLocator = contentModerationAuditTargetLocator(targetKind, targetVisibleIndex, targetRole)
	canonical = "[CONVERSATION-HISTORY]\n" + strings.TrimSpace(conversation)
	input = canonical + "\n\n" + targetLocator
	if summary != "" {
		input += "\n\n" + summary
	}
	return canonical, input, truncated, compacted, rewritten
}

func contentModerationAuditTargetLocator(targetKind string, visibleIndex int, targetRole string) string {
	return fmt.Sprintf("[AUDIT-TARGET-LOCATOR kind=%s turn_index=%d role=%s]\nOnly the referenced conversation turn is the current decision target.", targetKind, visibleIndex, targetRole)
}

// renderContentModerationAuditConversation preserves an early system/first
// user anchor when middle history is compacted. The returned indexes are the
// one-based positions visible to the model; compaction markers are not turns.
func renderContentModerationAuditConversation(turns []voteaiauditcontext.Turn, start int) (string, map[int]int) {
	if start < 0 {
		start = 0
	}
	if start > len(turns) {
		start = len(turns)
	}
	indexes := make([]int, 0, len(turns))
	if start > 0 {
		indexes = append(indexes, contentModerationAuditAnchorIndexes(turns, start)...)
	}
	for index := start; index < len(turns); index++ {
		indexes = append(indexes, index)
	}

	parts := make([]string, 0, len(indexes)+1)
	visible := make(map[int]int, len(indexes))
	anchorCount := 0
	if start > 0 {
		anchorCount = len(contentModerationAuditAnchorIndexes(turns, start))
	}
	position := 0
	for offset, index := range indexes {
		if start > 0 && offset == anchorCount {
			parts = append(parts, "[EARLIER-CONTEXT-COMPACTED]")
		}
		text := strings.TrimSpace(turns[index].Text)
		if index < start {
			text = contentModerationPinnedAnchorText(turns[index].Role, text)
		}
		if text == "" {
			continue
		}
		position++
		visible[index] = position
		parts = append(parts, fmt.Sprintf("[%s]\n%s", strings.ToUpper(string(turns[index].Role)), text))
	}
	if start > 0 && anchorCount == len(indexes) {
		parts = append(parts, "[EARLIER-CONTEXT-COMPACTED]")
	}
	return strings.Join(parts, "\n\n"), visible
}

func contentModerationPinnedAnchorText(role voteaiauditcontext.Role, text string) string {
	limit := contentModerationPinnedUserChars
	if role == voteaiauditcontext.RoleSystem {
		limit = contentModerationPinnedSystemChars
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit || limit < 1 {
		return string(runes)
	}
	marker := []rune("\n[EARLY-CONTEXT-TRUNCATED]\n")
	budget := limit - len(marker)
	if budget <= 1 {
		return string(runes[:limit])
	}
	head := budget * 2 / 3
	tail := budget - head
	return string(runes[:head]) + string(marker) + string(runes[len(runes)-tail:])
}

func contentModerationAuditAnchorIndexes(turns []voteaiauditcontext.Turn, before int) []int {
	if before > len(turns) {
		before = len(turns)
	}
	firstSystem := -1
	firstUser := -1
	for index := 0; index < before; index++ {
		if strings.TrimSpace(turns[index].Text) == "" {
			continue
		}
		switch turns[index].Role {
		case voteaiauditcontext.RoleSystem:
			if firstSystem < 0 {
				firstSystem = index
			}
		case voteaiauditcontext.RoleUser:
			if firstUser < 0 {
				firstUser = index
			}
		}
		if firstSystem >= 0 && firstUser >= 0 {
			break
		}
	}
	anchors := make([]int, 0, 2)
	if firstSystem >= 0 {
		anchors = append(anchors, firstSystem)
	}
	if firstUser >= 0 && firstUser != firstSystem {
		anchors = append(anchors, firstUser)
	}
	sort.Ints(anchors)
	return anchors
}

func findContentModerationPrefixAnchor(turns []voteaiauditcontext.Turn, state voteaiauditcontext.State) (int, bool) {
	previousHash := strings.TrimSpace(state.CanonicalPrefixHash)
	previousChars := state.LastPrefixChars
	if previousHash == "" || previousChars <= 0 || len(turns) == 0 {
		return 0, false
	}
	for start := 0; start < len(turns); start++ {
		conversation, _ := renderContentModerationAuditConversation(turns, start)
		candidate := "[CONVERSATION-HISTORY]\n" + strings.TrimSpace(conversation)
		runes := []rune(candidate)
		if len(runes) < previousChars {
			continue
		}
		if opaqueModerationRiskHash("audit-prefix", string(runes[:previousChars])) == previousHash {
			return start, true
		}
	}
	return 0, false
}

func renderContentModerationAuditTurns(turns []voteaiauditcontext.Turn) string {
	parts := make([]string, 0, len(turns))
	for _, turn := range turns {
		text := strings.TrimSpace(turn.Text)
		if text == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("[%s]\n%s", strings.ToUpper(string(turn.Role)), text))
	}
	return strings.Join(parts, "\n\n")
}

func sanitizeContentModerationToolOutput(value string, maxChars int) string {
	value = normalizeContentModerationToolOutput(value)
	if maxChars <= 0 {
		maxChars = 1000
	}
	return trimModerationContext(value, maxChars)
}

func normalizeContentModerationToolOutput(value string) string {
	value = voteaiauditcontext.RedactSecrets(strings.ReplaceAll(value, "\r\n", "\n"))
	value = strings.ReplaceAll(value, "\r", "\n")
	seen := make(map[string]struct{})
	lines := make([]string, 0, 64)
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if line == "" || strings.HasPrefix(lower, "wall time:") || strings.HasPrefix(lower, "process exited") ||
			strings.HasPrefix(lower, "total output lines:") {
			continue
		}
		if _, duplicate := seen[line]; duplicate {
			continue
		}
		seen[line] = struct{}{}
		lines = append(lines, line)
	}
	value = strings.Join(lines, "\n")
	return value
}

func sampleContentModerationHeadMiddleTail(value string, maxChars int) string {
	value = strings.TrimSpace(value)
	if maxChars <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}

	marker := []rune("\n\n[CONTEXT OMITTED]\n\n")
	markerChars := len(marker) * 2
	if maxChars <= markerChars+2 {
		// The normal configuration never reaches this branch, but keep all three
		// source regions represented even under a malformed tiny budget.
		head := maxChars / 3
		middle := (maxChars - head) / 2
		tail := maxChars - head - middle
		midStart := max(0, len(runes)/2-middle/2)
		return string(runes[:head]) + string(runes[midStart:midStart+middle]) + string(runes[len(runes)-tail:])
	}

	available := maxChars - markerChars
	headChars := (available + 2) / 3
	middleChars := (available + 1) / 3
	tailChars := available - headChars - middleChars
	middleStart := len(runes)/2 - middleChars/2
	if middleStart < headChars {
		middleStart = headChars
	}
	if middleStart+middleChars > len(runes)-tailChars {
		middleStart = len(runes) - tailChars - middleChars
	}

	return string(runes[:headChars]) + string(marker) +
		string(runes[middleStart:middleStart+middleChars]) + string(marker) +
		string(runes[len(runes)-tailChars:])
}

func contentModerationAuditStateSummary(state voteaiauditcontext.State) string {
	categories := append([]string(nil), state.Categories...)
	signals := append([]string(nil), state.Signals...)
	sort.Strings(categories)
	sort.Strings(signals)
	return fmt.Sprintf(
		"[PRIOR-RISK-STATE]\nturn=%d\ntier=%s\ntrend=%s\nscore=%.2f\ncategories=%s\nsignals=%s",
		state.TurnCount,
		defaultContentModerationString(state.Tier, voteaiauditcontext.TierLow),
		defaultContentModerationString(state.Trend, voteaiauditcontext.TrendStable),
		state.CurrentScore,
		defaultContentModerationString(strings.Join(categories, ","), "none"),
		defaultContentModerationString(strings.Join(signals, ","), "none"),
	)
}

func contentModerationAuditPolicyVersion(cfg *ContentModerationConfig) string {
	if cfg == nil {
		return "unknown"
	}
	promptVersion, _ := voteaimoderation.ClassifySystemPrompt(cfg.AIChat.SystemPrompt)
	promptHash := sha256.Sum256([]byte(voteaimoderation.NormalizeSystemPrompt(cfg.AIChat.SystemPrompt)))
	payload := strings.Join([]string{
		"v6",
		cfg.AuditProvider,
		strings.TrimSpace(cfg.AIChat.BaseURL),
		strings.TrimSpace(cfg.AIChat.Model),
		promptVersion,
		hex.EncodeToString(promptHash[:]),
		fmt.Sprintf("mode=%s", cfg.Mode),
		fmt.Sprintf("sample_rate=%d", cfg.SampleRate),
		fmt.Sprintf("pre_hash=%t", cfg.PreHashCheckEnabled),
		fmt.Sprintf("incremental=%t", cfg.AIChat.IncrementalAuditEnabled),
		fmt.Sprintf("provenance=%t", cfg.AIChat.InputProvenanceV2Enabled),
		fmt.Sprintf("provenance_version=%s", voteaiinputprovenance.NormalizationVersion),
		fmt.Sprintf("deterministic=%t", cfg.AIChat.DeterministicRiskV2Enabled),
		fmt.Sprintf("deterministic_rule=%s", voteaideterministicrisk.RuleCredentialBypassV2Version),
		fmt.Sprintf("audit_envelope=%s", voteaimoderation.AuditEnvelopeVersion),
		fmt.Sprintf("audit_state_version=%d", voteaiauditcontext.StateVersion),
		fmt.Sprintf("confidence_threshold=%.4f", cfg.AIChat.ConfidenceThreshold),
		fmt.Sprintf("observe_threshold=%.4f", cfg.AIChat.ObserveThreshold),
		fmt.Sprintf("category_thresholds=%s", contentModerationAuditThresholdFingerprint(cfg.activeThresholds())),
		fmt.Sprintf("full_threshold=%.4f", cfg.AIChat.FullReviewThreshold),
		fmt.Sprintf("risk_delta=%.4f", cfg.AIChat.FullReviewRiskDelta),
		fmt.Sprintf("risk_levels=%t", cfg.AIChat.RiskLevelsEnabled),
		fmt.Sprintf("session_risk=%t", cfg.AIChat.SessionRiskEnabled),
		fmt.Sprintf("actor_risk=%t", cfg.AIChat.ActorRiskEnabled),
		fmt.Sprintf("session_risk_ttl_minutes=%d", cfg.AIChat.SessionRiskTTLMinutes),
		fmt.Sprintf("risk_half_life_minutes=%d", cfg.AIChat.SessionRiskHalfLifeMinutes),
		fmt.Sprintf("session_block_cooldown_minutes=%d", cfg.AIChat.SessionRiskBlockCooldownMinutes),
		fmt.Sprintf("max_input=%d", cfg.AIChat.MaxInputChars),
		fmt.Sprintf("fast_input=%d", cfg.AIChat.FastInputChars),
		fmt.Sprintf("fallback_input=%d", cfg.AIChat.FallbackInputChars),
		fmt.Sprintf("summary=%d", cfg.AIChat.SummaryMaxChars),
		fmt.Sprintf("full_input=%d", cfg.AIChat.FullReviewMaxInputChars),
		fmt.Sprintf("recent=%d", cfg.AIChat.RecentUserTurns),
		fmt.Sprintf("periodic=%d", cfg.AIChat.PeriodicFullReviewTurns),
		fmt.Sprintf("audit_context_ttl_minutes=%d", cfg.AIChat.AuditContextTTLMinutes),
		fmt.Sprintf("sync_budget_ms=%d", cfg.AIChat.SynchronousBudgetMS),
		fmt.Sprintf("timeout_ms=%d", cfg.AIChat.TimeoutMS),
		fmt.Sprintf("failure_policy=%s", cfg.AIChat.FailurePolicy),
		fmt.Sprintf("thinking_mode=%s", cfg.AIChat.ThinkingMode),
		fmt.Sprintf("reasoning_effort=%s", cfg.AIChat.ReasoningEffort),
		fmt.Sprintf("fast_output=%d", cfg.AIChat.FastMaxOutputTokens),
		fmt.Sprintf("full_output=%d", cfg.AIChat.FullMaxOutputTokens),
		fmt.Sprintf("max_output=%d", cfg.AIChat.MaxReviewMaxOutputTokens),
	}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return "v6-" + hex.EncodeToString(sum[:8])
}

func contentModerationAuditThresholdFingerprint(thresholds map[string]float64) string {
	keys := make([]string, 0, len(thresholds))
	for key := range thresholds {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%.6f", strings.TrimSpace(key), thresholds[key]))
	}
	return strings.Join(parts, ",")
}

func contentModerationAuditRiskDigest(state voteaiauditcontext.State, policyVersion string, cfg *ContentModerationConfig) string {
	categories := append([]string(nil), state.Categories...)
	signals := append([]string(nil), state.Signals...)
	sort.Strings(categories)
	sort.Strings(signals)
	periodic := defaultAIChatPeriodicFullReviewTurns
	if cfg != nil && cfg.AIChat.PeriodicFullReviewTurns > 0 {
		periodic = cfg.AIChat.PeriodicFullReviewTurns
	}
	turnBucket := state.TurnCount / periodic
	tier := strings.ToLower(strings.TrimSpace(state.Tier))
	if tier == "" {
		tier = voteaiauditcontext.TierLow
	}
	trend := strings.ToLower(strings.TrimSpace(state.Trend))
	if trend == "" {
		trend = voteaiauditcontext.TrendStable
	}
	payload := fmt.Sprintf("%s\x00%d\x00%s\x00%s\x00%s\x00%s",
		policyVersion, turnBucket, tier, trend,
		strings.Join(categories, ","), strings.Join(signals, ","))
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:16])
}

func (s *ContentModerationService) recordContentModerationAuditUsage(result *moderationAPIResult, inputChars int, configs ...*ContentModerationConfig) {
	if s == nil || result == nil {
		return
	}
	if result.LocalDecision || result.ResultCacheHit {
		inputChars = 0
	} else if result.InputChars > 0 {
		inputChars = result.InputChars
	}
	if inputChars > 0 {
		s.auditInputChars.Add(int64(inputChars))
	}
	if result.LocalDecision {
		return
	}
	if result.ResultCacheHit {
		s.auditResultCacheHits.Add(1)
		return
	}
	s.recordContentModerationStageCall(result.Stage)
	if result.Usage == nil {
		s.auditUsageUnknown.Add(1)
		return
	}
	if !contentModerationUsageComplete(result.Usage) {
		s.auditUsageUnknown.Add(1)
		return
	}
	s.auditUsageComplete.Add(1)
	if result.Usage.PromptTokens != nil && *result.Usage.PromptTokens >= 0 {
		s.auditPromptTokens.Add(int64(*result.Usage.PromptTokens))
	}
	if result.Usage.CachedPromptTokens != nil && *result.Usage.CachedPromptTokens >= 0 {
		s.auditCachedInputTokens.Add(int64(*result.Usage.CachedPromptTokens))
	}
	if result.Usage.UncachedPromptTokens != nil && *result.Usage.UncachedPromptTokens >= 0 {
		s.auditUncachedInputTokens.Add(int64(*result.Usage.UncachedPromptTokens))
	}
	if result.Usage.CompletionTokens != nil && *result.Usage.CompletionTokens >= 0 {
		s.auditOutputTokens.Add(int64(*result.Usage.CompletionTokens))
	}
	var cfg *ContentModerationConfig
	if len(configs) > 0 {
		cfg = configs[0]
	}
	s.recordContentModerationAuditCost(result.Usage, cfg)
}

func (s *ContentModerationService) recordContentModerationAuditFailure(stage voteaimoderation.ReviewStage, latencyMS, inputChars int) {
	if s == nil {
		return
	}
	s.auditUsageUnknown.Add(1)
	if inputChars > 0 {
		s.auditInputChars.Add(int64(inputChars))
	}
	s.recordContentModerationStageCall(stage)
	s.recordContentModerationStageLatency(stage, latencyMS, nil)
}

func (s *ContentModerationService) recordContentModerationStageCall(stage voteaimoderation.ReviewStage) {
	if s == nil {
		return
	}
	switch stage {
	case voteaimoderation.StageFast:
		s.auditFastCalls.Add(1)
	case voteaimoderation.StageFull:
		s.auditFullCalls.Add(1)
	case voteaimoderation.StageMax:
		s.auditMaxCalls.Add(1)
	}
}

func (s *ContentModerationService) recordContentModerationStageLatency(stage voteaimoderation.ReviewStage, latencyMS int, result *moderationAPIResult) {
	if s == nil || (result != nil && (result.LocalDecision || result.ResultCacheHit)) {
		return
	}
	switch stage {
	case voteaimoderation.StageFast:
		s.auditFastLatency.observe(latencyMS)
	case voteaimoderation.StageFull:
		s.auditFullLatency.observe(latencyMS)
	case voteaimoderation.StageMax:
		s.auditMaxLatency.observe(latencyMS)
	}
}

func (s *ContentModerationService) recordContentModerationSessionSource(source string) {
	if s == nil {
		return
	}
	switch strings.TrimSpace(source) {
	case ContentModerationSessionSourceHeader:
		s.auditSessionHeader.Add(1)
	case ContentModerationSessionSourcePromptCacheKey:
		s.auditSessionPromptCache.Add(1)
	default:
		s.auditSessionNone.Add(1)
	}
}

func contentModerationConfiguredAuditStage(cfg *ContentModerationConfig) voteaimoderation.ReviewStage {
	if cfg == nil {
		return voteaimoderation.StageFull
	}
	switch voteaimoderation.ReviewStage(strings.TrimSpace(cfg.AIChat.auditStage)) {
	case voteaimoderation.StageFast:
		return voteaimoderation.StageFast
	case voteaimoderation.StageMax:
		return voteaimoderation.StageMax
	case voteaimoderation.StageFull:
		return voteaimoderation.StageFull
	}
	if strings.TrimSpace(cfg.AIChat.ThinkingMode) == "disabled" {
		return voteaimoderation.StageFast
	}
	if strings.TrimSpace(cfg.AIChat.ReasoningEffort) == "max" {
		return voteaimoderation.StageMax
	}
	return voteaimoderation.StageFull
}

func contentModerationSuccessfulStageDetails(stage voteaimoderation.ReviewStage, result *moderationAPIResult, latencyMS int) ContentModerationAuditStageDetails {
	details := ContentModerationAuditStageDetails{Stage: string(stage), LatencyMS: auditIntPtr(max(0, latencyMS))}
	if result == nil {
		return details
	}
	if result.Stage != "" {
		details.Stage = string(result.Stage)
	}
	details.ResultCacheHit = result.ResultCacheHit
	details.ProviderCalled = !result.LocalDecision && !result.ResultCacheHit
	details.InputChars = auditIntPtr(contentModerationResultInputChars(result))
	details.UsageKnown = details.ProviderCalled && contentModerationUsageComplete(result.Usage)
	if result.Usage != nil {
		details.PromptTokens = cloneIntPtr(result.Usage.PromptTokens)
		details.CachedInputTokens = cloneIntPtr(result.Usage.CachedPromptTokens)
		details.UncachedInputTokens = cloneIntPtr(result.Usage.UncachedPromptTokens)
		details.OutputTokens = cloneIntPtr(result.Usage.CompletionTokens)
	}
	return details
}

func contentModerationFailedStageDetails(stage voteaimoderation.ReviewStage, latencyMS, inputChars int) ContentModerationAuditStageDetails {
	return ContentModerationAuditStageDetails{
		Stage:          string(stage),
		ProviderCalled: true,
		Failed:         true,
		InputChars:     auditIntPtr(max(0, inputChars)),
		LatencyMS:      auditIntPtr(max(0, latencyMS)),
	}
}

func contentModerationSetStageDetails(result *moderationAPIResult, details ...ContentModerationAuditStageDetails) {
	if result == nil {
		return
	}
	result.StageDetails = append([]ContentModerationAuditStageDetails(nil), details...)
	allCached := len(details) > 0
	for _, stage := range details {
		if !stage.ResultCacheHit || stage.ProviderCalled || stage.Failed {
			allCached = false
			break
		}
	}
	result.ResultCacheHit = allCached
}

func contentModerationMergeCalledStageUsage(failed bool, results ...*moderationAPIResult) *voteaimoderation.Usage {
	usages := make([]*voteaimoderation.Usage, 0, len(results)+1)
	for _, result := range results {
		if result == nil || result.LocalDecision || result.ResultCacheHit {
			continue
		}
		usages = append(usages, result.Usage)
	}
	if failed {
		usages = append(usages, nil)
	}
	if len(usages) == 0 {
		return nil
	}
	return voteaimoderation.MergeStageUsage(usages...)
}

func contentModerationFullStagePromptTokens(result *moderationAPIResult) (int64, bool) {
	if result == nil {
		return 0, false
	}
	for index := len(result.StageDetails) - 1; index >= 0; index-- {
		stage := result.StageDetails[index]
		if stage.Stage != string(voteaimoderation.StageFull) && stage.Stage != string(voteaimoderation.StageMax) {
			continue
		}
		if !stage.ProviderCalled || stage.Failed || stage.ResultCacheHit || stage.PromptTokens == nil || *stage.PromptTokens < 0 {
			return 0, false
		}
		return int64(*stage.PromptTokens), true
	}
	// Compatibility for the legacy single-stage path.
	if (result.Stage == voteaimoderation.StageFull || result.Stage == voteaimoderation.StageMax) &&
		!result.ResultCacheHit && result.Usage != nil && result.Usage.PromptTokens != nil && *result.Usage.PromptTokens >= 0 {
		return int64(*result.Usage.PromptTokens), true
	}
	return 0, false
}

func (s *ContentModerationService) callIncrementalAIChatAudit(
	ctx context.Context,
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	content ContentModerationInput,
	trackKeyLoad bool,
) (*moderationAPIResult, *contentModerationIncrementalPlan, error) {
	plan, err := s.prepareIncrementalAudit(ctx, input, cfg, content)
	if err != nil {
		return nil, nil, err
	}
	if cfg.AIChat.forceFullReview {
		plan.escalationReasons = []string{voteaiauditcontext.ReviewReasonForced}
		plan.ensureReviewInput(cfg, false, content.auditTimings)
		fullCfg := cloneContentModerationConfig(cfg)
		fullCfg.AIChat.auditStage = string(voteaimoderation.StageFull)
		fullCfg.AIChat.riskStateDigest = contentModerationAuditRiskDigest(plan.state, plan.policyVersion, cfg)
		fullCfg.AIChat.MaxInputChars = max(fullCfg.AIChat.MaxInputChars, len([]rune(plan.fullInput)))
		stageStarted := time.Now()
		fullResult, fullErr := s.callModeration(ctx, fullCfg, plan.fullInput, trackKeyLoad)
		stageLatency := int(time.Since(stageStarted).Milliseconds())
		if content.auditTimings != nil {
			addModerationElapsedMS(&content.auditTimings.providerLatencyMS, stageStarted)
		}
		if fullErr != nil {
			s.recordContentModerationAuditFailure(voteaimoderation.StageFull, stageLatency, voteaimoderation.AttemptedInputChars(fullErr))
			return nil, plan, fullErr
		}
		contentModerationSetStageDetails(fullResult, contentModerationSuccessfulStageDetails(voteaimoderation.StageFull, fullResult, stageLatency))
		s.recordContentModerationStageLatency(voteaimoderation.StageFull, stageLatency, fullResult)
		plan.inputChars = contentModerationResultInputChars(fullResult)
		s.recordContentModerationAuditUsage(fullResult, len([]rune(plan.fullInput)), cfg)
		s.updateContentModerationAuditContext(ctx, input, cfg, plan, fullResult, true)
		s.updateContentModerationAuditPrefix(ctx, input, cfg, plan, fullResult)
		return fullResult, plan, nil
	}
	fastCfg := cloneContentModerationConfig(cfg)
	fastCfg.AIChat.auditStage = string(voteaimoderation.StageFast)
	fastCfg.AIChat.riskStateDigest = contentModerationAuditRiskDigest(plan.state, plan.policyVersion, cfg)
	fastStarted := time.Now()
	fastCtx, cancelFast := contentModerationFastStageContext(ctx, cfg.AIChat.FastStageBudgetMS)
	fastResult, err := s.callModeration(fastCtx, fastCfg, plan.fastInput.Text, trackKeyLoad)
	cancelFast()
	fastLatency := int(time.Since(fastStarted).Milliseconds())
	if content.auditTimings != nil {
		addModerationElapsedMS(&content.auditTimings.providerLatencyMS, fastStarted)
	}
	if err != nil {
		s.recordContentModerationAuditFailure(voteaimoderation.StageFast, fastLatency, voteaimoderation.AttemptedInputChars(err))
		return nil, plan, err
	}
	contentModerationSetStageDetails(fastResult, contentModerationSuccessfulStageDetails(voteaimoderation.StageFast, fastResult, fastLatency))
	s.recordContentModerationStageLatency(voteaimoderation.StageFast, fastLatency, fastResult)
	plan.inputChars = contentModerationResultInputChars(fastResult)
	if fastResult.LocalDecision {
		s.updateContentModerationAuditContext(ctx, input, cfg, plan, fastResult, false)
		return fastResult, plan, nil
	}

	decision := voteaiauditcontext.DecideFullReview(plan.state, voteaiauditcontext.ReviewInput{
		FastScore:            fastResult.CategoryScores["ai_risk"],
		Categories:           append([]string(nil), fastResult.Categories...),
		Signals:              append([]string(nil), fastResult.Signals...),
		LatestUserText:       plan.latestUserText,
		InputTruncated:       plan.inputTruncated,
		StableSession:        plan.stableSession,
		FullHistoryAvailable: plan.fullHistoryAvailable,
		Force:                cfg.AIChat.supplementalReview,
		At:                   time.Now().UTC(),
	}, cfg.auditContextConfig())
	plan.escalationReasons = append([]string(nil), decision.Reasons...)
	if !decision.Required {
		s.recordContentModerationAuditUsage(fastResult, len([]rune(plan.fastInput.Text)), cfg)
		s.updateContentModerationAuditContext(ctx, input, cfg, plan, fastResult, false)
		return fastResult, plan, nil
	}
	plan.ensureReviewInput(cfg, contentModerationUsesPeriodicTrajectory(decision.Reasons), content.auditTimings)

	stage := voteaimoderation.StageFull
	if voteaiauditcontext.HasStrongSignal(fastResult.Signals) && fastResult.CategoryScores["ai_risk"] >= cfg.AIChat.ConfidenceThreshold {
		stage = voteaimoderation.StageMax
	}
	fullCfg := cloneContentModerationConfig(cfg)
	fullCfg.AIChat.auditStage = string(stage)
	fullCfg.AIChat.riskStateDigest = contentModerationAuditRiskDigest(plan.state, plan.policyVersion, cfg)
	fullCfg.AIChat.MaxInputChars = max(fullCfg.AIChat.MaxInputChars, len([]rune(plan.fullInput)))
	fullStarted := time.Now()
	fullResult, reviewErr := s.callModeration(ctx, fullCfg, plan.fullInput, trackKeyLoad)
	fullLatency := int(time.Since(fullStarted).Milliseconds())
	if content.auditTimings != nil {
		addModerationElapsedMS(&content.auditTimings.providerLatencyMS, fullStarted)
	}
	s.recordContentModerationAuditUsage(fastResult, len([]rune(plan.fastInput.Text)), cfg)
	if reviewErr != nil {
		s.recordContentModerationAuditFailure(stage, fullLatency, voteaimoderation.AttemptedInputChars(reviewErr))
		fastResult.InputChars = contentModerationResultInputChars(fastResult) + voteaimoderation.AttemptedInputChars(reviewErr)
		fastResult.Usage = contentModerationMergeCalledStageUsage(true, fastResult)
		stageDetails := append(cloneContentModerationStageDetails(fastResult.StageDetails), contentModerationFailedStageDetails(stage, fullLatency, voteaimoderation.AttemptedInputChars(reviewErr)))
		contentModerationSetStageDetails(fastResult, stageDetails...)
		plan.inputChars = fastResult.InputChars
		fastResult.ReviewIncomplete = true
		fastResult.ReviewError = trimRunes(reviewErr.Error(), 500)
		return fastResult, plan, nil
	}
	fullStageDetails := contentModerationSuccessfulStageDetails(stage, fullResult, fullLatency)
	s.recordContentModerationStageLatency(stage, fullLatency, fullResult)
	s.recordContentModerationAuditUsage(fullResult, len([]rune(plan.fullInput)), cfg)
	plan.inputChars = contentModerationResultInputChars(fastResult) + contentModerationResultInputChars(fullResult)
	fullResult.InputChars = plan.inputChars
	fullResult.Usage = contentModerationMergeCalledStageUsage(false, fastResult, fullResult)
	stageDetails := append(cloneContentModerationStageDetails(fastResult.StageDetails), fullStageDetails)
	contentModerationSetStageDetails(fullResult, stageDetails...)
	s.updateContentModerationAuditContext(ctx, input, cfg, plan, fullResult, true)
	s.updateContentModerationAuditPrefix(ctx, input, cfg, plan, fullResult)
	slog.Info("content_moderation.full_review_completed", "reasons", decision.Reasons, "stage", fullResult.Stage)
	return fullResult, plan, nil
}

func contentModerationResultInputChars(result *moderationAPIResult) int {
	if result == nil || result.LocalDecision || result.ResultCacheHit || result.InputChars < 1 {
		return 0
	}
	return result.InputChars
}

func (s *ContentModerationService) updateContentModerationAuditContext(
	ctx context.Context,
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	plan *contentModerationIncrementalPlan,
	result *moderationAPIResult,
	fullReview bool,
) {
	if s == nil || cfg == nil || cfg.sideEffectsDisabled || plan == nil || result == nil || plan.stateKey == "" || result.ReviewIncomplete {
		return
	}
	store, ok := s.hashCache.(ContentModerationAuditContextStore)
	if !ok {
		return
	}
	event := voteaiauditcontext.AuditEvent{
		RiskScore:       result.CategoryScores["ai_risk"],
		Categories:      append([]string(nil), result.Categories...),
		Signals:         append([]string(nil), result.Signals...),
		Reason:          contentModerationPersistedAuditReason(result, cfg),
		RequestID:       input.RequestID,
		PolicyVersion:   plan.policyVersion,
		FullReview:      fullReview,
		TurnIncrement:   1,
		At:              plan.eventAt,
		NumericRiskOnly: !plan.stableSession,
	}
	if event.NumericRiskOnly {
		// Enforce the privacy boundary before invoking the storage interface, not
		// only inside the current Redis implementation.
		event.Categories = nil
		event.Signals = nil
		event.Reason = ""
	}
	state, err := store.UpdateContentModerationAuditContextForUser(
		ctx, input.UserID, plan.stateKey, event,
		cfg.auditContextConfig(), time.Duration(cfg.AIChat.AuditContextTTLMinutes)*time.Minute,
	)
	if err != nil {
		slog.Warn("content_moderation.audit_context_update_failed", "error", err)
		return
	}
	plan.state = state
}

// Low-risk explanations remain in the audit log. Persisting them in session
// state would make the next fast audit replay an otherwise unrelated user turn.
func contentModerationPersistedAuditReason(result *moderationAPIResult, cfg *ContentModerationConfig) string {
	if result == nil {
		return ""
	}
	threshold := voteaiauditcontext.DefaultConfig().HistoryRiskThreshold
	if cfg != nil {
		threshold = cfg.auditContextConfig().HistoryRiskThreshold
	}
	if result.CategoryScores["ai_risk"] < threshold && len(result.Categories) == 0 && len(result.Signals) == 0 {
		return ""
	}
	return result.Reason
}

func (s *ContentModerationService) recordContentModerationPrefixContinuity(state voteaiauditcontext.State) {
	if s == nil {
		return
	}
	if state.PrefixBaseline && !state.PrefixContinuity && strings.TrimSpace(state.PrefixBreakReason) == "" {
		return
	}
	if state.PrefixContinuity {
		s.auditPrefixContinuous.Add(1)
		return
	}
	s.auditPrefixBreaks.Add(1)
	switch strings.TrimSpace(state.PrefixBreakReason) {
	case voteaiauditcontext.PrefixBreakPolicyChanged:
		s.auditPrefixBreakPolicy.Add(1)
	case voteaiauditcontext.PrefixBreakModelChanged:
		s.auditPrefixBreakModel.Add(1)
	case voteaiauditcontext.PrefixBreakHistoryRewritten:
		s.auditPrefixBreakHistory.Add(1)
	case voteaiauditcontext.PrefixBreakHistoryTruncated:
		s.auditPrefixBreakTruncate.Add(1)
	case voteaiauditcontext.PrefixBreakCompactionEpoch:
		s.auditPrefixBreakCompact.Add(1)
	case voteaiauditcontext.PrefixBreakSessionChanged:
		s.auditPrefixBreakSession.Add(1)
	case voteaiauditcontext.PrefixBreakAuditKeyChanged:
		s.auditPrefixBreakKey.Add(1)
	default:
		s.auditPrefixBreakUnknown.Add(1)
	}
}

func (s *ContentModerationService) updateLocalDeterministicAuditContext(
	ctx context.Context,
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	content ContentModerationInput,
	result *moderationAPIResult,
) *contentModerationIncrementalPlan {
	plan, err := s.prepareIncrementalAudit(ctx, input, cfg, content)
	if err != nil {
		slog.Warn("content_moderation.local_audit_context_prepare_failed", "error", err)
		return nil
	}
	s.updateContentModerationAuditContext(ctx, input, cfg, plan, result, false)
	return plan
}

func (s *ContentModerationService) updateContentModerationAuditPrefix(
	ctx context.Context,
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	plan *contentModerationIncrementalPlan,
	result *moderationAPIResult,
) {
	if s == nil || cfg == nil || cfg.sideEffectsDisabled || plan == nil || result == nil || plan.stateKey == "" || !plan.stableSession {
		return
	}
	providerObserved := len(result.StageDetails) == 0 && !result.ResultCacheHit
	for _, stage := range result.StageDetails {
		if (stage.Stage == string(voteaimoderation.StageFull) || stage.Stage == string(voteaimoderation.StageMax)) && stage.ProviderCalled {
			providerObserved = true
			break
		}
	}
	if !providerObserved {
		return
	}
	store, ok := s.hashCache.(ContentModerationAuditContextStore)
	if !ok {
		return
	}
	canonicalRunes := []rune(plan.canonicalFullPrefix)
	previousPrefixHash := ""
	if plan.state.LastPrefixChars > 0 && len(canonicalRunes) >= plan.state.LastPrefixChars {
		previousPrefixHash = opaqueModerationRiskHash("audit-prefix", string(canonicalRunes[:plan.state.LastPrefixChars]))
	}
	promptTokens := int64(-1)
	if value, ok := contentModerationFullStagePromptTokens(result); ok {
		promptTokens = value
	}
	state, err := store.UpdateContentModerationAuditPrefixForUser(ctx, input.UserID, plan.stateKey, voteaiauditcontext.PrefixObservation{
		CanonicalPrefixHash: opaqueModerationRiskHash("audit-prefix", plan.canonicalFullPrefix),
		PreviousPrefixHash:  previousPrefixHash,
		PrefixChars:         len(canonicalRunes),
		PrefixTokens:        promptTokens,
		PolicyVersion:       plan.policyVersion,
		Model:               cfg.AIChat.Model,
		AuditKeyHash:        result.AuditKeyHash,
		Compacted:           plan.prefixCompacted,
		HistoryTruncated:    plan.fullHistoryTruncated,
		HistoryRewritten:    plan.prefixHistoryRewrite,
		AtUnixNano:          plan.eventAt.UnixNano(),
	}, time.Duration(cfg.AIChat.AuditContextTTLMinutes)*time.Minute)
	if err != nil {
		slog.Warn("content_moderation.audit_prefix_update_failed", "error", err)
		return
	}
	plan.state = state
	s.recordContentModerationPrefixContinuity(state)
}
