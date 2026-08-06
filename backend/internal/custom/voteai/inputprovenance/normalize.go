package inputprovenance

import (
	"regexp"
	"sort"
	"strings"
)

type metadataPattern struct {
	kind    MetadataKind
	pattern *regexp.Regexp
}

type metadataSpan struct {
	start int
	end   int
	kind  MetadataKind
	text  string
}

var trustedMetadataPatterns = []metadataPattern{
	{
		kind:    MetadataAmbientUI,
		pattern: regexp.MustCompile(`(?is)<in-app-browser-context\b[^>]*>.*?</in-app-browser-context\s*>`),
	},
	{
		kind:    MetadataEnvironment,
		pattern: regexp.MustCompile(`(?is)<environment_context\b[^>]*>.*?</environment_context\s*>`),
	},
	{
		kind:    MetadataContextHandoff,
		pattern: regexp.MustCompile(`(?is)<context[_-]handoff\b[^>]*>.*?</context[_-]handoff\s*>`),
	},
}

var contextHandoffPrefixes = []string{
	"another language model started to solve this problem and produced a summary of its thinking process.",
	"a previous language model started to solve this problem and produced a summary",
}

// NormalizeAndSelect classifies every turn by provenance and returns exactly
// one audit target. Trusted metadata is exempted only when both request-level
// multi-signal trust and turn-level structural provenance are present.
func NormalizeAndSelect(turns []Turn, trust TrustDecision) Result {
	trustedClient := trust.AllowsTrustedMetadata()
	hasCurrentMarker := false
	for _, turn := range turns {
		if turn.Current {
			hasCurrentMarker = true
			break
		}
	}

	normalized := make([]NormalizedTurn, 0, len(turns))
	ignoredMetadata := make([]MetadataKind, 0, 3)
	for index, input := range turns {
		role := normalizeRole(input.Role)
		text := normalizeText(input.Text)
		if text == "" {
			continue
		}

		metadataAllowed := trustedClient && (input.MetadataEnvelope || isStructuredMetadataRole(input.Role))
		outside := text
		var metadata []metadataSpan
		if metadataAllowed {
			outside, metadata = splitTrustedMetadata(text, role, input.MetadataEnvelope, input.MetadataHint)
		}

		for _, item := range metadata {
			normalized = append(normalized, NormalizedTurn{
				Role:          role,
				Source:        SourceTrustedMetadata,
				Purpose:       PurposeIgnored,
				MetadataKind:  item.kind,
				Text:          item.text,
				Truncated:     input.Truncated,
				Current:       input.Current,
				OriginalIndex: index,
			})
			ignoredMetadata = appendUniqueMetadataKind(ignoredMetadata, item.kind)
		}

		if outside == "" {
			continue
		}
		normalized = append(normalized, NormalizedTurn{
			Role:               role,
			Source:             sourceForRole(role),
			Purpose:            PurposeIgnored,
			MetadataKind:       MetadataNone,
			Text:               outside,
			ToolCall:           input.ToolCall,
			Truncated:          input.Truncated,
			Current:            input.Current,
			LinkedToUserIntent: input.LinkedToUserIntent,
			OriginalIndex:      index,
		})
	}

	targetIndex := -1
	targetKind := TargetNoNewUserIntent
	latestUser := -1
	latestTool := -1
	latestInstruction := -1
	hasExplicitUser := false
	for index := range normalized {
		turn := normalized[index]
		if hasCurrentMarker && !turn.Current {
			continue
		}
		switch turn.Source {
		case SourceEndUser:
			latestUser = index
			hasExplicitUser = true
		case SourceToolOutput:
			if turn.LinkedToUserIntent {
				latestTool = index
			}
		case SourceClientInstruction:
			latestInstruction = index
		}
	}
	// An explicit end-user turn always remains the decision target for this
	// request envelope. A linked tool result may become the target only for a
	// tool-only continuation; otherwise it is supporting context for the user
	// request and cannot overwrite that request's intent.
	if latestUser >= 0 {
		targetIndex = latestUser
		targetKind = TargetUserRequest
	} else if latestTool >= 0 {
		targetIndex = latestTool
		targetKind = TargetToolContinuation
	}
	if targetIndex < 0 && latestInstruction >= 0 {
		targetIndex = latestInstruction
		targetKind = TargetClientInstruction
	}

	target := AuditTarget{
		Kind:            TargetNoNewUserIntent,
		Source:          SourceNone,
		MetadataKind:    MetadataNone,
		OriginalIndex:   -1,
		NormalizedIndex: -1,
	}
	if targetIndex >= 0 {
		for index := range normalized {
			if index == targetIndex {
				normalized[index].Purpose = PurposeAuditTarget
				continue
			}
			normalized[index].Purpose = PurposeSupportingContext
		}
		selected := normalized[targetIndex]
		target = AuditTarget{
			Kind:            targetKind,
			Text:            selected.Text,
			Source:          selected.Source,
			MetadataKind:    selected.MetadataKind,
			OriginalIndex:   selected.OriginalIndex,
			NormalizedIndex: targetIndex,
		}
	}

	return Result{
		Turns:           normalized,
		Target:          target,
		HasExplicitUser: hasExplicitUser,
		TrustedClient:   trustedClient,
		IgnoredMetadata: ignoredMetadata,
	}
}

