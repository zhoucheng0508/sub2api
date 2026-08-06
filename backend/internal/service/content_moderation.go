package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	voteaiauditcontext "github.com/Wei-Shaw/sub2api/internal/custom/voteai/auditcontext"
	voteaideterministicrisk "github.com/Wei-Shaw/sub2api/internal/custom/voteai/deterministicrisk"
	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
	voteairiskstate "github.com/Wei-Shaw/sub2api/internal/custom/voteai/riskstate"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	openaipkg "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
)

const (
	ContentModerationModeOff      = "off"
	ContentModerationModeObserve  = "observe"
	ContentModerationModePreBlock = "pre_block"

	ContentModerationProviderOpenAIModerations = "openai_moderations"
	ContentModerationProviderAIChat            = "ai_chat"

	ContentModerationFailurePolicyAllow = "allow"
	ContentModerationFailurePolicyBlock = "block"

	contentModerationAPIKeysModeAppend  = "append"
	contentModerationAPIKeysModeReplace = "replace"

	ContentModerationActionAllow          = "allow"
	ContentModerationActionObserve        = "observe"
	ContentModerationActionBlock          = "block"
	ContentModerationActionHashBlock      = "hash_block"
	ContentModerationActionKeywordBlock   = "keyword_block"
	ContentModerationActionError          = "error"
	ContentModerationActionSkip           = "skip"
	ContentModerationActionUnavailable    = "audit_unavailable"
	ContentModerationErrorCodeUnavailable = "content_moderation_unavailable"
	ContentModerationActionCyberPolicy    = "cyber_policy" // cyber_policy 硬阻断的风控日志 action（封号计数排除按此值过滤）

	ContentModerationAuditStatusSuccess    = "success"
	ContentModerationAuditStatusSkipped    = "skipped"
	ContentModerationAuditStatusIncomplete = "incomplete"
	ContentModerationAuditStatusError      = "error"

	ContentModerationSideEffectStatusPending       = "pending"
	ContentModerationSideEffectStatusCompleted     = "completed"
	ContentModerationSideEffectStatusPartial       = "partial"
	ContentModerationSideEffectStatusFailed        = "failed"
	ContentModerationSideEffectStatusNotApplicable = "not_applicable"

	ContentModerationNotificationStatusPending      = "pending"
	ContentModerationNotificationStatusSent         = "sent"
	ContentModerationNotificationStatusDeduplicated = "deduplicated"
	ContentModerationNotificationStatusNotRequired  = "not_required"
	ContentModerationNotificationStatusFailed       = "failed"

	ContentModerationUnbanModeRestoreOnly         = "restore_only"
	ContentModerationUnbanModeRestoreAndClearRisk = "restore_and_clear_risk"
	ContentModerationUnbanModeClearRiskOnly       = "clear_risk_only"

	ContentModerationBanOutcomeApplied      = "applied"
	ContentModerationBanOutcomeAlreadyOwned = "already_owned"
	ContentModerationBanOutcomeIneligible   = "ineligible"

	ContentModerationCostCoverageNoSamples = "no_samples"
	ContentModerationCostCoverageUnknown   = "unknown"
	ContentModerationCostCoveragePartial   = "partial"
	ContentModerationCostCoverageComplete  = "complete"

	contentModerationKeywordCategory           = "keyword"
	contentModerationAuditCodeStaleCyberPolicy = "cyber_policy_stale_after_risk_reset"
	contentModerationAuditCodeNoNewUserIntent  = "skip_no_new_user_intent"

	ContentModerationKeywordModeKeywordOnly   = "keyword_only"
	ContentModerationKeywordModeKeywordAndAPI = "keyword_and_api"
	ContentModerationKeywordModeAPIOnly       = "api_only"

	ContentModerationModelFilterAll     = "all"
	ContentModerationModelFilterInclude = "include"
	ContentModerationModelFilterExclude = "exclude"

	ContentModerationScopeFilterAll     = "all"
	ContentModerationScopeFilterInclude = "include"
	ContentModerationScopeFilterExclude = "exclude"

	ContentModerationScopeReasonUserOutOfScope    = "skip_user_out_of_scope"
	ContentModerationScopeReasonAccountOutOfScope = "skip_account_out_of_scope"

	ContentModerationProtocolAnthropicMessages = "anthropic_messages"
	ContentModerationProtocolOpenAIResponses   = "openai_responses"
	ContentModerationProtocolOpenAIChat        = "openai_chat_completions"
	ContentModerationProtocolGemini            = "gemini"
	ContentModerationProtocolOpenAIImages      = "openai_images"

	defaultContentModerationBaseURL   = "https://api.openai.com"
	defaultContentModerationModel     = "omni-moderation-latest"
	defaultContentModerationTimeoutMS = 3000
	maxContentModerationTimeoutMS     = 30000
	legacyModerationInputRunes        = 12000
	maxModerationInputRunes           = 1000000
	maxModerationExcerptRunes         = 240

	defaultContentModerationWorkerCount           = 4
	maxContentModerationWorkerCount               = 32
	defaultContentModerationQueueSize             = 32768
	maxContentModerationQueueSize                 = 100000
	maxContentModerationSupplementalQueueSize     = 64
	maxContentModerationSupplementalRetainedRunes = 12800000
	defaultContentModerationBanThreshold          = 10
	defaultContentModerationViolationWindowHours  = 720
	defaultContentModerationBlockHTTPStatus       = http.StatusForbidden
	defaultContentModerationBlockMessage          = "内容审计命中风险规则，请调整输入后重试"
	defaultContentModerationRetryCount            = 2
	maxContentModerationRetryCount                = 5
	defaultContentModerationHitRetentionDays      = 180
	defaultContentModerationNonHitRetentionDays   = 3
	maxContentModerationRetentionDays             = 3650
	maxContentModerationNonHitRetentionDays       = 3
	maxContentModerationUSDPerMillionTokens       = 1000000
	contentModerationKeyRateLimitFreezeDuration   = time.Minute
	contentModerationKeyAuthFreezeDuration        = 10 * time.Minute
	contentModerationKeyHTTPErrorFreezeDuration   = 10 * time.Second
	maxContentModerationInputImages               = 1
	maxContentModerationTestImages                = maxContentModerationInputImages
	maxContentModerationTestImageBytes            = 8 * 1024 * 1024
	maxContentModerationTestImageDataURLBytes     = 12 * 1024 * 1024
	maxContentModerationBlockedKeywords           = 10000
	maxContentModerationBlockedKeywordRunes       = 200
	maxContentModerationModelFilterModels         = 1000
	maxContentModerationModelFilterRunes          = 200
	defaultAIChatBaseURL                          = "https://api.deepseek.com"
	defaultAIChatModel                            = "deepseek-v4-flash"
	defaultAIChatTimeoutMS                        = 15000
	defaultAIChatSynchronousBudgetMS              = 3500
	minAIChatSynchronousBudgetMS                  = 500
	maxAIChatSynchronousBudgetMS                  = 5000
	preferredAIChatNextAPIKeyBudget               = 1500 * time.Millisecond
	minimumAIChatAPIKeyBudget                     = 500 * time.Millisecond
	aiChatAPIKeyBudgetGuard                       = 50 * time.Millisecond
	defaultAIChatFastInputChars                   = 6000
	defaultAIChatFallbackInputChars               = 3000
	defaultAIChatConfidenceThreshold              = 0.7
	defaultAIChatCacheTTLSeconds                  = 3600
	defaultAIChatMaxInputChars                    = 60000
	minAIChatMaxInputChars                        = 1000
	defaultAIChatThinkingMode                     = "enabled"
	defaultAIChatReasoningEffort                  = "adaptive"
	defaultAIChatObserveThreshold                 = 0.35
	defaultAIChatSessionRiskTTLMinutes            = 120
	defaultAIChatSessionRiskHalfLifeMinutes       = 30
	defaultAIChatSessionRiskBlockCooldownMinutes  = 30
	defaultAIChatRecentUserTurns                  = 2
	defaultAIChatSummaryMaxChars                  = 800
	defaultAIChatFullReviewThreshold              = 0.40
	defaultAIChatFullReviewRiskDelta              = 0.15
	defaultAIChatPeriodicFullReviewTurns          = 10
	defaultAIChatFullReviewMaxInputChars          = 60000
	defaultAIChatFastMaxOutputTokens              = 256
	defaultAIChatFullMaxOutputTokens              = 1024
	defaultAIChatMaxReviewMaxOutputTokens         = 1536
	defaultAIChatAuditContextTTLMinutes           = 120
	maxAIChatCacheTTLSeconds                      = 86400
	maxAIChatSessionRiskTTLMinutes                = 1440
	maxAIChatSessionRiskHalfLifeMinutes           = 720
	maxAIChatSessionRiskBlockCooldownMinutes      = 1440
	maxAIChatRecentUserTurns                      = 8
	maxAIChatSummaryMaxChars                      = 4000
	maxAIChatPeriodicFullReviewTurns              = 100
	maxAIChatOutputTokens                         = 8192
	maxAIChatAuditContextTTLMinutes               = 1440

	contentModerationCleanupInterval = 24 * time.Hour
	contentModerationCleanupTimeout  = 30 * time.Minute
	contentModerationCleanupDelay    = 5 * time.Minute

	contentModerationRuntimeCacheTTL       = time.Second
	contentModerationRuntimeRefreshTimeout = 5 * time.Second
)

var contentModerationCategoryOrder = []string{
	"harassment",
	"harassment/threatening",
	"hate",
	"hate/threatening",
	"illicit",
	"illicit/violent",
	"self-harm",
	"self-harm/intent",
	"self-harm/instructions",
	"sexual",
	"sexual/minors",
	"violence",
	"violence/graphic",
}

func ContentModerationDefaultThresholds() map[string]float64 {
	return map[string]float64{
		"harassment":             0.98,
		"harassment/threatening": 0.90,
		"hate":                   0.65,
		"hate/threatening":       0.65,
		"illicit":                0.95,
		"illicit/violent":        0.95,
		"self-harm":              0.65,
		"self-harm/intent":       0.85,
		"self-harm/instructions": 0.65,
		"sexual":                 0.65,
		"sexual/minors":          0.65,
		"violence":               0.95,
		"violence/graphic":       0.95,
	}
}

func ContentModerationCategories() []string {
	out := make([]string, len(contentModerationCategoryOrder))
	copy(out, contentModerationCategoryOrder)
	return out
}

type ContentModerationConfig struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
	// CUSTOM(VOTE-AI-AI-AUDIT): provider defaults to the legacy Moderations endpoint.
	AuditProvider string                        `json:"audit_provider,omitempty"`
	AIChat        ContentModerationAIChatConfig `json:"ai_chat,omitempty"`
	BaseURL       string                        `json:"base_url"`
	Model         string                        `json:"model"`
	// ProxyID 指定审计请求使用的代理服务器（IP管理-代理服务器），nil 表示直连。
	ProxyID              *int64                         `json:"proxy_id,omitempty"`
	APIKey               string                         `json:"api_key,omitempty"`
	APIKeys              []string                       `json:"api_keys,omitempty"`
	TimeoutMS            int                            `json:"timeout_ms"`
	SampleRate           int                            `json:"sample_rate"`
	AllGroups            bool                           `json:"all_groups"`
	GroupIDs             []int64                        `json:"group_ids"`
	RecordNonHits        bool                           `json:"record_non_hits"`
	Thresholds           map[string]float64             `json:"thresholds"`
	WorkerCount          int                            `json:"worker_count"`
	QueueSize            int                            `json:"queue_size"`
	BlockStatus          int                            `json:"block_status"`
	BlockMessage         string                         `json:"block_message"`
	EmailOnHit           bool                           `json:"email_on_hit"`
	AutoBanEnabled       bool                           `json:"auto_ban_enabled"`
	BanThreshold         int                            `json:"ban_threshold"`
	ViolationWindowHours int                            `json:"violation_window_hours"`
	RetryCount           int                            `json:"retry_count"`
	HitRetentionDays     int                            `json:"hit_retention_days"`
	NonHitRetentionDays  int                            `json:"non_hit_retention_days"`
	PreHashCheckEnabled  bool                           `json:"pre_hash_check_enabled"`
	BlockedKeywords      []string                       `json:"blocked_keywords"`
	KeywordBlockingMode  string                         `json:"keyword_blocking_mode"`
	ModelFilter          ContentModerationModelFilter   `json:"model_filter"`
	UserFilter           ContentModerationUserFilter    `json:"user_filter"`
	AccountFilter        ContentModerationAccountFilter `json:"account_filter"`
	sideEffectsDisabled  bool
	// CyberPolicyExcludeFromBanCount 为 true 时，cyber_policy 命中不参与自动封号计数：
	// 当次不判定封号，且历史 cyber 行在 CountFlaggedByUserSince 中被排除。
	// 默认 false（计入，与历史行为一致；旧配置 JSON 无此字段时反序列化为 false）。
	CyberPolicyExcludeFromBanCount bool `json:"cyber_policy_exclude_from_ban_count"`
}

type ContentModerationAIChatConfig struct {
	BaseURL                         string   `json:"base_url"`
	Model                           string   `json:"model"`
	ProxyID                         *int64   `json:"proxy_id,omitempty"`
	APIKeys                         []string `json:"api_keys,omitempty"`
	TimeoutMS                       int      `json:"timeout_ms"`
	SynchronousBudgetMS             int      `json:"synchronous_budget_ms"`
	RetryCount                      int      `json:"retry_count"`
	ConfidenceThreshold             float64  `json:"confidence_threshold"`
	CacheEnabled                    bool     `json:"cache_enabled"`
	CacheTTLSeconds                 int      `json:"cache_ttl_seconds"`
	SystemPrompt                    string   `json:"system_prompt"`
	FailurePolicy                   string   `json:"failure_policy"`
	MaxInputChars                   int      `json:"max_input_chars"`
	FastInputChars                  int      `json:"fast_input_chars"`
	FallbackInputChars              int      `json:"fallback_input_chars"`
	ThinkingMode                    string   `json:"thinking_mode"`
	ReasoningEffort                 string   `json:"reasoning_effort"`
	RiskLevelsEnabled               bool     `json:"risk_levels_enabled"`
	ObserveThreshold                float64  `json:"observe_threshold"`
	SessionRiskEnabled              bool     `json:"session_risk_enabled"`
	SessionRiskTTLMinutes           int      `json:"session_risk_ttl_minutes"`
	SessionRiskHalfLifeMinutes      int      `json:"session_risk_half_life_minutes"`
	SessionRiskBlockCooldownMinutes int      `json:"session_risk_block_cooldown_minutes"`
	ActorRiskEnabled                bool     `json:"actor_risk_enabled"`
	// CUSTOM(VOTE-AI-AUDIT-COST/CONTEXT): opt-in incremental auditing keeps
	// legacy behavior unchanged until an administrator enables it.
	IncrementalAuditEnabled    bool    `json:"incremental_audit_enabled"`
	InputProvenanceV2Enabled   bool    `json:"input_provenance_v2_enabled"`
	DeterministicRiskV2Enabled bool    `json:"deterministic_risk_v2_enabled"`
	RecentUserTurns            int     `json:"recent_user_turns"`
	SummaryMaxChars            int     `json:"summary_max_chars"`
	FullReviewThreshold        float64 `json:"full_review_threshold"`
	FullReviewRiskDelta        float64 `json:"full_review_risk_delta"`
	PeriodicFullReviewTurns    int     `json:"periodic_full_review_turns"`
	FullReviewMaxInputChars    int     `json:"full_review_max_input_chars"`
	FastMaxOutputTokens        int     `json:"fast_max_output_tokens"`
	FullMaxOutputTokens        int     `json:"full_max_output_tokens"`
	MaxReviewMaxOutputTokens   int     `json:"max_review_max_output_tokens"`
	AuditContextTTLMinutes     int     `json:"audit_context_ttl_minutes"`
	// Pricing is intentionally nullable. A missing rate means cost is unknown,
	// never zero. PricingVersion identifies the rate snapshot used at call time.
	PricingVersion                   string   `json:"pricing_version,omitempty"`
	UncachedInputUSDPerMillionTokens *float64 `json:"uncached_input_usd_per_million_tokens,omitempty"`
	CachedInputUSDPerMillionTokens   *float64 `json:"cached_input_usd_per_million_tokens,omitempty"`
	OutputUSDPerMillionTokens        *float64 `json:"output_usd_per_million_tokens,omitempty"`
	existingRiskScore                float64
	cacheKeyAlias                    string
	supplementalReview               bool
	auditStage                       string
	riskStateDigest                  string
	forceFullReview                  bool
	inputHash                        string
}

type ContentModerationProviderProfileView struct {
	BaseURL          string   `json:"base_url"`
	Model            string   `json:"model"`
	ProxyID          *int64   `json:"proxy_id"`
	APIKeyConfigured bool     `json:"api_key_configured"`
	APIKeyCount      int      `json:"api_key_count"`
	APIKeyMasks      []string `json:"api_key_masks"`
	TimeoutMS        int      `json:"timeout_ms"`
	RetryCount       int      `json:"retry_count"`
}

type ContentModerationConfigView struct {
	Enabled                        bool                                 `json:"enabled"`
	Mode                           string                               `json:"mode"`
	AuditProvider                  string                               `json:"audit_provider"`
	OpenAIModerations              ContentModerationProviderProfileView `json:"openai_moderations"`
	AIChat                         ContentModerationAIChatConfigView    `json:"ai_chat"`
	BaseURL                        string                               `json:"base_url"`
	Model                          string                               `json:"model"`
	ProxyID                        *int64                               `json:"proxy_id"`
	APIKeyConfigured               bool                                 `json:"api_key_configured"`
	APIKeyMasked                   string                               `json:"api_key_masked"`
	APIKeyCount                    int                                  `json:"api_key_count"`
	APIKeyMasks                    []string                             `json:"api_key_masks"`
	APIKeyStatuses                 []ContentModerationAPIKeyStatus      `json:"api_key_statuses"`
	TimeoutMS                      int                                  `json:"timeout_ms"`
	SampleRate                     int                                  `json:"sample_rate"`
	AllGroups                      bool                                 `json:"all_groups"`
	GroupIDs                       []int64                              `json:"group_ids"`
	RecordNonHits                  bool                                 `json:"record_non_hits"`
	Thresholds                     map[string]float64                   `json:"thresholds"`
	WorkerCount                    int                                  `json:"worker_count"`
	QueueSize                      int                                  `json:"queue_size"`
	BlockStatus                    int                                  `json:"block_status"`
	BlockMessage                   string                               `json:"block_message"`
	EmailOnHit                     bool                                 `json:"email_on_hit"`
	AutoBanEnabled                 bool                                 `json:"auto_ban_enabled"`
	BanThreshold                   int                                  `json:"ban_threshold"`
	ViolationWindowHours           int                                  `json:"violation_window_hours"`
	RetryCount                     int                                  `json:"retry_count"`
	HitRetentionDays               int                                  `json:"hit_retention_days"`
	NonHitRetentionDays            int                                  `json:"non_hit_retention_days"`
	PreHashCheckEnabled            bool                                 `json:"pre_hash_check_enabled"`
	BlockedKeywords                []string                             `json:"blocked_keywords"`
	KeywordBlockingMode            string                               `json:"keyword_blocking_mode"`
	ModelFilter                    ContentModerationModelFilter         `json:"model_filter"`
	UserFilter                     ContentModerationUserFilter          `json:"user_filter"`
	AccountFilter                  ContentModerationAccountFilter       `json:"account_filter"`
	CyberPolicyExcludeFromBanCount bool                                 `json:"cyber_policy_exclude_from_ban_count"`
}

type ContentModerationAIChatConfigView struct {
	ContentModerationProviderProfileView
	ConfidenceThreshold              float64  `json:"confidence_threshold"`
	CacheEnabled                     bool     `json:"cache_enabled"`
	CacheTTLSeconds                  int      `json:"cache_ttl_seconds"`
	SystemPrompt                     string   `json:"system_prompt"`
	RecommendedSystemPrompt          string   `json:"recommended_system_prompt"`
	RecommendedPromptVersion         string   `json:"recommended_prompt_version"`
	SystemPromptVersion              string   `json:"system_prompt_version"`
	UsesRecommendedSystemPrompt      bool     `json:"uses_recommended_system_prompt"`
	FailurePolicy                    string   `json:"failure_policy"`
	MaxInputChars                    int      `json:"max_input_chars"`
	SynchronousBudgetMS              int      `json:"synchronous_budget_ms"`
	FastInputChars                   int      `json:"fast_input_chars"`
	FallbackInputChars               int      `json:"fallback_input_chars"`
	ThinkingMode                     string   `json:"thinking_mode"`
	ReasoningEffort                  string   `json:"reasoning_effort"`
	RiskLevelsEnabled                bool     `json:"risk_levels_enabled"`
	ObserveThreshold                 float64  `json:"observe_threshold"`
	SessionRiskEnabled               bool     `json:"session_risk_enabled"`
	SessionRiskTTLMinutes            int      `json:"session_risk_ttl_minutes"`
	SessionRiskHalfLifeMinutes       int      `json:"session_risk_half_life_minutes"`
	SessionRiskBlockCooldownMinutes  int      `json:"session_risk_block_cooldown_minutes"`
	ActorRiskEnabled                 bool     `json:"actor_risk_enabled"`
	IncrementalAuditEnabled          bool     `json:"incremental_audit_enabled"`
	InputProvenanceV2Enabled         bool     `json:"input_provenance_v2_enabled"`
	DeterministicRiskV2Enabled       bool     `json:"deterministic_risk_v2_enabled"`
	RecentUserTurns                  int      `json:"recent_user_turns"`
	SummaryMaxChars                  int      `json:"summary_max_chars"`
	FullReviewThreshold              float64  `json:"full_review_threshold"`
	FullReviewRiskDelta              float64  `json:"full_review_risk_delta"`
	PeriodicFullReviewTurns          int      `json:"periodic_full_review_turns"`
	FullReviewMaxInputChars          int      `json:"full_review_max_input_chars"`
	FastMaxOutputTokens              int      `json:"fast_max_output_tokens"`
	FullMaxOutputTokens              int      `json:"full_max_output_tokens"`
	MaxReviewMaxOutputTokens         int      `json:"max_review_max_output_tokens"`
	AuditContextTTLMinutes           int      `json:"audit_context_ttl_minutes"`
	PricingConfigured                bool     `json:"pricing_configured"`
	PricingVersion                   string   `json:"pricing_version"`
	UncachedInputUSDPerMillionTokens *float64 `json:"uncached_input_usd_per_million_tokens"`
	CachedInputUSDPerMillionTokens   *float64 `json:"cached_input_usd_per_million_tokens"`
	OutputUSDPerMillionTokens        *float64 `json:"output_usd_per_million_tokens"`
}

