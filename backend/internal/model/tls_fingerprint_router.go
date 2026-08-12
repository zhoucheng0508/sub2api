package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	TLSRouterMatchContains = "contains"
	TLSRouterMatchPrefix   = "prefix"
	TLSRouterMatchExact    = "exact"
	TLSRouterMatchRegex    = "regex"
)

// CUSTOM(VOTE-AI-OPENAI-TLS): keep UA routing isolated from upstream account logic.
type TLSFingerprintRouterRule struct {
	Name                    string `json:"name"`
	Enabled                 bool   `json:"enabled"`
	MatchType               string `json:"match_type"`
	Pattern                 string `json:"pattern"`
	CaseSensitive           bool   `json:"case_sensitive"`
	TLSFingerprintProfileID int64  `json:"tls_fingerprint_profile_id"`
	UpstreamUserAgent       string `json:"upstream_user_agent,omitempty"`
	UpstreamOriginator      string `json:"upstream_originator,omitempty"`
}

type TLSFingerprintRouter struct {
	ID          int64                      `json:"id"`
	Name        string                     `json:"name"`
	Description *string                    `json:"description"`
	Enabled     bool                       `json:"enabled"`
	Rules       []TLSFingerprintRouterRule `json:"rules"`
	CreatedAt   time.Time                  `json:"created_at"`
	UpdatedAt   time.Time                  `json:"updated_at"`
}

func (r *TLSFingerprintRouter) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	for i, rule := range r.Rules {
		if err := rule.Validate(); err != nil {
			return &ValidationError{Field: "rules", Message: fmt.Sprintf("rule %d: %v", i+1, err)}
		}
	}
	return nil
}

func (r TLSFingerprintRouterRule) Validate() error {
	if strings.TrimSpace(r.Pattern) == "" {
		return &ValidationError{Field: "pattern", Message: "pattern is required"}
	}
	if r.TLSFingerprintProfileID < -1 {
		return &ValidationError{Field: "tls_fingerprint_profile_id", Message: "profile ID must be -1, 0, or positive"}
	}
	switch NormalizeTLSRouterMatchType(r.MatchType) {
	case TLSRouterMatchContains, TLSRouterMatchPrefix, TLSRouterMatchExact:
		return nil
	case TLSRouterMatchRegex:
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return &ValidationError{Field: "pattern", Message: "regex is invalid"}
		}
		return nil
	default:
		return &ValidationError{Field: "match_type", Message: "unsupported match type"}
	}
}

func NormalizeTLSRouterMatchType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case TLSRouterMatchPrefix:
		return TLSRouterMatchPrefix
	case TLSRouterMatchExact:
		return TLSRouterMatchExact
	case TLSRouterMatchRegex:
		return TLSRouterMatchRegex
	default:
		return TLSRouterMatchContains
	}
}
