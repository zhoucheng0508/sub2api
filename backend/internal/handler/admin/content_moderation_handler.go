package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ContentModerationHandler struct {
	service *service.ContentModerationService
}

func NewContentModerationHandler(svc *service.ContentModerationService) *ContentModerationHandler {
	return &ContentModerationHandler{service: svc}
}

type contentModerationConfigRequest struct {
	Enabled       *bool   `json:"enabled"`
	Mode          *string `json:"mode"`
	AuditProvider *string `json:"audit_provider"`
	BaseURL       *string `json:"base_url"`
	Model         *string `json:"model"`
	// 审计请求使用的代理服务器：null 不修改；0 清除（直连）；>0 指定代理。
	ProxyID              *int64              `json:"proxy_id"`
	APIKey               *string             `json:"api_key"`
	APIKeys              *[]string           `json:"api_keys"`
	APIKeysMode          string              `json:"api_keys_mode"`
	DeleteAPIKeyHashes   *[]string           `json:"delete_api_key_hashes"`
	ClearAPIKey          bool                `json:"clear_api_key"`
	TimeoutMS            *int                `json:"timeout_ms"`
	SampleRate           *int                `json:"sample_rate"`
	AllGroups            *bool               `json:"all_groups"`
	GroupIDs             *[]int64            `json:"group_ids"`
	RecordNonHits        *bool               `json:"record_non_hits"`
	Thresholds           *map[string]float64 `json:"thresholds"`
	WorkerCount          *int                `json:"worker_count"`
	QueueSize            *int                `json:"queue_size"`
	BlockStatus          *int                `json:"block_status"`
	BlockMessage         *string             `json:"block_message"`
	EmailOnHit           *bool               `json:"email_on_hit"`
	AutoBanEnabled       *bool               `json:"auto_ban_enabled"`
	BanThreshold         *int                `json:"ban_threshold"`
	ViolationWindowHours *int                `json:"violation_window_hours"`
	// cyber_policy 命中是否排除出自动封号计数；前端 RiskControlView 已发送该字段，
	// service.UpdateContentModerationConfigInput 已支持，此前 handler 层缺透传导致开关静默失效。
	CyberPolicyExcludeFromBanCount    *bool                                   `json:"cyber_policy_exclude_from_ban_count"`
	RetryCount                        *int                                    `json:"retry_count"`
	HitRetentionDays                  *int                                    `json:"hit_retention_days"`
	NonHitRetentionDays               *int                                    `json:"non_hit_retention_days"`
	PreHashCheckEnabled               *bool                                   `json:"pre_hash_check_enabled"`
	BlockedKeywords                   *[]string                               `json:"blocked_keywords"`
	KeywordBlockingMode               *string                                 `json:"keyword_blocking_mode"`
	ModelFilter                       *service.ContentModerationModelFilter   `json:"model_filter"`
	UserFilter                        *service.ContentModerationUserFilter    `json:"user_filter"`
	AccountFilter                     *service.ContentModerationAccountFilter `json:"account_filter"`
	AIConfidenceThreshold             *float64                                `json:"ai_confidence_threshold"`
	AICacheEnabled                    *bool                                   `json:"ai_cache_enabled"`
	AICacheTTLSeconds                 *int                                    `json:"ai_cache_ttl_seconds"`
	AISystemPrompt                    *string                                 `json:"ai_system_prompt"`
	AIFailurePolicy                   *string                                 `json:"ai_failure_policy"`
	AIMaxInputChars                   *int                                    `json:"ai_max_input_chars"`
	AIThinkingMode                    *string                                 `json:"ai_thinking_mode"`
	AIReasoningEffort                 *string                                 `json:"ai_reasoning_effort"`
	AIRiskLevelsEnabled               *bool                                   `json:"ai_risk_levels_enabled"`
	AIObserveThreshold                *float64                                `json:"ai_observe_threshold"`
	AISessionRiskEnabled              *bool                                   `json:"ai_session_risk_enabled"`
	AISessionRiskTTLMinutes           *int                                    `json:"ai_session_risk_ttl_minutes"`
	AISessionRiskHalfLifeMinutes      *int                                    `json:"ai_session_risk_half_life_minutes"`
	AISessionRiskBlockCooldownMinutes *int                                    `json:"ai_session_risk_block_cooldown_minutes"`
	AIActorRiskEnabled                *bool                                   `json:"ai_actor_risk_enabled"`
}

type contentModerationAPIKeyTestRequest struct {
	APIKeys               []string `json:"api_keys"`
	AuditProvider         string   `json:"audit_provider"`
	BaseURL               string   `json:"base_url"`
	Model                 string   `json:"model"`
	TimeoutMS             int      `json:"timeout_ms"`
	ProxyID               *int64   `json:"proxy_id"`
	Prompt                string   `json:"prompt"`
	Images                []string `json:"images"`
	AIConfidenceThreshold float64  `json:"ai_confidence_threshold"`
	AISystemPrompt        string   `json:"ai_system_prompt"`
	AIMaxInputChars       int      `json:"ai_max_input_chars"`
	AIThinkingMode        string   `json:"ai_thinking_mode"`
	AIReasoningEffort     string   `json:"ai_reasoning_effort"`
}

type contentModerationHashRequest struct {
	InputHash string `json:"input_hash"`
}

