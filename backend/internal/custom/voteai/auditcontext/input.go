package auditcontext

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var ErrNoUserTurn = errors.New("audit context contains no user turn")

var literalRoleMarkerPattern = regexp.MustCompile(`(?i)\[(user|assistant|tool|system)\]`)

const (
	contentTruncatedMarker = "[CONTENT TRUNCATED]"
	sourceTruncatedMarker  = "[SOURCE TRUNCATED]"
)

// BuildFastAuditInput produces a deterministic user-message payload. The
// latest user turn has absolute priority; lower-priority history is dropped
// before that turn is truncated.
func BuildFastAuditInput(turns []Turn, state State, cfg Config) (FastInput, error) {
	latestUser := -1
	for i := len(turns) - 1; i >= 0; i-- {
		if normalizeRole(turns[i].Role) == RoleUser && strings.TrimSpace(turns[i].Text) != "" {
			latestUser = i
			break
		}
	}
	if latestUser < 0 {
		return FastInput{}, ErrNoUserTurn
	}
	return BuildFastAuditInputForTarget(turns, AuditTarget{
		Kind:          "user_request",
		Text:          turns[latestUser].Text,
		OriginalIndex: latestUser,
	}, state, cfg)
}

// BuildFastAuditInputForTarget renders one explicit decision target plus a
// bounded amount of supporting context. Supporting text can explain the
// target, but the stable system prompt instructs the model never to block on
// supporting text alone.
func BuildFastAuditInputForTarget(turns []Turn, target AuditTarget, state State, cfg Config) (FastInput, error) {
	cfg = NormalizeConfig(cfg)
	target.Text = normalizeTurnText(target.Text)
	target.Kind = strings.ToLower(strings.TrimSpace(target.Kind))
	if target.Kind == "" {
		target.Kind = "user_request"
	}
	if target.Text == "" {
		return FastInput{}, ErrNoUserTurn
	}
	targetIndex := target.OriginalIndex
	if targetIndex < 0 || targetIndex >= len(turns) {
		targetIndex = -1
	}

	previousUserLimit := cfg.RecentUserTurns
	if target.Kind == "user_request" && previousUserLimit > 0 {
		// RecentUserTurns counts the audit target itself. Treating it as an
		// additional-history limit leaks one extra user turn on every request.
		previousUserLimit--
	}
	userIndexes := make([]int, 0, previousUserLimit)
	start := len(turns) - 1
	if targetIndex >= 0 {
		start = targetIndex - 1
	}
	for i := start; i >= 0; i-- {
		if normalizeRole(turns[i].Role) != RoleUser || strings.TrimSpace(turns[i].Text) == "" {
			continue
		}
		if len(userIndexes) < previousUserLimit {
			userIndexes = append(userIndexes, i)
		}
	}

	selected := make(map[int]struct{}, len(userIndexes)+2)
	for _, index := range userIndexes {
		selected[index] = struct{}{}
	}
	contextIndex := -1
	if NeedsPreviousContext(target.Text) {
		start := len(turns) - 1
		if targetIndex >= 0 {
			start = targetIndex - 1
		}
		for i := start; i >= 0; i-- {
			role := normalizeRole(turns[i].Role)
			if (role == RoleAssistant || role == RoleTool) && strings.TrimSpace(turns[i].Text) != "" {
				contextIndex = i
				selected[i] = struct{}{}
				break
			}
			if role == RoleUser {
				break
			}
		}
	}

	indexes := sortedIndexes(selected)
	truncated := false
	prepared := make(map[int]Turn, len(indexes))
	for _, index := range indexes {
		turn := turns[index]
		turn.Role = normalizeRole(turn.Role)
		turn.Text = normalizeTurnText(turn.Text)
		if turn.Truncated {
			truncated = true
		}
		if turn.Role == RoleTool && runeLen(turn.Text) > cfg.ToolTurnMaxChars {
			turn.Text = truncateHeadTail(turn.Text, cfg.ToolTurnMaxChars, contentTruncatedMarker)
			turn.Truncated = true
			truncated = true
		}
		prepared[index] = turn
	}

	summary := formatSummary(state, cfg, false)
	text := renderFastTargetInput(target, summary, indexes, prepared, false)
	if runeLen(text) > cfg.FastInputChars {
		// A referential assistant/tool turn is more useful than an older user
		// turn. Drop older user turns first, then the referenced turn.
		for _, index := range append([]int(nil), indexes...) {
			if index == contextIndex {
				continue
			}
			indexes = removeIndex(indexes, index)
			truncated = true
			text = renderFastTargetInput(target, summary, indexes, prepared, false)
			if runeLen(text) <= cfg.FastInputChars {
				break
			}
		}
	}
	if runeLen(text) > cfg.FastInputChars && contextIndex >= 0 {
		indexes = removeIndex(indexes, contextIndex)
		truncated = true
		text = renderFastTargetInput(target, summary, indexes, prepared, false)
	}
	if runeLen(text) > cfg.FastInputChars {
		summary = formatSummary(state, cfg, true)
		text = renderFastTargetInput(target, summary, indexes, prepared, false)
	}

	targetTruncated := false
	if targetIndex >= 0 {
		targetTruncated = turns[targetIndex].Truncated
	}
	if runeLen(text) > cfg.FastInputChars {
		indexes = nil
		summary = ""
		emptyTarget := target
		emptyTarget.Text = ""
		base := runeLen(renderFastTargetInput(emptyTarget, summary, indexes, prepared, targetTruncated))
		available := cfg.FastInputChars - base
		if available >= runeLen(target.Text) {
			text = renderFastTargetInput(target, summary, indexes, prepared, targetTruncated)
		} else {
			targetTruncated = true
			base = runeLen(renderFastTargetInput(emptyTarget, summary, indexes, prepared, true))
			available = cfg.FastInputChars - base
			target.Text = truncateHeadMiddleTail(target.Text, max(0, available), contentTruncatedMarker)
			text = renderFastTargetInput(target, summary, indexes, prepared, targetTruncated)
			truncated = true
		}
	}

	includedIndexes := append([]int(nil), indexes...)
	if targetIndex >= 0 {
		includedIndexes = append(includedIndexes, targetIndex)
		sort.Ints(includedIndexes)
	}

	return FastInput{
		Text:                text,
		Truncated:           truncated,
		LastUserTruncated:   targetTruncated,
		IncludedTurnIndexes: includedIndexes,
	}, nil
}

