package handler

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/internalprobe"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	securityAuditCompletedContextKey = "sub2api.security_audit.completed"
	securityAuditBypassContextKey    = "sub2api.security_audit.bypass"
)

// cachesSecurityAuditCompletion reports whether a successful audit may be
// reused for the rest of the gin request. WebSocket turns share one Context
// across many response.create frames and must be audited independently.
func cachesSecurityAuditCompletion(stage string) bool {
	switch strings.TrimSpace(stage) {
	case "", "http":
		return true
	default:
		return false
	}
}

func (h *GatewayHandler) checkSecurityAudit(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAudit(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject, protocol, model, body, "http")
}

func (h *GatewayHandler) checkSecurityAuditForAccount(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, account *service.Account, protocol, model string, body []byte) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAuditForAccount(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject, account, protocol, model, body, "http")
}

func (h *OpenAIGatewayHandler) checkSecurityAudit(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAudit(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject, protocol, model, body, "http")
}

func (h *OpenAIGatewayHandler) checkSecurityAuditForAccount(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, account *service.Account, protocol, model string, body []byte) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAuditForAccount(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject, account, protocol, model, body, "http")
}

func (h *OpenAIGatewayHandler) checkSecurityAuditStage(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte, stage string) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAudit(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject, protocol, model, body, stage)
}

func (h *OpenAIGatewayHandler) checkSecurityAuditStageForAccount(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, account *service.Account, protocol, model string, body []byte, stage string) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAuditForAccount(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject, account, protocol, model, body, stage)
}

func runSecurityAudit(c *gin.Context, reqLog *zap.Logger, coordinator *securityaudit.Coordinator, legacy *service.ContentModerationService, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte, stage string) *securityaudit.Decision {
	return runSecurityAuditForAccount(c, reqLog, coordinator, legacy, apiKey, subject, nil, protocol, model, body, stage)
}

func runSecurityAuditForAccount(c *gin.Context, reqLog *zap.Logger, coordinator *securityaudit.Coordinator, legacy *service.ContentModerationService, apiKey *service.APIKey, subject middleware2.AuthSubject, account *service.Account, protocol, model string, body []byte, stage string) *securityaudit.Decision {
	if c == nil || c.Request == nil {
		return nil
	}
	if reason, exists := c.Get(securityAuditBypassContextKey); exists && strings.TrimSpace(asSecurityAuditString(reason)) != "" {
		return nil
	}
	if internalprobe.IsMarked(c.Request.Context()) {
		c.Set(securityAuditBypassContextKey, "skip_internal_probe")
		logSecurityAuditScopeSkip(reqLog, "skip_internal_probe", subject.UserID, nil)
		return nil
	}
	cacheCompletion := cachesSecurityAuditCompletion(stage)
	if cacheCompletion {
		if completed, exists := c.Get(securityAuditCompletedContextKey); exists && completed == true {
			return nil
		}
	}
	if legacy != nil {
		shouldAuditUser, reason, err := legacy.ShouldAuditUser(c.Request.Context(), subject.UserID)
		if err != nil {
			if reqLog != nil {
				reqLog.Warn("security_audit.scope_user_check_failed", zap.Int64("user_id", subject.UserID), zap.Error(err))
			}
		} else if !shouldAuditUser {
			c.Set(securityAuditBypassContextKey, reason)
			logSecurityAuditScopeSkip(reqLog, reason, subject.UserID, nil)
			return nil
		}

		if account == nil {
			requiresAccount, err := legacy.RequiresAccountScopeResolution(c.Request.Context())
			if err != nil {
				if reqLog != nil {
					reqLog.Warn("security_audit.scope_account_mode_check_failed", zap.Error(err))
				}
			} else if requiresAccount {
				if reqLog != nil {
					reqLog.Debug("security_audit.scope_deferred_until_account_selection", zap.Int64("user_id", subject.UserID))
				}
				return nil
			}
		} else {
			scopeAccountID := securityAuditScopeAccountID(account)
			shouldAuditAccount, reason, err := legacy.ShouldAuditAccount(c.Request.Context(), scopeAccountID)
			if err != nil {
				if reqLog != nil {
					reqLog.Warn("security_audit.scope_account_check_failed",
						zap.Int64("selected_account_id", account.ID),
						zap.Int64("scope_account_id", scopeAccountID),
						zap.Error(err))
				}
			} else if !shouldAuditAccount {
				logSecurityAuditScopeSkip(reqLog, reason, subject.UserID, account)
				return nil
			}
		}
	}
	if coordinator == nil {
		legacyDecision := runContentModeration(c, reqLog, legacy, apiKey, subject, protocol, model, body)
		if legacyDecision == nil {
			return nil
		}
		decision := securityaudit.Decision{Kind: securityaudit.DecisionAllow, HTTPStatus: http.StatusOK, AllowNextStage: true}
		decision.Legacy = &securityaudit.LegacyDecision{
			Allowed: legacyDecision.Allowed, Blocked: legacyDecision.Blocked, Flagged: legacyDecision.Flagged,
			Message: legacyDecision.Message, StatusCode: legacyDecision.StatusCode,
			ErrorCode: "content_policy_violation", Action: legacyDecision.Action,
		}
		if legacyDecision.Blocked {
			decision.Kind, decision.HTTPStatus, decision.ErrorCode, decision.ClientMessage, decision.AllowNextStage = securityaudit.DecisionBlock, contentModerationStatus(legacyDecision), "content_policy_violation", legacyDecision.Message, false
		}
		if decision.AllowNextStage && cacheCompletion {
			c.Set(securityAuditCompletedContextKey, true)
		}
		return &decision
	}
	request := buildSecurityAuditRequestForAccount(c, apiKey, subject, account, protocol, model, body, stage)
	if reqLog != nil {
		reqLog.Info("security_audit.gateway_check_start",
			zap.String("request_id", request.RequestID), zap.Int64("user_id", request.UserID),
			zap.Int64("api_key_id", request.APIKeyID), zap.Int64p("group_id", request.GroupID),
			zap.String("endpoint", request.Endpoint), zap.String("provider", request.Provider),
			zap.String("protocol", request.Protocol), zap.String("model", request.Model), zap.String("stage", request.Stage),
			zap.Int64("selected_account_id", request.AccountID), zap.String("selected_account_name", request.AccountName),
			zap.Int64("scope_account_id", securityAuditScopeAccountID(account)),
			zap.Int("body_bytes", len(body)))
	}
	decision := coordinator.Check(c.Request.Context(), request)
	if decision.AllowNextStage && cacheCompletion {
		c.Set(securityAuditCompletedContextKey, true)
	}
	if reqLog != nil {
		reqLog.Info("security_audit.gateway_check_done",
			zap.String("request_id", request.RequestID), zap.String("decision", string(decision.Kind)),
			zap.String("error_code", decision.ErrorCode), zap.Bool("allow_next_stage", decision.AllowNextStage),
			zap.String("stage", request.Stage))
	}
	return &decision
}