type ContentModerationAPIKeyStatus struct {
	Index          int        `json:"index"`
	KeyHash        string     `json:"key_hash"`
	Masked         string     `json:"masked"`
	Status         string     `json:"status"`
	FailureCount   int        `json:"failure_count"`
	SuccessCount   int64      `json:"success_count"`
	LastError      string     `json:"last_error"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	FrozenUntil    *time.Time `json:"frozen_until,omitempty"`
	LastLatencyMS  int        `json:"last_latency_ms"`
	LastHTTPStatus int        `json:"last_http_status"`
	LastTested     bool       `json:"last_tested"`
	Configured     bool       `json:"configured"`
}

type ContentModerationAPIKeyLoad struct {
	Index          int    `json:"index"`
	KeyHash        string `json:"key_hash"`
	Masked         string `json:"masked"`
	Status         string `json:"status"`
	Active         int64  `json:"active"`
	Total          int64  `json:"total"`
	Success        int64  `json:"success"`
	Errors         int64  `json:"errors"`
	AvgLatencyMS   int64  `json:"avg_latency_ms"`
	LastLatencyMS  int    `json:"last_latency_ms"`
	LastHTTPStatus int    `json:"last_http_status"`
}

type TestContentModerationAPIKeysInput struct {
	APIKeys       []string `json:"api_keys"`
	AuditProvider string   `json:"audit_provider"`
	BaseURL       string   `json:"base_url"`
	Model         string   `json:"model"`
	TimeoutMS     int      `json:"timeout_ms"`
	// ProxyID nil 表示沿用已保存配置的代理；<=0 表示强制直连测试；>0 表示指定代理测试。
	ProxyID               *int64   `json:"proxy_id"`
	Prompt                string   `json:"prompt"`
	Images                []string `json:"images"`
	AIConfidenceThreshold float64  `json:"ai_confidence_threshold"`
	AISystemPrompt        string   `json:"ai_system_prompt"`
	AIMaxInputChars       int      `json:"ai_max_input_chars"`
	AISynchronousBudgetMS int      `json:"ai_synchronous_budget_ms"`
	AIFastInputChars      int      `json:"ai_fast_input_chars"`
	AIFallbackInputChars  int      `json:"ai_fallback_input_chars"`
	AIThinkingMode        string   `json:"ai_thinking_mode"`
	AIReasoningEffort     string   `json:"ai_reasoning_effort"`
	AIRiskLevelsEnabled   *bool    `json:"ai_risk_levels_enabled"`
	AIObserveThreshold    float64  `json:"ai_observe_threshold"`
}

type TestContentModerationAPIKeysResult struct {
	Items       []ContentModerationAPIKeyStatus   `json:"items"`
	AuditResult *ContentModerationTestAuditResult `json:"audit_result,omitempty"`
	AuditError  *ContentModerationTestAuditError  `json:"audit_error,omitempty"`
	ImageCount  int                               `json:"image_count"`
}

type ContentModerationTestAuditError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type ContentModerationTestAuditResult struct {
	Flagged          bool               `json:"flagged"`
	RiskScore        float64            `json:"risk_score"`
	RiskTier         string             `json:"risk_tier"`
	Categories       []string           `json:"categories"`
	Signals          []string           `json:"signals"`
	HighestCategory  string             `json:"highest_category"`
	HighestScore     float64            `json:"highest_score"`
	CompositeScore   float64            `json:"composite_score"`
	CategoryScores   map[string]float64 `json:"category_scores"`
	Thresholds       map[string]float64 `json:"thresholds"`
	Reason           string             `json:"reason,omitempty"`
	ReviewIncomplete bool               `json:"review_incomplete"`
	ReviewError      string             `json:"review_error,omitempty"`
	Scope            string             `json:"scope"`
}

type UpdateContentModerationConfigInput struct {
	Enabled       *bool   `json:"enabled"`
	Mode          *string `json:"mode"`
	AuditProvider *string `json:"audit_provider"`
	BaseURL       *string `json:"base_url"`
	Model         *string `json:"model"`
	// ProxyID nil 表示不修改；<=0 表示清除代理（恢复直连）；>0 表示指定代理。
	ProxyID                            *int64                          `json:"proxy_id"`
	APIKey                             *string                         `json:"api_key"`
	APIKeys                            *[]string                       `json:"api_keys"`
	APIKeysMode                        string                          `json:"api_keys_mode"`
	DeleteAPIKeyHashes                 *[]string                       `json:"delete_api_key_hashes"`
	ClearAPIKey                        bool                            `json:"clear_api_key"`
	TimeoutMS                          *int                            `json:"timeout_ms"`
	SampleRate                         *int                            `json:"sample_rate"`
	AllGroups                          *bool                           `json:"all_groups"`
	GroupIDs                           *[]int64                        `json:"group_ids"`
	RecordNonHits                      *bool                           `json:"record_non_hits"`
	Thresholds                         *map[string]float64             `json:"thresholds"`
	WorkerCount                        *int                            `json:"worker_count"`
	QueueSize                          *int                            `json:"queue_size"`
	BlockStatus                        *int                            `json:"block_status"`
	BlockMessage                       *string                         `json:"block_message"`
	EmailOnHit                         *bool                           `json:"email_on_hit"`
	AutoBanEnabled                     *bool                           `json:"auto_ban_enabled"`
	BanThreshold                       *int                            `json:"ban_threshold"`
	ViolationWindowHours               *int                            `json:"violation_window_hours"`
	RetryCount                         *int                            `json:"retry_count"`
	HitRetentionDays                   *int                            `json:"hit_retention_days"`
	NonHitRetentionDays                *int                            `json:"non_hit_retention_days"`
	PreHashCheckEnabled                *bool                           `json:"pre_hash_check_enabled"`
	BlockedKeywords                    *[]string                       `json:"blocked_keywords"`
	KeywordBlockingMode                *string                         `json:"keyword_blocking_mode"`
	ModelFilter                        *ContentModerationModelFilter   `json:"model_filter"`
	UserFilter                         *ContentModerationUserFilter    `json:"user_filter"`
	AccountFilter                      *ContentModerationAccountFilter `json:"account_filter"`
	CyberPolicyExcludeFromBanCount     *bool                           `json:"cyber_policy_exclude_from_ban_count"`
	AIConfidenceThreshold              *float64                        `json:"ai_confidence_threshold"`
	AICacheEnabled                     *bool                           `json:"ai_cache_enabled"`
	AICacheTTLSeconds                  *int                            `json:"ai_cache_ttl_seconds"`
	AISystemPrompt                     *string                         `json:"ai_system_prompt"`
	AIFailurePolicy                    *string                         `json:"ai_failure_policy"`
	AIMaxInputChars                    *int                            `json:"ai_max_input_chars"`
	AISynchronousBudgetMS              *int                            `json:"ai_synchronous_budget_ms"`
	AIFastInputChars                   *int                            `json:"ai_fast_input_chars"`
	AIFallbackInputChars               *int                            `json:"ai_fallback_input_chars"`
	AIThinkingMode                     *string                         `json:"ai_thinking_mode"`
	AIReasoningEffort                  *string                         `json:"ai_reasoning_effort"`
	AIRiskLevelsEnabled                *bool                           `json:"ai_risk_levels_enabled"`
	AIObserveThreshold                 *float64                        `json:"ai_observe_threshold"`
	AISessionRiskEnabled               *bool                           `json:"ai_session_risk_enabled"`
	AISessionRiskTTLMinutes            *int                            `json:"ai_session_risk_ttl_minutes"`
	AISessionRiskHalfLifeMinutes       *int                            `json:"ai_session_risk_half_life_minutes"`
	AISessionRiskBlockCooldownMinutes  *int                            `json:"ai_session_risk_block_cooldown_minutes"`
	AIActorRiskEnabled                 *bool                           `json:"ai_actor_risk_enabled"`
	AIIncrementalAuditEnabled          *bool                           `json:"ai_incremental_audit_enabled"`
	AIInputProvenanceV2Enabled         *bool                           `json:"ai_input_provenance_v2_enabled"`
	AIDeterministicRiskV2Enabled       *bool                           `json:"ai_deterministic_risk_v2_enabled"`
	AIRecentUserTurns                  *int                            `json:"ai_recent_user_turns"`
	AISummaryMaxChars                  *int                            `json:"ai_summary_max_chars"`
	AIFullReviewThreshold              *float64                        `json:"ai_full_review_threshold"`
	AIFullReviewRiskDelta              *float64                        `json:"ai_full_review_risk_delta"`
	AIPeriodicFullReviewTurns          *int                            `json:"ai_periodic_full_review_turns"`
	AIFullReviewMaxInputChars          *int                            `json:"ai_full_review_max_input_chars"`
	AIFastMaxOutputTokens              *int                            `json:"ai_fast_max_output_tokens"`
	AIFullMaxOutputTokens              *int                            `json:"ai_full_max_output_tokens"`
	AIMaxReviewMaxOutputTokens         *int                            `json:"ai_max_review_max_output_tokens"`
	AIAuditContextTTLMinutes           *int                            `json:"ai_audit_context_ttl_minutes"`
	AIPricingConfigured                *bool                           `json:"ai_pricing_configured"`
	AIPricingVersion                   *string                         `json:"ai_pricing_version"`
	AIUncachedInputUSDPerMillionTokens *float64                        `json:"ai_uncached_input_usd_per_million_tokens"`
	AICachedInputUSDPerMillionTokens   *float64                        `json:"ai_cached_input_usd_per_million_tokens"`
	AIOutputUSDPerMillionTokens        *float64                        `json:"ai_output_usd_per_million_tokens"`
}

type ContentModerationModelFilter struct {
	Type   string   `json:"type"`
	Models []string `json:"models"`
}

type ContentModerationUserFilter struct {
	Type    string  `json:"type"`
	UserIDs []int64 `json:"user_ids"`
}

type ContentModerationAccountFilter struct {
	Type       string  `json:"type"`
	AccountIDs []int64 `json:"account_ids"`
}

type ContentModerationCheckInput struct {
	RequestID  string
	UserID     int64
	UserEmail  string
	APIKeyID   int64
	APIKeyName string
	// AccountID is the selected upstream account. Zero means moderation ran
	// before account selection or an older caller did not propagate it.
	AccountID int64
	SessionID string
	// SessionSource is the observable, bounded origin of SessionID: header,
	// prompt_cache_key, or none.
	SessionSource string
	GroupID       *int64
	GroupName     string
	Endpoint      string
	Provider      string
	Model         string
	Protocol      string
	Body          []byte
	// ClientHeaders contains only identity/fingerprint headers selected by the
	// gateway. It must never contain credentials or cookies.
	ClientHeaders http.Header
	// TrustedMetadataProvenance is set only by an internal gateway adapter after
	// it has authenticated the request provenance. Inbound headers and body
	// fields must never set this bit by themselves.
	TrustedMetadataProvenance bool

	ModerationEpoch    int64
	ModerationEpochSet bool
}

type ContentModerationInput struct {
	Text            string
	CurrentText     string
	Images          []string
	Turns           []ContentModerationTurn
	AuditTargetText string
	AuditTargetKind string
	HasExplicitUser bool
	TrustedClient   bool
	TrustedSignals  []string
	IgnoredMetadata []string
}

type ContentModerationTurn struct {
	Role               string
	Source             string
	Purpose            string
	MetadataKind       string
	Text               string
	ToolCall           bool
	Truncated          bool
	Current            bool
	LinkedToUserIntent bool
	MetadataEnvelope   bool
	MetadataHint       string
}

func (in *ContentModerationInput) Normalize() {
	if in == nil {
		return
	}
	in.Text = trimModerationContext(in.Text, maxModerationInputRunes)
	in.CurrentText = trimRunes(normalizeContentModerationText(in.CurrentText), maxModerationInputRunes)
	in.Images = normalizeModerationImages(in.Images)
	in.AuditTargetText = strings.TrimSpace(in.AuditTargetText)
	in.AuditTargetKind = strings.TrimSpace(in.AuditTargetKind)
	in.TrustedSignals = normalizeContentModerationStringValues(in.TrustedSignals)
	in.IgnoredMetadata = normalizeContentModerationStringValues(in.IgnoredMetadata)
	for idx := range in.Turns {
		in.Turns[idx].Role = strings.ToLower(strings.TrimSpace(in.Turns[idx].Role))
		in.Turns[idx].Source = strings.ToLower(strings.TrimSpace(in.Turns[idx].Source))
		in.Turns[idx].Purpose = strings.ToLower(strings.TrimSpace(in.Turns[idx].Purpose))
		in.Turns[idx].MetadataKind = strings.ToLower(strings.TrimSpace(in.Turns[idx].MetadataKind))
		in.Turns[idx].Text = strings.TrimSpace(in.Turns[idx].Text)
	}
}

func (in ContentModerationInput) IsEmpty() bool {
	return strings.TrimSpace(in.Text) == "" && len(in.Images) == 0
}

func (in ContentModerationInput) ModerationInput() any {
	return in.moderationInputWithLimit(legacyModerationInputRunes)
}

func (in ContentModerationInput) AIChatModerationInput() any {
	copyInput := in
	if contentModerationHasAuditTarget(in) {
		copyInput.Text = renderContentModerationAIChatInput(in, maxModerationInputRunes)
	}
	return copyInput.moderationInputWithLimit(maxModerationInputRunes)
}

func renderContentModerationAIChatInput(in ContentModerationInput, maxRunes int) string {
	targetKind := defaultContentModerationString(strings.TrimSpace(in.AuditTargetKind), "user_request")
	targetText := voteaiauditcontext.RedactSecrets(strings.TrimSpace(in.AuditTargetText))
	targetBlock := fmt.Sprintf("[AUDIT-TARGET kind=%s]\n%s", targetKind, targetText)
	if len([]rune(targetBlock)) >= maxRunes {
		return trimModerationContext(targetBlock, maxRunes)
	}

	turns := make([]voteaiauditcontext.Turn, 0, len(in.Turns))
	for _, turn := range in.Turns {
		if turn.Source == "trusted_metadata" || turn.Purpose == "ignored" || turn.Purpose == "audit_target" {
			continue
		}
		text := strings.TrimSpace(turn.Text)
		if turn.Role == "tool" {
			text = sanitizeContentModerationToolOutput(text, 1000)
		}
		if text == "" {
			continue
		}
		turns = append(turns, voteaiauditcontext.Turn{Role: voteaiauditcontext.Role(turn.Role), Text: text})
	}
	history := renderContentModerationAuditTurns(turns)
	historyLimit := max(0, maxRunes-len([]rune(targetBlock))-32)
	if len([]rune(history)) > historyLimit {
		history = trimModerationContext(history, historyLimit)
	}
	if history == "" {
		return targetBlock
	}
	return "[CONVERSATION-HISTORY]\n" + history + "\n\n" + targetBlock
}

func (in ContentModerationInput) moderationInputWithLimit(maxRunes int) any {
	images := limitContentModerationImages(in.Images)
	text := trimModerationContext(in.Text, maxRunes)
	if len(images) == 0 {
		return text
	}
	parts := make([]moderationAPIInputPart, 0, len(images)+1)
	if strings.TrimSpace(text) != "" {
		parts = append(parts, moderationAPIInputPart{Type: "text", Text: text})
	}
	for _, image := range images {
		parts = append(parts, moderationAPIInputPart{
			Type:     "image_url",
			ImageURL: &moderationAPIImageURLRef{URL: image},
		})
	}
	return parts
}

func (in ContentModerationInput) ExcerptText() string {
	if strings.TrimSpace(in.CurrentText) != "" {
		return in.CurrentText
	}
	return in.Text
}

func (in ContentModerationInput) Hash() string {
	h := sha256.New()
	_, _ = h.Write([]byte("text:"))
	_, _ = h.Write([]byte(in.Text))
	for _, image := range in.Images {
		imageHash := sha256.Sum256([]byte(image))
		_, _ = h.Write([]byte("\nimage:"))
		_, _ = h.Write([]byte(hex.EncodeToString(imageHash[:])))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (in ContentModerationInput) AuditTargetHash(policyVersion string) string {
	h := sha256.New()
	_, _ = h.Write([]byte("content-moderation-target:v2\x00"))
	_, _ = h.Write([]byte(strings.TrimSpace(policyVersion)))
	_, _ = h.Write([]byte("\x00"))
	_, _ = h.Write([]byte(strings.TrimSpace(in.AuditTargetKind)))
	_, _ = h.Write([]byte("\x00"))
	_, _ = h.Write([]byte(voteaiauditcontext.RedactSecrets(normalizeContentModerationText(in.AuditTargetText))))
	_, _ = h.Write([]byte("\x00supporting-context\x00"))
	_, _ = h.Write([]byte(in.auditTargetHashSupportingContext()))
	return hex.EncodeToString(h.Sum(nil))
}

// auditTargetHashSupportingContext returns at most one directly adjacent,
// provenance-normalized turn for referential targets such as "continue".
// Unrelated history and trusted machine metadata must not affect a global hash.
func (in ContentModerationInput) auditTargetHashSupportingContext() string {
	if !voteaiauditcontext.NeedsPreviousContext(in.AuditTargetText) {
		return ""
	}
	targetIndex := -1
	for index, turn := range in.Turns {
		if strings.EqualFold(strings.TrimSpace(turn.Purpose), "audit_target") {
			targetIndex = index
			break
		}
	}
	if targetIndex <= 0 {
		return ""
	}
	for index := targetIndex - 1; index >= 0; index-- {
		turn := in.Turns[index]
		if !strings.EqualFold(strings.TrimSpace(turn.Purpose), "supporting_context") ||
			strings.EqualFold(strings.TrimSpace(turn.Source), "trusted_metadata") {
			continue
		}
		text := voteaiauditcontext.RedactSecrets(normalizeContentModerationText(turn.Text))
		if text == "" {
			continue
		}
		text = trimModerationContext(text, 2000)
		return strings.ToLower(strings.TrimSpace(turn.Role)) + "\x00" +
			strings.ToLower(strings.TrimSpace(turn.Source)) + "\x00" + text
	}
	return ""
}

func normalizeContentModerationStringValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

type ContentModerationDecision struct {
	Allowed                 bool               `json:"allowed"`
	Blocked                 bool               `json:"blocked"`
	Flagged                 bool               `json:"flagged"`
	Message                 string             `json:"message"`
	StatusCode              int                `json:"status_code"`
	InputHash               string             `json:"input_hash,omitempty"`
	HighestCategory         string             `json:"highest_category"`
	HighestScore            float64            `json:"highest_score"`
	CategoryScores          map[string]float64 `json:"category_scores"`
	Action                  string             `json:"action"`
	RiskTier                string             `json:"risk_tier,omitempty"`
	CurrentRiskScore        float64            `json:"current_risk_score,omitempty"`
	CumulativeRiskScore     float64            `json:"cumulative_risk_score,omitempty"`
	requestVerdictCacheable bool               `json:"-"`
}

type ContentModerationLog struct {
	ID                  int64                         `json:"id"`
	RequestID           string                        `json:"request_id"`
	SessionID           string                        `json:"-"`
	SessionSource       string                        `json:"-"`
	InputHash           string                        `json:"-"`
	UserID              *int64                        `json:"user_id,omitempty"`
	UserEmail           string                        `json:"user_email"`
	APIKeyID            *int64                        `json:"api_key_id,omitempty"`
	APIKeyName          string                        `json:"api_key_name"`
	GroupID             *int64                        `json:"group_id,omitempty"`
	GroupName           string                        `json:"group_name"`
	Endpoint            string                        `json:"endpoint"`
	Provider            string                        `json:"provider"`
	Model               string                        `json:"model"`
	Mode                string                        `json:"mode"`
	Action              string                        `json:"action"`
	Flagged             bool                          `json:"flagged"`
	HighestCategory     string                        `json:"highest_category"`
	HighestScore        float64                       `json:"highest_score"`
	MatchedKeyword      string                        `json:"matched_keyword"`
	CategoryScores      map[string]float64            `json:"category_scores"`
	ThresholdSnapshot   map[string]float64            `json:"threshold_snapshot"`
	InputExcerpt        string                        `json:"input_excerpt"`
	UpstreamLatencyMS   *int                          `json:"upstream_latency_ms,omitempty"`
	Error               string                        `json:"error"`
	AuditStatus         string                        `json:"audit_status"`
	AuditCode           string                        `json:"audit_code"`
	AuditRetryable      bool                          `json:"audit_retryable"`
	ViolationCount      int                           `json:"violation_count"`
	AutoBanned          bool                          `json:"auto_banned"`
	EmailSent           bool                          `json:"email_sent"`
	SideEffectStatus    string                        `json:"side_effect_status"`
	NotificationStatus  string                        `json:"notification_status"`
	SideEffectError     string                        `json:"side_effect_error"`
	UserStatus          string                        `json:"user_status"`
	ModerationBanActive bool                          `json:"moderation_ban_active"`
	UnbanBlockReason    string                        `json:"unban_block_reason,omitempty"`
	QueueDelayMS        *int                          `json:"queue_delay_ms,omitempty"`
	AuditDetails        ContentModerationAuditDetails `json:"audit_details"`
	CreatedAt           time.Time                     `json:"created_at"`
}

type ContentModerationLogFilter struct {
	Pagination pagination.PaginationParams
	Result     string
	GroupID    *int64
	Endpoint   string
	Search     string
	From       *time.Time
	To         *time.Time
}

type ContentModerationCleanupResult struct {
	DeletedHit    int64     `json:"deleted_hit"`
	DeletedNonHit int64     `json:"deleted_non_hit"`
	FinishedAt    time.Time `json:"finished_at"`
}

type ContentModerationRuntimeStatus struct {
	Enabled                      bool                            `json:"enabled"`
	RiskControlEnabled           bool                            `json:"risk_control_enabled"`
	Mode                         string                          `json:"mode"`
	WorkerCount                  int                             `json:"worker_count"`
	MaxWorkers                   int                             `json:"max_workers"`
	ActiveWorkers                int                             `json:"active_workers"`
	IdleWorkers                  int                             `json:"idle_workers"`
	QueueSize                    int                             `json:"queue_size"`
	QueueLength                  int                             `json:"queue_length"`
	QueueUsagePercent            float64                         `json:"queue_usage_percent"`
	Enqueued                     int64                           `json:"enqueued"`
	Dropped                      int64                           `json:"dropped"`
	Processed                    int64                           `json:"processed"`
	Errors                       int64                           `json:"errors"`
	PreBlockActive               int                             `json:"pre_block_active"`
	PreBlockChecked              int64                           `json:"pre_block_checked"`
	PreBlockAllowed              int64                           `json:"pre_block_allowed"`
	PreBlockBlocked              int64                           `json:"pre_block_blocked"`
	PreBlockErrors               int64                           `json:"pre_block_errors"`
	PreBlockAvgLatencyMS         int64                           `json:"pre_block_avg_latency_ms"`
	PreBlockAPIKeyActive         int64                           `json:"pre_block_api_key_active"`
	PreBlockAPIKeyAvailableCount int64                           `json:"pre_block_api_key_available_count"`
	PreBlockAPIKeyTotalCalls     int64                           `json:"pre_block_api_key_total_calls"`
	PreBlockAPIKeyLoads          []ContentModerationAPIKeyLoad   `json:"pre_block_api_key_loads"`
	APIKeyStatuses               []ContentModerationAPIKeyStatus `json:"api_key_statuses"`
	// CUSTOM(VOTE-AI-AUDIT-COST/METRICS): process-lifetime counters for incremental audit cost visibility.
	AuditFastCalls           int64                                      `json:"audit_fast_calls"`
	AuditFullCalls           int64                                      `json:"audit_full_calls"`
	AuditMaxCalls            int64                                      `json:"audit_max_calls"`
	AuditResultCacheHits     int64                                      `json:"audit_result_cache_hits"`
	AuditPromptTokens        int64                                      `json:"audit_prompt_tokens"`
	AuditCachedInputTokens   int64                                      `json:"audit_cached_input_tokens"`
	AuditUncachedInputTokens int64                                      `json:"audit_uncached_input_tokens"`
	AuditOutputTokens        int64                                      `json:"audit_output_tokens"`
	AuditUsageComplete       int64                                      `json:"audit_usage_complete"`
	AuditUsageUnknown        int64                                      `json:"audit_usage_unknown"`
	AuditInputChars          int64                                      `json:"audit_input_chars"`
	MetricsStartedAt         time.Time                                  `json:"metrics_started_at"`
	AuditEstimatedCostUSD    *float64                                   `json:"audit_estimated_cost_usd"`
	AuditCostCoverage        string                                     `json:"audit_cost_coverage"`
	AuditCostComplete        bool                                       `json:"audit_cost_complete"`
	AuditCostPartial         bool                                       `json:"audit_cost_partial"`
	AuditCostPricedSamples   int64                                      `json:"audit_cost_priced_samples"`
	AuditCostUnpricedSamples int64                                      `json:"audit_cost_unpriced_samples"`
	AuditCostByVersionUSD    map[string]float64                         `json:"audit_cost_by_pricing_version_usd"`
	BusinessActualCostUSD    *float64                                   `json:"business_actual_cost_usd"`
	AuditCostPerBusinessUSD  *float64                                   `json:"audit_cost_per_business_usd"`
	AuditStageLatency        map[string]ContentModerationLatencySummary `json:"audit_stage_latency"`
	AuditSessionSources      map[string]int64                           `json:"audit_session_sources"`
	AuditPrefixContinuity    map[string]int64                           `json:"audit_prefix_continuity"`
	FlaggedHashCount         int64                                      `json:"flagged_hash_count"`
	LastCleanupAt            *time.Time                                 `json:"last_cleanup_at,omitempty"`
	LastCleanupDeletedHit    int64                                      `json:"last_cleanup_deleted_hit"`
	LastCleanupDeletedNonHit int64                                      `json:"last_cleanup_deleted_non_hit"`
}

type ContentModerationLatencySummary struct {
	Count      int64 `json:"count"`
	AverageMS  int64 `json:"average_ms"`
	P95UpperMS int64 `json:"p95_upper_ms"`
}

var contentModerationLatencyBucketUpperMS = [...]int64{250, 500, 1000, 2000, 3500, 5000, 10000, 30000, 60000}

type contentModerationLatencyMetrics struct {
	count   atomic.Int64
	totalMS atomic.Int64
	buckets [len(contentModerationLatencyBucketUpperMS)]atomic.Int64
}

func (m *contentModerationLatencyMetrics) observe(latencyMS int) {
	if m == nil {
		return
	}
	if latencyMS < 0 {
		latencyMS = 0
	}
	m.count.Add(1)
	m.totalMS.Add(int64(latencyMS))
	for index, upper := range contentModerationLatencyBucketUpperMS {
		if int64(latencyMS) <= upper {
			m.buckets[index].Add(1)
			return
		}
	}
	m.buckets[len(m.buckets)-1].Add(1)
}

func (m *contentModerationLatencyMetrics) snapshot() ContentModerationLatencySummary {
	if m == nil {
		return ContentModerationLatencySummary{}
	}
	count := m.count.Load()
	if count <= 0 {
		return ContentModerationLatencySummary{}
	}
	target := (count*95 + 99) / 100
	cumulative := int64(0)
	p95 := contentModerationLatencyBucketUpperMS[len(contentModerationLatencyBucketUpperMS)-1]
	for index, upper := range contentModerationLatencyBucketUpperMS {
		cumulative += m.buckets[index].Load()
		if cumulative >= target {
			p95 = upper
			break
		}
	}
	return ContentModerationLatencySummary{Count: count, AverageMS: m.totalMS.Load() / count, P95UpperMS: p95}
}

type ContentModerationUnbanUserResult struct {
	UserID           int64  `json:"user_id"`
	Status           string `json:"status"`
	Mode             string `json:"mode"`
	Restored         bool   `json:"restored"`
	RiskStateCleared bool   `json:"risk_state_cleared"`
	Warning          string `json:"warning,omitempty"`
}

type ContentModerationLogEffectsPatch struct {
	ViolationCount     int
	AutoBanned         bool
	EmailSent          bool
	SideEffectStatus   string
	NotificationStatus string
	SideEffectError    string
	AuditDetails       *ContentModerationAuditDetails
}

type ContentModerationUserState struct {
	UserID                  int64
	ModerationOwnedDisabled bool
	DisabledLogID           *int64
	DisabledAt              *time.Time
	UpdatedAt               time.Time
}

type ContentModerationDeleteHashResult struct {
	InputHash string `json:"input_hash"`
	Deleted   bool   `json:"deleted"`
}

type ContentModerationClearHashesResult struct {
	Deleted int64 `json:"deleted"`
}

type ContentModerationRepository interface {
	CreateLog(ctx context.Context, log *ContentModerationLog) error
	ListLogs(ctx context.Context, filter ContentModerationLogFilter) ([]ContentModerationLog, *pagination.PaginationResult, error)
	// CountFlaggedByUserSince 统计窗口内计入封号的违规次数（排除 hash_block；
	// excludeCyberPolicy 为 true 时额外排除 cyber_policy 行）。
	CountFlaggedByUserSince(ctx context.Context, userID int64, since time.Time, excludeCyberPolicy bool) (int, error)
	CleanupExpiredLogs(ctx context.Context, hitBefore time.Time, nonHitBefore time.Time) (*ContentModerationCleanupResult, error)
	// UpdateLogEmailSent 回写邮件发送结果（F7：CreateLog 先行后补 EmailSent）。
	UpdateLogEmailSent(ctx context.Context, id int64, sent bool) error
}

// ContentModerationBusinessCostReader is an optional, narrow capability used
// only by runtime observability. Existing repository mocks do not need to
// implement it.
type ContentModerationBusinessCostReader interface {
	SumBusinessActualCostSince(ctx context.Context, since time.Time) (float64, error)
}

type ContentModerationLifecycleRepository interface {
	UpdateLogEffects(ctx context.Context, logID int64, patch ContentModerationLogEffectsPatch) error
	GetModerationUserState(ctx context.Context, userID int64) (*ContentModerationUserState, error)
	TryApplyModerationOwnedBan(ctx context.Context, userID, logID int64, disabledAt time.Time) (string, error)
	RestoreModerationOwnedBan(ctx context.Context, userID int64) (bool, error)
}

type ContentModerationHashCache interface {
	RecordFlaggedInputHash(ctx context.Context, inputHash string) error
	HasFlaggedInputHash(ctx context.Context, inputHash string) (bool, error)
	DeleteFlaggedInputHash(ctx context.Context, inputHash string) (bool, error)
	ClearFlaggedInputHashes(ctx context.Context) (int64, error)
	CountFlaggedInputHashes(ctx context.Context) (int64, error)
}

// CUSTOM(VOTE-AI-AI-AUDIT): optional TTL cache for successful semantic audit results.
type ContentModerationResultCache interface {
	GetContentModerationResult(ctx context.Context, key string) ([]byte, bool, error)
	SetContentModerationResult(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

// ContentModerationFlaggedHashLifecycleStore fences stale result-cache reads
// and delayed async promotion after an administrator removes a false positive.
type ContentModerationFlaggedHashLifecycleStore interface {
	RecordFlaggedInputHashIfAllowed(ctx context.Context, inputHash string) (bool, error)
	IsFlaggedInputHashSuppressed(ctx context.Context, inputHash string) (bool, error)
}

type ContentModerationSessionRiskStore interface {
	GetContentModerationSessionRisk(ctx context.Context, key string) (voteairiskstate.State, bool, error)
	UpdateContentModerationSessionRisk(ctx context.Context, key string, event voteairiskstate.Event, cfg voteairiskstate.Config) (voteairiskstate.State, error)
}

// CUSTOM(VOTE-AI-AUDIT-CONTEXT): optional short-lived structured state store.
// Implementations must use opaque keys and must never persist raw conversation text.
type ContentModerationAuditContextStore interface {
	GetContentModerationAuditContext(ctx context.Context, key string) (voteaiauditcontext.State, bool, error)
	UpdateContentModerationAuditContextForUser(ctx context.Context, userID int64, key string, event voteaiauditcontext.AuditEvent, cfg voteaiauditcontext.Config, ttl time.Duration) (voteaiauditcontext.State, error)
	UpdateContentModerationAuditPrefixForUser(ctx context.Context, userID int64, key string, observation voteaiauditcontext.PrefixObservation, ttl time.Duration) (voteaiauditcontext.State, error)
}

type ContentModerationUserStateCleaner interface {
	ClearContentModerationUserState(ctx context.Context, userID int64) (int64, error)
}

// ContentModerationUserStateEpochStore fences queued moderation work across an
// administrator-initiated unban. Clearing user state advances the same epoch.
type ContentModerationUserStateEpochStore interface {
	GetContentModerationUserEpoch(ctx context.Context, userID int64) (int64, error)
	AdvanceContentModerationUserEpoch(ctx context.Context, userID int64) (int64, error)
}

// ContentModerationAccountScopeRepository is the narrow account lookup used to
// canonicalize Spark shadow IDs in persisted moderation scope filters.
type ContentModerationAccountScopeRepository interface {
	GetByIDs(ctx context.Context, ids []int64) ([]*Account, error)
}

type ContentModerationService struct {
	settingRepo              SettingRepository
	repo                     ContentModerationRepository
	hashCache                ContentModerationHashCache
	accountScopeRepo         ContentModerationAccountScopeRepository
	groupRepo                GroupRepository
	userRepo                 UserRepository
	proxyRepo                ProxyRepository
	authCacheInvalidator     APIKeyAuthCacheInvalidator
	emailService             *EmailService
	httpClient               *http.Client
	moderationProxyCache     atomic.Pointer[moderationProxyURLCacheEntry]
	asyncQueue               chan contentModerationTask
	workerCount              int
	apiKeyCursor             atomic.Uint64
	asyncActive              atomic.Int64
	asyncEnqueued            atomic.Int64
	asyncDropped             atomic.Int64
	asyncProcessed           atomic.Int64
	asyncErrors              atomic.Int64
	supplementalPending      atomic.Int64
	preBlockActive           atomic.Int64
	preBlockChecked          atomic.Int64
	preBlockAllowed          atomic.Int64
	preBlockBlocked          atomic.Int64
	preBlockErrors           atomic.Int64
	preBlockLatencyTotalMS   atomic.Int64
	auditFastCalls           atomic.Int64
	auditFullCalls           atomic.Int64
	auditMaxCalls            atomic.Int64
	auditResultCacheHits     atomic.Int64
	auditInputChars          atomic.Int64
	auditPromptTokens        atomic.Int64
	auditCachedInputTokens   atomic.Int64
	auditUncachedInputTokens atomic.Int64
	auditOutputTokens        atomic.Int64
	auditUsageComplete       atomic.Int64
	auditUsageUnknown        atomic.Int64
	auditCostPricedSamples   atomic.Int64
	auditCostUnpricedSamples atomic.Int64
	metricsStartedUnixNano   atomic.Int64
	auditCostMu              sync.RWMutex
	auditEstimatedCostUSD    float64
	auditCostByVersionUSD    map[string]float64
	auditSessionHeader       atomic.Int64
	auditSessionPromptCache  atomic.Int64
	auditSessionNone         atomic.Int64
	auditFastLatency         contentModerationLatencyMetrics
	auditFullLatency         contentModerationLatencyMetrics
	auditMaxLatency          contentModerationLatencyMetrics
	auditPrefixContinuous    atomic.Int64
	auditPrefixBreaks        atomic.Int64
	auditPrefixBreakPolicy   atomic.Int64
	auditPrefixBreakModel    atomic.Int64
	auditPrefixBreakHistory  atomic.Int64
	auditPrefixBreakTruncate atomic.Int64
	auditPrefixBreakCompact  atomic.Int64
	auditPrefixBreakSession  atomic.Int64
	auditPrefixBreakKey      atomic.Int64
	auditPrefixBreakUnknown  atomic.Int64
	lastCleanupUnix          atomic.Int64
	lastCleanupDeletedHit    atomic.Int64
	lastCleanupDeletedNonHit atomic.Int64
	runtimeSnapshot          atomic.Pointer[contentModerationRuntimeSnapshot]
	runtimeRefreshMu         sync.Mutex
	runtimeCacheTTL          time.Duration
	runtimeRefreshRetryAt    atomic.Int64
	keyHealthMu              sync.Mutex
	keyHealth                map[string]*contentModerationKeyHealth
	moderationUserStateLocks [64]sync.RWMutex
	requestVerdictFlight     contentModerationRequestVerdictFlight
}

type contentModerationRuntimeSnapshot struct {
	riskControlEnabled       bool
	config                   *ContentModerationConfig
	keywordMatcher           *contentModerationKeywordMatcher
	configDigest             [sha256.Size]byte
	runtimeGeneration        [sha256.Size]byte
	accountScopeFallback     bool
	engineFingerprintSignals []openaipkg.EngineFingerprintSignal
	loadedAt                 time.Time
}

type contentModerationTask struct {
	input            ContentModerationCheckInput
	content          ContentModerationInput
	inputHash        string
	log              *ContentModerationLog
	config           *ContentModerationConfig
	configGeneration [sha256.Size]byte
	recordHash       bool
	applySideEffects bool
	supplemental     bool
	enqueuedAt       time.Time
}

type contentModerationKeyHealth struct {
	Hash           string
	Masked         string
	FailureCount   int
	SuccessCount   int64
	LastError      string
	LastCheckedAt  time.Time
	FrozenUntil    time.Time
	LastLatencyMS  int
	LastHTTPStatus int
	LastTested     bool
	SyncActive     int64
	SyncTotal      int64
	SyncSuccess    int64
	SyncErrors     int64
	SyncLatencyMS  int64
}

func NewContentModerationService(
	settingRepo SettingRepository,
	repo ContentModerationRepository,
	hashCache ContentModerationHashCache,
	groupRepo GroupRepository,
	userRepo UserRepository,
	proxyRepo ProxyRepository,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	emailService *EmailService,
) *ContentModerationService {
	return newContentModerationService(
		settingRepo,
		repo,
		hashCache,
		groupRepo,
		userRepo,
		proxyRepo,
		authCacheInvalidator,
		emailService,
		nil,
	)
}

func newContentModerationService(
	settingRepo SettingRepository,
	repo ContentModerationRepository,
	hashCache ContentModerationHashCache,
	groupRepo GroupRepository,
	userRepo UserRepository,
	proxyRepo ProxyRepository,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	emailService *EmailService,
	accountScopeRepo ContentModerationAccountScopeRepository,
) *ContentModerationService {
	svc := &ContentModerationService{
		settingRepo:           settingRepo,
		repo:                  repo,
		hashCache:             hashCache,
		accountScopeRepo:      accountScopeRepo,
		groupRepo:             groupRepo,
		userRepo:              userRepo,
		proxyRepo:             proxyRepo,
		authCacheInvalidator:  authCacheInvalidator,
		emailService:          emailService,
		httpClient:            servertiming.InstrumentClient(nil),
		workerCount:           maxContentModerationWorkerCount,
		asyncQueue:            make(chan contentModerationTask, maxContentModerationQueueSize),
		keyHealth:             make(map[string]*contentModerationKeyHealth),
		auditCostByVersionUSD: make(map[string]float64),
	}
	svc.metricsStartedUnixNano.Store(time.Now().UTC().UnixNano())
	if settingRepo != nil && repo != nil {
		for i := 0; i < svc.workerCount; i++ {
			go svc.worker(i)
		}
		go svc.cleanupWorker()
	}
	return svc
}

func (s *ContentModerationService) GetConfig(ctx context.Context) (*ContentModerationConfigView, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s.configView(cfg), nil
}

func (s *ContentModerationService) UpdateConfig(ctx context.Context, input UpdateContentModerationConfigInput) (*ContentModerationConfigView, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if input.Enabled != nil {
		cfg.Enabled = *input.Enabled
	}
	if input.Mode != nil {
		cfg.Mode = strings.TrimSpace(*input.Mode)
	}
	if input.AuditProvider != nil {
		cfg.AuditProvider = normalizeContentModerationProvider(*input.AuditProvider)
	}
	if input.BaseURL != nil {
		if cfg.AuditProvider == ContentModerationProviderAIChat {
			cfg.AIChat.BaseURL = strings.TrimSpace(*input.BaseURL)
		} else {
			cfg.BaseURL = strings.TrimSpace(*input.BaseURL)
		}
	}
	if input.Model != nil {
		if cfg.AuditProvider == ContentModerationProviderAIChat {
			cfg.AIChat.Model = strings.TrimSpace(*input.Model)
		} else {
			cfg.Model = strings.TrimSpace(*input.Model)
		}
	}
	if input.ProxyID != nil {
		if *input.ProxyID > 0 {
			id := *input.ProxyID
			if cfg.AuditProvider == ContentModerationProviderAIChat {
				cfg.AIChat.ProxyID = &id
			} else {
				cfg.ProxyID = &id
			}
		} else {
			if cfg.AuditProvider == ContentModerationProviderAIChat {
				cfg.AIChat.ProxyID = nil
			} else {
				cfg.ProxyID = nil
			}
		}
	}
	if input.TimeoutMS != nil {
		if cfg.AuditProvider == ContentModerationProviderAIChat {
			cfg.AIChat.TimeoutMS = *input.TimeoutMS
		} else {
			cfg.TimeoutMS = *input.TimeoutMS
		}
	}
	if input.SampleRate != nil {
		cfg.SampleRate = *input.SampleRate
	}
	if input.WorkerCount != nil {
		cfg.WorkerCount = *input.WorkerCount
	}
	if input.QueueSize != nil {
		cfg.QueueSize = *input.QueueSize
	}
	if input.BlockStatus != nil {
		cfg.BlockStatus = *input.BlockStatus
	}
	if input.BlockMessage != nil {
		cfg.BlockMessage = strings.TrimSpace(*input.BlockMessage)
	}
	if input.EmailOnHit != nil {
		cfg.EmailOnHit = *input.EmailOnHit
	}
	if input.AutoBanEnabled != nil {
		cfg.AutoBanEnabled = *input.AutoBanEnabled
	}
	if input.BanThreshold != nil {
		cfg.BanThreshold = *input.BanThreshold
	}
	if input.ViolationWindowHours != nil {
		cfg.ViolationWindowHours = *input.ViolationWindowHours
	}
	if input.RetryCount != nil {
		if cfg.AuditProvider == ContentModerationProviderAIChat {
			cfg.AIChat.RetryCount = *input.RetryCount
		} else {
			cfg.RetryCount = *input.RetryCount
		}
	}
	if input.AIConfidenceThreshold != nil {
		cfg.AIChat.ConfidenceThreshold = *input.AIConfidenceThreshold
	}
	if input.AICacheEnabled != nil {
		cfg.AIChat.CacheEnabled = *input.AICacheEnabled
	}
	if input.AICacheTTLSeconds != nil {
		cfg.AIChat.CacheTTLSeconds = *input.AICacheTTLSeconds
	}
	if input.AISystemPrompt != nil {
		cfg.AIChat.SystemPrompt = strings.TrimSpace(*input.AISystemPrompt)
	}
	if input.AIFailurePolicy != nil {
		cfg.AIChat.FailurePolicy = strings.TrimSpace(*input.AIFailurePolicy)
	}
	if input.AIMaxInputChars != nil {
		cfg.AIChat.MaxInputChars = *input.AIMaxInputChars
	}
	if input.AISynchronousBudgetMS != nil {
		cfg.AIChat.SynchronousBudgetMS = *input.AISynchronousBudgetMS
	}
	if input.AIFastInputChars != nil {
		cfg.AIChat.FastInputChars = *input.AIFastInputChars
	}
	if input.AIFallbackInputChars != nil {
		cfg.AIChat.FallbackInputChars = *input.AIFallbackInputChars
	}
	if input.AIThinkingMode != nil {
		cfg.AIChat.ThinkingMode = strings.TrimSpace(*input.AIThinkingMode)
	}
	if input.AIReasoningEffort != nil {
		cfg.AIChat.ReasoningEffort = strings.TrimSpace(*input.AIReasoningEffort)
	}
	if input.AIRiskLevelsEnabled != nil {
		cfg.AIChat.RiskLevelsEnabled = *input.AIRiskLevelsEnabled
	}
	if input.AIObserveThreshold != nil {
		cfg.AIChat.ObserveThreshold = *input.AIObserveThreshold
	}
	if input.AISessionRiskEnabled != nil {
		cfg.AIChat.SessionRiskEnabled = *input.AISessionRiskEnabled
	}
	if input.AISessionRiskTTLMinutes != nil {
		cfg.AIChat.SessionRiskTTLMinutes = *input.AISessionRiskTTLMinutes
	}
	if input.AISessionRiskHalfLifeMinutes != nil {
		cfg.AIChat.SessionRiskHalfLifeMinutes = *input.AISessionRiskHalfLifeMinutes
	}
	if input.AISessionRiskBlockCooldownMinutes != nil {
		cfg.AIChat.SessionRiskBlockCooldownMinutes = *input.AISessionRiskBlockCooldownMinutes
	}
	if input.AIActorRiskEnabled != nil {
		cfg.AIChat.ActorRiskEnabled = *input.AIActorRiskEnabled
	}
	if input.AIIncrementalAuditEnabled != nil {
		cfg.AIChat.IncrementalAuditEnabled = *input.AIIncrementalAuditEnabled
	}
	if input.AIInputProvenanceV2Enabled != nil {
		cfg.AIChat.InputProvenanceV2Enabled = *input.AIInputProvenanceV2Enabled
	}
	if input.AIDeterministicRiskV2Enabled != nil {
		cfg.AIChat.DeterministicRiskV2Enabled = *input.AIDeterministicRiskV2Enabled
	}
	if input.AIRecentUserTurns != nil {
		cfg.AIChat.RecentUserTurns = *input.AIRecentUserTurns
	}
	if input.AISummaryMaxChars != nil {
		cfg.AIChat.SummaryMaxChars = *input.AISummaryMaxChars
	}
	if input.AIFullReviewThreshold != nil {
		cfg.AIChat.FullReviewThreshold = *input.AIFullReviewThreshold
	}
	if input.AIFullReviewRiskDelta != nil {
		cfg.AIChat.FullReviewRiskDelta = *input.AIFullReviewRiskDelta
	}
	if input.AIPeriodicFullReviewTurns != nil {
		cfg.AIChat.PeriodicFullReviewTurns = *input.AIPeriodicFullReviewTurns
	}
	if input.AIFullReviewMaxInputChars != nil {
		cfg.AIChat.FullReviewMaxInputChars = *input.AIFullReviewMaxInputChars
	}
	if input.AIFastMaxOutputTokens != nil {
		cfg.AIChat.FastMaxOutputTokens = *input.AIFastMaxOutputTokens
	}
	if input.AIFullMaxOutputTokens != nil {
		cfg.AIChat.FullMaxOutputTokens = *input.AIFullMaxOutputTokens
	}
	if input.AIMaxReviewMaxOutputTokens != nil {
		cfg.AIChat.MaxReviewMaxOutputTokens = *input.AIMaxReviewMaxOutputTokens
	}
	if input.AIAuditContextTTLMinutes != nil {
		cfg.AIChat.AuditContextTTLMinutes = *input.AIAuditContextTTLMinutes
	}
	if input.AIPricingConfigured != nil && !*input.AIPricingConfigured {
		cfg.AIChat.PricingVersion = ""
		cfg.AIChat.UncachedInputUSDPerMillionTokens = nil
		cfg.AIChat.CachedInputUSDPerMillionTokens = nil
		cfg.AIChat.OutputUSDPerMillionTokens = nil
	} else {
		if input.AIPricingVersion != nil {
			cfg.AIChat.PricingVersion = strings.TrimSpace(*input.AIPricingVersion)
		}
		if input.AIUncachedInputUSDPerMillionTokens != nil {
			cfg.AIChat.UncachedInputUSDPerMillionTokens = cloneFloat64Ptr(input.AIUncachedInputUSDPerMillionTokens)
		}
		if input.AICachedInputUSDPerMillionTokens != nil {
			cfg.AIChat.CachedInputUSDPerMillionTokens = cloneFloat64Ptr(input.AICachedInputUSDPerMillionTokens)
		}
		if input.AIOutputUSDPerMillionTokens != nil {
			cfg.AIChat.OutputUSDPerMillionTokens = cloneFloat64Ptr(input.AIOutputUSDPerMillionTokens)
		}
	}
	if input.HitRetentionDays != nil {
		cfg.HitRetentionDays = *input.HitRetentionDays
	}
	if input.NonHitRetentionDays != nil {
		cfg.NonHitRetentionDays = *input.NonHitRetentionDays
	}
	if input.PreHashCheckEnabled != nil {
		cfg.PreHashCheckEnabled = *input.PreHashCheckEnabled
	}
	if input.BlockedKeywords != nil {
		cfg.BlockedKeywords = normalizeBlockedKeywords(*input.BlockedKeywords)
	}
	if input.KeywordBlockingMode != nil {
		cfg.KeywordBlockingMode = strings.TrimSpace(*input.KeywordBlockingMode)
	}
	if input.ModelFilter != nil {
		cfg.ModelFilter = *input.ModelFilter
	}
	if input.UserFilter != nil {
		cfg.UserFilter = *input.UserFilter
	}
	if input.AccountFilter != nil {
		cfg.AccountFilter = *input.AccountFilter
	}
	if input.AllGroups != nil {
		cfg.AllGroups = *input.AllGroups
	}
	if input.GroupIDs != nil {
		cfg.GroupIDs = normalizeInt64IDs(*input.GroupIDs)
	}
	if input.RecordNonHits != nil {
		cfg.RecordNonHits = *input.RecordNonHits
	}
	if input.CyberPolicyExcludeFromBanCount != nil {
		cfg.CyberPolicyExcludeFromBanCount = *input.CyberPolicyExcludeFromBanCount
	}
	if input.Thresholds != nil {
		cfg.Thresholds = mergeContentModerationThresholds(ContentModerationDefaultThresholds(), *input.Thresholds)
	}
	if input.ClearAPIKey {
		if cfg.AuditProvider == ContentModerationProviderAIChat {
			cfg.AIChat.APIKeys = []string{}
		} else {
			cfg.APIKey = ""
			cfg.APIKeys = []string{}
		}
	} else {
		apiKeysMode := normalizeContentModerationAPIKeysMode(input.APIKeysMode)
		if input.DeleteAPIKeyHashes != nil && apiKeysMode != contentModerationAPIKeysModeReplace {
			updatedKeys := deleteModerationAPIKeysByHash(cfg.apiKeys(), *input.DeleteAPIKeyHashes)
			if cfg.AuditProvider == ContentModerationProviderAIChat {
				cfg.AIChat.APIKeys = updatedKeys
			} else {
				cfg.APIKeys = updatedKeys
				cfg.APIKey = ""
			}
		}
		if input.APIKeys != nil {
			var updatedKeys []string
			if apiKeysMode == contentModerationAPIKeysModeReplace {
				updatedKeys = normalizeModerationAPIKeys(*input.APIKeys)
			} else {
				updatedKeys = normalizeModerationAPIKeys(append(cfg.apiKeys(), *input.APIKeys...))
			}
			if cfg.AuditProvider == ContentModerationProviderAIChat {
				cfg.AIChat.APIKeys = updatedKeys
			} else {
				cfg.APIKeys = updatedKeys
				cfg.APIKey = ""
			}
		}
		if input.APIKey != nil && strings.TrimSpace(*input.APIKey) != "" {
			updatedKeys := normalizeModerationAPIKeys(append(cfg.apiKeys(), *input.APIKey))
			if cfg.AuditProvider == ContentModerationProviderAIChat {
				cfg.AIChat.APIKeys = updatedKeys
			} else {
				cfg.APIKeys = updatedKeys
				cfg.APIKey = ""
			}
		}
	}
	if err := s.validateConfig(ctx, cfg); err != nil {
		return nil, err
	}
	cfg.AccountFilter, err = s.normalizeAccountScopeFilter(ctx, cfg.AccountFilter)
	if err != nil {
		return nil, err
	}
	cfg.normalize()
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal content moderation config: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyContentModerationConfig, string(raw)); err != nil {
		return nil, fmt.Errorf("save content moderation config: %w", err)
	}
	s.replaceRuntimeConfig(cfg)
	// 代理选择可能已变化，丢弃已解析的代理 URL 缓存，下次调用即时生效。
	s.moderationProxyCache.Store(nil)
	return s.configView(cfg), nil
}

func (s *ContentModerationService) TestAPIKeys(ctx context.Context, input TestContentModerationAPIKeysInput) (*TestContentModerationAPIKeysResult, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.AuditProvider) != "" {
		cfg.AuditProvider = normalizeContentModerationProvider(input.AuditProvider)
	}
	keys := normalizeModerationAPIKeys(input.APIKeys)
	configured := false
	if len(keys) == 0 {
		keys = cfg.apiKeys()
		configured = true
	}
	if strings.TrimSpace(input.BaseURL) != "" {
		if cfg.AuditProvider == ContentModerationProviderAIChat {
			cfg.AIChat.BaseURL = input.BaseURL
		} else {
			cfg.BaseURL = input.BaseURL
		}
	}
	if strings.TrimSpace(input.Model) != "" {
		if cfg.AuditProvider == ContentModerationProviderAIChat {
			cfg.AIChat.Model = input.Model
		} else {
			cfg.Model = input.Model
		}
	}
	if input.TimeoutMS > 0 {
		if cfg.AuditProvider == ContentModerationProviderAIChat {
			cfg.AIChat.TimeoutMS = input.TimeoutMS
		} else {
			cfg.TimeoutMS = input.TimeoutMS
		}
	}
	if input.ProxyID != nil {
		if *input.ProxyID > 0 {
			id := *input.ProxyID
			if cfg.AuditProvider == ContentModerationProviderAIChat {
				cfg.AIChat.ProxyID = &id
			} else {
				cfg.ProxyID = &id
			}
		} else {
			if cfg.AuditProvider == ContentModerationProviderAIChat {
				cfg.AIChat.ProxyID = nil
			} else {
				cfg.ProxyID = nil
			}
		}
	}
	if input.AIConfidenceThreshold > 0 {
		cfg.AIChat.ConfidenceThreshold = input.AIConfidenceThreshold
	}
	if strings.TrimSpace(input.AISystemPrompt) != "" {
		cfg.AIChat.SystemPrompt = input.AISystemPrompt
	}
	if input.AIMaxInputChars > 0 {
		cfg.AIChat.MaxInputChars = input.AIMaxInputChars
	}
	if input.AISynchronousBudgetMS > 0 {
		cfg.AIChat.SynchronousBudgetMS = input.AISynchronousBudgetMS
	}
	if input.AIFastInputChars > 0 {
		cfg.AIChat.FastInputChars = input.AIFastInputChars
	}
	if input.AIFallbackInputChars > 0 {
		cfg.AIChat.FallbackInputChars = input.AIFallbackInputChars
	}
	if strings.TrimSpace(input.AIThinkingMode) != "" {
		cfg.AIChat.ThinkingMode = strings.TrimSpace(input.AIThinkingMode)
	}
	if strings.TrimSpace(input.AIReasoningEffort) != "" {
		cfg.AIChat.ReasoningEffort = strings.TrimSpace(input.AIReasoningEffort)
	}
	if input.AIRiskLevelsEnabled != nil {
		cfg.AIChat.RiskLevelsEnabled = *input.AIRiskLevelsEnabled
	}
	if input.AIObserveThreshold > 0 {
		cfg.AIChat.ObserveThreshold = input.AIObserveThreshold
	}
	cfg.normalize()
	testInput, imageCount, err := buildModerationTestInput(input.Prompt, input.Images)
	if err != nil {
		return nil, err
	}
	auditOnly := contentModerationTestHasAuditInput(input.Prompt, input.Images)
	if configured && auditOnly {
		key, ok := s.nextUsableAPIKey(cfg)
		if !ok {
			return &TestContentModerationAPIKeysResult{
				Items:      s.apiKeyStatuses(keys),
				AuditError: &ContentModerationTestAuditError{Code: "no_usable_api_key", Message: "no usable moderation API key is available"},
				ImageCount: imageCount,
			}, nil
		}
		keys = []string{key}
	}
	if len(keys) == 0 {
		result := &TestContentModerationAPIKeysResult{Items: []ContentModerationAPIKeyStatus{}, ImageCount: imageCount}
		if auditOnly {
			result.AuditError = &ContentModerationTestAuditError{Code: "no_api_key", Message: "no moderation API key is configured"}
		}
		return result, nil
	}
	items := make([]ContentModerationAPIKeyStatus, 0, len(keys))
	var auditResult *ContentModerationTestAuditResult
	var auditError *ContentModerationTestAuditError
	for idx, key := range keys {
		start := time.Now()
		httpStatus := 0
		requestCtx := ctx
		cancelRequest := func() {}
		if cfg.AuditProvider == ContentModerationProviderAIChat {
			requestCtx, cancelRequest = context.WithTimeout(ctx, time.Duration(cfg.AIChat.SynchronousBudgetMS)*time.Millisecond)
		}
		result, err := s.callModerationOnceWithInput(requestCtx, cfg, key, testInput, &httpStatus)
		cancelRequest()
		latency := int(time.Since(start).Milliseconds())
		keyHash := moderationAPIKeyHash(key)
		if err != nil {
			s.markAPIKeyError(key, err.Error(), latency, httpStatus)
			if auditOnly && auditError == nil {
				auditError = &ContentModerationTestAuditError{
					Code:       "audit_request_failed",
					Message:    err.Error(),
					HTTPStatus: httpStatus,
				}
			}
		} else {
			s.markAPIKeySuccess(key, latency, httpStatus)
			if auditResult == nil {
				var riskThresholds []float64
				if cfg.AuditProvider == ContentModerationProviderAIChat && cfg.AIChat.RiskLevelsEnabled {
					riskThresholds = []float64{
						cfg.AIChat.ObserveThreshold,
						cfg.AIChat.ConfidenceThreshold,
					}
				}
				auditResult = buildContentModerationTestAuditResult(
					result,
					cfg.activeThresholds(),
					riskThresholds...,
				)
			}
		}
		status := s.apiKeyStatusForHash(idx, keyHash, maskSecretTail(key), configured)
		status.LastTested = true
		items = append(items, status)
	}
	if auditResult != nil {
		auditError = nil
	} else if auditOnly && auditError == nil {
		auditError = &ContentModerationTestAuditError{Code: "empty_audit_result", Message: "moderation provider returned no audit result"}
	}
	return &TestContentModerationAPIKeysResult{Items: items, AuditResult: auditResult, AuditError: auditError, ImageCount: imageCount}, nil
}

func (s *ContentModerationService) Check(ctx context.Context, input ContentModerationCheckInput) (*ContentModerationDecision, error) {
	allow := &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow}
	if s == nil || s.settingRepo == nil || s.repo == nil {
		slog.Info("content_moderation.skip_unavailable",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol)
		return allow, nil
	}
	runtimeSnapshot, err := s.loadRuntimeSnapshot(ctx)
	if err != nil {
		slog.Warn("content_moderation.skip_config_load_failed",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"error", err)
		return allow, nil
	}
	if !runtimeSnapshot.riskControlEnabled {
		slog.Info("content_moderation.skip_feature_disabled",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol)
		return allow, nil
	}
	cfg := runtimeSnapshot.config
	inGroupScope := cfg.includesGroup(input.GroupID)
	inModelScope := cfg.includesModel(input.Model)
	slog.Info("content_moderation.config_loaded",
		"user_id", input.UserID,
		"api_key_id", input.APIKeyID,
		"group_id", contentModerationLogGroupID(input.GroupID),
		"group_name", input.GroupName,
		"endpoint", input.Endpoint,
		"provider", input.Provider,
		"protocol", input.Protocol,
		"model", input.Model,
		"enabled", cfg.Enabled,
		"mode", cfg.Mode,
		"audit_provider", cfg.AuditProvider,
		"ai_reasoning_effort", cfg.AIChat.ReasoningEffort,
		"all_groups", cfg.AllGroups,
		"configured_group_ids", cfg.GroupIDs,
		"in_group_scope", inGroupScope,
		"model_filter_type", cfg.ModelFilter.Type,
		"configured_models", cfg.ModelFilter.Models,
		"in_model_scope", inModelScope,
		"sample_rate", cfg.SampleRate,
		"api_key_count", len(cfg.apiKeys()),
		"pre_hash_check_enabled", cfg.PreHashCheckEnabled,
		"record_non_hits", cfg.RecordNonHits)
	if !cfg.Enabled {
		slog.Info("content_moderation.skip_config_disabled",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol)
		return allow, nil
	}
	if cfg.Mode == ContentModerationModeOff {
		slog.Info("content_moderation.skip_mode_off",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol)
		return allow, nil
	}
	if !inGroupScope {
		slog.Info("content_moderation.skip_group_out_of_scope",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"group_name", input.GroupName,
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"all_groups", cfg.AllGroups,
			"configured_group_ids", cfg.GroupIDs)
		return allow, nil
	}
	if !inModelScope {
		slog.Info("content_moderation.skip_model_out_of_scope",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"group_name", input.GroupName,
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"model", input.Model,
			"model_filter_type", cfg.ModelFilter.Type,
			"configured_models", cfg.ModelFilter.Models)
		return allow, nil
	}
	s.captureContentModerationEpoch(ctx, &input)
	extraction := ExtractContentModerationInputOutcome(input.Protocol, input.Body)
	if extraction.Err != nil {
		action := ContentModerationActionError
		if extraction.Status == ContentModerationExtractionStatusEmptyContent {
			action = ContentModerationActionSkip
		}
		errText := fmt.Sprintf("input_extraction_%s: %s", extraction.Status, extraction.Err)
		slog.Warn("content_moderation.input_extraction_failed",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"body_bytes", len(input.Body),
			"extraction_status", extraction.Status,
			"error", extraction.Err)
		log := s.buildLog(input, cfg, action, false, "", 0, nil, "", nil, nil, errText)
		if err := s.repo.CreateLog(ctx, log); err != nil {
			slog.Warn("content_moderation.extraction_log_failed", "error", err)
		}
		failClosed := extraction.Status != ContentModerationExtractionStatusEmptyContent &&
			cfg.Mode == ContentModerationModePreBlock &&
			cfg.AuditProvider == ContentModerationProviderAIChat &&
			cfg.AIChat.FailurePolicy == ContentModerationFailurePolicyBlock
		if cfg.Mode == ContentModerationModePreBlock {
			s.recordPreBlockSyncMetric(0, action)
		}
		if failClosed {
			return contentModerationUnavailableDecision(), nil
		}
		return allow, nil
	}
	content := extraction.Input
	content.Normalize()
	normalizeContentModerationProvenance(input, runtimeSnapshot, &content)
	slog.Info("content_moderation.input_extracted",
		"user_id", input.UserID,
		"api_key_id", input.APIKeyID,
		"group_id", contentModerationLogGroupID(input.GroupID),
		"endpoint", input.Endpoint,
		"protocol", input.Protocol,
		"text_runes", len([]rune(content.Text)),
		"audit_target_kind", content.AuditTargetKind,
		"audit_target_runes", len([]rune(content.AuditTargetText)),
		"has_explicit_user", content.HasExplicitUser,
		"trusted_client", content.TrustedClient,
		"trusted_signals", content.TrustedSignals,
		"ignored_metadata", content.IgnoredMetadata,
		"image_count", len(content.Images),
		"extraction_truncated", extraction.Truncated)
	hashText := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))
	if cfg.AuditProvider == ContentModerationProviderAIChat {
		cfg = cloneContentModerationConfig(cfg)
		cfg.AIChat.inputHash = hashText
	}
	verdictKey := contentModerationRequestVerdictCacheKey(input, cfg, content, hashText, contentModerationRequestVerdictExecutionMode(cfg, nil, true))
	if !s.contentModerationCachedVerdictAllowed(ctx, hashText) {
		verdictKey = ""
	}
	if verdictKey != "" {
		if cached, hit, cacheErr := s.getContentModerationRequestVerdict(ctx, verdictKey); cacheErr != nil {
			slog.Warn("content_moderation.request_verdict_cache_get_failed",
				"user_id", input.UserID,
				"api_key_id", input.APIKeyID,
				"request_id", input.RequestID,
				"error", cacheErr)
		} else if hit {
			slog.Info("content_moderation.request_verdict_cache_hit",
				"user_id", input.UserID,
				"api_key_id", input.APIKeyID,
				"request_id", input.RequestID)
			return cached, nil
		}
	}
	if cfg.AuditProvider == ContentModerationProviderAIChat && !contentModerationHasAuditTarget(content) {
		state, found, riskErr := s.getSessionRisk(ctx, input, cfg)
		if riskErr != nil {
			slog.Warn("content_moderation.session_risk_get_failed", "user_id", input.UserID, "error", riskErr)
			if cfg.Mode == ContentModerationModePreBlock && cfg.AIChat.FailurePolicy == ContentModerationFailurePolicyBlock {
				s.recordPreBlockSyncMetric(0, ContentModerationActionError)
				return contentModerationUnavailableDecision(), nil
			}
		}
		blockedByCooldown := cfg.Mode == ContentModerationModePreBlock && found && voteairiskstate.IsBlocked(state, time.Now())
		localDecision := allow
		var scores map[string]float64
		if blockedByCooldown {
			scores = map[string]float64{contentModerationSessionRiskCategory: state.Score}
			localDecision = &ContentModerationDecision{
				Allowed:             false,
				Blocked:             true,
				Flagged:             true,
				Message:             cfg.BlockMessage,
				StatusCode:          cfg.BlockStatus,
				HighestCategory:     contentModerationSessionRiskCategory,
				HighestScore:        state.Score,
				CategoryScores:      scores,
				Action:              ContentModerationActionBlock,
				RiskTier:            voteairiskstate.TierHigh,
				CumulativeRiskScore: state.Score,
			}
		}
		decision := s.localContentModerationVerdictIdempotent(ctx, input, cfg, verdictKey, localDecision, func(workCtx context.Context) *ContentModerationDecision {
			slog.Info("content_moderation.skip_no_new_user_intent",
				"user_id", input.UserID,
				"api_key_id", input.APIKeyID,
				"endpoint", input.Endpoint,
				"protocol", input.Protocol,
				"existing_cooldown", blockedByCooldown)
			if !blockedByCooldown {
				if cfg.RecordNonHits {
					log := s.buildLog(input, cfg, ContentModerationActionSkip, false, "", 0, nil, content.ExcerptText(), nil, nil, "")
					log.AuditCode = contentModerationAuditCodeNoNewUserIntent
					populateContentModerationAuditDetails(log, cfg, content, nil, nil, false)
					s.enqueueRecord(input, cfg, log, hashText, false, false)
				}
				if cfg.Mode == ContentModerationModePreBlock {
					s.recordPreBlockSyncMetric(0, ContentModerationActionAllow)
				}
				return localDecision
			}
			log := s.buildLog(input, cfg, ContentModerationActionBlock, true, contentModerationSessionRiskCategory, state.Score, scores, content.ExcerptText(), nil, nil, "")
			s.enqueueRecord(input, cfg, log, hashText, false, false)
			s.recordPreBlockSyncMetric(0, ContentModerationActionBlock)
			return localDecision
		})
		return decision, nil
	}
	if cfg.Mode == ContentModerationModePreBlock {
		if cfg.KeywordBlockingMode != ContentModerationKeywordModeAPIOnly && len(cfg.BlockedKeywords) > 0 {
			if keyword, hit := runtimeSnapshot.matchBlockedKeyword(content.AuditTargetText); hit {
				scores := map[string]float64{contentModerationKeywordCategory: 1.0}
				localDecision := &ContentModerationDecision{
					Allowed:         false,
					Blocked:         true,
					Flagged:         true,
					Message:         cfg.BlockMessage,
					StatusCode:      cfg.BlockStatus,
					HighestCategory: contentModerationKeywordCategory,
					HighestScore:    1.0,
					CategoryScores:  scores,
					Action:          ContentModerationActionKeywordBlock,
				}
				decision := s.localContentModerationVerdictIdempotent(ctx, input, cfg, verdictKey, localDecision, func(context.Context) *ContentModerationDecision {
					s.recordPreBlockSyncMetric(0, ContentModerationActionKeywordBlock)
					slog.Info("content_moderation.keyword_block",
						"user_id", input.UserID,
						"api_key_id", input.APIKeyID,
						"group_id", contentModerationLogGroupID(input.GroupID),
						"endpoint", input.Endpoint,
						"protocol", input.Protocol,
						"keyword_blocking_mode", cfg.KeywordBlockingMode,
						"keyword", keyword)
					log := s.buildLog(input, cfg, ContentModerationActionKeywordBlock, true, contentModerationKeywordCategory, 1.0, scores, content.ExcerptText(), nil, nil, "")
					log.MatchedKeyword = keyword
					s.enqueueRecord(input, cfg, log, hashText, false, true)
					return localDecision
				})
				return decision, nil
			}
		}
		if cfg.KeywordBlockingMode == ContentModerationKeywordModeKeywordOnly {
			decision := s.localContentModerationVerdictIdempotent(ctx, input, cfg, verdictKey, allow, func(context.Context) *ContentModerationDecision {
				s.recordPreBlockSyncMetric(0, ContentModerationActionAllow)
				slog.Info("content_moderation.skip_api_keyword_only",
					"user_id", input.UserID,
					"api_key_id", input.APIKeyID,
					"group_id", contentModerationLogGroupID(input.GroupID),
					"endpoint", input.Endpoint,
					"protocol", input.Protocol)
				return allow
			})
			return decision, nil
		}
	}
	if cfg.PreHashCheckEnabled && s.hashCache != nil {
		matched, err := s.hashCache.HasFlaggedInputHash(ctx, hashText)
		if err != nil {
			slog.Warn("content_moderation.hash_check_failed", "user_id", input.UserID, "endpoint", input.Endpoint, "error", err)
		}
		if matched {
			message := cfg.BlockMessage
			if message != "" {
				message = fmt.Sprintf("%s（hash: %s）", message, hashText)
			}
			scores := map[string]float64{"hash": 1.0}
			localDecision := &ContentModerationDecision{
				Allowed:    false,
				Blocked:    true,
				Flagged:    true,
				Message:    message,
				StatusCode: cfg.BlockStatus,
				InputHash:  hashText,
				Action:     ContentModerationActionHashBlock,
			}
			decision := s.localContentModerationVerdictIdempotent(ctx, input, cfg, verdictKey, localDecision, func(context.Context) *ContentModerationDecision {
				if cfg.Mode == ContentModerationModePreBlock {
					s.recordPreBlockSyncMetric(0, ContentModerationActionHashBlock)
				}
				slog.Info("content_moderation.hash_block",
					"user_id", input.UserID,
					"api_key_id", input.APIKeyID,
					"group_id", contentModerationLogGroupID(input.GroupID),
					"endpoint", input.Endpoint,
					"protocol", input.Protocol,
					"input_hash", hashText)
				log := s.buildLog(input, cfg, ContentModerationActionHashBlock, true, "hash", 1.0, scores, content.ExcerptText(), nil, nil, "")
				s.enqueueRecord(input, cfg, log, hashText, false, false)
				return localDecision
			})
			return decision, nil
		}
	}
	if !cfg.shouldSample(hashText) {
		decision := s.localContentModerationVerdictIdempotent(ctx, input, cfg, verdictKey, allow, func(context.Context) *ContentModerationDecision {
			if cfg.Mode == ContentModerationModePreBlock {
				s.recordPreBlockSyncMetric(0, ContentModerationActionAllow)
			}
			slog.Info("content_moderation.skip_sample_rate",
				"user_id", input.UserID,
				"api_key_id", input.APIKeyID,
				"group_id", contentModerationLogGroupID(input.GroupID),
				"endpoint", input.Endpoint,
				"protocol", input.Protocol,
				"sample_rate", cfg.SampleRate)
			return allow
		})
		return decision, nil
	}
	if len(cfg.apiKeys()) == 0 {
		fallback := allow
		if cfg.Mode == ContentModerationModePreBlock && cfg.AuditProvider == ContentModerationProviderAIChat && cfg.AIChat.FailurePolicy == ContentModerationFailurePolicyBlock {
			fallback = contentModerationUnavailableDecision()
		}
		decision := s.localContentModerationVerdictIdempotent(ctx, input, cfg, verdictKey, fallback, func(workCtx context.Context) *ContentModerationDecision {
			if cfg.Mode == ContentModerationModePreBlock {
				s.recordPreBlockSyncMetric(0, ContentModerationActionError)
			}
			slog.Warn("content_moderation.skip_no_audit_api_keys",
				"user_id", input.UserID,
				"api_key_id", input.APIKeyID,
				"group_id", contentModerationLogGroupID(input.GroupID),
				"endpoint", input.Endpoint,
				"protocol", input.Protocol)
			errMessage := "audit_no_usable_key: no usable audit API keys configured"
			log := s.buildLog(input, cfg, ContentModerationActionError, false, "", 0, nil, content.ExcerptText(), nil, nil, errMessage)
			_ = s.repo.CreateLog(workCtx, log)
			return fallback
		})
		return decision, nil
	}
	if cfg.Mode == ContentModerationModeObserve {
		slog.Info("content_moderation.enqueue_observe",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"queue_len", len(s.asyncQueue))
		decision := s.enqueueContentModerationObserveIdempotent(ctx, input, cfg, content, hashText, verdictKey)
		if decision != nil {
			return decision, nil
		}
		return allow, nil
	}

	return s.checkSyncIdempotent(ctx, input, cfg, content, hashText, nil, true), nil
}

func (s *ContentModerationService) checkSync(ctx context.Context, input ContentModerationCheckInput, cfg *ContentModerationConfig, content ContentModerationInput, hashText string, queueDelay *int, allowBlock bool) *ContentModerationDecision {
	allow := &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow}
	if cfg != nil && cfg.AuditProvider == ContentModerationProviderAIChat {
		inputHash := strings.TrimSpace(hashText)
		if inputHash != "" && cfg.AIChat.inputHash != inputHash {
			cfg = cloneContentModerationConfig(cfg)
			cfg.AIChat.inputHash = inputHash
		}
	}
	trackPreBlock := queueDelay == nil && allowBlock && cfg != nil && cfg.Mode == ContentModerationModePreBlock
	if trackPreBlock {
		s.preBlockActive.Add(1)
		defer s.preBlockActive.Add(-1)
	}
	existingRiskScore := 0.0
	if cfg.AuditProvider == ContentModerationProviderAIChat {
		state, found, riskErr := s.getSessionRisk(ctx, input, cfg)
		if riskErr != nil {
			slog.Warn("content_moderation.session_risk_get_failed", "user_id", input.UserID, "error", riskErr)
			if trackPreBlock && cfg.AIChat.FailurePolicy == ContentModerationFailurePolicyBlock {
				s.recordPreBlockSyncMetric(0, ContentModerationActionError)
				return contentModerationUnavailableDecision()
			}
		}
		if found {
			existingRiskScore = state.Score
		}
		if allowBlock && found && voteairiskstate.IsBlocked(state, time.Now()) {
			scores := map[string]float64{contentModerationSessionRiskCategory: state.Score}
			log := s.buildLog(input, cfg, ContentModerationActionBlock, true, contentModerationSessionRiskCategory, state.Score, scores, content.ExcerptText(), nil, queueDelay, "")
			if queueDelay == nil {
				s.enqueueRecord(input, cfg, log, hashText, false, false)
			} else {
				s.persistContentModerationLog(ctx, cfg, log, hashText, false, false)
			}
			if trackPreBlock {
				s.recordPreBlockSyncMetric(0, ContentModerationActionBlock)
			}
			return &ContentModerationDecision{
				Allowed:                 false,
				Blocked:                 true,
				Flagged:                 true,
				Message:                 cfg.BlockMessage,
				StatusCode:              cfg.BlockStatus,
				HighestCategory:         contentModerationSessionRiskCategory,
				HighestScore:            state.Score,
				CategoryScores:          scores,
				Action:                  ContentModerationActionBlock,
				RiskTier:                voteairiskstate.TierHigh,
				CumulativeRiskScore:     state.Score,
				requestVerdictCacheable: true,
			}
		}
	}
	localRisk := voteaideterministicrisk.None()
	if cfg.AuditProvider == ContentModerationProviderAIChat && cfg.AIChat.DeterministicRiskV2Enabled {
		s.recordContentModerationSessionSource(normalizeContentModerationSessionSource(input.SessionSource, input.SessionID))
		localRisk = evaluateContentModerationDeterministicRisk(content)
	} else if cfg.AuditProvider == ContentModerationProviderAIChat {
		s.recordContentModerationSessionSource(normalizeContentModerationSessionSource(input.SessionSource, input.SessionID))
	}
	if localRisk.Level == voteaideterministicrisk.LevelCandidate {
		cfg = cloneContentModerationConfig(cfg)
		cfg.AIChat.forceFullReview = true
		cfg.AIChat.auditStage = string(voteaimoderation.StageFull)
	}

	start := time.Now()
	moderationInput := content.ModerationInput()
	if cfg.AuditProvider == ContentModerationProviderAIChat {
		moderationInput = content.AIChatModerationInput()
		if existingRiskScore > 0 {
			cfg = cloneContentModerationConfig(cfg)
			cfg.AIChat.existingRiskScore = existingRiskScore
		}
		if targetChars := len([]rune(content.AuditTargetText)); targetChars > cfg.AIChat.MaxInputChars {
			cfg = cloneContentModerationConfig(cfg)
			cfg.AIChat.MaxInputChars = max(targetChars, len([]rune(aiChatTextFromModerationInput(moderationInput))))
		}
	}
	auditCtx := ctx
	cancelAudit := func() {}
	if trackPreBlock && cfg.AuditProvider == ContentModerationProviderAIChat {
		budget := time.Duration(cfg.AIChat.SynchronousBudgetMS) * time.Millisecond
		auditCtx, cancelAudit = context.WithTimeout(ctx, budget)
	}
	defer cancelAudit()
	result := moderationAPIResultFromDeterministicRisk(localRisk)
	var incrementalPlan *contentModerationIncrementalPlan
	var err error
	if result != nil {
		// Confirmed V2 matches are resolved locally with structured diagnostics.
		if cfg.AuditProvider == ContentModerationProviderAIChat && cfg.AIChat.IncrementalAuditEnabled {
			incrementalPlan = s.updateLocalDeterministicAuditContext(auditCtx, input, cfg, content, result)
		}
	} else if cfg.AuditProvider == ContentModerationProviderAIChat && cfg.AIChat.IncrementalAuditEnabled {
		result, incrementalPlan, err = s.callIncrementalAIChatAudit(auditCtx, input, cfg, content, trackPreBlock)
	} else {
		legacyStarted := time.Now()
		result, err = s.callModeration(auditCtx, cfg, moderationInput, trackPreBlock)
		if cfg.AuditProvider == ContentModerationProviderAIChat {
			legacyLatency := int(time.Since(legacyStarted).Milliseconds())
			if err != nil {
				s.recordContentModerationAuditFailure(contentModerationConfiguredAuditStage(cfg), legacyLatency, voteaimoderation.AttemptedInputChars(err))
			} else if result != nil {
				contentModerationSetStageDetails(
					result,
					contentModerationSuccessfulStageDetails(contentModerationConfiguredAuditStage(cfg), result, legacyLatency),
				)
				s.recordContentModerationStageLatency(result.Stage, legacyLatency, result)
				s.recordContentModerationAuditUsage(result, len([]rune(aiChatTextFromModerationInput(moderationInput))), cfg)
			}
		}
	}
	annotateModerationResultWithDeterministicRisk(result, localRisk)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		if trackPreBlock {
			s.recordPreBlockSyncMetric(latency, ContentModerationActionError)
		}
		slog.Warn("content_moderation.audit_api_failed",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"mode", cfg.Mode,
			"allow_block", allowBlock,
			"queue_delay_ms", queueDelay,
			"latency_ms", latency,
			"error", err)
		if queueDelay != nil {
			s.asyncErrors.Add(1)
		}
		failClosed := allowBlock && cfg.Mode == ContentModerationModePreBlock && cfg.AuditProvider == ContentModerationProviderAIChat && cfg.AIChat.FailurePolicy == ContentModerationFailurePolicyBlock
		log := s.buildLog(input, cfg, ContentModerationActionError, false, "", 0, nil, content.ExcerptText(), &latency, queueDelay, contentModerationAuditErrorText(err))
		populateContentModerationAuditDetails(log, cfg, content, result, incrementalPlan, false)
		_ = s.repo.CreateLog(ctx, log)
		if failClosed {
			return contentModerationUnavailableDecision()
		}
		if trackPreBlock && !cfg.AIChat.supplementalReview && cfg.AuditProvider == ContentModerationProviderAIChat &&
			(errors.Is(err, voteaimoderation.ErrAuditTimeout) || errors.Is(err, voteaimoderation.ErrTemporary) || errors.Is(err, context.DeadlineExceeded)) {
			// Preserve the synchronous latency budget, then complete a full review in
			// the worker pool so session/hash risk still benefits future requests.
			s.enqueueAsync(input, cfg, content, hashText, true)
		}
		return allow
	}

	if result.ReviewIncomplete {
		reviewError := "audit_review_incomplete"
		if strings.TrimSpace(result.ReviewError) != "" {
			reviewError += ": " + trimRunes(result.ReviewError, 500)
		}
		slog.Warn("content_moderation.audit_review_incomplete",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"latency_ms", latency,
			"error", result.ReviewError)
		if allowBlock && cfg.Mode == ContentModerationModePreBlock && cfg.AIChat.FailurePolicy == ContentModerationFailurePolicyBlock {
			if trackPreBlock {
				s.recordPreBlockSyncMetric(latency, ContentModerationActionError)
			}
			log := s.buildLog(input, cfg, ContentModerationActionError, false, "", 0, result.CategoryScores, content.ExcerptText(), &latency, queueDelay, reviewError)
			populateContentModerationAuditDetails(log, cfg, content, result, incrementalPlan, false)
			_ = s.repo.CreateLog(ctx, log)
			return contentModerationUnavailableDecision()
		}
		if cfg.AIChat.supplementalReview {
			// A supplemental task is the final bounded attempt. Persist one
			// explicit failure row and stop; re-enqueueing here would create an
			// unbounded chain while the audit provider remains unhealthy.
			finalReviewError := reviewError + ": supplemental_review_final_failure"
			log := s.buildLog(input, cfg, ContentModerationActionError, false, "", 0, result.CategoryScores, content.ExcerptText(), &latency, queueDelay, finalReviewError)
			populateContentModerationAuditDetails(log, cfg, content, result, incrementalPlan, false)
			s.persistContentModerationLog(ctx, cfg, log, "", false, false)
			_, highestCategory, highestScore := evaluateModerationScores(result.CategoryScores, cfg.activeThresholds())
			return &ContentModerationDecision{
				Allowed:         true,
				Flagged:         false,
				HighestCategory: highestCategory,
				HighestScore:    highestScore,
				CategoryScores:  result.CategoryScores,
				Action:          ContentModerationActionError,
			}
		}
		if !s.enqueueAsync(input, cfg, content, hashText, true) {
			// The provisional result is intentionally side-effect free. If the
			// supplemental queue cannot accept the final review, retain an explicit
			// incomplete audit row so operators can distinguish lost capacity from
			// a successfully completed allow decision.
			droppedReviewError := reviewError + ": supplemental_queue_unavailable"
			log := s.buildLog(input, cfg, ContentModerationActionError, false, "", 0, result.CategoryScores, content.ExcerptText(), &latency, queueDelay, droppedReviewError)
			populateContentModerationAuditDetails(log, cfg, content, result, incrementalPlan, false)
			s.persistContentModerationLog(ctx, cfg, log, "", false, false)
		}
		if trackPreBlock {
			s.recordPreBlockSyncMetric(latency, ContentModerationActionError)
		}
		// The fast result is provisional. Return its diagnostics to the caller, but
		// leave logging, hashes, violation counts, emails, and risk-state mutation
		// to the single supplemental review queued above.
		_, highestCategory, highestScore := evaluateModerationScores(result.CategoryScores, cfg.activeThresholds())
		return &ContentModerationDecision{
			Allowed:             true,
			Flagged:             result.Flagged,
			HighestCategory:     highestCategory,
			HighestScore:        highestScore,
			CategoryScores:      cloneFloatMap(result.CategoryScores),
			Action:              ContentModerationActionAllow,
			CurrentRiskScore:    result.CategoryScores["ai_risk"],
			CumulativeRiskScore: result.CategoryScores["ai_risk"],
		}
	}

	flagged, highestCategory, highestScore := evaluateModerationScores(result.CategoryScores, cfg.activeThresholds())
	riskTier := ""
	currentRiskScore := highestScore
	cumulativeRiskScore := highestScore
	sideEffectsAllowed := !cfg.sideEffectsDisabled
	if !sideEffectsAllowed {
		result.HashPromotionVeto = true
		result.HashPromotionReason = "configuration_generation_changed"
	}
	if cfg.AuditProvider == ContentModerationProviderAIChat && strings.TrimSpace(hashText) != "" {
		allowed, suppressionErr := s.contentModerationHashPromotionAllowed(ctx, hashText)
		if suppressionErr != nil {
			sideEffectsAllowed = false
			result.HashPromotionVeto = true
			result.HashPromotionReason = "promotion_state_unavailable"
			slog.Warn("content_moderation.hash_promotion_check_failed", "user_id", input.UserID, "error", suppressionErr)
		} else if !allowed {
			sideEffectsAllowed = false
			result.HashPromotionVeto = true
			result.HashPromotionReason = "administrator_suppression"
		}
	}
	if cfg.AuditProvider == ContentModerationProviderAIChat {
		if cfg.AIChat.RiskLevelsEnabled {
			tierResult := contentModerationTierResult{
				Tier:            voteairiskstate.TierForScore(result.CategoryScores["ai_risk"], cfg.AIChat.ObserveThreshold, cfg.AIChat.ConfidenceThreshold),
				CurrentScore:    result.CategoryScores["ai_risk"],
				CumulativeScore: result.CategoryScores["ai_risk"],
			}
			if sideEffectsAllowed {
				var riskErr error
				tierResult, riskErr = s.applyAIChatRiskState(ctx, input, cfg, result)
				if riskErr != nil {
					slog.Warn("content_moderation.session_risk_update_failed", "user_id", input.UserID, "error", riskErr)
					if trackPreBlock && cfg.AIChat.FailurePolicy == ContentModerationFailurePolicyBlock {
						s.recordPreBlockSyncMetric(latency, ContentModerationActionError)
						log := s.buildLog(input, cfg, ContentModerationActionError, false, "", 0, result.CategoryScores, content.ExcerptText(), &latency, queueDelay, "session_risk_update_failed: "+trimRunes(riskErr.Error(), 400))
						populateContentModerationAuditDetails(log, cfg, content, result, incrementalPlan, false)
						_ = s.repo.CreateLog(ctx, log)
						return contentModerationUnavailableDecision()
					}
				}
			} else if contentModerationResultHasOnlyWeakSignals(result) {
				tierResult.Tier = voteairiskstate.TierLow
				tierResult.CumulativeScore = 0
			}
			riskTier = tierResult.Tier
			currentRiskScore = tierResult.CurrentScore
			cumulativeRiskScore = tierResult.CumulativeScore
			result.CategoryScores[contentModerationCurrentRiskCategory] = currentRiskScore
			result.CategoryScores[contentModerationSessionRiskCategory] = tierResult.CumulativeScore
			if tierResult.ActorBonus > 0 {
				result.CategoryScores[contentModerationActorRiskCategory] = tierResult.ActorBonus
			}
			if cumulativeRiskScore > highestScore {
				highestCategory = contentModerationSessionRiskCategory
				highestScore = cumulativeRiskScore
			}
			flagged = riskTier == voteairiskstate.TierHigh
		} else {
			flagged = result.Flagged && flagged
		}
	}
	action := ContentModerationActionAllow
	blocked := false
	if allowBlock && flagged && cfg.Mode == ContentModerationModePreBlock {
		action = ContentModerationActionBlock
		blocked = true
	} else if riskTier == voteairiskstate.TierObserve || riskTier == voteairiskstate.TierHigh {
		action = ContentModerationActionObserve
	}
	if trackPreBlock {
		s.recordPreBlockSyncMetric(latency, action)
	}
	slog.Info("content_moderation.audit_result",
		"user_id", input.UserID,
		"api_key_id", input.APIKeyID,
		"group_id", contentModerationLogGroupID(input.GroupID),
		"group_name", input.GroupName,
		"endpoint", input.Endpoint,
		"protocol", input.Protocol,
		"has_session_id", strings.TrimSpace(input.SessionID) != "",
		"mode", cfg.Mode,
		"allow_block", allowBlock,
		"flagged", flagged,
		"blocked", blocked,
		"action", action,
		"risk_tier", riskTier,
		"current_risk_score", currentRiskScore,
		"cumulative_risk_score", cumulativeRiskScore,
		"highest_category", highestCategory,
		"highest_score", highestScore,
		"latency_ms", latency,
		"queue_delay_ms", queueDelay)
	if flagged || action == ContentModerationActionObserve || cfg.AIChat.supplementalReview || cfg.RecordNonHits || !sideEffectsAllowed || result.LocalRuleLevel == string(voteaideterministicrisk.LevelCandidate) {
		recordHash := sideEffectsAllowed && contentModerationShouldPromoteHash(cfg, content, result, flagged)
		log := s.buildLog(input, cfg, action, flagged, highestCategory, highestScore, result.CategoryScores, content.ExcerptText(), &latency, queueDelay, "")
		populateContentModerationAuditDetails(log, cfg, content, result, incrementalPlan, recordHash)
		if !sideEffectsAllowed {
			if result.HashPromotionReason == "administrator_suppression" {
				markContentModerationHashPromotionSuppressed(log)
			} else {
				log.AuditDetails.HashState = "candidate"
				log.AuditDetails.HashPromotionReason = defaultContentModerationString(result.HashPromotionReason, "promotion_state_unavailable")
			}
		}
		if queueDelay == nil && cfg.Mode == ContentModerationModePreBlock {
			s.enqueueRecord(input, cfg, log, hashText, recordHash, sideEffectsAllowed && flagged)
		} else {
			s.persistContentModerationLog(ctx, cfg, log, hashText, recordHash, sideEffectsAllowed && flagged)
		}
	}
	if blocked {
		return &ContentModerationDecision{
			Allowed:                 false,
			Blocked:                 true,
			Flagged:                 true,
			Message:                 cfg.BlockMessage,
			StatusCode:              cfg.BlockStatus,
			HighestCategory:         highestCategory,
			HighestScore:            highestScore,
			CategoryScores:          result.CategoryScores,
			Action:                  action,
			RiskTier:                riskTier,
			CurrentRiskScore:        currentRiskScore,
			CumulativeRiskScore:     cumulativeRiskScore,
			requestVerdictCacheable: true,
		}
	}
	return &ContentModerationDecision{
		Allowed:                 true,
		Flagged:                 flagged,
		Message:                 "",
		HighestCategory:         highestCategory,
		HighestScore:            highestScore,
		CategoryScores:          result.CategoryScores,
		Action:                  action,
		RiskTier:                riskTier,
		CurrentRiskScore:        currentRiskScore,
		CumulativeRiskScore:     cumulativeRiskScore,
		requestVerdictCacheable: true,
	}
}

func contentModerationUnavailableDecision() *ContentModerationDecision {
	return &ContentModerationDecision{
		Allowed:    false,
		Blocked:    true,
		Message:    "Content audit service is temporarily unavailable; please retry later",
		StatusCode: http.StatusServiceUnavailable,
		Action:     ContentModerationActionUnavailable,
	}
}

func contentModerationAuditErrorText(err error) string {
	if err == nil {
		return ""
	}
	code := "audit_request_failed"
	switch {
	case errors.Is(err, voteaimoderation.ErrAuditTimeout), errors.Is(err, context.DeadlineExceeded):
		code = "audit_timeout"
	case errors.Is(err, voteaimoderation.ErrInvalidJSON):
		code = "audit_invalid_json"
	case errors.Is(err, voteaimoderation.ErrEmptyContent):
		code = "audit_empty_response"
	case errors.Is(err, voteaimoderation.ErrTemporary):
		code = "audit_temporary_failure"
	}
	return code + ": " + trimRunes(err.Error(), 500)
}

func contentModerationShouldPromoteHash(cfg *ContentModerationConfig, content ContentModerationInput, result *moderationAPIResult, flagged bool) bool {
	if cfg == nil || result == nil || !flagged || result.ReviewIncomplete || result.HashPromotionVeto {
		return false
	}
	// Referential targets are not semantically self-contained. Even a correct
	// full review of "continue" must not create a cross-session permanent hash.
	if voteaiauditcontext.NeedsPreviousContext(content.AuditTargetText) {
		return false
	}
	if cfg.AuditProvider != ContentModerationProviderAIChat {
		return true
	}
	if !cfg.AIChat.InputProvenanceV2Enabled || !contentModerationHasAuditTarget(content) {
		return false
	}
	if result.LocalDecision && result.HashPromotionEligible {
		return true
	}
	if result.ResultCacheHit || (result.Stage != voteaimoderation.StageFull && result.Stage != voteaimoderation.StageMax) {
		return false
	}
	return result.CategoryScores["ai_risk"] >= cfg.AIChat.ConfidenceThreshold &&
		voteaiauditcontext.HasStrongSignal(result.Signals)
}

func (s *ContentModerationService) recordPreBlockSyncMetric(latencyMS int, action string) {
	if s == nil {
		return
	}
	s.preBlockChecked.Add(1)
	if latencyMS < 0 {
		latencyMS = 0
	}
	s.preBlockLatencyTotalMS.Add(int64(latencyMS))
	switch action {
	case ContentModerationActionBlock, ContentModerationActionHashBlock, ContentModerationActionKeywordBlock:
		s.preBlockBlocked.Add(1)
	case ContentModerationActionError:
		s.preBlockErrors.Add(1)
	default:
		s.preBlockAllowed.Add(1)
	}
}

func (s *ContentModerationService) enqueueAsync(input ContentModerationCheckInput, cfg *ContentModerationConfig, content ContentModerationInput, hashText string, supplemental bool) bool {
	if s == nil || s.asyncQueue == nil {
		return false
	}
	queueSize := defaultContentModerationQueueSize
	if cfg != nil && cfg.QueueSize > 0 {
		queueSize = cfg.QueueSize
	}
	if supplemental {
		queueSize = contentModerationSupplementalQueueLimit(cfg, queueSize)
	}
	if len(s.asyncQueue) >= queueSize {
		slog.Warn("content_moderation.async_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint, "queue_size", queueSize)
		s.asyncDropped.Add(1)
		return false
	}
	reservedSupplemental := false
	originalCacheKeyAlias := ""
	if supplemental {
		if !s.reserveSupplementalSlot(int64(queueSize)) {
			slog.Warn("content_moderation.supplemental_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint, "queue_size", queueSize)
			s.asyncDropped.Add(1)
			return false
		}
		reservedSupplemental = true
		if cfg != nil {
			originalCacheKeyAlias = contentModerationAIResultCacheKey(cfg, content.AIChatModerationInput())
		}
		content = compactSupplementalModerationContent(content, cfg)
	}
	input.Body = nil
	var taskCfg *ContentModerationConfig
	if cfg != nil {
		taskCfg = cloneContentModerationConfig(cfg)
	}
	if supplemental && taskCfg != nil {
		taskCfg.AIChat.cacheKeyAlias = originalCacheKeyAlias
		taskCfg.AIChat.ReasoningEffort = "high"
		taskCfg.AIChat.existingRiskScore = 0
		taskCfg.AIChat.supplementalReview = true
	}
	task := contentModerationTask{
		input:            input,
		content:          content,
		inputHash:        hashText,
		config:           taskCfg,
		configGeneration: s.contentModerationAsyncTaskGeneration(cfg),
		supplemental:     supplemental,
		enqueuedAt:       time.Now(),
	}
	select {
	case s.asyncQueue <- task:
		s.asyncEnqueued.Add(1)
		return true
	default:
		if reservedSupplemental {
			s.supplementalPending.Add(-1)
		}
		slog.Warn("content_moderation.async_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint)
		s.asyncDropped.Add(1)
		return false
	}
}

func (s *ContentModerationService) enqueueRecord(input ContentModerationCheckInput, cfg *ContentModerationConfig, log *ContentModerationLog, inputHash string, recordHash bool, applySideEffects bool) {
	if s == nil || s.asyncQueue == nil || log == nil {
		return
	}
	queueSize := defaultContentModerationQueueSize
	if cfg != nil && cfg.QueueSize > 0 {
		queueSize = cfg.QueueSize
	}
	if len(s.asyncQueue) >= queueSize {
		slog.Warn("content_moderation.record_queue_full",
			"user_id", input.UserID,
			"endpoint", input.Endpoint,
			"action", log.Action,
			"queue_size", queueSize)
		s.asyncDropped.Add(1)
		return
	}
	input.Body = nil
	task := contentModerationTask{
		input:            input,
		inputHash:        inputHash,
		log:              log,
		config:           cloneContentModerationConfig(cfg),
		configGeneration: s.contentModerationAsyncTaskGeneration(cfg),
		recordHash:       recordHash,
		applySideEffects: applySideEffects,
		enqueuedAt:       time.Now(),
	}
	select {
	case s.asyncQueue <- task:
		s.asyncEnqueued.Add(1)
	default:
		slog.Warn("content_moderation.record_queue_full",
			"user_id", input.UserID,
			"endpoint", input.Endpoint,
			"action", log.Action)
		s.asyncDropped.Add(1)
	}
}

func (s *ContentModerationService) worker(id int) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), maxContentModerationTimeoutMS*time.Millisecond+10*time.Second)
		runtimeSnapshot, err := s.loadRuntimeSnapshot(ctx)
		if err != nil || runtimeSnapshot == nil || runtimeSnapshot.config == nil || id >= runtimeSnapshot.config.WorkerCount {
			cancel()
			time.Sleep(time.Second)
			continue
		}
		cfg := runtimeSnapshot.config
		task, ok := s.dequeueAsyncTask(ctx, time.Second)
		if !ok {
			cancel()
			continue
		}
		s.processAsyncTask(ctx, cfg, id, task)
		cancel()
	}
}

func (s *ContentModerationService) processAsyncTask(ctx context.Context, cfg *ContentModerationConfig, workerID int, task contentModerationTask) {
	if task.supplemental {
		defer s.supplementalPending.Add(-1)
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("content_moderation.worker_panic", "worker_id", workerID, "recover", r)
		}
	}()
	unlockUser := s.lockContentModerationUserState(task.input.UserID, false)
	defer unlockUser()
	if !s.contentModerationTaskIsCurrent(ctx, task) {
		s.asyncProcessed.Add(1)
		slog.Info("content_moderation.stale_async_task_skipped",
			"worker_id", workerID,
			"user_id", task.input.UserID,
			"request_id", task.input.RequestID,
			"supplemental", task.supplemental)
		return
	}
	taskCfg := task.config
	if taskCfg == nil {
		taskCfg = cfg
	}
	configChanged := false
	scopeSideEffectsDisabled := false
	scopeSideEffectsReason := ""
	if s.settingRepo != nil {
		runtimeSnapshot, err := s.loadFreshRuntimeSnapshot(ctx)
		if err != nil || runtimeSnapshot == nil || runtimeSnapshot.config == nil {
			s.asyncErrors.Add(1)
			slog.Warn("content_moderation.async_runtime_refresh_failed",
				"worker_id", workerID,
				"user_id", task.input.UserID,
				"request_id", task.input.RequestID,
				"error", err)
			return
		}
		liveCfg := runtimeSnapshot.config
		providerExecutionPending := task.log == nil
		if !runtimeSnapshot.riskControlEnabled || !liveCfg.Enabled || liveCfg.Mode == ContentModerationModeOff ||
			(providerExecutionPending && len(liveCfg.apiKeys()) == 0) {
			slog.Info("content_moderation.async_task_skipped_runtime_disabled",
				"worker_id", workerID,
				"user_id", task.input.UserID,
				"request_id", task.input.RequestID)
			return
		}
		if !liveCfg.includesGroup(task.input.GroupID) || !liveCfg.includesModel(task.input.Model) || !liveCfg.includesUser(task.input.UserID) {
			slog.Info("content_moderation.async_task_skipped_runtime_scope",
				"worker_id", workerID,
				"user_id", task.input.UserID,
				"request_id", task.input.RequestID)
			return
		}
		if taskCfg == nil || taskCfg.AuditProvider != liveCfg.AuditProvider {
			slog.Info("content_moderation.async_task_skipped_provider_changed",
				"worker_id", workerID,
				"user_id", task.input.UserID,
				"request_id", task.input.RequestID)
			return
		}
		liveAccountFilter := normalizeContentModerationAccountFilter(liveCfg.AccountFilter)
		if liveAccountFilter.Type != ContentModerationScopeFilterAll {
			if task.input.AccountID <= 0 {
				slog.Warn("content_moderation.async_task_account_scope_identity_unavailable",
					"worker_id", workerID,
					"user_id", task.input.UserID,
					"request_id", task.input.RequestID,
					"provider_execution_pending", providerExecutionPending,
					"scope_reason", "account_scope_identity_unavailable")
				if providerExecutionPending {
					return
				}
				scopeSideEffectsDisabled = true
				scopeSideEffectsReason = "account_scope_identity_unavailable"
			} else {
				scopeAccountID, scopeErr := s.contentModerationAsyncScopeAccountID(ctx, task.input.AccountID)
				if scopeErr != nil {
					slog.Warn("content_moderation.async_task_account_scope_resolution_failed",
						"worker_id", workerID,
						"user_id", task.input.UserID,
						"request_id", task.input.RequestID,
						"account_id", task.input.AccountID,
						"error", scopeErr)
					return
				}
				if !liveCfg.includesAccount(scopeAccountID) {
					slog.Info("content_moderation.async_task_skipped_runtime_account_scope",
						"worker_id", workerID,
						"user_id", task.input.UserID,
						"request_id", task.input.RequestID,
						"account_id", task.input.AccountID,
						"scope_account_id", scopeAccountID)
					return
				}
			}
		}
		taskGeneration := task.configGeneration
		if taskGeneration == ([sha256.Size]byte{}) {
			taskGeneration = contentModerationRuntimeGeneration(taskCfg, runtimeSnapshot.engineFingerprintSignals)
		}
		liveGeneration := runtimeSnapshot.runtimeGeneration
		if liveGeneration == ([sha256.Size]byte{}) {
			liveGeneration = contentModerationRuntimeGeneration(liveCfg, runtimeSnapshot.engineFingerprintSignals)
		}
		configChanged = taskGeneration != liveGeneration
		taskCfg = contentModerationAsyncExecutionConfig(taskCfg, liveCfg)
		if taskCfg == nil {
			return
		}
		taskCfg.sideEffectsDisabled = taskCfg.sideEffectsDisabled || configChanged || scopeSideEffectsDisabled
	}
	if task.log != nil {
		s.asyncActive.Add(1)
		defer s.asyncActive.Add(-1)
		queueDelay := int(time.Since(task.enqueuedAt).Milliseconds())
		task.log.QueueDelayMS = &queueDelay
		sideEffectsSuppressed := configChanged || scopeSideEffectsDisabled
		recordHash := task.recordHash && !sideEffectsSuppressed
		applySideEffects := task.applySideEffects && !sideEffectsSuppressed
		if scopeSideEffectsReason != "" {
			task.log.AuditDetails.HashState = "candidate"
			task.log.AuditDetails.HashPromotionReason = scopeSideEffectsReason
		} else if configChanged && (task.recordHash || task.applySideEffects) {
			task.log.AuditDetails.HashState = "candidate"
			task.log.AuditDetails.HashPromotionReason = "configuration_generation_changed"
		}
		s.persistContentModerationLog(ctx, taskCfg, task.log, task.inputHash, recordHash, applySideEffects)
		s.asyncProcessed.Add(1)
		return
	}
	if taskCfg == nil || !taskCfg.Enabled || taskCfg.Mode == ContentModerationModeOff || len(taskCfg.apiKeys()) == 0 {
		return
	}
	if !taskCfg.includesGroup(task.input.GroupID) {
		return
	}
	if !taskCfg.includesModel(task.input.Model) {
		return
	}
	if task.log == nil && !task.supplemental && taskCfg.Mode == ContentModerationModeObserve && !task.enqueuedAt.IsZero() {
		maxQueueWait := contentModerationObserveMaxQueueWait(taskCfg)
		if maxQueueWait <= 0 || time.Since(task.enqueuedAt) > maxQueueWait {
			s.asyncDropped.Add(1)
			s.asyncProcessed.Add(1)
			slog.Warn("content_moderation.stale_observe_task_dropped",
				"worker_id", workerID,
				"user_id", task.input.UserID,
				"request_id", task.input.RequestID,
				"queue_delay_ms", time.Since(task.enqueuedAt).Milliseconds())
			return
		}
	}
	s.asyncActive.Add(1)
	defer s.asyncActive.Add(-1)
	queueDelay := int(time.Since(task.enqueuedAt).Milliseconds())
	_ = s.checkSyncIdempotent(ctx, task.input, taskCfg, task.content, task.inputHash, &queueDelay, false)
	s.asyncProcessed.Add(1)
}

func contentModerationAsyncExecutionConfig(taskCfg, liveCfg *ContentModerationConfig) *ContentModerationConfig {
	if taskCfg == nil || liveCfg == nil || taskCfg.AuditProvider != liveCfg.AuditProvider {
		return nil
	}
	executionCfg := cloneContentModerationConfig(taskCfg)
	if executionCfg.AuditProvider == ContentModerationProviderAIChat {
		executionCfg.AIChat.BaseURL = liveCfg.AIChat.BaseURL
		executionCfg.AIChat.ProxyID = cloneInt64Ptr(liveCfg.AIChat.ProxyID)
		executionCfg.AIChat.APIKeys = append([]string(nil), liveCfg.apiKeys()...)
		return executionCfg
	}
	executionCfg.BaseURL = liveCfg.BaseURL
	executionCfg.ProxyID = cloneInt64Ptr(liveCfg.ProxyID)
	executionCfg.APIKey = ""
	executionCfg.APIKeys = append([]string(nil), liveCfg.apiKeys()...)
	return executionCfg
}

func (s *ContentModerationService) contentModerationAsyncScopeAccountID(ctx context.Context, accountID int64) (int64, error) {
	if accountID <= 0 {
		return 0, errors.New("selected moderation account is unavailable")
	}
	if s == nil || s.accountScopeRepo == nil {
		return accountID, nil
	}
	accounts, err := s.accountScopeRepo.GetByIDs(ctx, []int64{accountID})
	if err != nil {
		return 0, fmt.Errorf("resolve selected moderation account scope: %w", err)
	}
	for _, account := range accounts {
		if account == nil || account.ID != accountID {
			continue
		}
		if account.ParentAccountID != nil && *account.ParentAccountID > 0 {
			return *account.ParentAccountID, nil
		}
		return account.ID, nil
	}
	return 0, fmt.Errorf("selected moderation account %d was not found", accountID)
}

func (s *ContentModerationService) captureContentModerationEpoch(ctx context.Context, input *ContentModerationCheckInput) {
	if s == nil || input == nil || input.UserID <= 0 {
		return
	}
	if input.ModerationEpochSet {
		return
	}
	store, ok := s.hashCache.(ContentModerationUserStateEpochStore)
	if !ok {
		return
	}
	input.ModerationEpochSet = true
	epoch, err := store.GetContentModerationUserEpoch(ctx, input.UserID)
	if err != nil {
		input.ModerationEpoch = -1
		slog.Warn("content_moderation.user_epoch_get_failed", "user_id", input.UserID, "error", err)
		return
	}
	input.ModerationEpoch = epoch
}

// SnapshotContentModerationUserEpoch captures the user's moderation generation
// before asynchronous work is detached from the request context.
func (s *ContentModerationService) SnapshotContentModerationUserEpoch(ctx context.Context, userID int64) (int64, bool) {
	input := ContentModerationCheckInput{UserID: userID}
	s.captureContentModerationEpoch(ctx, &input)
	return input.ModerationEpoch, input.ModerationEpochSet
}

// IsContentModerationUserEpochCurrent verifies a snapshot without applying any
// side effects. Callers that protect an existing block must treat errors as
// indeterminate and keep the block until a later successful check.
func (s *ContentModerationService) IsContentModerationUserEpochCurrent(ctx context.Context, userID, epoch int64, epochSet bool) (bool, error) {
	if !epochSet || userID <= 0 {
		return true, nil
	}
	if s == nil {
		return false, errors.New("content moderation service is unavailable")
	}
	store, ok := s.hashCache.(ContentModerationUserStateEpochStore)
	if !ok {
		return false, errors.New("content moderation user epoch store is unavailable")
	}
	currentEpoch, err := store.GetContentModerationUserEpoch(ctx, userID)
	if err != nil {
		return false, err
	}
	return currentEpoch == epoch, nil
}

func (s *ContentModerationService) contentModerationEpochIsCurrent(ctx context.Context, userID, epoch int64, epochSet bool) bool {
	current, err := s.IsContentModerationUserEpochCurrent(ctx, userID, epoch, epochSet)
	if err != nil {
		slog.Warn("content_moderation.user_epoch_check_failed", "user_id", userID, "error", err)
		return false
	}
	return current
}

func (s *ContentModerationService) contentModerationTaskIsCurrent(ctx context.Context, task contentModerationTask) bool {
	return s.contentModerationEpochIsCurrent(
		ctx,
		task.input.UserID,
		task.input.ModerationEpoch,
		task.input.ModerationEpochSet,
	)
}

func (s *ContentModerationService) lockContentModerationUserState(userID int64, exclusive bool) func() {
	if s == nil || userID <= 0 {
		return func() {}
	}
	lock := &s.moderationUserStateLocks[uint64(userID)%uint64(len(s.moderationUserStateLocks))]
	if exclusive {
		lock.Lock()
		return lock.Unlock
	}
	lock.RLock()
	return lock.RUnlock
}

func (s *ContentModerationService) reserveSupplementalSlot(limit int64) bool {
	if s == nil || limit <= 0 {
		return false
	}
	for {
		pending := s.supplementalPending.Load()
		if pending >= limit {
			return false
		}
		if s.supplementalPending.CompareAndSwap(pending, pending+1) {
			return true
		}
	}
}

func compactSupplementalModerationContent(content ContentModerationInput, cfg *ContentModerationConfig) ContentModerationInput {
	maxRunes := defaultAIChatMaxInputChars
	if cfg != nil && cfg.AIChat.MaxInputChars > 0 {
		maxRunes = cfg.AIChat.MaxInputChars
	}
	if maxRunes > maxModerationInputRunes {
		maxRunes = maxModerationInputRunes
	}
	content.Text = trimModerationContext(content.Text, maxRunes)
	content.CurrentText = trimRunes(content.CurrentText, maxModerationExcerptRunes)
	content.Images = nil
	return content
}

func contentModerationSupplementalQueueLimit(cfg *ContentModerationConfig, configuredLimit int) int {
	limit := min(configuredLimit, maxContentModerationSupplementalQueueSize)
	maxInputRunes := defaultAIChatMaxInputChars
	if cfg != nil && cfg.AIChat.MaxInputChars > 0 {
		maxInputRunes = min(cfg.AIChat.MaxInputChars, maxModerationInputRunes)
	}
	if maxInputRunes > 0 {
		limit = min(limit, max(1, maxContentModerationSupplementalRetainedRunes/maxInputRunes))
	}
	return max(1, limit)
}

func (s *ContentModerationService) dequeueAsyncTask(ctx context.Context, idleWait time.Duration) (contentModerationTask, bool) {
	var zero contentModerationTask
	if s == nil || s.asyncQueue == nil {
		return zero, false
	}
	if idleWait <= 0 {
		idleWait = time.Second
	}
	timer := time.NewTimer(idleWait)
	defer timer.Stop()
	select {
	case task, ok := <-s.asyncQueue:
		return task, ok
	case <-ctx.Done():
		return zero, false
	case <-timer.C:
		return zero, false
	}
}

func (s *ContentModerationService) ListLogs(ctx context.Context, filter ContentModerationLogFilter) ([]ContentModerationLog, *pagination.PaginationResult, error) {
	if filter.Pagination.Page <= 0 {
		filter.Pagination.Page = 1
	}
	if filter.Pagination.PageSize <= 0 {
		filter.Pagination.PageSize = 20
	}
	if filter.Pagination.PageSize > 100 {
		filter.Pagination.PageSize = 100
	}
	if filter.Pagination.SortOrder == "" {
		filter.Pagination.SortOrder = pagination.SortOrderDesc
	}
	return s.repo.ListLogs(ctx, filter)
}

func (s *ContentModerationService) UnbanUser(ctx context.Context, userID int64, requestedMode ...string) (*ContentModerationUnbanUserResult, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("CONTENT_MODERATION_REPOSITORY_UNAVAILABLE", "内容风控仓储不可用")
	}
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER_ID", "用户 ID 无效")
	}
	unlockUser := s.lockContentModerationUserState(userID, true)
	defer unlockUser()
	lifecycleRepo, ok := s.repo.(ContentModerationLifecycleRepository)
	if !ok {
		return nil, infraerrors.InternalServer("CONTENT_MODERATION_LIFECYCLE_REPOSITORY_UNAVAILABLE", "内容风控生命周期仓储不可用")
	}
	mode := ContentModerationUnbanModeRestoreAndClearRisk
	if len(requestedMode) > 0 && strings.TrimSpace(requestedMode[0]) != "" {
		mode = strings.TrimSpace(requestedMode[0])
	}
	if mode != ContentModerationUnbanModeRestoreOnly && mode != ContentModerationUnbanModeRestoreAndClearRisk && mode != ContentModerationUnbanModeClearRiskOnly {
		return nil, infraerrors.BadRequest("INVALID_CONTENT_MODERATION_UNBAN_MODE", "无效的内容风控解禁模式")
	}
	state, err := lifecycleRepo.GetModerationUserState(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get content moderation user state: %w", err)
	}
	if state != nil && state.UserID != userID {
		return nil, infraerrors.InternalServer("CONTENT_MODERATION_STATE_USER_MISMATCH", "风控用户状态与请求用户不匹配")
	}
	if mode == ContentModerationUnbanModeClearRiskOnly {
		if state == nil {
			return nil, infraerrors.Conflict("CONTENT_MODERATION_RISK_CLEAR_NOT_ELIGIBLE", "该用户没有可重试清理的风控生命周期记录")
		}
		if state.ModerationOwnedDisabled {
			return nil, infraerrors.Conflict("CONTENT_MODERATION_BAN_STILL_ACTIVE", "该用户仍处于风控封禁状态，请使用恢复解禁模式")
		}
		currentStatus := ""
		if s.userRepo != nil {
			user, err := s.userRepo.GetByID(ctx, userID)
			if err != nil {
				return nil, fmt.Errorf("get user before content moderation risk cleanup: %w", err)
			}
			if user == nil || user.ID != userID {
				return nil, infraerrors.InternalServer("CONTENT_MODERATION_USER_ID_MISMATCH", "风险清理用户与查询结果不匹配")
			}
			currentStatus = user.Status
		}
		cleaner, ok := s.hashCache.(ContentModerationUserStateCleaner)
		if !ok {
			return nil, infraerrors.InternalServer("CONTENT_MODERATION_STATE_CLEANER_UNAVAILABLE", "短期风控状态清理器不可用")
		}
		if _, err := cleaner.ClearContentModerationUserState(ctx, userID); err != nil {
			return nil, fmt.Errorf("clear content moderation user state: %w", err)
		}
		return &ContentModerationUnbanUserResult{
			UserID:           userID,
			Status:           currentStatus,
			Mode:             mode,
			Restored:         false,
			RiskStateCleared: true,
		}, nil
	}
	if state == nil || !state.ModerationOwnedDisabled {
		return nil, infraerrors.Conflict("CONTENT_MODERATION_BAN_NOT_OWNED", "该用户不是由内容风控自动封禁，不能在风控中心解禁")
	}
	var cleaner ContentModerationUserStateCleaner
	if mode == ContentModerationUnbanModeRestoreAndClearRisk {
		var ok bool
		cleaner, ok = s.hashCache.(ContentModerationUserStateCleaner)
		if !ok {
			return nil, infraerrors.InternalServer("CONTENT_MODERATION_STATE_CLEANER_UNAVAILABLE", "短期风控状态清理器不可用")
		}
	}
	restored, err := lifecycleRepo.RestoreModerationOwnedBan(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("restore moderation-owned user: %w", err)
	}
	if !restored {
		return nil, infraerrors.Conflict("CONTENT_MODERATION_UNBAN_RACE", "风控封禁状态已变化，未执行解禁")
	}
	result := &ContentModerationUnbanUserResult{
		UserID:           userID,
		Status:           StatusActive,
		Mode:             mode,
		Restored:         true,
		RiskStateCleared: false,
	}
	if cleaner != nil {
		if _, err := cleaner.ClearContentModerationUserState(ctx, userID); err != nil {
			result.Warning = "账号已恢复，但短期风控状态清理失败，请检查 Redis 后重试清理"
			slog.Warn("content_moderation.unban_risk_state_clear_failed", "user_id", userID, "error", err)
		} else {
			result.RiskStateCleared = true
		}
	} else if epochStore, ok := s.hashCache.(ContentModerationUserStateEpochStore); ok {
		if _, err := epochStore.AdvanceContentModerationUserEpoch(ctx, userID); err != nil {
			result.Warning = "账号已恢复，但旧审核任务隔离失败，请检查 Redis"
			slog.Warn("content_moderation.unban_epoch_advance_failed", "user_id", userID, "error", err)
		}
	}
	// Invalidate authentication only after the Redis cleanup attempt. The
	// account is already restored, so cleanup failure is returned as a warning.
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	return result, nil
}

func (s *ContentModerationService) DeleteFlaggedInputHash(ctx context.Context, inputHash string) (*ContentModerationDeleteHashResult, error) {
	inputHash = normalizeContentModerationHash(inputHash)
	if inputHash == "" {
		return nil, infraerrors.BadRequest("INVALID_CONTENT_MODERATION_HASH", "风险输入哈希无效")
	}
	if s == nil || s.hashCache == nil {
		return nil, infraerrors.InternalServer("CONTENT_MODERATION_HASH_CACHE_UNAVAILABLE", "内容审计哈希缓存不可用")
	}
	deleted, err := s.hashCache.DeleteFlaggedInputHash(ctx, inputHash)
	if err != nil {
		return nil, fmt.Errorf("delete content moderation flagged hash: %w", err)
	}
	return &ContentModerationDeleteHashResult{
		InputHash: inputHash,
		Deleted:   deleted,
	}, nil
}

func (s *ContentModerationService) ClearFlaggedInputHashes(ctx context.Context) (*ContentModerationClearHashesResult, error) {
	if s == nil || s.hashCache == nil {
		return nil, infraerrors.InternalServer("CONTENT_MODERATION_HASH_CACHE_UNAVAILABLE", "内容审计哈希缓存不可用")
	}
	deleted, err := s.hashCache.ClearFlaggedInputHashes(ctx)
	if err != nil {
		return nil, fmt.Errorf("clear content moderation flagged hashes: %w", err)
	}
	return &ContentModerationClearHashesResult{Deleted: deleted}, nil
}

func (s *ContentModerationService) GetStatus(ctx context.Context) (*ContentModerationRuntimeStatus, error) {
	if s == nil {
		return &ContentModerationRuntimeStatus{}, nil
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	riskEnabled := s.isRiskControlEnabled(ctx)
	active := int(s.asyncActive.Load())
	if active < 0 {
		active = 0
	}
	if active > cfg.WorkerCount {
		active = cfg.WorkerCount
	}
	preBlockActive := int(s.preBlockActive.Load())
	if preBlockActive < 0 {
		preBlockActive = 0
	}
	preBlockChecked := s.preBlockChecked.Load()
	preBlockAvgLatency := int64(0)
	if preBlockChecked > 0 {
		preBlockAvgLatency = s.preBlockLatencyTotalMS.Load() / preBlockChecked
	}
	queueLength := 0
	if s.asyncQueue != nil {
		queueLength = len(s.asyncQueue)
	}
	queueUsage := 0.0
	if cfg.QueueSize > 0 {
		queueUsage = float64(queueLength) * 100 / float64(cfg.QueueSize)
	}
	var flaggedHashCount int64
	if s.hashCache != nil {
		if n, err := s.hashCache.CountFlaggedInputHashes(ctx); err == nil {
			flaggedHashCount = n
		} else {
			slog.Warn("content_moderation.hash_count_failed", "error", err)
		}
	}
	var lastCleanupAt *time.Time
	if unix := s.lastCleanupUnix.Load(); unix > 0 {
		t := time.Unix(unix, 0)
		lastCleanupAt = &t
	}
	metricsStartedAt := s.contentModerationMetricsStartedAt()
	costSnapshot := s.contentModerationAuditCostSnapshot()
	usageUnknown := s.auditUsageUnknown.Load()
	costCoverage := contentModerationCostCoverage(costSnapshot.priced, costSnapshot.unpriced, usageUnknown)
	var estimatedCostUSD *float64
	if costSnapshot.priced > 0 {
		estimatedCostUSD = cloneFloat64Ptr(&costSnapshot.estimatedUSD)
	}
	businessActualCostUSD, businessCostErr := s.contentModerationBusinessCostUSD(ctx, metricsStartedAt)
	if businessCostErr != nil {
		slog.Warn("content_moderation.business_cost_query_failed", "since", metricsStartedAt, "error", businessCostErr)
		businessActualCostUSD = nil
	}
	var auditCostPerBusinessUSD *float64
	if estimatedCostUSD != nil && businessActualCostUSD != nil && *businessActualCostUSD > 0 {
		ratio := *estimatedCostUSD / *businessActualCostUSD
		auditCostPerBusinessUSD = &ratio
	}
	return &ContentModerationRuntimeStatus{
		Enabled:                      cfg.Enabled,
		RiskControlEnabled:           riskEnabled,
		Mode:                         cfg.Mode,
		WorkerCount:                  cfg.WorkerCount,
		MaxWorkers:                   maxContentModerationWorkerCount,
		ActiveWorkers:                active,
		IdleWorkers:                  cfg.WorkerCount - active,
		QueueSize:                    cfg.QueueSize,
		QueueLength:                  queueLength,
		QueueUsagePercent:            queueUsage,
		Enqueued:                     s.asyncEnqueued.Load(),
		Dropped:                      s.asyncDropped.Load(),
		Processed:                    s.asyncProcessed.Load(),
		Errors:                       s.asyncErrors.Load(),
		PreBlockActive:               preBlockActive,
		PreBlockChecked:              preBlockChecked,
		PreBlockAllowed:              s.preBlockAllowed.Load(),
		PreBlockBlocked:              s.preBlockBlocked.Load(),
		PreBlockErrors:               s.preBlockErrors.Load(),
		PreBlockAvgLatencyMS:         preBlockAvgLatency,
		PreBlockAPIKeyActive:         s.preBlockAPIKeyActive(cfg.apiKeys()),
		PreBlockAPIKeyAvailableCount: s.preBlockAPIKeyAvailableCount(cfg.apiKeys()),
		PreBlockAPIKeyTotalCalls:     s.preBlockAPIKeyTotalCalls(cfg.apiKeys()),
		PreBlockAPIKeyLoads:          s.preBlockAPIKeyLoads(cfg.apiKeys()),
		APIKeyStatuses:               s.apiKeyStatuses(cfg.apiKeys()),
		AuditFastCalls:               s.auditFastCalls.Load(),
		AuditFullCalls:               s.auditFullCalls.Load(),
		AuditMaxCalls:                s.auditMaxCalls.Load(),
		AuditResultCacheHits:         s.auditResultCacheHits.Load(),
		AuditPromptTokens:            s.auditPromptTokens.Load(),
		AuditCachedInputTokens:       s.auditCachedInputTokens.Load(),
		AuditUncachedInputTokens:     s.auditUncachedInputTokens.Load(),
		AuditOutputTokens:            s.auditOutputTokens.Load(),
		AuditUsageComplete:           s.auditUsageComplete.Load(),
		AuditUsageUnknown:            usageUnknown,
		AuditInputChars:              s.auditInputChars.Load(),
		MetricsStartedAt:             metricsStartedAt,
		AuditEstimatedCostUSD:        estimatedCostUSD,
		AuditCostCoverage:            costCoverage,
		AuditCostComplete:            costCoverage == ContentModerationCostCoverageComplete,
		AuditCostPartial:             costCoverage == ContentModerationCostCoveragePartial,
		AuditCostPricedSamples:       costSnapshot.priced,
		AuditCostUnpricedSamples:     costSnapshot.unpriced,
		AuditCostByVersionUSD:        costSnapshot.byVersionUSD,
		BusinessActualCostUSD:        businessActualCostUSD,
		AuditCostPerBusinessUSD:      auditCostPerBusinessUSD,
		AuditStageLatency: map[string]ContentModerationLatencySummary{
			string(voteaimoderation.StageFast): s.auditFastLatency.snapshot(),
			string(voteaimoderation.StageFull): s.auditFullLatency.snapshot(),
			string(voteaimoderation.StageMax):  s.auditMaxLatency.snapshot(),
		},
		AuditSessionSources: map[string]int64{
			ContentModerationSessionSourceHeader:         s.auditSessionHeader.Load(),
			ContentModerationSessionSourcePromptCacheKey: s.auditSessionPromptCache.Load(),
			ContentModerationSessionSourceNone:           s.auditSessionNone.Load(),
		},
		AuditPrefixContinuity: map[string]int64{
			"continuous":        s.auditPrefixContinuous.Load(),
			"breaks":            s.auditPrefixBreaks.Load(),
			"policy_changed":    s.auditPrefixBreakPolicy.Load(),
			"model_changed":     s.auditPrefixBreakModel.Load(),
			"history_rewritten": s.auditPrefixBreakHistory.Load(),
			"history_truncated": s.auditPrefixBreakTruncate.Load(),
			"compaction_epoch":  s.auditPrefixBreakCompact.Load(),
			"session_changed":   s.auditPrefixBreakSession.Load(),
			"audit_key_changed": s.auditPrefixBreakKey.Load(),
			"unknown":           s.auditPrefixBreakUnknown.Load(),
		},
		FlaggedHashCount:         flaggedHashCount,
		LastCleanupAt:            lastCleanupAt,
		LastCleanupDeletedHit:    s.lastCleanupDeletedHit.Load(),
		LastCleanupDeletedNonHit: s.lastCleanupDeletedNonHit.Load(),
	}, nil
}

func (s *ContentModerationService) cleanupWorker() {
	timer := time.NewTimer(contentModerationCleanupDelay)
	defer timer.Stop()
	for {
		<-timer.C
		s.runCleanupOnce()
		timer.Reset(contentModerationCleanupInterval)
	}
}

func (s *ContentModerationService) runCleanupOnce() {
	if s == nil || s.repo == nil || s.settingRepo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), contentModerationCleanupTimeout)
	defer cancel()
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		slog.Warn("content_moderation.cleanup_load_config_failed", "error", err)
		return
	}
	now := time.Now()
	hitBefore := now.AddDate(0, 0, -cfg.HitRetentionDays)
	nonHitBefore := now.AddDate(0, 0, -cfg.NonHitRetentionDays)
	result, err := s.repo.CleanupExpiredLogs(ctx, hitBefore, nonHitBefore)
	if err != nil {
		slog.Warn("content_moderation.cleanup_failed", "error", err)
		return
	}
	if result == nil {
		return
	}
	s.lastCleanupUnix.Store(result.FinishedAt.Unix())
	s.lastCleanupDeletedHit.Store(result.DeletedHit)
	s.lastCleanupDeletedNonHit.Store(result.DeletedNonHit)
}

func (s *ContentModerationService) loadConfig(ctx context.Context) (*ContentModerationConfig, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyContentModerationConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			raw = ""
		} else {
			return nil, fmt.Errorf("get content moderation config: %w", err)
		}
	}
	cfg, err := parseContentModerationConfig(raw)
	if err != nil {
		return nil, err
	}
	cfg.AccountFilter, err = s.normalizeAccountScopeFilter(ctx, cfg.AccountFilter)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func parseContentModerationConfig(raw string) (*ContentModerationConfig, error) {
	cfg := defaultContentModerationConfig()
	if strings.TrimSpace(raw) == "" {
		cfg.normalize()
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, infraerrors.BadRequest("INVALID_CONTENT_MODERATION_CONFIG", "内容审计配置不是有效 JSON")
	}
	cfg.normalize()
	return cfg, nil
}

func (s *ContentModerationService) loadRuntimeSnapshot(ctx context.Context) (*contentModerationRuntimeSnapshot, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("content moderation setting repository unavailable")
	}
	now := time.Now()
	if snapshot := s.runtimeSnapshot.Load(); snapshot != nil {
		if now.Sub(snapshot.loadedAt) < s.runtimeSnapshotTTL() {
			return snapshot, nil
		}
		s.triggerRuntimeSnapshotRefresh()
		return snapshot, nil
	}

	s.runtimeRefreshMu.Lock()
	defer s.runtimeRefreshMu.Unlock()
	if snapshot := s.runtimeSnapshot.Load(); snapshot != nil {
		return snapshot, nil
	}
	return s.refreshRuntimeSnapshot(ctx)
}

func (s *ContentModerationService) loadFreshRuntimeSnapshot(ctx context.Context) (*contentModerationRuntimeSnapshot, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("content moderation setting repository unavailable")
	}
	s.runtimeRefreshMu.Lock()
	defer s.runtimeRefreshMu.Unlock()
	return s.refreshRuntimeSnapshot(ctx)
}

func (s *ContentModerationService) runtimeSnapshotTTL() time.Duration {
	if s != nil && s.runtimeCacheTTL > 0 {
		return s.runtimeCacheTTL
	}
	return contentModerationRuntimeCacheTTL
}

func (s *ContentModerationService) triggerRuntimeSnapshotRefresh() {
	if s == nil || s.runtimeRefreshDeferred() || !s.runtimeRefreshMu.TryLock() {
		return
	}
	if s.runtimeRefreshDeferred() {
		s.runtimeRefreshMu.Unlock()
		return
	}
	go func() {
		defer s.runtimeRefreshMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), contentModerationRuntimeRefreshTimeout)
		defer cancel()
		if _, err := s.refreshRuntimeSnapshot(ctx); err != nil {
			s.runtimeRefreshRetryAt.Store(time.Now().Add(s.runtimeSnapshotTTL()).UnixNano())
			slog.Warn("content_moderation.runtime_snapshot_refresh_failed", "error", err)
		}
	}()
}

func (s *ContentModerationService) runtimeRefreshDeferred() bool {
	if s == nil {
		return false
	}
	return time.Now().UnixNano() < s.runtimeRefreshRetryAt.Load()
}

func (s *ContentModerationService) refreshRuntimeSnapshot(ctx context.Context) (*contentModerationRuntimeSnapshot, error) {
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyRiskControlEnabled,
		SettingKeyContentModerationConfig,
		SettingKeyCodexCLIOnlyEngineFingerprintSignals,
	})
	if err != nil {
		return nil, fmt.Errorf("get content moderation runtime settings: %w", err)
	}
	rawConfig := values[SettingKeyContentModerationConfig]
	rawEngineFingerprintSignals := values[SettingKeyCodexCLIOnlyEngineFingerprintSignals]
	configDigest := sha256.Sum256([]byte(rawConfig + "\x00" + rawEngineFingerprintSignals))
	if current := s.runtimeSnapshot.Load(); current != nil && !current.accountScopeFallback && current.configDigest == configDigest {
		snapshot := &contentModerationRuntimeSnapshot{
			riskControlEnabled:       values[SettingKeyRiskControlEnabled] == "true",
			config:                   current.config,
			keywordMatcher:           current.keywordMatcher,
			configDigest:             configDigest,
			runtimeGeneration:        current.runtimeGeneration,
			engineFingerprintSignals: append([]openaipkg.EngineFingerprintSignal(nil), current.engineFingerprintSignals...),
			loadedAt:                 time.Now(),
		}
		s.runtimeSnapshot.Store(snapshot)
		s.runtimeRefreshRetryAt.Store(0)
		return snapshot, nil
	}
	cfg, err := parseContentModerationConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	normalizedAccountFilter, normalizeErr := s.normalizeAccountScopeFilter(ctx, cfg.AccountFilter)
	if normalizeErr != nil {
		// Runtime filtering must fail secure: if shadow-account resolution is
		// temporarily unavailable, audit every selected account instead of
		// allowing requests to bypass moderation during a cold start.
		slog.Warn("content_moderation.account_scope_normalize_failed_fallback_all",
			"filter_type", cfg.AccountFilter.Type,
			"account_ids", cfg.AccountFilter.AccountIDs,
			"error", normalizeErr)
		cfg.AccountFilter = ContentModerationAccountFilter{
			Type:       ContentModerationScopeFilterAll,
			AccountIDs: []int64{},
		}
	} else {
		cfg.AccountFilter = normalizedAccountFilter
	}
	engineFingerprintSignals, validFingerprintSignals := openaipkg.ParseEngineFingerprintSignals(rawEngineFingerprintSignals)
	if strings.TrimSpace(rawEngineFingerprintSignals) == "" || !validFingerprintSignals {
		slog.Warn("content_moderation.invalid_engine_fingerprint_signals_fallback_default")
		engineFingerprintSignals = append([]openaipkg.EngineFingerprintSignal(nil), openaipkg.DefaultEngineFingerprintSignals...)
	}
	snapshot := &contentModerationRuntimeSnapshot{
		riskControlEnabled:       values[SettingKeyRiskControlEnabled] == "true",
		config:                   cfg,
		keywordMatcher:           newContentModerationKeywordMatcher(cfg.BlockedKeywords),
		configDigest:             configDigest,
		runtimeGeneration:        contentModerationRuntimeGeneration(cfg, engineFingerprintSignals),
		accountScopeFallback:     normalizeErr != nil,
		engineFingerprintSignals: append([]openaipkg.EngineFingerprintSignal(nil), engineFingerprintSignals...),
		loadedAt:                 time.Now(),
	}
	s.runtimeSnapshot.Store(snapshot)
	s.runtimeRefreshRetryAt.Store(0)
	return snapshot, nil
}

func (s *ContentModerationService) replaceRuntimeConfig(cfg *ContentModerationConfig) {
	if s == nil || cfg == nil {
		return
	}
	s.runtimeRefreshMu.Lock()
	hasSnapshot := s.runtimeSnapshot.Load() != nil
	s.runtimeRefreshMu.Unlock()
	if !hasSnapshot {
		return
	}
	config := cloneContentModerationConfig(cfg)
	keywordMatcher := newContentModerationKeywordMatcher(cfg.BlockedKeywords)
	s.runtimeRefreshMu.Lock()
	defer s.runtimeRefreshMu.Unlock()
	current := s.runtimeSnapshot.Load()
	if current == nil {
		return
	}
	s.runtimeSnapshot.Store(&contentModerationRuntimeSnapshot{
		riskControlEnabled:       current.riskControlEnabled,
		config:                   config,
		keywordMatcher:           keywordMatcher,
		configDigest:             [sha256.Size]byte{},
		runtimeGeneration:        contentModerationRuntimeGeneration(config, current.engineFingerprintSignals),
		accountScopeFallback:     current.accountScopeFallback,
		engineFingerprintSignals: append([]openaipkg.EngineFingerprintSignal(nil), current.engineFingerprintSignals...),
		loadedAt:                 time.Now(),
	})
}

func (s *contentModerationRuntimeSnapshot) matchBlockedKeyword(text string) (string, bool) {
	if s == nil || s.config == nil {
		return "", false
	}
	if s.keywordMatcher != nil {
		return s.keywordMatcher.Match(text)
	}
	return matchBlockedKeyword(text, s.config.BlockedKeywords)
}

func (s *ContentModerationService) isRiskControlEnabled(ctx context.Context) bool {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyRiskControlEnabled)
	if err != nil {
		return false
	}
	return raw == "true"
}

func (s *ContentModerationService) validateConfig(ctx context.Context, cfg *ContentModerationConfig) error {
	if cfg == nil {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_CONFIG", "内容审计配置不能为空")
	}
	if !isValidContentModerationScopeFilterType(cfg.UserFilter.Type) {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_USER_FILTER", "用户过滤类型必须是 all、include 或 exclude")
	}
	if !isValidContentModerationScopeFilterType(cfg.AccountFilter.Type) {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_ACCOUNT_FILTER", "账号过滤类型必须是 all、include 或 exclude")
	}
	if normalizeContentModerationProvider(cfg.AuditProvider) == ContentModerationProviderAIChat {
		if err := validateContentModerationIncrementalConfig(cfg); err != nil {
			return err
		}
	}
	cfg.normalize()
	switch cfg.Mode {
	case ContentModerationModeOff, ContentModerationModeObserve, ContentModerationModePreBlock:
	default:
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_MODE", "内容审计模式无效")
	}
	if _, err := url.ParseRequestURI(cfg.activeBaseURL()); err != nil {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_BASE_URL", "OpenAI Base URL 无效")
	}
	if cfg.AuditProvider == ContentModerationProviderAIChat {
		if strings.TrimSpace(cfg.AIChat.Model) == "" {
			return infraerrors.BadRequest("INVALID_AI_AUDIT_MODEL", "AI audit model is required")
		}
		if cfg.AIChat.ConfidenceThreshold <= 0 || cfg.AIChat.ConfidenceThreshold > 1 {
			return infraerrors.BadRequest("INVALID_AI_AUDIT_THRESHOLD", "AI audit confidence threshold must be between 0 and 1")
		}
	}
	if proxyID := cfg.activeProxyID(); proxyID != nil && s.proxyRepo != nil {
		if _, err := s.proxyRepo.GetByID(ctx, *proxyID); err != nil {
			cfg.ProxyID = proxyID
			return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_PROXY", fmt.Sprintf("代理服务器不存在: %d", *cfg.ProxyID))
		}
	}
	if cfg.BlockStatus < 400 || cfg.BlockStatus > 599 {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_BLOCK_STATUS", "拦截 HTTP 状态码必须在 400-599 之间")
	}
	if cfg.ModelFilter.Type != ContentModerationModelFilterAll && len(cfg.ModelFilter.Models) == 0 {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_MODEL_FILTER", "指定或排除模型时至少需要配置 1 个模型")
	}
	if !cfg.AllGroups && len(cfg.GroupIDs) > 0 && s.groupRepo != nil {
		for _, groupID := range cfg.GroupIDs {
			if _, err := s.groupRepo.GetByIDLite(ctx, groupID); err != nil {
				return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_GROUP", fmt.Sprintf("审计分组不存在: %d", groupID))
			}
		}
	}
	return nil
}

func validateContentModerationIncrementalConfig(cfg *ContentModerationConfig) error {
	if cfg == nil {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_CONFIG", "内容审计配置不能为空")
	}
	if cfg.AIChat.IncrementalAuditEnabled && !cfg.AIChat.InputProvenanceV2Enabled {
		return infraerrors.BadRequest("INVALID_AI_INCREMENTAL_PROVENANCE", "启用增量上下文审核前必须启用输入来源归一化 V2")
	}
	if cfg.AIChat.IncrementalAuditEnabled {
		if cfg.AIChat.RecentUserTurns < 1 || cfg.AIChat.RecentUserTurns > maxAIChatRecentUserTurns {
			return infraerrors.BadRequest("INVALID_AI_RECENT_USER_TURNS", "最近用户轮数必须在 1-8 之间")
		}
		if cfg.AIChat.SummaryMaxChars < 1 || cfg.AIChat.SummaryMaxChars > maxAIChatSummaryMaxChars {
			return infraerrors.BadRequest("INVALID_AI_SUMMARY_MAX_CHARS", "风险摘要最大字符数必须在 1-4000 之间")
		}
		if cfg.AIChat.FullReviewThreshold < 0.01 || cfg.AIChat.FullReviewThreshold > 1 ||
			cfg.AIChat.FullReviewThreshold >= cfg.AIChat.ConfidenceThreshold {
			return infraerrors.BadRequest("INVALID_AI_FULL_REVIEW_THRESHOLD", "完整复核阈值必须低于高风险拦截阈值")
		}
		if cfg.AIChat.FullReviewRiskDelta < 0.01 || cfg.AIChat.FullReviewRiskDelta > 1 {
			return infraerrors.BadRequest("INVALID_AI_FULL_REVIEW_RISK_DELTA", "风险上升触发差值必须在 0.01-1 之间")
		}
		if cfg.AIChat.PeriodicFullReviewTurns < 1 || cfg.AIChat.PeriodicFullReviewTurns > maxAIChatPeriodicFullReviewTurns {
			return infraerrors.BadRequest("INVALID_AI_PERIODIC_FULL_REVIEW_TURNS", "周期完整复核轮数必须在 1-100 之间")
		}
		if cfg.AIChat.FullReviewMaxInputChars < minAIChatMaxInputChars ||
			cfg.AIChat.FullReviewMaxInputChars > cfg.AIChat.MaxInputChars {
			return infraerrors.BadRequest("INVALID_AI_FULL_REVIEW_MAX_INPUT_CHARS", "完整复核最大字符数必须在 1000 与 AI 审核最大字符数之间")
		}
		for name, value := range map[string]int{
			"fast": cfg.AIChat.FastMaxOutputTokens,
			"full": cfg.AIChat.FullMaxOutputTokens,
			"max":  cfg.AIChat.MaxReviewMaxOutputTokens,
		} {
			if value < 1 || value > maxAIChatOutputTokens {
				return infraerrors.BadRequest("INVALID_AI_"+strings.ToUpper(name)+"_MAX_OUTPUT_TOKENS", "审核输出 Token 上限必须在 1-8192 之间")
			}
		}
		if cfg.AIChat.AuditContextTTLMinutes < 1 || cfg.AIChat.AuditContextTTLMinutes > maxAIChatAuditContextTTLMinutes {
			return infraerrors.BadRequest("INVALID_AI_AUDIT_CONTEXT_TTL", "增量审核状态 TTL 必须在 1-1440 分钟之间")
		}
	}
	if err := validateContentModerationPricingConfig(cfg.AIChat); err != nil {
		return err
	}
	return nil
}

func validateContentModerationPricingConfig(cfg ContentModerationAIChatConfig) error {
	rates := []*float64{
		cfg.UncachedInputUSDPerMillionTokens,
		cfg.CachedInputUSDPerMillionTokens,
		cfg.OutputUSDPerMillionTokens,
	}
	configuredRates := 0
	for _, rate := range rates {
		if rate == nil {
			continue
		}
		configuredRates++
		if *rate < 0 || *rate > maxContentModerationUSDPerMillionTokens {
			return infraerrors.BadRequest("INVALID_AI_AUDIT_PRICING_RATE", "DeepSeek 审核单价必须在 0-1000000 USD/百万 Token 之间")
		}
	}
	version := strings.TrimSpace(cfg.PricingVersion)
	if configuredRates == 0 && version == "" {
		return nil
	}
	if configuredRates != len(rates) {
		return infraerrors.BadRequest("INCOMPLETE_AI_AUDIT_PRICING", "必须同时配置未缓存输入、缓存输入和输出单价")
	}
	if version == "" || len(version) > 100 {
		return infraerrors.BadRequest("INVALID_AI_AUDIT_PRICING_VERSION", "审核费率版本不能为空且不能超过 100 个字符")
	}
	return nil
}

func contentModerationPricingConfigured(cfg ContentModerationAIChatConfig) bool {
	return strings.TrimSpace(cfg.PricingVersion) != "" &&
		cfg.UncachedInputUSDPerMillionTokens != nil &&
		cfg.CachedInputUSDPerMillionTokens != nil &&
		cfg.OutputUSDPerMillionTokens != nil
}

func (s *ContentModerationService) callModeration(ctx context.Context, cfg *ContentModerationConfig, input any, trackKeyLoad ...bool) (*moderationAPIResult, error) {
	attempts := cfg.activeRetryCount() + 1
	if attempts <= 0 {
		attempts = 1
	}
	if attempts > maxContentModerationRetryCount+1 {
		attempts = maxContentModerationRetryCount + 1
	}
	trackLoad := len(trackKeyLoad) > 0 && trackKeyLoad[0]
	aiChatAudit := cfg != nil && cfg.AuditProvider == ContentModerationProviderAIChat
	triedKeyHashes := make(map[string]struct{}, min(attempts, len(cfg.apiKeys())))
	cacheKey := ""
	if cfg.AuditProvider == ContentModerationProviderAIChat && cfg.AIChat.CacheEnabled && s.contentModerationAIResultCacheAllowed(ctx, cfg) {
		if content := aiChatTextFromModerationInput(input); content != "" {
			cacheKey = contentModerationAIResultCacheKey(cfg, content)
			if resultCache, ok := s.hashCache.(ContentModerationResultCache); ok {
				if raw, hit, err := resultCache.GetContentModerationResult(ctx, cacheKey); err != nil {
					slog.Warn("content_moderation.ai_cache_get_failed", "error", err)
				} else if hit {
					var cached moderationAPIResult
					if json.Unmarshal(raw, &cached) == nil {
						cached.ResultCacheHit = true
						cached.Usage = nil
						cached.AuditKeyHash = ""
						return &cached, nil
					}
				}
			}
		}
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return nil, fmt.Errorf("audit deadline exhausted after attempt %d: %w", attempt, err)
			}
			return nil, err
		}
		var key string
		var ok bool
		if aiChatAudit {
			key, ok = s.nextUsableAPIKeyExcluding(cfg, triedKeyHashes)
			if !ok {
				// Preserve the configured retry semantics after every distinct usable
				// key has received one attempt.
				key, ok = s.nextUsableAPIKey(cfg)
			}
		} else {
			key, ok = s.nextUsableAPIKey(cfg)
		}
		if !ok {
			lastErr = errors.New("no moderation api key available")
			break
		}
		if aiChatAudit {
			triedKeyHashes[moderationAPIKeyHash(key)] = struct{}{}
		}
		if trackLoad {
			s.beginModerationAPIKeyCall(key)
		}
		start := time.Now()
		httpStatus := 0
		nextWait := time.Duration(100*(attempt+1)) * time.Millisecond
		hasNextUsableKey := aiChatAudit && attempt < attempts-1 && s.hasUsableAPIKeyExcluding(cfg, triedKeyHashes)
		attemptCtx, cancelAttempt := aiChatModerationAttemptContext(ctx, nextWait, hasNextUsableKey)
		result, err := s.callModerationOnceWithInput(attemptCtx, cfg, key, input, &httpStatus)
		cancelAttempt()
		latency := int(time.Since(start).Milliseconds())
		if err == nil {
			if trackLoad {
				s.finishModerationAPIKeyCall(key, latency, true)
			}
			s.markAPIKeySuccess(key, latency, httpStatus)
			if cacheKey != "" && result != nil && !result.ReviewIncomplete {
				if resultCache, ok := s.hashCache.(ContentModerationResultCache); ok {
					if raw, marshalErr := json.Marshal(result); marshalErr == nil {
						ttl := time.Duration(cfg.AIChat.CacheTTLSeconds) * time.Second
						cacheKeys := []string{cacheKey}
						if alias := strings.TrimSpace(cfg.AIChat.cacheKeyAlias); alias != "" && alias != cacheKey {
							cacheKeys = append(cacheKeys, alias)
						}
						for _, resultCacheKey := range cacheKeys {
							if cacheErr := resultCache.SetContentModerationResult(ctx, resultCacheKey, raw, ttl); cacheErr != nil {
								slog.Warn("content_moderation.ai_cache_set_failed", "error", cacheErr)
							}
						}
					}
				}
			}
			return result, nil
		}
		if trackLoad {
			s.finishModerationAPIKeyCall(key, latency, false)
		}
		s.markAPIKeyError(key, err.Error(), latency, httpStatus)
		lastErr = err
		if httpStatus == http.StatusBadRequest || errors.Is(err, context.Canceled) {
			break
		}
		if ctx.Err() != nil || (!aiChatAudit && errors.Is(err, context.DeadlineExceeded)) {
			break
		}
		if attempt == attempts-1 {
			break
		}
		wait := nextWait
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 250*time.Millisecond {
				break
			}
			if wait > remaining-200*time.Millisecond {
				wait = remaining - 200*time.Millisecond
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, lastErr
}

func aiChatModerationAttemptBudget(remaining time.Duration, nextBackoff time.Duration, hasNextUsableKey bool) time.Duration {
	if remaining <= 0 {
		return 0
	}
	if !hasNextUsableKey {
		return remaining
	}
	if nextBackoff < 0 {
		nextBackoff = 0
	}
	maxReserve := remaining - nextBackoff - aiChatAPIKeyBudgetGuard - minimumAIChatAPIKeyBudget
	if maxReserve < minimumAIChatAPIKeyBudget {
		return remaining
	}
	nextKeyReserve := min(preferredAIChatNextAPIKeyBudget, maxReserve)
	return remaining - nextBackoff - aiChatAPIKeyBudgetGuard - nextKeyReserve
}

func aiChatModerationAttemptContext(ctx context.Context, nextBackoff time.Duration, hasNextUsableKey bool) (context.Context, context.CancelFunc) {
	noop := func() {}
	if ctx == nil || !hasNextUsableKey {
		return ctx, noop
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return ctx, noop
	}
	remaining := time.Until(deadline)
	budget := aiChatModerationAttemptBudget(remaining, nextBackoff, true)
	if budget <= 0 || budget >= remaining {
		return ctx, noop
	}
	return context.WithTimeout(ctx, budget)
}

func (s *ContentModerationService) callModerationOnceWithInput(ctx context.Context, cfg *ContentModerationConfig, apiKey string, input any, httpStatus *int) (*moderationAPIResult, error) {
	if cfg != nil && cfg.AuditProvider == ContentModerationProviderAIChat {
		return s.callAIChatAuditOnce(ctx, cfg, apiKey, input, httpStatus)
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	endpoint, err := url.JoinPath(base, "/v1/moderations")
	if err != nil {
		return nil, err
	}
	payload := moderationAPIRequest{
		Model: cfg.Model,
		Input: input,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client, err := s.moderationHTTPClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if httpStatus != nil {
		*httpStatus = resp.StatusCode
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("moderation api status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out moderationAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Results) == 0 {
		return nil, errors.New("moderation api returned empty results")
	}
	return &out.Results[0], nil
}

// CUSTOM(VOTE-AI-AI-AUDIT): adapt an OpenAI-compatible chat model to the normalized moderation result.
func (s *ContentModerationService) callAIChatAuditOnce(ctx context.Context, cfg *ContentModerationConfig, apiKey string, input any, httpStatus *int) (*moderationAPIResult, error) {
	content := aiChatTextFromModerationInput(input)
	if content == "" {
		return nil, errors.New("AI chat audit currently requires text input")
	}
	timeout := time.Duration(cfg.activeTimeoutMS()) * time.Millisecond
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client, err := s.moderationHTTPClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	adapterConfig := voteaimoderation.Config{
		BaseURL:                  cfg.AIChat.BaseURL,
		Model:                    cfg.AIChat.Model,
		SystemPrompt:             cfg.AIChat.SystemPrompt,
		MaxInputChars:            cfg.AIChat.MaxInputChars,
		FastInputChars:           cfg.AIChat.FastInputChars,
		FallbackInputChars:       cfg.AIChat.FallbackInputChars,
		FastMaxOutputTokens:      cfg.AIChat.FastMaxOutputTokens,
		FullMaxOutputTokens:      cfg.AIChat.FullMaxOutputTokens,
		MaxReviewMaxOutputTokens: cfg.AIChat.MaxReviewMaxOutputTokens,
		StageOutputLimitsEnabled: cfg.AIChat.IncrementalAuditEnabled,
		EscalationThreshold:      cfg.AIChat.ConfidenceThreshold,
		ExistingRiskScore:        cfg.AIChat.existingRiskScore,
		ThinkingMode:             cfg.AIChat.ThinkingMode,
		ReasoningEffort:          cfg.AIChat.ReasoningEffort,
	}
	var result *voteaimoderation.Result
	if stage := voteaimoderation.ReviewStage(strings.TrimSpace(cfg.AIChat.auditStage)); stage != "" {
		result, err = voteaimoderation.AuditStage(reqCtx, client, adapterConfig, apiKey, content, stage, httpStatus)
	} else {
		result, err = voteaimoderation.Audit(reqCtx, client, adapterConfig, apiKey, content, httpStatus)
	}
	if err != nil {
		return nil, err
	}
	converted := moderationAPIResultFromAIChat(result)
	converted.AuditKeyHash = moderationAPIKeyHash(apiKey)
	return converted, nil
}

func moderationAPIResultFromAIChat(result *voteaimoderation.Result) *moderationAPIResult {
	if result == nil {
		return &moderationAPIResult{}
	}
	scores := map[string]float64{"ai_risk": result.RiskScore}
	for _, category := range result.Categories {
		scores[category] = result.RiskScore
	}
	return &moderationAPIResult{
		Flagged:          result.Flagged,
		CategoryScores:   scores,
		Categories:       append([]string{}, result.Categories...),
		Signals:          result.Signals,
		Reason:           result.Reason,
		ReviewIncomplete: result.ReviewIncomplete,
		ReviewError:      result.ReviewError,
		Usage:            result.Usage,
		InputChars:       result.InputChars,
		Stage:            result.Stage,
	}
}

func contentModerationAIResultCacheKey(cfg *ContentModerationConfig, input any) string {
	if cfg == nil || cfg.AuditProvider != ContentModerationProviderAIChat || !cfg.AIChat.CacheEnabled {
		return ""
	}
	content := aiChatTextFromModerationInput(input)
	if content == "" {
		return ""
	}
	return voteaimoderation.CacheKey(
		"result-cache:v4",
		cfg.AIChat.BaseURL,
		cfg.AIChat.Model,
		cfg.AIChat.SystemPrompt,
		content,
		cfg.AIChat.ThinkingMode,
		cfg.AIChat.ReasoningEffort,
		fmt.Sprintf("max=%d", cfg.AIChat.MaxInputChars),
		fmt.Sprintf("fast=%d", cfg.AIChat.FastInputChars),
		fmt.Sprintf("fallback=%d", cfg.AIChat.FallbackInputChars),
		fmt.Sprintf("escalate=%.4f", cfg.AIChat.ConfidenceThreshold),
		fmt.Sprintf("existing=%.4f", cfg.AIChat.existingRiskScore),
		"stage="+defaultContentModerationString(strings.TrimSpace(cfg.AIChat.auditStage), "legacy"),
		"policy="+contentModerationAuditPolicyVersion(cfg),
		"risk="+strings.TrimSpace(cfg.AIChat.riskStateDigest),
		fmt.Sprintf("fast_output=%d", cfg.AIChat.FastMaxOutputTokens),
		fmt.Sprintf("full_output=%d", cfg.AIChat.FullMaxOutputTokens),
		fmt.Sprintf("max_output=%d", cfg.AIChat.MaxReviewMaxOutputTokens),
	)
}

func (s *ContentModerationService) contentModerationAIResultCacheAllowed(ctx context.Context, cfg *ContentModerationConfig) bool {
	if s == nil || cfg == nil || strings.TrimSpace(cfg.AIChat.inputHash) == "" {
		return true
	}
	lifecycle, ok := s.hashCache.(ContentModerationFlaggedHashLifecycleStore)
	if !ok {
		return true
	}
	suppressed, err := lifecycle.IsFlaggedInputHashSuppressed(ctx, cfg.AIChat.inputHash)
	if err != nil {
		// An administrator suppression is a correctness fence. If Redis cannot
		// confirm it, bypass the old result cache and obtain a fresh verdict.
		slog.Warn("content_moderation.flagged_hash_suppression_check_failed", "error", err)
		return false
	}
	return !suppressed
}

func (s *ContentModerationService) contentModerationCachedVerdictAllowed(ctx context.Context, inputHash string) bool {
	allowed, err := s.contentModerationHashPromotionAllowed(ctx, inputHash)
	if err != nil {
		// Suppression is an administrator correctness fence. When its state cannot
		// be confirmed, bypass cached verdicts and obtain a fresh provider result.
		slog.Warn("content_moderation.flagged_hash_suppression_check_failed", "error", err)
		return false
	}
	return allowed
}

func aiChatTextFromModerationInput(input any) string {
	switch value := input.(type) {
	case string:
		return strings.TrimSpace(value)
	case []moderationAPIInputPart:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if strings.TrimSpace(item.Text) != "" {
				parts = append(parts, strings.TrimSpace(item.Text))
			}
		}
		return strings.Join(parts, "\n")
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(raw))
	}
}

// moderationProxyURLCacheEntry 缓存 proxy_id 到代理 URL 的解析结果，
// 避免审计热路径上每次调用都查询数据库。
type moderationProxyURLCacheEntry struct {
	proxyID   int64
	url       string
	expiresAt time.Time
}

const contentModerationProxyURLCacheTTL = time.Minute

// moderationHTTPClient 返回本次审计调用应使用的 HTTP 客户端。
// 未配置代理时沿用默认客户端；配置了代理时通过共享客户端池构建，
// 代理解析/构建失败直接返回错误，绝不回退直连（避免 IP 关联风险）。
func (s *ContentModerationService) moderationHTTPClient(ctx context.Context, cfg *ContentModerationConfig) (*http.Client, error) {
	proxyID := cfg.activeProxyID()
	if cfg == nil || proxyID == nil {
		if s.httpClient == nil {
			return http.DefaultClient, nil
		}
		return s.httpClient, nil
	}
	proxyURL, err := s.resolveModerationProxyURL(ctx, *proxyID)
	if err != nil {
		return nil, err
	}
	client, err := httpclient.GetClient(httpclient.Options{ProxyURL: proxyURL})
	if err != nil {
		return nil, fmt.Errorf("build moderation proxy client: %w", err)
	}
	return client, nil
}

func (s *ContentModerationService) resolveModerationProxyURL(ctx context.Context, proxyID int64) (string, error) {
	now := time.Now()
	prev := s.moderationProxyCache.Load()
	if prev != nil && prev.proxyID == proxyID && now.Before(prev.expiresAt) {
		return prev.url, nil
	}
	if s.proxyRepo == nil {
		return "", errors.New("moderation proxy repository unavailable")
	}
	px, err := s.proxyRepo.GetByID(ctx, proxyID)
	if err != nil {
		return "", fmt.Errorf("resolve moderation proxy %d: %w", proxyID, err)
	}
	if !px.IsActive() || px.IsExpired(now) {
		slog.Warn("content_moderation.proxy_not_active",
			"proxy_id", proxyID,
			"proxy_name", px.Name,
			"status", px.Status,
			"expired", px.IsExpired(now))
	}
	proxyURL := px.URL()
	if prev == nil || prev.proxyID != proxyID || prev.url != proxyURL {
		// 不打印完整 URL（可能含认证信息），仅记录可定位的地址。
		slog.Info("content_moderation.proxy_enabled",
			"proxy_id", proxyID,
			"proxy_name", px.Name,
			"proxy_addr", fmt.Sprintf("%s://%s:%d", px.Protocol, px.Host, px.Port))
	}
	s.moderationProxyCache.Store(&moderationProxyURLCacheEntry{
		proxyID:   proxyID,
		url:       proxyURL,
		expiresAt: now.Add(contentModerationProxyURLCacheTTL),
	})
	return proxyURL, nil
}

func (s *ContentModerationService) buildLog(input ContentModerationCheckInput, cfg *ContentModerationConfig, action string, flagged bool, highestCategory string, highestScore float64, scores map[string]float64, text string, latency *int, queueDelay *int, errText string) *ContentModerationLog {
	var userID *int64
	if input.UserID > 0 {
		userID = &input.UserID
	}
	var apiKeyID *int64
	if input.APIKeyID > 0 {
		apiKeyID = &input.APIKeyID
	}
	auditStatus, auditCode, auditRetryable := contentModerationAuditMetadata(action, errText)
	return &ContentModerationLog{
		RequestID:          input.RequestID,
		SessionID:          strings.TrimSpace(input.SessionID),
		SessionSource:      normalizeContentModerationSessionSource(input.SessionSource, input.SessionID),
		UserID:             userID,
		UserEmail:          input.UserEmail,
		APIKeyID:           apiKeyID,
		APIKeyName:         input.APIKeyName,
		GroupID:            cloneInt64Ptr(input.GroupID),
		GroupName:          input.GroupName,
		Endpoint:           input.Endpoint,
		Provider:           input.Provider,
		Model:              input.Model,
		Mode:               cfg.Mode,
		Action:             action,
		Flagged:            flagged,
		HighestCategory:    highestCategory,
		HighestScore:       highestScore,
		CategoryScores:     cloneFloatMap(scores),
		ThresholdSnapshot:  cloneFloatMap(cfg.activeThresholds()),
		InputExcerpt:       trimRunes(redactContentModerationSecrets(text), maxModerationExcerptRunes),
		UpstreamLatencyMS:  latency,
		QueueDelayMS:       queueDelay,
		Error:              errText,
		AuditStatus:        auditStatus,
		AuditCode:          auditCode,
		AuditRetryable:     auditRetryable,
		SideEffectStatus:   ContentModerationSideEffectStatusNotApplicable,
		NotificationStatus: ContentModerationNotificationStatusNotRequired,
	}
}

func contentModerationAuditMetadata(action, errText string) (status, code string, retryable bool) {
	code = strings.TrimSpace(strings.SplitN(strings.TrimSpace(errText), ":", 2)[0])
	if code == "" {
		code = action
	}
	switch {
	case action == ContentModerationActionSkip:
		status = ContentModerationAuditStatusSkipped
	case code == "audit_review_incomplete":
		status = ContentModerationAuditStatusIncomplete
	case action == ContentModerationActionError || action == ContentModerationActionUnavailable:
		status = ContentModerationAuditStatusError
	default:
		status = ContentModerationAuditStatusSuccess
	}
	switch code {
	case "audit_timeout", "audit_temporary_failure", "audit_review_incomplete":
		retryable = true
	}
	return status, code, retryable
}

type contentModerationSideEffectTracker struct {
	succeeded int
	failed    int
	errors    []string
}

func (t *contentModerationSideEffectTracker) success() {
	if t != nil {
		t.succeeded++
	}
}

func (t *contentModerationSideEffectTracker) failure(label string, err error) {
	if t == nil {
		return
	}
	t.failed++
	message := strings.TrimSpace(label)
	if err != nil {
		if message != "" {
			message += ": "
		}
		message += err.Error()
	}
	if message != "" {
		t.errors = append(t.errors, message)
	}
}

func (t *contentModerationSideEffectTracker) notification(status string, sent bool, err error) {
	if err != nil {
		t.failure("notification", err)
	}
	if sent || status == ContentModerationNotificationStatusSent || status == ContentModerationNotificationStatusDeduplicated {
		t.success()
		return
	}
	if status == ContentModerationNotificationStatusFailed && err == nil {
		t.failure("notification", errors.New("notification failed without an error"))
	}
}

func (t *contentModerationSideEffectTracker) finalize(log *ContentModerationLog, applicable bool) {
	if t == nil || log == nil {
		return
	}
	log.SideEffectError = trimRunes(strings.Join(t.errors, "; "), 1000)
	if !applicable {
		log.SideEffectStatus = ContentModerationSideEffectStatusNotApplicable
		return
	}
	switch {
	case t.failed == 0:
		log.SideEffectStatus = ContentModerationSideEffectStatusCompleted
	case t.succeeded == 0:
		log.SideEffectStatus = ContentModerationSideEffectStatusFailed
	default:
		log.SideEffectStatus = ContentModerationSideEffectStatusPartial
	}
}

func (s *ContentModerationService) persistContentModerationLog(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog, hashText string, recordHash bool, applySideEffects bool) {
	if s == nil || s.repo == nil || log == nil {
		return
	}
	log.InputHash = strings.TrimSpace(hashText)
	log.AuditDetails.InputHash = log.InputHash
	if (recordHash || applySideEffects) && log.InputHash != "" {
		allowed, err := s.contentModerationHashPromotionAllowed(ctx, hashText)
		if err != nil {
			slog.Warn("content_moderation.hash_promotion_check_failed", "user_id", contentModerationEmailUserID(log), "error", err)
			recordHash = false
			applySideEffects = false
			log.AuditDetails.HashState = "candidate"
			log.AuditDetails.HashPromotionReason = "promotion_state_unavailable"
		} else if !allowed {
			recordHash = false
			applySideEffects = false
			markContentModerationHashPromotionSuppressed(log)
		} else if recordHash {
			log.AuditDetails.HashState = "confirmed"
		}
	}
	if applySideEffects {
		log.SideEffectStatus = ContentModerationSideEffectStatusPending
		log.NotificationStatus = ContentModerationNotificationStatusPending
	} else {
		log.SideEffectStatus = ContentModerationSideEffectStatusNotApplicable
		log.NotificationStatus = ContentModerationNotificationStatusNotRequired
	}
	lifecycleRepo, lifecycleAvailable := s.repo.(ContentModerationLifecycleRepository)
	if (applySideEffects || recordHash) && !lifecycleAvailable {
		log.SideEffectStatus = ContentModerationSideEffectStatusFailed
		log.NotificationStatus = ContentModerationNotificationStatusNotRequired
		log.SideEffectError = "content moderation lifecycle repository is unavailable"
		if err := s.repo.CreateLog(ctx, log); err != nil {
			slog.Error("content_moderation.create_failed_lifecycle_log_failed", "user_id", contentModerationEmailUserID(log), "error", err)
		} else {
			slog.Error("content_moderation.lifecycle_repository_unavailable", "log_id", log.ID, "user_id", contentModerationEmailUserID(log))
		}
		return
	}
	if err := s.repo.CreateLog(ctx, log); err != nil {
		slog.Warn("content_moderation.create_log_failed", "user_id", contentModerationEmailUserID(log), "endpoint", log.Endpoint, "action", log.Action, "error", err)
		return
	}

	tracker := &contentModerationSideEffectTracker{}
	if recordHash {
		if s.hashCache == nil {
			tracker.failure("record_hash", errors.New("content moderation hash cache is unavailable"))
		} else if lifecycle, ok := s.hashCache.(ContentModerationFlaggedHashLifecycleStore); ok {
			allowed, err := lifecycle.RecordFlaggedInputHashIfAllowed(ctx, hashText)
			if err != nil {
				slog.Warn("content_moderation.record_hash_failed", "user_id", contentModerationEmailUserID(log), "endpoint", log.Endpoint, "error", err)
				tracker.failure("record_hash", err)
				applySideEffects = false
			} else if !allowed {
				recordHash = false
				applySideEffects = false
				markContentModerationHashPromotionSuppressed(log)
				tracker.success()
			} else {
				tracker.success()
			}
		} else if err := s.hashCache.RecordFlaggedInputHash(ctx, hashText); err != nil {
			slog.Warn("content_moderation.record_hash_failed", "user_id", contentModerationEmailUserID(log), "endpoint", log.Endpoint, "error", err)
			tracker.failure("record_hash", err)
			applySideEffects = false
		} else {
			tracker.success()
		}
	}
	notificationStatus := ContentModerationNotificationStatusNotRequired
	if applySideEffects {
		banOutcome, err := s.applyFlaggedAccountSideEffects(ctx, cfg, log)
		if err != nil {
			tracker.failure("account", err)
		} else {
			tracker.success()
		}
		var notificationErr error
		notificationStatus, log.EmailSent, notificationErr = s.sendFlaggedNotificationSideEffectsForBanOutcome(ctx, cfg, log, banOutcome)
		tracker.notification(notificationStatus, log.EmailSent, notificationErr)
	}
	log.NotificationStatus = notificationStatus
	tracker.finalize(log, applySideEffects || recordHash)
	patch := ContentModerationLogEffectsPatch{
		ViolationCount:     log.ViolationCount,
		AutoBanned:         log.AutoBanned,
		EmailSent:          log.EmailSent,
		SideEffectStatus:   log.SideEffectStatus,
		NotificationStatus: log.NotificationStatus,
		SideEffectError:    log.SideEffectError,
		AuditDetails:       &log.AuditDetails,
	}
	if lifecycleRepo != nil {
		if err := lifecycleRepo.UpdateLogEffects(ctx, log.ID, patch); err != nil {
			slog.Warn("content_moderation.update_log_effects_failed", "log_id", log.ID, "error", err)
		}
	}
}

func (s *ContentModerationService) contentModerationHashPromotionAllowed(ctx context.Context, inputHash string) (bool, error) {
	if s == nil || s.hashCache == nil || strings.TrimSpace(inputHash) == "" {
		return true, nil
	}
	lifecycle, ok := s.hashCache.(ContentModerationFlaggedHashLifecycleStore)
	if !ok {
		return true, nil
	}
	suppressed, err := lifecycle.IsFlaggedInputHashSuppressed(ctx, inputHash)
	return !suppressed, err
}

func markContentModerationHashPromotionSuppressed(log *ContentModerationLog) {
	if log == nil {
		return
	}
	log.AuditDetails.HashState = "suppressed"
	log.AuditDetails.HashPromotionReason = "administrator_suppression"
}

func (s *ContentModerationService) applyFlaggedAccountSideEffects(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog) (string, error) {
	if s == nil || cfg == nil || log == nil || !log.Flagged || log.UserID == nil || *log.UserID <= 0 {
		return ContentModerationBanOutcomeIneligible, nil
	}
	count := 1
	if s.repo != nil && cfg.ViolationWindowHours > 0 {
		since := time.Now().Add(-time.Duration(cfg.ViolationWindowHours) * time.Hour)
		n, err := s.repo.CountFlaggedByUserSince(ctx, *log.UserID, since, cfg.CyberPolicyExcludeFromBanCount)
		if err != nil {
			return ContentModerationBanOutcomeIneligible, fmt.Errorf("count violations: %w", err)
		}
		count = max(1, n)
	}
	log.ViolationCount = count
	if !cfg.AutoBanEnabled || cfg.BanThreshold <= 0 || count < cfg.BanThreshold || s.repo == nil {
		return ContentModerationBanOutcomeIneligible, nil
	}
	lifecycleRepo, ok := s.repo.(ContentModerationLifecycleRepository)
	if !ok {
		return ContentModerationBanOutcomeIneligible, errors.New("content moderation lifecycle repository is unavailable")
	}
	outcome, err := lifecycleRepo.TryApplyModerationOwnedBan(ctx, *log.UserID, log.ID, time.Now().UTC())
	if err != nil {
		return ContentModerationBanOutcomeIneligible, fmt.Errorf("apply moderation-owned ban: %w", err)
	}
	switch outcome {
	case ContentModerationBanOutcomeApplied:
		log.AutoBanned = true
		log.ModerationBanActive = true
		log.UserStatus = StatusDisabled
		if s.authCacheInvalidator != nil {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, *log.UserID)
		}
		return ContentModerationBanOutcomeApplied, nil
	case ContentModerationBanOutcomeAlreadyOwned:
		log.ModerationBanActive = true
		log.UserStatus = StatusDisabled
		return ContentModerationBanOutcomeAlreadyOwned, nil
	case ContentModerationBanOutcomeIneligible:
		return ContentModerationBanOutcomeIneligible, nil
	default:
		return ContentModerationBanOutcomeIneligible, fmt.Errorf("unknown moderation ban outcome %q", outcome)
	}
}

func (s *ContentModerationService) sendFlaggedNotificationSideEffects(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog, autoBanJustApplied bool) (string, bool, error) {
	banOutcome := ContentModerationBanOutcomeIneligible
	if autoBanJustApplied {
		banOutcome = ContentModerationBanOutcomeApplied
	}
	return s.sendFlaggedNotificationSideEffectsForBanOutcome(ctx, cfg, log, banOutcome)
}

func (s *ContentModerationService) sendFlaggedNotificationSideEffectsForBanOutcome(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog, banOutcome string) (string, bool, error) {
	if s == nil || cfg == nil || log == nil || !log.Flagged {
		return ContentModerationNotificationStatusNotRequired, false, nil
	}
	if banOutcome == ContentModerationBanOutcomeAlreadyOwned {
		return ContentModerationNotificationStatusNotRequired, false, nil
	}
	if s.emailService == nil || strings.TrimSpace(log.UserEmail) == "" {
		return ContentModerationNotificationStatusNotRequired, false, nil
	}
	if banOutcome == ContentModerationBanOutcomeApplied {
		if err := s.sendAccountDisabledEmail(ctx, cfg, log); err != nil {
			slog.Warn("content_moderation.ban_email_failed", "user_id", contentModerationEmailUserID(log), "recipient_hash", notificationEmailHash(log.UserEmail), "error", err)
			return ContentModerationNotificationStatusFailed, false, err
		}
		return ContentModerationNotificationStatusSent, true, nil
	}
	if !cfg.EmailOnHit {
		return ContentModerationNotificationStatusNotRequired, false, nil
	}
	decision := s.reserveContentModerationEmailForLog(ctx, log)
	dedupeErr := contentModerationEmailDedupeDecisionError(decision)
	if dedupeErr != nil {
		slog.Warn("content_moderation.email_dedupe_failed_open", "user_id", contentModerationEmailUserID(log), "scope", decision.Scope, "error", decision.Error)
	}
	if !decision.ShouldSend {
		return ContentModerationNotificationStatusDeduplicated, false, nil
	}
	if err := s.sendViolationEmail(ctx, cfg, log); err != nil {
		slog.Warn("content_moderation.email_failed", "user_id", contentModerationEmailUserID(log), "recipient_hash", notificationEmailHash(log.UserEmail), "error", err)
		releaseErr := s.releaseContentModerationEmailReservation(ctx, decision)
		if releaseErr != nil {
			releaseErr = fmt.Errorf("release content moderation email dedupe reservation: %w", releaseErr)
			slog.Warn("content_moderation.email_dedupe_release_failed", "user_id", contentModerationEmailUserID(log), "scope", decision.Scope, "error", releaseErr)
		}
		return ContentModerationNotificationStatusFailed, false, errors.Join(dedupeErr, err, releaseErr)
	}
	return ContentModerationNotificationStatusSent, true, dedupeErr
}

func (s *ContentModerationService) sendViolationEmail(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog) error {
	siteName := s.siteName(ctx)
	if s.emailService.notificationEmailService != nil {
		if err := s.emailService.notificationEmailService.Send(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventContentModerationViolation,
			RecipientEmail: log.UserEmail,
			RecipientName:  emailRecipientName(log.UserEmail),
			UserID:         contentModerationEmailUserID(log),
			SourceType:     "content_moderation",
			SourceID:       contentModerationEmailSourceID(log),
			Variables:      contentModerationEmailVariables(log, cfg),
		}); err == nil {
			return nil
		} else {
			if !shouldFallbackNotificationEmail(err) {
				return err
			}
			slog.Warn("template content moderation violation email failed; falling back to built-in body", "log_id", log.ID, "recipient_hash", notificationEmailHash(log.UserEmail), "err", err.Error())
		}
	}
	subject := fmt.Sprintf("[%s] 账户风控提醒 / Risk Control Notice", sanitizeEmailHeader(siteName))
	body := buildContentModerationViolationEmailBody(siteName, log, cfg)
	return s.emailService.SendEmail(ctx, log.UserEmail, subject, body)
}

func (s *ContentModerationService) sendAccountDisabledEmail(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog) error {
	siteName := s.siteName(ctx)
	if s.emailService.notificationEmailService != nil {
		if err := s.emailService.notificationEmailService.Send(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventContentModerationDisabled,
			RecipientEmail: log.UserEmail,
			RecipientName:  emailRecipientName(log.UserEmail),
			UserID:         contentModerationEmailUserID(log),
			SourceType:     "content_moderation",
			SourceID:       contentModerationEmailSourceID(log),
			Variables:      contentModerationEmailVariables(log, cfg),
		}); err == nil {
			return nil
		} else {
			if !shouldFallbackNotificationEmail(err) {
				return err
			}
			slog.Warn("template content moderation disabled email failed; falling back to built-in body", "log_id", log.ID, "recipient_hash", notificationEmailHash(log.UserEmail), "err", err.Error())
		}
	}
	subject := fmt.Sprintf("[%s] 账户已被禁用 / Account Disabled", sanitizeEmailHeader(siteName))
	body := buildContentModerationAccountDisabledEmailBody(siteName, log, cfg)
	return s.emailService.SendEmail(ctx, log.UserEmail, subject, body)
}

func contentModerationEmailUserID(log *ContentModerationLog) int64 {
	if log == nil || log.UserID == nil {
		return 0
	}
	return *log.UserID
}

func contentModerationEmailSourceID(log *ContentModerationLog) string {
	if log == nil || log.ID <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", log.ID)
}

func contentModerationEmailVariables(log *ContentModerationLog, cfg *ContentModerationConfig) map[string]string {
	variables := map[string]string{
		"triggered_at":        time.Now().UTC().Format(time.RFC3339),
		"group_name":          "-",
		"moderation_category": "-",
		"moderation_score":    "0.000",
		"violation_count":     "0",
		"ban_threshold":       "0",
	}
	if log != nil {
		if !log.CreatedAt.IsZero() {
			variables["triggered_at"] = log.CreatedAt.UTC().Format(time.RFC3339)
		}
		if strings.TrimSpace(log.GroupName) != "" {
			variables["group_name"] = strings.TrimSpace(log.GroupName)
		}
		if strings.TrimSpace(log.HighestCategory) != "" {
			variables["moderation_category"] = strings.TrimSpace(log.HighestCategory)
		}
		variables["moderation_score"] = fmt.Sprintf("%.3f", log.HighestScore)
		variables["violation_count"] = fmt.Sprintf("%d", log.ViolationCount)
	}
	if cfg != nil {
		variables["ban_threshold"] = fmt.Sprintf("%d", cfg.BanThreshold)
	}
	return variables
}

func (s *ContentModerationService) siteName(ctx context.Context) string {
	if s == nil || s.settingRepo == nil {
		return "Sub2API"
	}
	name, err := s.settingRepo.GetValue(ctx, SettingKeySiteName)
	if err != nil || strings.TrimSpace(name) == "" {
		return "Sub2API"
	}
	return strings.TrimSpace(name)
}

func defaultContentModerationConfig() *ContentModerationConfig {
	return &ContentModerationConfig{
		Enabled:       false,
		Mode:          ContentModerationModePreBlock,
		AuditProvider: ContentModerationProviderOpenAIModerations,
		AIChat: ContentModerationAIChatConfig{
			BaseURL:                         defaultAIChatBaseURL,
			Model:                           defaultAIChatModel,
			APIKeys:                         []string{},
			TimeoutMS:                       defaultAIChatTimeoutMS,
			SynchronousBudgetMS:             defaultAIChatSynchronousBudgetMS,
			RetryCount:                      1,
			ConfidenceThreshold:             defaultAIChatConfidenceThreshold,
			CacheEnabled:                    true,
			CacheTTLSeconds:                 defaultAIChatCacheTTLSeconds,
			SystemPrompt:                    voteaimoderation.DefaultSystemPrompt,
			FailurePolicy:                   ContentModerationFailurePolicyAllow,
			MaxInputChars:                   defaultAIChatMaxInputChars,
			FastInputChars:                  defaultAIChatFastInputChars,
			FallbackInputChars:              defaultAIChatFallbackInputChars,
			ThinkingMode:                    defaultAIChatThinkingMode,
			ReasoningEffort:                 defaultAIChatReasoningEffort,
			RiskLevelsEnabled:               true,
			ObserveThreshold:                defaultAIChatObserveThreshold,
			SessionRiskEnabled:              true,
			SessionRiskTTLMinutes:           defaultAIChatSessionRiskTTLMinutes,
			SessionRiskHalfLifeMinutes:      defaultAIChatSessionRiskHalfLifeMinutes,
			SessionRiskBlockCooldownMinutes: defaultAIChatSessionRiskBlockCooldownMinutes,
			ActorRiskEnabled:                true,
			IncrementalAuditEnabled:         false,
			InputProvenanceV2Enabled:        true,
			DeterministicRiskV2Enabled:      true,
			RecentUserTurns:                 defaultAIChatRecentUserTurns,
			SummaryMaxChars:                 defaultAIChatSummaryMaxChars,
			FullReviewThreshold:             defaultAIChatFullReviewThreshold,
			FullReviewRiskDelta:             defaultAIChatFullReviewRiskDelta,
			PeriodicFullReviewTurns:         defaultAIChatPeriodicFullReviewTurns,
			FullReviewMaxInputChars:         defaultAIChatFullReviewMaxInputChars,
			FastMaxOutputTokens:             defaultAIChatFastMaxOutputTokens,
			FullMaxOutputTokens:             defaultAIChatFullMaxOutputTokens,
			MaxReviewMaxOutputTokens:        defaultAIChatMaxReviewMaxOutputTokens,
			AuditContextTTLMinutes:          defaultAIChatAuditContextTTLMinutes,
		},
		BaseURL:              defaultContentModerationBaseURL,
		Model:                defaultContentModerationModel,
		TimeoutMS:            defaultContentModerationTimeoutMS,
		SampleRate:           100,
		AllGroups:            true,
		GroupIDs:             []int64{},
		RecordNonHits:        false,
		Thresholds:           ContentModerationDefaultThresholds(),
		WorkerCount:          defaultContentModerationWorkerCount,
		QueueSize:            defaultContentModerationQueueSize,
		BlockStatus:          defaultContentModerationBlockHTTPStatus,
		BlockMessage:         defaultContentModerationBlockMessage,
		EmailOnHit:           true,
		AutoBanEnabled:       true,
		BanThreshold:         defaultContentModerationBanThreshold,
		ViolationWindowHours: defaultContentModerationViolationWindowHours,
		RetryCount:           defaultContentModerationRetryCount,
		HitRetentionDays:     defaultContentModerationHitRetentionDays,
		NonHitRetentionDays:  defaultContentModerationNonHitRetentionDays,
		PreHashCheckEnabled:  false,
		BlockedKeywords:      []string{},
		KeywordBlockingMode:  ContentModerationKeywordModeKeywordAndAPI,
		ModelFilter: ContentModerationModelFilter{
			Type:   ContentModerationModelFilterAll,
			Models: []string{},
		},
		UserFilter: ContentModerationUserFilter{
			Type:    ContentModerationScopeFilterAll,
			UserIDs: []int64{},
		},
		AccountFilter: ContentModerationAccountFilter{
			Type:       ContentModerationScopeFilterAll,
			AccountIDs: []int64{},
		},
		CyberPolicyExcludeFromBanCount: false,
	}
}

func cloneContentModerationConfig(cfg *ContentModerationConfig) *ContentModerationConfig {
	if cfg == nil {
		return nil
	}
	clone := *cfg
	clone.ProxyID = cloneInt64Ptr(cfg.ProxyID)
	clone.APIKeys = append([]string(nil), cfg.APIKeys...)
	clone.AIChat.ProxyID = cloneInt64Ptr(cfg.AIChat.ProxyID)
	clone.AIChat.APIKeys = append([]string(nil), cfg.AIChat.APIKeys...)
	clone.AIChat.UncachedInputUSDPerMillionTokens = cloneFloat64Ptr(cfg.AIChat.UncachedInputUSDPerMillionTokens)
	clone.AIChat.CachedInputUSDPerMillionTokens = cloneFloat64Ptr(cfg.AIChat.CachedInputUSDPerMillionTokens)
	clone.AIChat.OutputUSDPerMillionTokens = cloneFloat64Ptr(cfg.AIChat.OutputUSDPerMillionTokens)
	clone.GroupIDs = append([]int64(nil), cfg.GroupIDs...)
	clone.BlockedKeywords = append([]string(nil), cfg.BlockedKeywords...)
	clone.Thresholds = cloneFloatMap(cfg.Thresholds)
	clone.ModelFilter = ContentModerationModelFilter{
		Type:   cfg.ModelFilter.Type,
		Models: append([]string(nil), cfg.ModelFilter.Models...),
	}
	clone.UserFilter = ContentModerationUserFilter{
		Type:    cfg.UserFilter.Type,
		UserIDs: append([]int64(nil), cfg.UserFilter.UserIDs...),
	}
	clone.AccountFilter = ContentModerationAccountFilter{
		Type:       cfg.AccountFilter.Type,
		AccountIDs: append([]int64(nil), cfg.AccountFilter.AccountIDs...),
	}
	return &clone
}

func contentModerationRuntimeGeneration(
	cfg *ContentModerationConfig,
	engineFingerprintSignals []openaipkg.EngineFingerprintSignal,
) [sha256.Size]byte {
	if cfg == nil {
		return [sha256.Size]byte{}
	}
	normalized := cloneContentModerationConfig(cfg)
	normalized.normalize()
	rawConfig, configErr := json.Marshal(normalized)
	rawSignals, signalsErr := json.Marshal(engineFingerprintSignals)
	if configErr != nil || signalsErr != nil {
		return [sha256.Size]byte{}
	}
	payload := make([]byte, 0, len(rawConfig)+1+len(rawSignals))
	payload = append(payload, rawConfig...)
	payload = append(payload, 0)
	payload = append(payload, rawSignals...)
	return sha256.Sum256(payload)
}

func (s *ContentModerationService) contentModerationAsyncTaskGeneration(cfg *ContentModerationConfig) [sha256.Size]byte {
	signals := openaipkg.DefaultEngineFingerprintSignals
	if s != nil {
		if snapshot := s.runtimeSnapshot.Load(); snapshot != nil {
			signals = snapshot.engineFingerprintSignals
		}
	}
	return contentModerationRuntimeGeneration(cfg, signals)
}

func (cfg *ContentModerationConfig) normalize() {
	cfg.AuditProvider = normalizeContentModerationProvider(cfg.AuditProvider)
	if cfg.APIKey != "" {
		cfg.APIKeys = normalizeModerationAPIKeys(append(cfg.APIKeys, cfg.APIKey))
		cfg.APIKey = ""
	} else {
		cfg.APIKeys = normalizeModerationAPIKeys(cfg.APIKeys)
	}
	if cfg.Mode == "" {
		cfg.Mode = ContentModerationModePreBlock
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultContentModerationBaseURL
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.Model == "" {
		cfg.Model = defaultContentModerationModel
	}
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.normalizeAIChat()
	if cfg.ProxyID != nil && *cfg.ProxyID <= 0 {
		cfg.ProxyID = nil
	}
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = defaultContentModerationTimeoutMS
	}
	if cfg.TimeoutMS > maxContentModerationTimeoutMS {
		cfg.TimeoutMS = maxContentModerationTimeoutMS
	}
	if cfg.SampleRate < 0 {
		cfg.SampleRate = 0
	}
	if cfg.SampleRate > 100 {
		cfg.SampleRate = 100
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = defaultContentModerationWorkerCount
	}
	if cfg.WorkerCount > maxContentModerationWorkerCount {
		cfg.WorkerCount = maxContentModerationWorkerCount
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = defaultContentModerationQueueSize
	}
	if cfg.QueueSize > maxContentModerationQueueSize {
		cfg.QueueSize = maxContentModerationQueueSize
	}
	if strings.TrimSpace(cfg.BlockMessage) == "" {
		cfg.BlockMessage = defaultContentModerationBlockMessage
	}
	cfg.BlockMessage = strings.TrimSpace(cfg.BlockMessage)
	if cfg.BlockStatus <= 0 {
		cfg.BlockStatus = defaultContentModerationBlockHTTPStatus
	}
	if cfg.BanThreshold <= 0 {
		cfg.BanThreshold = defaultContentModerationBanThreshold
	}
	if cfg.ViolationWindowHours <= 0 {
		cfg.ViolationWindowHours = defaultContentModerationViolationWindowHours
	}
	if cfg.RetryCount < 0 {
		cfg.RetryCount = 0
	}
	if cfg.RetryCount > maxContentModerationRetryCount {
		cfg.RetryCount = maxContentModerationRetryCount
	}
	if cfg.HitRetentionDays <= 0 {
		cfg.HitRetentionDays = defaultContentModerationHitRetentionDays
	}
	if cfg.HitRetentionDays > maxContentModerationRetentionDays {
		cfg.HitRetentionDays = maxContentModerationRetentionDays
	}
	if cfg.NonHitRetentionDays <= 0 {
		cfg.NonHitRetentionDays = defaultContentModerationNonHitRetentionDays
	}
	if cfg.NonHitRetentionDays > maxContentModerationNonHitRetentionDays {
		cfg.NonHitRetentionDays = maxContentModerationNonHitRetentionDays
	}
	cfg.GroupIDs = normalizeInt64IDs(cfg.GroupIDs)
	cfg.Thresholds = mergeContentModerationThresholds(ContentModerationDefaultThresholds(), cfg.Thresholds)
	cfg.BlockedKeywords = normalizeBlockedKeywords(cfg.BlockedKeywords)
	cfg.KeywordBlockingMode = normalizeKeywordBlockingMode(cfg.KeywordBlockingMode)
	cfg.ModelFilter = normalizeContentModerationModelFilter(cfg.ModelFilter)
	cfg.UserFilter = normalizeContentModerationUserFilter(cfg.UserFilter)
	cfg.AccountFilter = normalizeContentModerationAccountFilter(cfg.AccountFilter)
}

func (cfg *ContentModerationConfig) includesGroup(groupID *int64) bool {
	if cfg.AllGroups {
		return true
	}
	if groupID == nil {
		return false
	}
	for _, id := range cfg.GroupIDs {
		if id == *groupID {
			return true
		}
	}
	return false
}

func (cfg *ContentModerationConfig) includesModel(model string) bool {
	if cfg == nil {
		return true
	}
	filter := normalizeContentModerationModelFilter(cfg.ModelFilter)
	switch filter.Type {
	case ContentModerationModelFilterInclude:
		return contentModerationModelListContains(filter.Models, model)
	case ContentModerationModelFilterExclude:
		return !contentModerationModelListContains(filter.Models, model)
	default:
		return true
	}
}

func (cfg *ContentModerationConfig) includesUser(userID int64) bool {
	if cfg == nil {
		return true
	}
	filter := normalizeContentModerationUserFilter(cfg.UserFilter)
	return contentModerationIDFilterIncludes(filter.Type, filter.UserIDs, userID)
}

func (cfg *ContentModerationConfig) includesAccount(accountID int64) bool {
	if cfg == nil {
		return true
	}
	filter := normalizeContentModerationAccountFilter(cfg.AccountFilter)
	return contentModerationIDFilterIncludes(filter.Type, filter.AccountIDs, accountID)
}

// ShouldAuditUser applies only the configured user scope. Other audit gates,
// such as the global feature switch, group and model scopes, remain owned by
// the security-audit coordinator and engines.
func (s *ContentModerationService) ShouldAuditUser(ctx context.Context, userID int64) (bool, string, error) {
	cfg, err := s.contentModerationScopeConfig(ctx)
	if err != nil {
		return true, "", err
	}
	if cfg.includesUser(userID) {
		return true, "", nil
	}
	return false, ContentModerationScopeReasonUserOutOfScope, nil
}

// RequiresAccountScopeResolution reports whether account selection must happen
// before the coordinator can decide whether to audit the request.
func (s *ContentModerationService) RequiresAccountScopeResolution(ctx context.Context) (bool, error) {
	cfg, err := s.contentModerationScopeConfig(ctx)
	if err != nil {
		return false, err
	}
	return normalizeContentModerationAccountFilter(cfg.AccountFilter).Type != ContentModerationScopeFilterAll, nil
}

// ShouldAuditAccount evaluates one selected upstream account. It is safe to
// call again after failover because the decision contains no request state.
func (s *ContentModerationService) ShouldAuditAccount(ctx context.Context, accountID int64) (bool, string, error) {
	cfg, err := s.contentModerationScopeConfig(ctx)
	if err != nil {
		return true, "", err
	}
	if cfg.includesAccount(accountID) {
		return true, "", nil
	}
	return false, ContentModerationScopeReasonAccountOutOfScope, nil
}

func (s *ContentModerationService) contentModerationScopeConfig(ctx context.Context) (*ContentModerationConfig, error) {
	if s == nil || s.settingRepo == nil {
		return defaultContentModerationConfig(), nil
	}
	snapshot, err := s.loadRuntimeSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if snapshot == nil || snapshot.config == nil {
		return defaultContentModerationConfig(), nil
	}
	return snapshot.config, nil
}

func contentModerationLogGroupID(groupID *int64) int64 {
	if groupID == nil {
		return 0
	}
	return *groupID
}

func (cfg *ContentModerationConfig) shouldSample(hashText string) bool {
	if cfg.SampleRate >= 100 {
		return true
	}
	if cfg.SampleRate <= 0 {
		return false
	}
	raw, err := hex.DecodeString(hashText)
	if err != nil || len(raw) < 2 {
		return true
	}
	return int(binary.BigEndian.Uint16(raw[:2])%100) < cfg.SampleRate
}

func (cfg *ContentModerationConfig) apiKeys() []string {
	if cfg == nil {
		return nil
	}
	if cfg.AuditProvider == ContentModerationProviderAIChat {
		return normalizeModerationAPIKeys(cfg.AIChat.APIKeys)
	}
	return normalizeModerationAPIKeys(cfg.APIKeys)
}

func (cfg *ContentModerationConfig) normalizeAIChat() {
	if strings.TrimSpace(cfg.AIChat.BaseURL) == "" {
		cfg.AIChat.BaseURL = defaultAIChatBaseURL
	}
	cfg.AIChat.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.AIChat.BaseURL), "/")
	if strings.TrimSpace(cfg.AIChat.Model) == "" {
		cfg.AIChat.Model = defaultAIChatModel
	}
	cfg.AIChat.Model = strings.TrimSpace(cfg.AIChat.Model)
	cfg.AIChat.APIKeys = normalizeModerationAPIKeys(cfg.AIChat.APIKeys)
	if cfg.AIChat.ProxyID != nil && *cfg.AIChat.ProxyID <= 0 {
		cfg.AIChat.ProxyID = nil
	}
	if cfg.AIChat.TimeoutMS <= 0 {
		cfg.AIChat.TimeoutMS = defaultAIChatTimeoutMS
	}
	if cfg.AIChat.TimeoutMS > maxContentModerationTimeoutMS {
		cfg.AIChat.TimeoutMS = maxContentModerationTimeoutMS
	}
	if cfg.AIChat.SynchronousBudgetMS < minAIChatSynchronousBudgetMS || cfg.AIChat.SynchronousBudgetMS > maxAIChatSynchronousBudgetMS {
		cfg.AIChat.SynchronousBudgetMS = defaultAIChatSynchronousBudgetMS
	}
	if cfg.AIChat.RetryCount < 0 {
		cfg.AIChat.RetryCount = 0
	}
	if cfg.AIChat.RetryCount > maxContentModerationRetryCount {
		cfg.AIChat.RetryCount = maxContentModerationRetryCount
	}
	if cfg.AIChat.ConfidenceThreshold <= 0 || cfg.AIChat.ConfidenceThreshold > 1 {
		cfg.AIChat.ConfidenceThreshold = defaultAIChatConfidenceThreshold
	}
	if cfg.AIChat.CacheTTLSeconds <= 0 {
		cfg.AIChat.CacheTTLSeconds = defaultAIChatCacheTTLSeconds
	}
	if cfg.AIChat.CacheTTLSeconds > maxAIChatCacheTTLSeconds {
		cfg.AIChat.CacheTTLSeconds = maxAIChatCacheTTLSeconds
	}
	cfg.AIChat.SystemPrompt = voteaimoderation.NormalizeSystemPrompt(cfg.AIChat.SystemPrompt)
	if cfg.AIChat.FailurePolicy != ContentModerationFailurePolicyBlock {
		cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyAllow
	}
	if cfg.AIChat.MaxInputChars < minAIChatMaxInputChars || cfg.AIChat.MaxInputChars > maxModerationInputRunes {
		cfg.AIChat.MaxInputChars = defaultAIChatMaxInputChars
	}
	if cfg.AIChat.FastInputChars <= 0 || cfg.AIChat.FastInputChars > cfg.AIChat.MaxInputChars {
		cfg.AIChat.FastInputChars = min(defaultAIChatFastInputChars, cfg.AIChat.MaxInputChars)
	}
	if cfg.AIChat.FallbackInputChars <= 0 || cfg.AIChat.FallbackInputChars > cfg.AIChat.FastInputChars {
		cfg.AIChat.FallbackInputChars = min(defaultAIChatFallbackInputChars, cfg.AIChat.FastInputChars)
	}
	if cfg.AIChat.ThinkingMode != "disabled" {
		cfg.AIChat.ThinkingMode = defaultAIChatThinkingMode
	}
	switch cfg.AIChat.ReasoningEffort {
	case "adaptive", "low", "high", "max":
	default:
		cfg.AIChat.ReasoningEffort = defaultAIChatReasoningEffort
	}
	if cfg.AIChat.ObserveThreshold <= 0 || cfg.AIChat.ObserveThreshold >= cfg.AIChat.ConfidenceThreshold {
		cfg.AIChat.ObserveThreshold = defaultAIChatObserveThreshold
	}
	if cfg.AIChat.SessionRiskTTLMinutes <= 0 || cfg.AIChat.SessionRiskTTLMinutes > maxAIChatSessionRiskTTLMinutes {
		cfg.AIChat.SessionRiskTTLMinutes = defaultAIChatSessionRiskTTLMinutes
	}
	if cfg.AIChat.SessionRiskHalfLifeMinutes <= 0 || cfg.AIChat.SessionRiskHalfLifeMinutes > maxAIChatSessionRiskHalfLifeMinutes {
		cfg.AIChat.SessionRiskHalfLifeMinutes = defaultAIChatSessionRiskHalfLifeMinutes
	}
	if cfg.AIChat.SessionRiskBlockCooldownMinutes < 0 || cfg.AIChat.SessionRiskBlockCooldownMinutes > maxAIChatSessionRiskBlockCooldownMinutes {
		cfg.AIChat.SessionRiskBlockCooldownMinutes = defaultAIChatSessionRiskBlockCooldownMinutes
	}
	if cfg.AIChat.RecentUserTurns <= 0 || cfg.AIChat.RecentUserTurns > maxAIChatRecentUserTurns {
		cfg.AIChat.RecentUserTurns = defaultAIChatRecentUserTurns
	}
	if cfg.AIChat.SummaryMaxChars <= 0 || cfg.AIChat.SummaryMaxChars > maxAIChatSummaryMaxChars {
		cfg.AIChat.SummaryMaxChars = defaultAIChatSummaryMaxChars
	}
	if cfg.AIChat.FullReviewThreshold <= 0 || cfg.AIChat.FullReviewThreshold >= cfg.AIChat.ConfidenceThreshold {
		cfg.AIChat.FullReviewThreshold = min(defaultAIChatFullReviewThreshold, cfg.AIChat.ConfidenceThreshold/2)
		if cfg.AIChat.FullReviewThreshold < 0.01 && cfg.AIChat.ConfidenceThreshold > 0.01 {
			cfg.AIChat.FullReviewThreshold = 0.01
		}
	}
	if cfg.AIChat.FullReviewRiskDelta <= 0 || cfg.AIChat.FullReviewRiskDelta > 1 {
		cfg.AIChat.FullReviewRiskDelta = defaultAIChatFullReviewRiskDelta
	}
	if cfg.AIChat.PeriodicFullReviewTurns <= 0 || cfg.AIChat.PeriodicFullReviewTurns > maxAIChatPeriodicFullReviewTurns {
		cfg.AIChat.PeriodicFullReviewTurns = defaultAIChatPeriodicFullReviewTurns
	}
	if cfg.AIChat.FullReviewMaxInputChars < minAIChatMaxInputChars || cfg.AIChat.FullReviewMaxInputChars > cfg.AIChat.MaxInputChars {
		cfg.AIChat.FullReviewMaxInputChars = min(defaultAIChatFullReviewMaxInputChars, cfg.AIChat.MaxInputChars)
	}
	if cfg.AIChat.FastMaxOutputTokens <= 0 || cfg.AIChat.FastMaxOutputTokens > maxAIChatOutputTokens {
		cfg.AIChat.FastMaxOutputTokens = defaultAIChatFastMaxOutputTokens
	}
	if cfg.AIChat.FullMaxOutputTokens <= 0 || cfg.AIChat.FullMaxOutputTokens > maxAIChatOutputTokens {
		cfg.AIChat.FullMaxOutputTokens = defaultAIChatFullMaxOutputTokens
	}
	if cfg.AIChat.MaxReviewMaxOutputTokens <= 0 || cfg.AIChat.MaxReviewMaxOutputTokens > maxAIChatOutputTokens {
		cfg.AIChat.MaxReviewMaxOutputTokens = defaultAIChatMaxReviewMaxOutputTokens
	}
	if cfg.AIChat.AuditContextTTLMinutes <= 0 || cfg.AIChat.AuditContextTTLMinutes > maxAIChatAuditContextTTLMinutes {
		cfg.AIChat.AuditContextTTLMinutes = defaultAIChatAuditContextTTLMinutes
	}
	cfg.AIChat.PricingVersion = strings.TrimSpace(cfg.AIChat.PricingVersion)
	if !cfg.AIChat.InputProvenanceV2Enabled {
		// Provenance normalization is the security boundary that identifies the
		// sole audit target. Never run incremental review without it, including
		// when loading an older or manually edited persisted configuration.
		cfg.AIChat.IncrementalAuditEnabled = false
	}
}

func normalizeContentModerationProvider(provider string) string {
	if strings.TrimSpace(provider) == ContentModerationProviderAIChat {
		return ContentModerationProviderAIChat
	}
	return ContentModerationProviderOpenAIModerations
}

func (cfg *ContentModerationConfig) activeBaseURL() string {
	if cfg != nil && cfg.AuditProvider == ContentModerationProviderAIChat {
		return cfg.AIChat.BaseURL
	}
	return cfg.BaseURL
}

func (cfg *ContentModerationConfig) activeModel() string {
	if cfg != nil && cfg.AuditProvider == ContentModerationProviderAIChat {
		return cfg.AIChat.Model
	}
	return cfg.Model
}

func (cfg *ContentModerationConfig) activeTimeoutMS() int {
	if cfg != nil && cfg.AuditProvider == ContentModerationProviderAIChat {
		return cfg.AIChat.TimeoutMS
	}
	return cfg.TimeoutMS
}

func (cfg *ContentModerationConfig) activeRetryCount() int {
	if cfg != nil && cfg.AuditProvider == ContentModerationProviderAIChat {
		return cfg.AIChat.RetryCount
	}
	return cfg.RetryCount
}

func (cfg *ContentModerationConfig) activeProxyID() *int64 {
	if cfg != nil && cfg.AuditProvider == ContentModerationProviderAIChat {
		return cfg.AIChat.ProxyID
	}
	if cfg == nil {
		return nil
	}
	return cfg.ProxyID
}

func (cfg *ContentModerationConfig) activeThresholds() map[string]float64 {
	if cfg != nil && cfg.AuditProvider == ContentModerationProviderAIChat {
		return map[string]float64{"ai_risk": cfg.AIChat.ConfidenceThreshold}
	}
	if cfg == nil {
		return ContentModerationDefaultThresholds()
	}
	return cfg.Thresholds
}

func (s *ContentModerationService) nextUsableAPIKey(cfg *ContentModerationConfig) (string, bool) {
	keys := cfg.apiKeys()
	if len(keys) == 0 {
		return "", false
	}
	now := time.Now()
	for i := 0; i < len(keys); i++ {
		idx := int(s.apiKeyCursor.Add(1)-1) % len(keys)
		key := keys[idx]
		if !s.isAPIKeyFrozen(key, now) {
			return key, true
		}
	}
	return "", false
}

func (s *ContentModerationService) nextUsableAPIKeyExcluding(cfg *ContentModerationConfig, excludedHashes map[string]struct{}) (string, bool) {
	keys := cfg.apiKeys()
	if len(keys) == 0 {
		return "", false
	}
	now := time.Now()
	for i := 0; i < len(keys); i++ {
		idx := int(s.apiKeyCursor.Add(1)-1) % len(keys)
		key := keys[idx]
		if _, excluded := excludedHashes[moderationAPIKeyHash(key)]; excluded {
			continue
		}
		if !s.isAPIKeyFrozen(key, now) {
			return key, true
		}
	}
	return "", false
}

func (s *ContentModerationService) hasUsableAPIKeyExcluding(cfg *ContentModerationConfig, excludedHashes map[string]struct{}) bool {
	now := time.Now()
	for _, key := range cfg.apiKeys() {
		if _, excluded := excludedHashes[moderationAPIKeyHash(key)]; excluded {
			continue
		}
		if !s.isAPIKeyFrozen(key, now) {
			return true
		}
	}
	return false
}

func (s *ContentModerationService) isAPIKeyFrozen(key string, now time.Time) bool {
	hash := moderationAPIKeyHash(key)
	if hash == "" || s == nil {
		return false
	}
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.keyHealth[hash]
	return state != nil && state.FrozenUntil.After(now)
}

func (s *ContentModerationService) beginModerationAPIKeyCall(key string) {
	hash := moderationAPIKeyHash(key)
	if hash == "" || s == nil {
		return
	}
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.ensureAPIKeyHealthLocked(hash, maskSecretTail(key))
	state.SyncActive++
}

func (s *ContentModerationService) finishModerationAPIKeyCall(key string, latencyMS int, success bool) {
	hash := moderationAPIKeyHash(key)
	if hash == "" || s == nil {
		return
	}
	if latencyMS < 0 {
		latencyMS = 0
	}
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.ensureAPIKeyHealthLocked(hash, maskSecretTail(key))
	if state.SyncActive > 0 {
		state.SyncActive--
	}
	state.SyncTotal++
	state.SyncLatencyMS += int64(latencyMS)
	if success {
		state.SyncSuccess++
		return
	}
	state.SyncErrors++
}

func (s *ContentModerationService) markAPIKeySuccess(key string, latencyMS int, httpStatus int) {
	hash := moderationAPIKeyHash(key)
	if hash == "" || s == nil {
		return
	}
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.ensureAPIKeyHealthLocked(hash, maskSecretTail(key))
	state.FailureCount = 0
	state.SuccessCount++
	state.LastError = ""
	state.LastCheckedAt = time.Now()
	state.FrozenUntil = time.Time{}
	state.LastLatencyMS = latencyMS
	state.LastHTTPStatus = httpStatus
	state.LastTested = true
}

func (s *ContentModerationService) markAPIKeyError(key string, errText string, latencyMS int, httpStatus int) {
	hash := moderationAPIKeyHash(key)
	if hash == "" || s == nil {
		return
	}
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.ensureAPIKeyHealthLocked(hash, maskSecretTail(key))
	if contentModerationFreezeDurationForHTTPStatus(httpStatus) > 0 {
		state.FailureCount++
	}
	state.LastError = trimRunes(errText, 180)
	state.LastCheckedAt = time.Now()
	state.LastLatencyMS = latencyMS
	state.LastHTTPStatus = httpStatus
	state.LastTested = true
	if freezeDuration := contentModerationFreezeDurationForHTTPStatus(httpStatus); freezeDuration > 0 {
		state.FrozenUntil = time.Now().Add(freezeDuration)
	}
}

func contentModerationFreezeDurationForHTTPStatus(httpStatus int) time.Duration {
	if httpStatus >= http.StatusOK && httpStatus < http.StatusMultipleChoices {
		return 0
	}
	switch httpStatus {
	case 0, http.StatusBadRequest:
		return 0
	case http.StatusUnauthorized, http.StatusForbidden:
		return contentModerationKeyAuthFreezeDuration
	case http.StatusTooManyRequests, 529:
		return contentModerationKeyRateLimitFreezeDuration
	default:
		return contentModerationKeyHTTPErrorFreezeDuration
	}
}

func (s *ContentModerationService) ensureAPIKeyHealthLocked(hash string, masked string) *contentModerationKeyHealth {
	if s.keyHealth == nil {
		s.keyHealth = make(map[string]*contentModerationKeyHealth)
	}
	state := s.keyHealth[hash]
	if state == nil {
		state = &contentModerationKeyHealth{Hash: hash}
		s.keyHealth[hash] = state
	}
	if strings.TrimSpace(masked) != "" {
		state.Masked = masked
	}
	return state
}

func (s *ContentModerationService) configView(cfg *ContentModerationConfig) *ContentModerationConfigView {
	keys := cfg.apiKeys()
	masks := make([]string, 0, len(keys))
	for _, key := range keys {
		masks = append(masks, maskSecretTail(key))
	}
	apiKeyMasked := ""
	if len(masks) > 0 {
		apiKeyMasked = masks[0]
	}
	openAIProfile := contentModerationProviderProfileView(
		cfg.BaseURL, cfg.Model, cfg.ProxyID, cfg.APIKeys, cfg.TimeoutMS, cfg.RetryCount,
	)
	aiChatProfile := contentModerationProviderProfileView(
		cfg.AIChat.BaseURL, cfg.AIChat.Model, cfg.AIChat.ProxyID, cfg.AIChat.APIKeys, cfg.AIChat.TimeoutMS, cfg.AIChat.RetryCount,
	)
	systemPromptVersion, usesRecommendedSystemPrompt := voteaimoderation.ClassifySystemPrompt(cfg.AIChat.SystemPrompt)
	return &ContentModerationConfigView{
		Enabled:           cfg.Enabled,
		Mode:              cfg.Mode,
		AuditProvider:     cfg.AuditProvider,
		OpenAIModerations: openAIProfile,
		AIChat: ContentModerationAIChatConfigView{
			ContentModerationProviderProfileView: aiChatProfile,
			ConfidenceThreshold:                  cfg.AIChat.ConfidenceThreshold,
			CacheEnabled:                         cfg.AIChat.CacheEnabled,
			CacheTTLSeconds:                      cfg.AIChat.CacheTTLSeconds,
			SystemPrompt:                         cfg.AIChat.SystemPrompt,
			RecommendedSystemPrompt:              voteaimoderation.RecommendedSystemPrompt,
			RecommendedPromptVersion:             voteaimoderation.RecommendedSystemPromptVersion,
			SystemPromptVersion:                  systemPromptVersion,
			UsesRecommendedSystemPrompt:          usesRecommendedSystemPrompt,
			FailurePolicy:                        cfg.AIChat.FailurePolicy,
			MaxInputChars:                        cfg.AIChat.MaxInputChars,
			SynchronousBudgetMS:                  cfg.AIChat.SynchronousBudgetMS,
			FastInputChars:                       cfg.AIChat.FastInputChars,
			FallbackInputChars:                   cfg.AIChat.FallbackInputChars,
			ThinkingMode:                         cfg.AIChat.ThinkingMode,
			ReasoningEffort:                      cfg.AIChat.ReasoningEffort,
			RiskLevelsEnabled:                    cfg.AIChat.RiskLevelsEnabled,
			ObserveThreshold:                     cfg.AIChat.ObserveThreshold,
			SessionRiskEnabled:                   cfg.AIChat.SessionRiskEnabled,
			SessionRiskTTLMinutes:                cfg.AIChat.SessionRiskTTLMinutes,
			SessionRiskHalfLifeMinutes:           cfg.AIChat.SessionRiskHalfLifeMinutes,
			SessionRiskBlockCooldownMinutes:      cfg.AIChat.SessionRiskBlockCooldownMinutes,
			ActorRiskEnabled:                     cfg.AIChat.ActorRiskEnabled,
			IncrementalAuditEnabled:              cfg.AIChat.IncrementalAuditEnabled,
			InputProvenanceV2Enabled:             cfg.AIChat.InputProvenanceV2Enabled,
			DeterministicRiskV2Enabled:           cfg.AIChat.DeterministicRiskV2Enabled,
			RecentUserTurns:                      cfg.AIChat.RecentUserTurns,
			SummaryMaxChars:                      cfg.AIChat.SummaryMaxChars,
			FullReviewThreshold:                  cfg.AIChat.FullReviewThreshold,
			FullReviewRiskDelta:                  cfg.AIChat.FullReviewRiskDelta,
			PeriodicFullReviewTurns:              cfg.AIChat.PeriodicFullReviewTurns,
			FullReviewMaxInputChars:              cfg.AIChat.FullReviewMaxInputChars,
			FastMaxOutputTokens:                  cfg.AIChat.FastMaxOutputTokens,
			FullMaxOutputTokens:                  cfg.AIChat.FullMaxOutputTokens,
			MaxReviewMaxOutputTokens:             cfg.AIChat.MaxReviewMaxOutputTokens,
			AuditContextTTLMinutes:               cfg.AIChat.AuditContextTTLMinutes,
			PricingConfigured:                    contentModerationPricingConfigured(cfg.AIChat),
			PricingVersion:                       cfg.AIChat.PricingVersion,
			UncachedInputUSDPerMillionTokens:     cloneFloat64Ptr(cfg.AIChat.UncachedInputUSDPerMillionTokens),
			CachedInputUSDPerMillionTokens:       cloneFloat64Ptr(cfg.AIChat.CachedInputUSDPerMillionTokens),
			OutputUSDPerMillionTokens:            cloneFloat64Ptr(cfg.AIChat.OutputUSDPerMillionTokens),
		},
		BaseURL:                        cfg.activeBaseURL(),
		Model:                          cfg.activeModel(),
		ProxyID:                        cloneInt64Ptr(cfg.activeProxyID()),
		APIKeyConfigured:               len(keys) > 0,
		APIKeyMasked:                   apiKeyMasked,
		APIKeyCount:                    len(keys),
		APIKeyMasks:                    masks,
		APIKeyStatuses:                 s.apiKeyStatuses(keys),
		TimeoutMS:                      cfg.activeTimeoutMS(),
		SampleRate:                     cfg.SampleRate,
		AllGroups:                      cfg.AllGroups,
		GroupIDs:                       append([]int64(nil), cfg.GroupIDs...),
		RecordNonHits:                  cfg.RecordNonHits,
		Thresholds:                     cloneFloatMap(cfg.Thresholds),
		WorkerCount:                    cfg.WorkerCount,
		QueueSize:                      cfg.QueueSize,
		BlockStatus:                    cfg.BlockStatus,
		BlockMessage:                   cfg.BlockMessage,
		EmailOnHit:                     cfg.EmailOnHit,
		AutoBanEnabled:                 cfg.AutoBanEnabled,
		BanThreshold:                   cfg.BanThreshold,
		ViolationWindowHours:           cfg.ViolationWindowHours,
		RetryCount:                     cfg.activeRetryCount(),
		HitRetentionDays:               cfg.HitRetentionDays,
		NonHitRetentionDays:            cfg.NonHitRetentionDays,
		PreHashCheckEnabled:            cfg.PreHashCheckEnabled,
		BlockedKeywords:                append([]string(nil), cfg.BlockedKeywords...),
		KeywordBlockingMode:            cfg.KeywordBlockingMode,
		ModelFilter:                    cloneContentModerationModelFilter(cfg.ModelFilter),
		UserFilter:                     cloneContentModerationUserFilter(cfg.UserFilter),
		AccountFilter:                  cloneContentModerationAccountFilter(cfg.AccountFilter),
		CyberPolicyExcludeFromBanCount: cfg.CyberPolicyExcludeFromBanCount,
	}
}

func contentModerationProviderProfileView(baseURL, model string, proxyID *int64, keys []string, timeoutMS, retryCount int) ContentModerationProviderProfileView {
	keys = normalizeModerationAPIKeys(keys)
	masks := make([]string, 0, len(keys))
	for _, key := range keys {
		masks = append(masks, maskSecretTail(key))
	}
	return ContentModerationProviderProfileView{
		BaseURL:          baseURL,
		Model:            model,
		ProxyID:          cloneInt64Ptr(proxyID),
		APIKeyConfigured: len(keys) > 0,
		APIKeyCount:      len(keys),
		APIKeyMasks:      masks,
		TimeoutMS:        timeoutMS,
		RetryCount:       retryCount,
	}
}

func (s *ContentModerationService) apiKeyStatuses(keys []string) []ContentModerationAPIKeyStatus {
	out := make([]ContentModerationAPIKeyStatus, 0, len(keys))
	for idx, key := range keys {
		out = append(out, s.apiKeyStatusForHash(idx, moderationAPIKeyHash(key), maskSecretTail(key), true))
	}
	return out
}

func (s *ContentModerationService) preBlockAPIKeyLoads(keys []string) []ContentModerationAPIKeyLoad {
	out := make([]ContentModerationAPIKeyLoad, 0, len(keys))
	for idx, key := range keys {
		out = append(out, s.preBlockAPIKeyLoadForHash(idx, moderationAPIKeyHash(key), maskSecretTail(key)))
	}
	return out
}

func (s *ContentModerationService) preBlockAPIKeyActive(keys []string) int64 {
	var total int64
	for _, item := range s.preBlockAPIKeyLoads(keys) {
		total += item.Active
	}
	return total
}

func (s *ContentModerationService) preBlockAPIKeyAvailableCount(keys []string) int64 {
	now := time.Now()
	var count int64
	for _, key := range keys {
		if !s.isAPIKeyFrozen(key, now) {
			count++
		}
	}
	return count
}

func (s *ContentModerationService) preBlockAPIKeyTotalCalls(keys []string) int64 {
	var total int64
	for _, item := range s.preBlockAPIKeyLoads(keys) {
		total += item.Total
	}
	return total
}

func (s *ContentModerationService) preBlockAPIKeyLoadForHash(index int, hash string, masked string) ContentModerationAPIKeyLoad {
	load := ContentModerationAPIKeyLoad{
		Index:   index,
		KeyHash: hash,
		Masked:  masked,
		Status:  "unknown",
	}
	status := s.apiKeyStatusForHash(index, hash, masked, true)
	load.Status = status.Status
	load.LastLatencyMS = status.LastLatencyMS
	load.LastHTTPStatus = status.LastHTTPStatus
	if hash == "" || s == nil {
		return load
	}
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.keyHealth[hash]
	if state == nil {
		return load
	}
	load.Active = state.SyncActive
	load.Total = state.SyncTotal
	load.Success = state.SyncSuccess
	load.Errors = state.SyncErrors
	if state.SyncTotal > 0 {
		load.AvgLatencyMS = state.SyncLatencyMS / state.SyncTotal
	}
	return load
}

func (s *ContentModerationService) apiKeyStatusForHash(index int, hash string, masked string, configured bool) ContentModerationAPIKeyStatus {
	status := ContentModerationAPIKeyStatus{
		Index:      index,
		KeyHash:    hash,
		Masked:     masked,
		Status:     "unknown",
		Configured: configured,
	}
	if hash == "" || s == nil {
		return status
	}
	now := time.Now()
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.keyHealth[hash]
	if state == nil {
		return status
	}
	status.FailureCount = state.FailureCount
	status.SuccessCount = state.SuccessCount
	status.LastError = state.LastError
	status.LastLatencyMS = state.LastLatencyMS
	status.LastHTTPStatus = state.LastHTTPStatus
	status.LastTested = state.LastTested
	if !state.LastCheckedAt.IsZero() {
		t := state.LastCheckedAt
		status.LastCheckedAt = &t
	}
	if state.FrozenUntil.After(now) {
		t := state.FrozenUntil
		status.FrozenUntil = &t
		status.Status = "frozen"
		return status
	}
	if state.LastError != "" {
		status.Status = "error"
		return status
	}
	if state.SuccessCount > 0 || state.LastTested {
		status.Status = "ok"
	}
	return status
}

func moderationAPIKeyHash(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func buildModerationTestInput(prompt string, images []string) (any, int, error) {
	prompt = trimRunes(normalizeContentModerationText(prompt), maxModerationInputRunes)
	normalizedImages := make([]string, 0, len(images))
	for _, image := range images {
		image = strings.TrimSpace(image)
		if image == "" {
			continue
		}
		if len(normalizedImages) >= maxContentModerationTestImages {
			return nil, 0, infraerrors.BadRequest("TOO_MANY_MODERATION_TEST_IMAGES", fmt.Sprintf("最多上传 %d 张测试图片", maxContentModerationTestImages))
		}
		if err := validateModerationTestImageDataURL(image); err != nil {
			return nil, 0, err
		}
		normalizedImages = append(normalizedImages, image)
	}
	if prompt == "" && len(normalizedImages) == 0 {
		return "hello", 0, nil
	}
	if len(normalizedImages) == 0 {
		return prompt, 0, nil
	}
	parts := make([]moderationAPIInputPart, 0, len(normalizedImages)+1)
	if prompt != "" {
		parts = append(parts, moderationAPIInputPart{Type: "text", Text: prompt})
	}
	for _, image := range normalizedImages {
		parts = append(parts, moderationAPIInputPart{
			Type:     "image_url",
			ImageURL: &moderationAPIImageURLRef{URL: image},
		})
	}
	return parts, len(normalizedImages), nil
}

func contentModerationTestHasAuditInput(prompt string, images []string) bool {
	if normalizeContentModerationText(prompt) != "" {
		return true
	}
	for _, image := range images {
		if strings.TrimSpace(image) != "" {
			return true
		}
	}
	return false
}

func validateModerationTestImageDataURL(value string) error {
	if len(value) > maxContentModerationTestImageDataURLBytes {
		return infraerrors.BadRequest("MODERATION_TEST_IMAGE_TOO_LARGE", "测试图片不能超过 8MB")
	}
	if !strings.HasPrefix(value, "data:image/") {
		return infraerrors.BadRequest("INVALID_MODERATION_TEST_IMAGE", "测试图片必须是 data:image/* base64")
	}
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 || !strings.Contains(parts[0], ";base64") {
		return infraerrors.BadRequest("INVALID_MODERATION_TEST_IMAGE", "测试图片必须是 base64 data URL")
	}
	raw, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return infraerrors.BadRequest("INVALID_MODERATION_TEST_IMAGE", "测试图片 base64 无效")
	}
	if len(raw) > maxContentModerationTestImageBytes {
		return infraerrors.BadRequest("MODERATION_TEST_IMAGE_TOO_LARGE", "测试图片不能超过 8MB")
	}
	return nil
}

func buildContentModerationTestAuditResult(result *moderationAPIResult, thresholds map[string]float64, riskThresholds ...float64) *ContentModerationTestAuditResult {
	if result == nil {
		return nil
	}
	scores := make(map[string]float64, len(result.CategoryScores))
	for category, score := range result.CategoryScores {
		scores[category] = score
	}
	thresholdSnapshot := cloneFloatMap(thresholds)
	if _, aiAudit := thresholdSnapshot["ai_risk"]; !aiAudit {
		thresholdSnapshot = mergeContentModerationThresholds(ContentModerationDefaultThresholds(), thresholds)
	}
	flagged, highestCategory, highestScore := evaluateModerationScores(scores, thresholdSnapshot)
	if _, aiAudit := thresholdSnapshot["ai_risk"]; aiAudit {
		flagged = result.Flagged && flagged
	}
	riskScore := highestScore
	if aiRiskScore, ok := scores["ai_risk"]; ok {
		riskScore = aiRiskScore
	}
	observeThreshold := defaultAIChatObserveThreshold
	highThreshold := defaultAIChatConfidenceThreshold
	if len(riskThresholds) > 0 && riskThresholds[0] > 0 && riskThresholds[0] < 1 {
		observeThreshold = riskThresholds[0]
	}
	if len(riskThresholds) > 1 && riskThresholds[1] > observeThreshold && riskThresholds[1] <= 1 {
		highThreshold = riskThresholds[1]
	}
	riskTier := ""
	if len(riskThresholds) >= 2 {
		riskTier = "low"
		if riskScore >= highThreshold {
			riskTier = "high"
		} else if riskScore >= observeThreshold {
			riskTier = "observe"
		}
	}
	if contentModerationResultHasOnlyWeakSignals(result) {
		flagged = false
		riskTier = "low"
	}
	compositeScore := highestScore
	return &ContentModerationTestAuditResult{
		Flagged:          flagged,
		RiskScore:        riskScore,
		RiskTier:         riskTier,
		Categories:       append([]string{}, result.Categories...),
		Signals:          append([]string{}, result.Signals...),
		HighestCategory:  highestCategory,
		HighestScore:     highestScore,
		CompositeScore:   compositeScore,
		CategoryScores:   scores,
		Thresholds:       thresholdSnapshot,
		Reason:           result.Reason,
		ReviewIncomplete: result.ReviewIncomplete,
		ReviewError:      result.ReviewError,
		Scope:            "request",
	}
}

type moderationAPIRequest struct {
	Model string `json:"model"`
	Input any    `json:"input"`
}

type moderationAPIInputPart struct {
	Type     string                    `json:"type"`
	Text     string                    `json:"text,omitempty"`
	ImageURL *moderationAPIImageURLRef `json:"image_url,omitempty"`
}

type moderationAPIImageURLRef struct {
	URL string `json:"url"`
}

type moderationAPIResponse struct {
	Results []moderationAPIResult `json:"results"`
}

type moderationAPIResult struct {
	Flagged               bool                                            `json:"flagged"`
	CategoryScores        map[string]float64                              `json:"category_scores"`
	Categories            []string                                        `json:"ai_categories"`
	Signals               []string                                        `json:"signals"`
	Reason                string                                          `json:"reason,omitempty"`
	ReviewIncomplete      bool                                            `json:"-"`
	ReviewError           string                                          `json:"-"`
	Stage                 voteaimoderation.ReviewStage                    `json:"audit_stage,omitempty"`
	Usage                 *voteaimoderation.Usage                         `json:"-"`
	InputChars            int                                             `json:"-"`
	AuditKeyHash          string                                          `json:"-"`
	ResultCacheHit        bool                                            `json:"-"`
	LocalDecision         bool                                            `json:"-"`
	HashPromotionEligible bool                                            `json:"-"`
	HashPromotionVeto     bool                                            `json:"-"`
	HashPromotionReason   string                                          `json:"-"`
	LocalRuleLevel        string                                          `json:"local_rule_level,omitempty"`
	LocalRuleMatch        *voteaideterministicrisk.DeterministicRiskMatch `json:"local_rule_match,omitempty"`
	StageDetails          []ContentModerationAuditStageDetails            `json:"-"`
}

func evaluateModerationScores(scores map[string]float64, thresholds map[string]float64) (bool, string, float64) {
	flagged := false
	highestCategory := ""
	highestScore := 0.0
	for _, category := range contentModerationCategoryOrder {
		score := scores[category]
		if score > highestScore || highestCategory == "" {
			highestScore = score
			highestCategory = category
		}
		if threshold, ok := thresholds[category]; ok && score >= threshold {
			flagged = true
		}
	}
	for category, score := range scores {
		if score > highestScore || highestCategory == "" {
			highestScore = score
			highestCategory = category
		}
		if threshold, ok := thresholds[category]; ok && score >= threshold {
			flagged = true
		}
	}
	return flagged, highestCategory, highestScore
}

func mergeContentModerationThresholds(base map[string]float64, override map[string]float64) map[string]float64 {
	out := cloneFloatMap(base)
	if out == nil {
		out = map[string]float64{}
	}
	for _, category := range contentModerationCategoryOrder {
		if v, ok := override[category]; ok {
			if v < 0 {
				v = 0
			}
			if v > 1 {
				v = 1
			}
			out[category] = v
		}
	}
	return out
}

func normalizeInt64IDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func normalizeBlockedKeywords(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		kw := strings.TrimSpace(raw)
		if kw == "" {
			continue
		}
		kw = trimRunes(kw, maxContentModerationBlockedKeywordRunes)
		key := strings.ToLower(kw)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, kw)
		if len(out) >= maxContentModerationBlockedKeywords {
			break
		}
	}
	return out
}

func normalizeKeywordBlockingMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case ContentModerationKeywordModeKeywordOnly:
		return ContentModerationKeywordModeKeywordOnly
	case ContentModerationKeywordModeAPIOnly:
		return ContentModerationKeywordModeAPIOnly
	case ContentModerationKeywordModeKeywordAndAPI:
		return ContentModerationKeywordModeKeywordAndAPI
	default:
		return ContentModerationKeywordModeKeywordAndAPI
	}
}

func normalizeContentModerationModelFilter(filter ContentModerationModelFilter) ContentModerationModelFilter {
	out := ContentModerationModelFilter{
		Type:   normalizeContentModerationModelFilterType(filter.Type),
		Models: normalizeContentModerationModelNames(filter.Models),
	}
	if out.Type == ContentModerationModelFilterAll {
		out.Models = []string{}
	}
	return out
}

func cloneContentModerationModelFilter(filter ContentModerationModelFilter) ContentModerationModelFilter {
	normalized := normalizeContentModerationModelFilter(filter)
	normalized.Models = append([]string(nil), normalized.Models...)
	return normalized
}

func normalizeContentModerationModelFilterType(filterType string) string {
	switch strings.ToLower(strings.TrimSpace(filterType)) {
	case ContentModerationModelFilterInclude:
		return ContentModerationModelFilterInclude
	case ContentModerationModelFilterExclude:
		return ContentModerationModelFilterExclude
	case ContentModerationModelFilterAll:
		return ContentModerationModelFilterAll
	default:
		return ContentModerationModelFilterAll
	}
}

func normalizeContentModerationModelNames(models []string) []string {
	if len(models) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, raw := range models {
		model := trimRunes(strings.TrimSpace(raw), maxContentModerationModelFilterRunes)
		if model == "" {
			continue
		}
		key := strings.ToLower(model)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, model)
		if len(out) >= maxContentModerationModelFilterModels {
			break
		}
	}
	return out
}

func contentModerationModelListContains(models []string, model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
	}
	for _, candidate := range models {
		if strings.ToLower(strings.TrimSpace(candidate)) == model {
			return true
		}
	}
	return false
}

func normalizeContentModerationUserFilter(filter ContentModerationUserFilter) ContentModerationUserFilter {
	out := ContentModerationUserFilter{
		Type:    normalizeContentModerationScopeFilterType(filter.Type),
		UserIDs: normalizeInt64IDs(filter.UserIDs),
	}
	if out.Type == ContentModerationScopeFilterAll {
		out.UserIDs = []int64{}
	}
	return out
}

func cloneContentModerationUserFilter(filter ContentModerationUserFilter) ContentModerationUserFilter {
	normalized := normalizeContentModerationUserFilter(filter)
	normalized.UserIDs = append([]int64(nil), normalized.UserIDs...)
	return normalized
}

func normalizeContentModerationAccountFilter(filter ContentModerationAccountFilter) ContentModerationAccountFilter {
	out := ContentModerationAccountFilter{
		Type:       normalizeContentModerationScopeFilterType(filter.Type),
		AccountIDs: normalizeInt64IDs(filter.AccountIDs),
	}
	if out.Type == ContentModerationScopeFilterAll {
		out.AccountIDs = []int64{}
	}
	return out
}

func (s *ContentModerationService) normalizeAccountScopeFilter(ctx context.Context, filter ContentModerationAccountFilter) (ContentModerationAccountFilter, error) {
	normalized := normalizeContentModerationAccountFilter(filter)
	if normalized.Type == ContentModerationScopeFilterAll || len(normalized.AccountIDs) == 0 || s == nil || s.accountScopeRepo == nil {
		return normalized, nil
	}

	accounts, err := s.accountScopeRepo.GetByIDs(ctx, normalized.AccountIDs)
	if err != nil {
		return ContentModerationAccountFilter{}, fmt.Errorf("resolve content moderation account scope IDs: %w", err)
	}
	canonicalByID := make(map[int64]int64, len(accounts))
	for _, account := range accounts {
		if account == nil || account.ID <= 0 {
			continue
		}
		canonicalID := account.ID
		if account.ParentAccountID != nil && *account.ParentAccountID > 0 {
			canonicalID = *account.ParentAccountID
		}
		canonicalByID[account.ID] = canonicalID
	}

	canonicalIDs := make([]int64, 0, len(normalized.AccountIDs))
	for _, id := range normalized.AccountIDs {
		if canonicalID, ok := canonicalByID[id]; ok {
			canonicalIDs = append(canonicalIDs, canonicalID)
		} else {
			canonicalIDs = append(canonicalIDs, id)
		}
	}
	normalized.AccountIDs = normalizeInt64IDs(canonicalIDs)
	return normalized, nil
}

func cloneContentModerationAccountFilter(filter ContentModerationAccountFilter) ContentModerationAccountFilter {
	normalized := normalizeContentModerationAccountFilter(filter)
	if len(normalized.AccountIDs) == 0 {
		normalized.AccountIDs = []int64{}
		return normalized
	}
	normalized.AccountIDs = append([]int64(nil), normalized.AccountIDs...)
	return normalized
}

func normalizeContentModerationScopeFilterType(filterType string) string {
	switch strings.ToLower(strings.TrimSpace(filterType)) {
	case ContentModerationScopeFilterInclude:
		return ContentModerationScopeFilterInclude
	case ContentModerationScopeFilterExclude:
		return ContentModerationScopeFilterExclude
	case ContentModerationScopeFilterAll:
		return ContentModerationScopeFilterAll
	default:
		return ContentModerationScopeFilterAll
	}
}

func isValidContentModerationScopeFilterType(filterType string) bool {
	switch strings.ToLower(strings.TrimSpace(filterType)) {
	case "", ContentModerationScopeFilterAll, ContentModerationScopeFilterInclude, ContentModerationScopeFilterExclude:
		return true
	default:
		return false
	}
}

func contentModerationIDFilterIncludes(filterType string, ids []int64, id int64) bool {
	contains := false
	for _, candidate := range ids {
		if candidate == id {
			contains = true
			break
		}
	}
	switch normalizeContentModerationScopeFilterType(filterType) {
	case ContentModerationScopeFilterInclude:
		return contains
	case ContentModerationScopeFilterExclude:
		return !contains
	default:
		return true
	}
}

func matchBlockedKeyword(text string, keywords []string) (string, bool) {
	if text == "" || len(keywords) == 0 {
		return "", false
	}
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(kw)) {
			return kw, true
		}
	}
	return "", false
}

func normalizeModerationAPIKeys(keys []string) []string {
	if len(keys) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func deleteModerationAPIKeysByHash(keys []string, hashes []string) []string {
	keys = normalizeModerationAPIKeys(keys)
	deleteHashes := make(map[string]struct{}, len(hashes))
	for _, hash := range hashes {
		hash = normalizeContentModerationHash(hash)
		if hash != "" {
			deleteHashes[hash] = struct{}{}
		}
	}
	if len(deleteHashes) == 0 {
		return keys
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := deleteHashes[moderationAPIKeyHash(key)]; ok {
			continue
		}
		out = append(out, key)
	}
	return out
}

func normalizeContentModerationAPIKeysMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case contentModerationAPIKeysModeReplace:
		return contentModerationAPIKeysModeReplace
	default:
		return contentModerationAPIKeysModeAppend
	}
}

func normalizeContentModerationHash(inputHash string) string {
	inputHash = strings.ToLower(strings.TrimSpace(inputHash))
	if len(inputHash) != sha256.Size*2 {
		return ""
	}
	if _, err := hex.DecodeString(inputHash); err != nil {
		return ""
	}
	return inputHash
}

func cloneFloatMap(in map[string]float64) map[string]float64 {
	if in == nil {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneInt64Ptr(in *int64) *int64 {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func cloneFloat64Ptr(in *float64) *float64 {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func trimRunes(text string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max])
}

func maskSecretTail(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return "****"
	}
	return strings.Repeat("*", 8) + secret[len(secret)-4:]
}

// CyberPolicyRecordInput 是一次 cyber_policy 硬阻断的风控记录入参。
type CyberPolicyRecordInput struct {
	RequestID       string
	SessionID       string
	InputHash       string
	UserID          int64
	UserEmail       string
	APIKeyID        int64
	APIKeyName      string
	GroupID         *int64
	GroupName       string
	Endpoint        string
	Model           string
	UpstreamMessage string
	UpstreamBody    string
	UpstreamStatus  int
	UpstreamInTok   int
	UpstreamOutTok  int
	ModerationEpoch int64
	EpochSet        bool
}

// RecordCyberPolicyEvent 把一次 cyber_policy 硬阻断写入风控中心日志、计入违规计数、
// 并给用户发邮件。当前请求已由 gateway 透传给用户；本方法仅做事后记录/通知/计数。
// 仅受 risk_control_enabled 总开关约束（不受内容审核 Enabled/Mode/scope/sample 约束）。
// The return value reports whether session-level cyber enforcement may be
// applied. A false result means the event belongs to a pre-reset user epoch.
func (s *ContentModerationService) RecordCyberPolicyEvent(ctx context.Context, in CyberPolicyRecordInput) bool {
	if s == nil {
		return true
	}
	unlockUser := s.lockContentModerationUserState(in.UserID, false)
	defer unlockUser()
	epochCurrent := s.contentModerationEpochIsCurrent(ctx, in.UserID, in.ModerationEpoch, in.EpochSet)
	if s.repo == nil {
		return epochCurrent
	}
	if !s.isRiskControlEnabled(ctx) {
		return epochCurrent
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		slog.Warn("content_moderation.cyber_load_config_failed", "error", err)
		cfg = &ContentModerationConfig{}
	}
	var userID *int64
	if in.UserID > 0 {
		userID = &in.UserID
	}
	var apiKeyID *int64
	if in.APIKeyID > 0 {
		apiKeyID = &in.APIKeyID
	}
	errBody := strings.TrimSpace(in.UpstreamMessage)
	if b := strings.TrimSpace(in.UpstreamBody); b != "" {
		// 原始 body 不在此预脱敏；写入 log.Error 前由 redactContentModerationSecrets 统一脱敏。
		errBody = strings.TrimSpace(errBody + "\n" + b)
	}
	if in.UpstreamInTok > 0 || in.UpstreamOutTok > 0 {
		errBody = fmt.Sprintf("%s\nupstream_usage=in:%d,out:%d", errBody, in.UpstreamInTok, in.UpstreamOutTok)
	}
	log := &ContentModerationLog{
		RequestID:          in.RequestID,
		SessionID:          strings.TrimSpace(in.SessionID),
		InputHash:          strings.TrimSpace(in.InputHash),
		UserID:             userID,
		UserEmail:          in.UserEmail,
		APIKeyID:           apiKeyID,
		APIKeyName:         in.APIKeyName,
		GroupID:            cloneInt64Ptr(in.GroupID),
		GroupName:          in.GroupName,
		Endpoint:           in.Endpoint,
		Provider:           "openai",
		Model:              in.Model,
		Mode:               "post_upstream",
		Action:             ContentModerationActionCyberPolicy,
		Flagged:            true,
		HighestCategory:    "cyber_policy",
		HighestScore:       1.0,
		Error:              trimRunes(redactContentModerationSecrets(errBody), maxModerationExcerptRunes*4),
		AuditStatus:        ContentModerationAuditStatusSuccess,
		AuditCode:          ContentModerationActionCyberPolicy,
		SideEffectStatus:   ContentModerationSideEffectStatusPending,
		NotificationStatus: ContentModerationNotificationStatusPending,
		CreatedAt:          time.Now(),
	}
	if !epochCurrent {
		// Retain the upstream evidence for operators, but keep the row out of
		// future violation counts and suppress every enforcement side effect.
		log.Flagged = false
		log.AuditStatus = ContentModerationAuditStatusSkipped
		log.AuditCode = contentModerationAuditCodeStaleCyberPolicy
		log.SideEffectStatus = ContentModerationSideEffectStatusNotApplicable
		log.NotificationStatus = ContentModerationNotificationStatusNotRequired
		if err := s.repo.CreateLog(ctx, log); err != nil {
			slog.Warn("content_moderation.stale_cyber_create_log_failed", "user_id", in.UserID, "error", err)
		} else {
			slog.Info("content_moderation.stale_cyber_policy_event_retained",
				"log_id", log.ID,
				"user_id", in.UserID,
				"request_id", in.RequestID)
		}
		return false
	}
	lifecycleRepo, lifecycleAvailable := s.repo.(ContentModerationLifecycleRepository)
	if !lifecycleAvailable {
		log.SideEffectStatus = ContentModerationSideEffectStatusFailed
		log.NotificationStatus = ContentModerationNotificationStatusNotRequired
		log.SideEffectError = "content moderation lifecycle repository is unavailable"
		if err := s.repo.CreateLog(ctx, log); err != nil {
			slog.Error("content_moderation.cyber_create_failed_lifecycle_log_failed", "user_id", in.UserID, "error", err)
		} else {
			slog.Error("content_moderation.cyber_lifecycle_repository_unavailable", "log_id", log.ID, "user_id", in.UserID)
		}
		return true
	}
	if err := s.repo.CreateLog(ctx, log); err != nil {
		slog.Warn("content_moderation.cyber_create_log_failed", "user_id", in.UserID, "error", err)
		return true
	}
	tracker := &contentModerationSideEffectTracker{}
	// 开关开时 cyber_policy 不参与封号计数：当次不判定（此处跳过），
	// 历史行由 CountFlaggedByUserSince 的 excludeCyberPolicy 排除。
	banOutcome := ContentModerationBanOutcomeIneligible
	if !cfg.CyberPolicyExcludeFromBanCount {
		var banErr error
		banOutcome, banErr = s.applyFlaggedAccountSideEffects(ctx, cfg, log)
		if banErr != nil {
			tracker.failure("account", banErr)
		} else {
			tracker.success()
		}
	}
	log.NotificationStatus = ContentModerationNotificationStatusNotRequired
	if s.emailService != nil && strings.TrimSpace(log.UserEmail) != "" {
		switch banOutcome {
		case ContentModerationBanOutcomeApplied:
			emailErr := s.sendAccountDisabledEmail(ctx, cfg, log)
			if emailErr != nil {
				log.NotificationStatus = ContentModerationNotificationStatusFailed
				slog.Warn("content_moderation.cyber_ban_email_failed", "user_id", in.UserID, "error", emailErr)
			} else {
				log.EmailSent = true
				log.NotificationStatus = ContentModerationNotificationStatusSent
			}
			tracker.notification(log.NotificationStatus, log.EmailSent, emailErr)
		case ContentModerationBanOutcomeAlreadyOwned:
			// The dedicated disabled-account notification was handled by the
			// event that originally acquired moderation ban ownership.
		default:
			decision := s.reserveContentModerationEmailForLog(ctx, log)
			if dedupeErr := contentModerationEmailDedupeDecisionError(decision); dedupeErr != nil {
				tracker.failure("email_dedupe", dedupeErr)
				slog.Warn("content_moderation.cyber_email_dedupe_failed_open", "user_id", in.UserID, "scope", decision.Scope, "error", decision.Error)
			}
			if !decision.ShouldSend {
				log.NotificationStatus = ContentModerationNotificationStatusDeduplicated
				tracker.notification(log.NotificationStatus, false, nil)
			} else {
				emailErr := s.sendCyberPolicyEmail(ctx, log)
				if emailErr != nil {
					log.NotificationStatus = ContentModerationNotificationStatusFailed
					slog.Warn("content_moderation.cyber_email_failed", "user_id", in.UserID, "error", emailErr)
					if releaseErr := s.releaseContentModerationEmailReservation(ctx, decision); releaseErr != nil {
						tracker.failure("email_dedupe_release", releaseErr)
						slog.Warn("content_moderation.cyber_email_dedupe_release_failed", "user_id", in.UserID, "scope", decision.Scope, "error", releaseErr)
					}
				} else {
					log.EmailSent = true
					log.NotificationStatus = ContentModerationNotificationStatusSent
				}
				tracker.notification(log.NotificationStatus, log.EmailSent, emailErr)
			}
		}
	}
	tracker.finalize(log, true)
	if err := lifecycleRepo.UpdateLogEffects(ctx, log.ID, ContentModerationLogEffectsPatch{
		ViolationCount:     log.ViolationCount,
		AutoBanned:         log.AutoBanned,
		EmailSent:          log.EmailSent,
		SideEffectStatus:   log.SideEffectStatus,
		NotificationStatus: log.NotificationStatus,
		SideEffectError:    log.SideEffectError,
	}); err != nil {
		slog.Warn("content_moderation.cyber_update_effects_failed", "log_id", log.ID, "error", err)
	}
	return true
}

func (s *ContentModerationService) sendCyberPolicyEmail(ctx context.Context, log *ContentModerationLog) error {
	siteName := s.siteName(ctx)
	if s.emailService.notificationEmailService != nil {
		variables := map[string]string{
			"triggered_at":     log.CreatedAt.UTC().Format(time.RFC3339),
			"model":            defaultContentModerationString(log.Model, "-"),
			"group_name":       defaultContentModerationString(log.GroupName, "-"),
			"upstream_message": defaultContentModerationString(log.Error, "-"),
		}
		err := s.emailService.notificationEmailService.Send(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventCyberPolicyNotice,
			RecipientEmail: log.UserEmail,
			RecipientName:  emailRecipientName(log.UserEmail),
			UserID:         contentModerationEmailUserID(log),
			SourceType:     "content_moderation",
			SourceID:       contentModerationEmailSourceID(log),
			Variables:      variables,
		})
		if err == nil {
			return nil
		}
		if !shouldFallbackNotificationEmail(err) {
			return err
		}
		slog.Warn("template cyber policy email failed; falling back", "err", err.Error())
	}
	subject := fmt.Sprintf("[%s] 网络安全策略拦截 / Cyber Policy Notice", sanitizeEmailHeader(siteName))
	return s.emailService.SendEmail(ctx, log.UserEmail, subject, buildCyberPolicyNoticeEmailBody(siteName, log))
}
