package deterministicrisk

import "github.com/Wei-Shaw/sub2api/internal/custom/voteai/inputprovenance"

const (
	RuleCredentialBypassV2        = "LOCAL-CREDENTIAL-BYPASS-V2"
	RuleCredentialBypassV2Version = "2.0.0"
)

type Level string

const (
	LevelNone      Level = "none"
	LevelCandidate Level = "candidate"
	LevelConfirmed Level = "confirmed"
)

type AuditTarget struct {
	Kind               inputprovenance.TargetKind   `json:"kind"`
	Source             inputprovenance.Source       `json:"source"`
	MetadataKind       inputprovenance.MetadataKind `json:"metadata_kind,omitempty"`
	Text               string                       `json:"text"`
	LinkedToUserIntent bool                         `json:"linked_to_user_intent,omitempty"`
}

// SupportingContext is eligible only when the protocol adapter explicitly
// links it to the selected target. Detect never scans unrelated history.
type SupportingContext struct {
	Role           inputprovenance.Role         `json:"role"`
	Source         inputprovenance.Source       `json:"source"`
	Purpose        inputprovenance.Purpose      `json:"purpose"`
	MetadataKind   inputprovenance.MetadataKind `json:"metadata_kind,omitempty"`
	Text           string                       `json:"text"`
	DirectlyLinked bool                         `json:"directly_linked"`
}

type Input struct {
	Target            AuditTarget         `json:"target"`
	SupportingContext []SupportingContext `json:"supporting_context,omitempty"`
	MetadataExcluded  []string            `json:"metadata_excluded,omitempty"`
}

type DeterministicRiskMatch struct {
	RuleID            string   `json:"rule_id"`
	RuleVersion       string   `json:"rule_version"`
	Level             Level    `json:"level"`
	TargetKind        string   `json:"target_kind"`
	TargetSource      string   `json:"target_source"`
	MatchedIntent     []string `json:"matched_intent,omitempty"`
	MatchedTarget     []string `json:"matched_target,omitempty"`
	MatchedAction     []string `json:"matched_action,omitempty"`
	MatchedExcerpt    string   `json:"matched_excerpt,omitempty"`
	LexicalTypes      []string `json:"lexical_types,omitempty"`
	NegationDetected  bool     `json:"negation_detected"`
	DefensiveDetected bool     `json:"defensive_detected"`
	MetadataExcluded  []string `json:"metadata_excluded,omitempty"`
}

// Result deliberately omits a numeric score for candidates. A candidate must
// be resolved by semantic review; only a confirmed rule may suggest 0.95.
type Result struct {
	Level              Level                   `json:"level"`
	SuggestedRiskScore *float64                `json:"suggested_risk_score,omitempty"`
	Match              *DeterministicRiskMatch `json:"match,omitempty"`
}

func None() Result {
	return Result{Level: LevelNone}
}