func buildSecurityAuditRequest(c *gin.Context, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte, stage string) securityaudit.Request {
	return buildSecurityAuditRequestForAccount(c, apiKey, subject, nil, protocol, model, body, stage)
}

func buildSecurityAuditRequestForAccount(c *gin.Context, apiKey *service.APIKey, subject middleware2.AuthSubject, account *service.Account, protocol, model string, body []byte, stage string) securityaudit.Request {
	legacy := buildContentModerationInput(c, apiKey, subject, protocol, model, body)
	request := securityaudit.Request{
		RequestID: legacy.RequestID, SessionID: legacy.SessionID, UserID: legacy.UserID, UserEmail: legacy.UserEmail,
		APIKeyID: legacy.APIKeyID, APIKeyName: legacy.APIKeyName, GroupID: cloneSecurityAuditGroupID(legacy.GroupID),
		GroupName: legacy.GroupName, Provider: legacy.Provider, Endpoint: legacy.Endpoint,
		Protocol: legacy.Protocol, Model: legacy.Model, Body: body, Stage: strings.TrimSpace(stage),
	}
	if account != nil {
		request.AccountID = account.ID
		request.AccountName = account.Name
	}
	if apiKey != nil && apiKey.User != nil {
		request.Username = apiKey.User.Username
		if request.UserEmail == "" {
			request.UserEmail = apiKey.User.Email
		}
	}
	if request.Stage == "" {
		request.Stage = "http"
	}
	return request
}

func logSecurityAuditScopeSkip(reqLog *zap.Logger, reason string, userID int64, account *service.Account) {
	if reqLog == nil {
		return
	}
	fields := []zap.Field{zap.String("reason", strings.TrimSpace(reason)), zap.Int64("user_id", userID)}
	if account != nil {
		fields = append(fields,
			zap.Int64("selected_account_id", account.ID),
			zap.String("selected_account_name", account.Name),
			zap.Int64("scope_account_id", securityAuditScopeAccountID(account)))
	}
	reqLog.Info("security_audit.scope_skipped", fields...)
}

func securityAuditScopeAccountID(account *service.Account) int64 {
	if account == nil {
		return 0
	}
	if account.ParentAccountID != nil && *account.ParentAccountID > 0 {
		return *account.ParentAccountID
	}
	return account.ID
}

func asSecurityAuditString(value any) string {
	text, _ := value.(string)
	return text
}

func releaseSecurityAuditSelection(selection *service.AccountSelectionResult) {
	if selection == nil || selection.ReleaseFunc == nil {
		return
	}
	selection.ReleaseFunc()
	selection.ReleaseFunc = nil
	selection.Acquired = false
}

func securityAuditStatus(decision *securityaudit.Decision) int {
	if decision == nil || decision.HTTPStatus < 400 || decision.HTTPStatus > 599 {
		return http.StatusForbidden
	}
	return decision.HTTPStatus
}

func securityAuditErrorCode(decision *securityaudit.Decision) string {
	if decision == nil || strings.TrimSpace(decision.ErrorCode) == "" {
		return "content_policy_violation"
	}
	return decision.ErrorCode
}

func securityAuditMessage(decision *securityaudit.Decision) string {
	if decision == nil {
		return "Request blocked by content policy"
	}
	if decision.Legacy != nil && decision.Legacy.Blocked && strings.TrimSpace(decision.Legacy.Message) != "" {
		return decision.Legacy.Message
	}
	if strings.TrimSpace(decision.ClientMessage) != "" {
		return decision.ClientMessage
	}
	return "Request blocked by content policy"
}

func cloneSecurityAuditGroupID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