func (h *ContentModerationHandler) GetConfig(c *gin.Context) {
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *ContentModerationHandler) UpdateConfig(c *gin.Context) {
	var req contentModerationConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.service.UpdateConfig(c.Request.Context(), service.UpdateContentModerationConfigInput{
		Enabled:                           req.Enabled,
		Mode:                              req.Mode,
		AuditProvider:                     req.AuditProvider,
		BaseURL:                           req.BaseURL,
		Model:                             req.Model,
		ProxyID:                           req.ProxyID,
		APIKey:                            req.APIKey,
		APIKeys:                           req.APIKeys,
		APIKeysMode:                       req.APIKeysMode,
		DeleteAPIKeyHashes:                req.DeleteAPIKeyHashes,
		ClearAPIKey:                       req.ClearAPIKey,
		TimeoutMS:                         req.TimeoutMS,
		SampleRate:                        req.SampleRate,
		AllGroups:                         req.AllGroups,
		GroupIDs:                          req.GroupIDs,
		RecordNonHits:                     req.RecordNonHits,
		Thresholds:                        req.Thresholds,
		WorkerCount:                       req.WorkerCount,
		QueueSize:                         req.QueueSize,
		BlockStatus:                       req.BlockStatus,
		BlockMessage:                      req.BlockMessage,
		EmailOnHit:                        req.EmailOnHit,
		AutoBanEnabled:                    req.AutoBanEnabled,
		BanThreshold:                      req.BanThreshold,
		ViolationWindowHours:              req.ViolationWindowHours,
		CyberPolicyExcludeFromBanCount:    req.CyberPolicyExcludeFromBanCount,
		RetryCount:                        req.RetryCount,
		HitRetentionDays:                  req.HitRetentionDays,
		NonHitRetentionDays:               req.NonHitRetentionDays,
		PreHashCheckEnabled:               req.PreHashCheckEnabled,
		BlockedKeywords:                   req.BlockedKeywords,
		KeywordBlockingMode:               req.KeywordBlockingMode,
		ModelFilter:                       req.ModelFilter,
		UserFilter:                        req.UserFilter,
		AccountFilter:                     req.AccountFilter,
		AIConfidenceThreshold:             req.AIConfidenceThreshold,
		AICacheEnabled:                    req.AICacheEnabled,
		AICacheTTLSeconds:                 req.AICacheTTLSeconds,
		AISystemPrompt:                    req.AISystemPrompt,
		AIFailurePolicy:                   req.AIFailurePolicy,
		AIMaxInputChars:                   req.AIMaxInputChars,
		AIThinkingMode:                    req.AIThinkingMode,
		AIReasoningEffort:                 req.AIReasoningEffort,
		AIRiskLevelsEnabled:               req.AIRiskLevelsEnabled,
		AIObserveThreshold:                req.AIObserveThreshold,
		AISessionRiskEnabled:              req.AISessionRiskEnabled,
		AISessionRiskTTLMinutes:           req.AISessionRiskTTLMinutes,
		AISessionRiskHalfLifeMinutes:      req.AISessionRiskHalfLifeMinutes,
		AISessionRiskBlockCooldownMinutes: req.AISessionRiskBlockCooldownMinutes,
		AIActorRiskEnabled:                req.AIActorRiskEnabled,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *ContentModerationHandler) TestAPIKeys(c *gin.Context) {
	var req contentModerationAPIKeyTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.service.TestAPIKeys(c.Request.Context(), service.TestContentModerationAPIKeysInput{
		APIKeys:               req.APIKeys,
		AuditProvider:         req.AuditProvider,
		BaseURL:               req.BaseURL,
		Model:                 req.Model,
		TimeoutMS:             req.TimeoutMS,
		ProxyID:               req.ProxyID,
		Prompt:                req.Prompt,
		Images:                req.Images,
		AIConfidenceThreshold: req.AIConfidenceThreshold,
		AISystemPrompt:        req.AISystemPrompt,
		AIMaxInputChars:       req.AIMaxInputChars,
		AIThinkingMode:        req.AIThinkingMode,
		AIReasoningEffort:     req.AIReasoningEffort,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) GetStatus(c *gin.Context) {
	status, err := h.service.GetStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *ContentModerationHandler) ListLogs(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := service.ContentModerationLogFilter{
		Pagination: pagination.PaginationParams{
			Page:      page,
			PageSize:  pageSize,
			SortOrder: pagination.SortOrderDesc,
		},
		Result:   c.Query("result"),
		Endpoint: c.Query("endpoint"),
		Search:   c.Query("search"),
	}
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		groupID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || groupID <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &groupID
	}
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		t, _, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid from")
			return
		}
		filter.From = &t
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		t, dateOnly, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid to")
			return
		}
		if dateOnly {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		filter.To = &t
	}
	items, pageResult, err := h.service.ListLogs(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, pageResult.Total, pageResult.Page, pageResult.PageSize)
}

func (h *ContentModerationHandler) UnbanUser(c *gin.Context) {
	userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user_id")
		return
	}
	result, err := h.service.UnbanUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) DeleteFlaggedHash(c *gin.Context) {
	var req contentModerationHashRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.service.DeleteFlaggedInputHash(c.Request.Context(), req.InputHash)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) ClearFlaggedHashes(c *gin.Context) {
	result, err := h.service.ClearFlaggedInputHashes(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func parseContentModerationDate(raw string) (time.Time, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, false, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	return t, err == nil, err
}