func renderFastTargetInput(target AuditTarget, summary string, indexes []int, turns map[int]Turn, targetTruncated bool) string {
	var builder strings.Builder
	write := func(value string) {
		_, _ = builder.WriteString(value)
	}
	write("[AUDIT-TARGET kind=")
	write(target.Kind)
	write("]\n")
	write(target.Text)
	if targetTruncated {
		write("\n")
		write(sourceTruncatedMarker)
	}
	write("\n\n[SUPPORTING-CONTEXT]")
	if len(indexes) == 0 {
		write("\nnone")
	}
	for _, index := range indexes {
		turn, ok := turns[index]
		if !ok {
			continue
		}
		write("\n[")
		write(strings.ToUpper(string(turn.Role)))
		write("]\n")
		write(turn.Text)
		if turn.Truncated {
			write("\n")
			write(sourceTruncatedMarker)
		}
	}
	write("\n\n")
	write(summary)
	return builder.String()
}

func NeedsPreviousContext(text string) bool {
	value := strings.ToLower(strings.TrimSpace(text))
	if value == "" {
		return false
	}
	phrases := []string{
		"继续", "接着", "上面", "上述", "前面", "刚才", "这个", "这些", "那个", "那些",
		"按这个", "照此", "基于结果", "根据结果", "写成脚本", "再具体", "更详细", "优化一下",
		"continue", "go on", "the above", "above result", "previous", "that result", "this result",
		"write it", "turn it into", "make it", "more specific", "more detail",
	}
	for _, phrase := range phrases {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	return false
}

func formatSummary(state State, cfg Config, compact bool) string {
	categories := strings.Join(normalizeValues(state.Categories), ",")
	if categories == "" {
		categories = "none"
	}
	signals := strings.Join(normalizeValues(state.Signals), ",")
	if signals == "" {
		signals = "none"
	}
	tier := strings.ToLower(strings.TrimSpace(state.Tier))
	if tier == "" {
		tier = TierLow
	}
	trend := strings.ToLower(strings.TrimSpace(state.Trend))
	if trend == "" {
		trend = TrendStable
	}
	if compact {
		return fmt.Sprintf("[SESSION-RISK-SUMMARY]\nturn=%d\ncurrent_tier=%s\ntrend=%s", state.TurnCount, tier, trend)
	}
	reasons := make([]string, 0, len(state.RecentReasons))
	start := len(state.RecentReasons) - cfg.RecentReasonLimit
	if start < 0 {
		start = 0
	}
	for _, reason := range state.RecentReasons[start:] {
		if reason = SanitizeReason(reason, cfg.ReasonMaxChars); reason != "" {
			reasons = append(reasons, reason)
		}
	}
	reasonText := strings.Join(reasons, " | ")
	if reasonText == "" {
		reasonText = "none"
	}
	summary := fmt.Sprintf(
		"[SESSION-RISK-SUMMARY]\nturn=%d\ncurrent_tier=%s\ncurrent_score=%.2f\nmax_score=%.2f\ntrend=%s\ncategories=%s\nsignals=%s\nrecent_reasons=%s",
		state.TurnCount, tier, clampScore(state.CurrentScore), clampScore(state.MaxScore), trend, categories, signals, reasonText,
	)
	return truncateRunes(summary, cfg.SummaryMaxChars, contentTruncatedMarker)
}

func normalizeTurnText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.TrimSpace(value)
	value = literalRoleMarkerPattern.ReplaceAllStringFunc(value, func(marker string) string {
		role := strings.TrimSuffix(strings.TrimPrefix(marker, "["), "]")
		return "[LITERAL-" + strings.ToUpper(role) + "]"
	})
	return value
}

