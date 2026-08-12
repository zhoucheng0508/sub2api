package auditcontext

import "time"

const (
	StateVersion = 2

	// RecentRequestIDLimit bounds persisted idempotency metadata while covering
	// realistic concurrent/retry windows for one moderation session.
	RecentRequestIDLimit = 32

	TrendRising  = "rising"
	TrendStable  = "stable"
	TrendFalling = "falling"

	TierLow     = "low"
	TierObserve = "observe"
	TierHigh    = "high"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

// Turn is a protocol-independent conversation turn. Callers must derive Role
// from the parsed request structure, never from role-like text supplied by a user.
type Turn struct {
	Role      Role   `json:"role"`
	Text      string `json:"text"`
	ToolCall  bool   `json:"tool_call,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type AuditTarget struct {
	Kind          string `json:"kind"`
	Text          string `json:"text"`
	OriginalIndex int    `json:"original_index"`
}

// State is safe to persist as JSON. It intentionally contains no raw
// conversation content or credentials.
type State struct {
	Version            int      `json:"version"`
	TurnCount          int      `json:"turn_count"`
	CurrentScore       float64  `json:"current_score"`
	MaxScore           float64  `json:"max_score"`
	Trend              string   `json:"trend"`
	Tier               string   `json:"tier"`
	Categories         []string `json:"categories,omitempty"`
	Signals            []string `json:"signals,omitempty"`
	RecentReasons      []string `json:"recent_reasons,omitempty"`
	LastFullReviewTurn int      `json:"last_full_review_turn,omitempty"`
	LastFullReviewAt   int64    `json:"last_full_review_at,omitempty"`
	LastRequestID      string   `json:"last_request_id,omitempty"`
	RecentRequestIDs   []string `json:"recent_request_ids,omitempty"`
	PolicyVersion      string   `json:"policy_version,omitempty"`
	UpdatedAt          int64    `json:"updated_at"`
	UpdatedAtUnixNano  int64    `json:"updated_at_unix_nano,omitempty"`

	PrefixEpoch             int    `json:"prefix_epoch"`
	CanonicalPrefixHash     string `json:"canonical_prefix_hash,omitempty"`
	LastPrefixChars         int    `json:"last_prefix_chars,omitempty"`
	LastPrefixTokens        int64  `json:"last_prefix_tokens,omitempty"`
	PrefixContinuity        bool   `json:"prefix_continuity"`
	PrefixBaseline          bool   `json:"prefix_baseline,omitempty"`
	PrefixBreakReason       string `json:"prefix_break_reason,omitempty"`
	PrefixModel             string `json:"prefix_model,omitempty"`
	AuditKeyHash            string `json:"audit_key_hash,omitempty"`
	PrefixUpdatedAtUnixNano int64  `json:"prefix_updated_at_unix_nano,omitempty"`
}

type Config struct {
	FastInputChars            int
	SummaryMaxChars           int
	ToolTurnMaxChars          int
	RecentUserTurns           int
	ReasonMaxChars            int
	RecentReasonLimit         int
	FullReviewThreshold       float64
	CumulativeReviewThreshold float64
	RiskRiseThreshold         float64
	RiskFallThreshold         float64
	HistoryRiskThreshold      float64
	ObserveThreshold          float64
	BlockThreshold            float64
	PeriodicFullReviewTurns   int
	RiskHalfLife              time.Duration
}

func DefaultConfig() Config {
	return Config{
		FastInputChars:            6000,
		SummaryMaxChars:           1000,
		ToolTurnMaxChars:          1000,
		RecentUserTurns:           2,
		ReasonMaxChars:            200,
		RecentReasonLimit:         3,
		FullReviewThreshold:       0.40,
		CumulativeReviewThreshold: 0.30,
		RiskRiseThreshold:         0.15,
		RiskFallThreshold:         0.10,
		HistoryRiskThreshold:      0.20,
		ObserveThreshold:          0.35,
		BlockThreshold:            0.80,
		PeriodicFullReviewTurns:   10,
		RiskHalfLife:              6 * time.Minute,
	}
}

func NormalizeConfig(cfg Config) Config {
	defaults := DefaultConfig()
	if cfg.FastInputChars < 128 {
		cfg.FastInputChars = defaults.FastInputChars
	}
	if cfg.SummaryMaxChars <= 0 {
		cfg.SummaryMaxChars = defaults.SummaryMaxChars
	}
	if cfg.SummaryMaxChars > cfg.FastInputChars/2 {
		cfg.SummaryMaxChars = cfg.FastInputChars / 2
	}
	if cfg.ToolTurnMaxChars <= 0 {
		cfg.ToolTurnMaxChars = defaults.ToolTurnMaxChars
	}
	if cfg.RecentUserTurns <= 0 {
		cfg.RecentUserTurns = defaults.RecentUserTurns
	}
	if cfg.ReasonMaxChars <= 0 {
		cfg.ReasonMaxChars = defaults.ReasonMaxChars
	}
	if cfg.RecentReasonLimit <= 0 {
		cfg.RecentReasonLimit = defaults.RecentReasonLimit
	}
	if cfg.FullReviewThreshold <= 0 || cfg.FullReviewThreshold > 1 {
		cfg.FullReviewThreshold = defaults.FullReviewThreshold
	}
	if cfg.CumulativeReviewThreshold <= 0 || cfg.CumulativeReviewThreshold > 1 {
		cfg.CumulativeReviewThreshold = defaults.CumulativeReviewThreshold
	}
	if cfg.RiskRiseThreshold <= 0 || cfg.RiskRiseThreshold > 1 {
		cfg.RiskRiseThreshold = defaults.RiskRiseThreshold
	}
	if cfg.RiskFallThreshold <= 0 || cfg.RiskFallThreshold > 1 {
		cfg.RiskFallThreshold = defaults.RiskFallThreshold
	}
	if cfg.HistoryRiskThreshold <= 0 || cfg.HistoryRiskThreshold > 1 {
		cfg.HistoryRiskThreshold = defaults.HistoryRiskThreshold
	}
	if cfg.ObserveThreshold <= 0 || cfg.ObserveThreshold >= 1 {
		cfg.ObserveThreshold = defaults.ObserveThreshold
	}
	if cfg.BlockThreshold <= cfg.ObserveThreshold || cfg.BlockThreshold > 1 {
		cfg.BlockThreshold = defaults.BlockThreshold
	}
	if cfg.PeriodicFullReviewTurns <= 0 {
		cfg.PeriodicFullReviewTurns = defaults.PeriodicFullReviewTurns
	}
	if cfg.RiskHalfLife <= 0 {
		cfg.RiskHalfLife = defaults.RiskHalfLife
	}
	return cfg
}

type AuditEvent struct {
	RiskScore     float64
	Categories    []string
	Signals       []string
	Reason        string
	RequestID     string
	PolicyVersion string
	FullReview    bool
	TurnIncrement int
	At            time.Time
	// NumericRiskOnly is used for an actor fallback that is not bound to a
	// stable conversation. It preserves the actor's numeric risk semantics but
	// prevents content-derived session details from crossing conversation
	// boundaries.
	NumericRiskOnly bool
}

type FastInput struct {
	Text                string
	Truncated           bool
	LastUserTruncated   bool
	IncludedTurnIndexes []int
}

type ReviewInput struct {
	FastScore            float64
	Categories           []string
	Signals              []string
	LatestUserText       string
	InputTruncated       bool
	StableSession        bool
	FullHistoryAvailable bool
	Force                bool
	At                   time.Time
}

type ReviewDecision struct {
	Required bool
	Reasons  []string
}

type PrefixObservation struct {
	CanonicalPrefixHash string
	PreviousPrefixHash  string
	PrefixChars         int
	PrefixTokens        int64
	PolicyVersion       string
	Model               string
	// AuditKeyHash must be a stable, non-secret digest. Never pass a raw key.
	AuditKeyHash     string
	Compacted        bool
	HistoryTruncated bool
	HistoryRewritten bool
	SessionChanged   bool
	AtUnixNano       int64
}