func splitTrustedMetadata(text string, role Role, metadataEnvelope bool, hint MetadataKind) (string, []metadataSpan) {
	spans := make([]metadataSpan, 0, 3)
	for _, candidate := range trustedMetadataPatterns {
		for _, match := range candidate.pattern.FindAllStringIndex(text, -1) {
			spans = append(spans, metadataSpan{
				start: match[0],
				end:   match[1],
				kind:  candidate.kind,
				text:  strings.TrimSpace(text[match[0]:match[1]]),
			})
		}
	}

	if len(spans) == 0 && (role == RoleSystem || role == RoleDeveloper || metadataEnvelope) &&
		(hint == MetadataContextHandoff || hasContextHandoffShape(text)) {
		if hasContextHandoffShape(text) {
			return "", []metadataSpan{{start: 0, end: len(text), kind: MetadataContextHandoff, text: text}}
		}
	}
	if len(spans) == 0 {
		return text, nil
	}

	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].start == spans[j].start {
			return spans[i].end > spans[j].end
		}
		return spans[i].start < spans[j].start
	})
	nonOverlapping := spans[:0]
	lastEnd := -1
	for _, span := range spans {
		if span.start < lastEnd {
			continue
		}
		nonOverlapping = append(nonOverlapping, span)
		lastEnd = span.end
	}

	var outside strings.Builder
	position := 0
	for _, span := range nonOverlapping {
		appendOutsideFragment(&outside, text[position:span.start])
		position = span.end
	}
	appendOutsideFragment(&outside, text[position:])
	return normalizeText(outside.String()), nonOverlapping
}

func appendOutsideFragment(builder *strings.Builder, fragment string) {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return
	}
	if builder.Len() > 0 {
		builder.WriteString("\n\n")
	}
	builder.WriteString(fragment)
}

func hasContextHandoffShape(text string) bool {
	value := strings.ToLower(strings.TrimSpace(text))
	for _, prefix := range contextHandoffPrefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func normalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

func normalizeRole(role Role) Role {
	switch Role(strings.ToLower(strings.TrimSpace(string(role)))) {
	case RoleUser:
		return RoleUser
	case RoleAssistant:
		return RoleAssistant
	case RoleTool, "function":
		return RoleTool
	case RoleDeveloper, "client_developer":
		return RoleDeveloper
	case RoleSystem, "client_system":
		return RoleSystem
	default:
		// Unknown non-user roles remain auditable client instructions instead
		// of being silently discarded.
		return RoleSystem
	}
}

func isStructuredMetadataRole(role Role) bool {
	switch Role(strings.ToLower(strings.TrimSpace(string(role)))) {
	case RoleSystem, RoleDeveloper, "client_system", "client_developer":
		return true
	default:
		return false
	}
}

func sourceForRole(role Role) Source {
	switch role {
	case RoleUser:
		return SourceEndUser
	case RoleAssistant:
		return SourceAssistantResponse
	case RoleTool:
		return SourceToolOutput
	default:
		return SourceClientInstruction
	}
}

func appendUniqueMetadataKind(values []MetadataKind, value MetadataKind) []MetadataKind {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