func normalizeRole(role Role) Role {
	switch Role(strings.ToLower(strings.TrimSpace(string(role)))) {
	case RoleUser:
		return RoleUser
	case RoleAssistant:
		return RoleAssistant
	case RoleTool:
		return RoleTool
	case RoleSystem:
		return RoleSystem
	default:
		return RoleTool
	}
}

func normalizeValues(values []string) []string {
	return mergeValues(nil, values, 16)
}

func sortedIndexes(values map[int]struct{}) []int {
	indexes := make([]int, 0, len(values))
	for index := range values {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	return indexes
}

func removeIndex(indexes []int, remove int) []int {
	out := make([]int, 0, len(indexes))
	for _, index := range indexes {
		if index != remove {
			out = append(out, index)
		}
	}
	return out
}

func truncateRunes(value string, maxChars int, marker string) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	markerRunes := []rune(marker)
	if len(markerRunes) >= maxChars {
		return string(markerRunes[:maxChars])
	}
	return string(runes[:maxChars-len(markerRunes)]) + marker
}

func truncateHeadTail(value string, maxChars int, marker string) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	markerRunes := []rune(marker)
	if len(markerRunes) >= maxChars {
		return string(markerRunes[:maxChars])
	}
	remaining := maxChars - len(markerRunes)
	head := (remaining + 1) / 2
	tail := remaining - head
	if tail == 0 {
		return string(runes[:head]) + marker
	}
	return string(runes[:head]) + marker + string(runes[len(runes)-tail:])
}

func truncateHeadMiddleTail(value string, maxChars int, marker string) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	markerRunes := []rune(marker)
	markerChars := len(markerRunes) * 2
	if maxChars <= markerChars+2 {
		head := maxChars / 3
		middle := (maxChars - head) / 2
		tail := maxChars - head - middle
		middleStart := max(0, len(runes)/2-middle/2)
		return string(runes[:head]) + string(runes[middleStart:middleStart+middle]) + string(runes[len(runes)-tail:])
	}

	available := maxChars - markerChars
	head := (available + 2) / 3
	middle := (available + 1) / 3
	tail := available - head - middle
	middleStart := len(runes)/2 - middle/2
	if middleStart < head {
		middleStart = head
	}
	if middleStart+middle > len(runes)-tail {
		middleStart = len(runes) - tail - middle
	}
	return string(runes[:head]) + marker +
		string(runes[middleStart:middleStart+middle]) + marker +
		string(runes[len(runes)-tail:])
}

func runeLen(value string) int {
	return len([]rune(value))
}
