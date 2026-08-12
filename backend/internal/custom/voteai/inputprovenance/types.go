package inputprovenance

const NormalizationVersion = "2.1.0"

// Role is derived from the parsed protocol envelope. Text that looks like a
// role marker must never be used to populate this field.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
)

type Source string

const (
	SourceNone              Source = "none"
	SourceEndUser           Source = "end_user"
	SourceToolOutput        Source = "tool_output"
	SourceClientInstruction Source = "client_instruction"
	SourceTrustedMetadata   Source = "trusted_metadata"
	SourceAssistantResponse Source = "assistant_response"
)

type Purpose string

const (
	PurposeAuditTarget       Purpose = "audit_target"
	PurposeSupportingContext Purpose = "supporting_context"
	PurposeIgnored           Purpose = "ignored"
)

type MetadataKind string

const (
	MetadataNone           MetadataKind = "none"
	MetadataAmbientUI      MetadataKind = "ambient_ui"
	MetadataContextHandoff MetadataKind = "context_handoff"
	MetadataEnvironment    MetadataKind = "environment"
)

type TargetKind string

const (
	TargetUserRequest       TargetKind = "user_request"
	TargetToolContinuation  TargetKind = "tool_continuation"
	TargetClientInstruction TargetKind = "client_instruction"
	TargetNoNewUserIntent   TargetKind = "no_new_user_intent"
)

// TrustSignal records evidence evaluated by the existing request identity
// layer. This package deliberately does not derive any of these signals from
// message text, XML-like tags, or a loose User-Agent check.
type TrustSignal string

const (
	TrustInternalProvenance TrustSignal = "internal_provenance"
	TrustStrictUserAgent    TrustSignal = "strict_user_agent"
	TrustOfficialOriginator TrustSignal = "official_originator"
	TrustEngineFingerprint  TrustSignal = "engine_fingerprint"
	TrustStructuredMessage  TrustSignal = "structured_message"
)

// TrustDecision is supplied by the caller after strict client identity
// normalization. Verified alone is insufficient: metadata exemptions require
// an internal provenance marker, every strict transport identity signal, and
// structured-message evidence.
type TrustDecision struct {
	Verified bool          `json:"verified"`
	Signals  []TrustSignal `json:"signals,omitempty"`
}

func (decision TrustDecision) AllowsTrustedMetadata() bool {
	if !decision.Verified {
		return false
	}
	seen := make(map[TrustSignal]struct{}, len(decision.Signals))
	structured := false
	internalProvenance := false
	strictUserAgent := false
	officialOriginator := false
	engineFingerprint := false
	for _, signal := range decision.Signals {
		if _, exists := seen[signal]; exists {
			continue
		}
		seen[signal] = struct{}{}
		switch signal {
		case TrustInternalProvenance:
			internalProvenance = true
		case TrustStrictUserAgent:
			strictUserAgent = true
		case TrustOfficialOriginator:
			officialOriginator = true
		case TrustEngineFingerprint:
			engineFingerprint = true
		case TrustStructuredMessage:
			structured = true
		}
	}
	return internalProvenance && strictUserAgent && officialOriginator && engineFingerprint && structured
}

// Turn is a protocol-independent input turn.
//
// Current limits target selection to the current request envelope when at
// least one turn has it set. LinkedToUserIntent must be set by the protocol
// adapter for a tool-only continuation. MetadataEnvelope is structural
// provenance supplied by a trusted client adapter; it must never be inferred
// from Text. MetadataHint is accepted only when the text also has a known
// metadata shape.
type Turn struct {
	Role               Role         `json:"role"`
	Text               string       `json:"text"`
	ToolCall           bool         `json:"tool_call,omitempty"`
	Truncated          bool         `json:"truncated,omitempty"`
	Current            bool         `json:"current,omitempty"`
	LinkedToUserIntent bool         `json:"linked_to_user_intent,omitempty"`
	MetadataEnvelope   bool         `json:"metadata_envelope,omitempty"`
	MetadataHint       MetadataKind `json:"metadata_hint,omitempty"`
}

type NormalizedTurn struct {
	Role               Role         `json:"role"`
	Source             Source       `json:"source"`
	Purpose            Purpose      `json:"purpose"`
	MetadataKind       MetadataKind `json:"metadata_kind"`
	Text               string       `json:"text"`
	ToolCall           bool         `json:"tool_call,omitempty"`
	Truncated          bool         `json:"truncated,omitempty"`
	Current            bool         `json:"current,omitempty"`
	LinkedToUserIntent bool         `json:"linked_to_user_intent,omitempty"`
	OriginalIndex      int          `json:"original_index"`
}

type AuditTarget struct {
	Kind            TargetKind   `json:"kind"`
	Text            string       `json:"text,omitempty"`
	Source          Source       `json:"source"`
	MetadataKind    MetadataKind `json:"metadata_kind"`
	OriginalIndex   int          `json:"original_index"`
	NormalizedIndex int          `json:"normalized_index"`
}

type Result struct {
	Turns           []NormalizedTurn `json:"turns"`
	Target          AuditTarget      `json:"target"`
	HasExplicitUser bool             `json:"has_explicit_user"`
	TrustedClient   bool             `json:"trusted_client"`
	IgnoredMetadata []MetadataKind   `json:"ignored_metadata,omitempty"`
}
