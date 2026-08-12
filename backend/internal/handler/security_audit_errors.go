package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIGatewayHandler) openAISecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	if securityAuditResponseCommitted(c) {
		h.handleStreamingAwareErrorWithCode(c, securityAuditStatus(decision), securityAuditStreamErrorType(decision), securityAuditErrorCode(decision), securityAuditMessage(decision), true, false)
		return
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		h.errorResponse(c, securityAuditStatus(decision), securityAuditErrorCode(decision), securityAuditMessage(decision))
		return
	}
	errType := "api_error"
	if decision.Kind == securityaudit.DecisionBlock {
		errType = "permission_error"
	}
	c.JSON(securityAuditStatus(decision), gin.H{"error": gin.H{
		"type": errType, "code": securityAuditErrorCode(decision), "message": securityAuditMessage(decision),
	}})
}

func (h *GatewayHandler) openAISecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	if securityAuditResponseCommitted(c) {
		h.handleStreamingAwareError(c, securityAuditStatus(decision), securityAuditStreamErrorType(decision), securityAuditMessage(decision), true)
		return
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		h.chatCompletionsErrorResponse(c, securityAuditStatus(decision), securityAuditErrorCode(decision), securityAuditMessage(decision))
		return
	}
	errType := "api_error"
	if decision.Kind == securityaudit.DecisionBlock {
		errType = "permission_error"
	}
	c.JSON(securityAuditStatus(decision), gin.H{"error": gin.H{
		"type": errType, "code": securityAuditErrorCode(decision), "message": securityAuditMessage(decision),
	}})
}

func (h *GatewayHandler) responsesSecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	if securityAuditResponseCommitted(c) {
		h.handleStreamingAwareError(c, securityAuditStatus(decision), securityAuditStreamErrorType(decision), securityAuditMessage(decision), true)
		return
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		h.responsesErrorResponse(c, securityAuditStatus(decision), securityAuditErrorCode(decision), securityAuditMessage(decision))
		return
	}
	c.JSON(securityAuditStatus(decision), gin.H{"error": gin.H{
		"type": "api_error", "code": securityAuditErrorCode(decision), "message": securityAuditMessage(decision),
	}})
}

func (h *GatewayHandler) anthropicSecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	if securityAuditResponseCommitted(c) {
		h.handleStreamingAwareError(c, securityAuditStatus(decision), securityAuditStreamErrorType(decision), securityAuditMessage(decision), true)
		return
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		h.errorResponse(c, securityAuditStatus(decision), securityAuditErrorCode(decision), securityAuditMessage(decision))
		return
	}
	errType := "api_error"
	if decision.Kind == securityaudit.DecisionBlock {
		errType = "permission_error"
	}
	c.JSON(securityAuditStatus(decision), gin.H{"type": "error", "error": gin.H{
		"type": errType, "code": securityAuditErrorCode(decision), "message": securityAuditMessage(decision),
	}})
}

func (h *OpenAIGatewayHandler) anthropicSecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	if securityAuditResponseCommitted(c) {
		h.handleStreamingAwareErrorWithCode(c, securityAuditStatus(decision), securityAuditStreamErrorType(decision), securityAuditErrorCode(decision), securityAuditMessage(decision), true, false)
		return
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		h.anthropicErrorResponse(c, securityAuditStatus(decision), securityAuditErrorCode(decision), securityAuditMessage(decision))
		return
	}
	errType := "api_error"
	if decision.Kind == securityaudit.DecisionBlock {
		errType = "permission_error"
	}
	c.JSON(securityAuditStatus(decision), gin.H{"type": "error", "error": gin.H{
		"type": errType, "code": securityAuditErrorCode(decision), "message": securityAuditMessage(decision),
	}})
}

func securityAuditStreamErrorType(decision *securityaudit.Decision) string {
	if decision == nil {
		return "api_error"
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		if decision.Legacy.Action == service.ContentModerationActionUnavailable || decision.Legacy.ErrorCode == service.ContentModerationErrorCodeUnavailable {
			return "api_error"
		}
		return securityAuditErrorCode(decision)
	}
	if decision.Kind == securityaudit.DecisionBlock {
		return "permission_error"
	}
	return "api_error"
}

func securityAuditResponseCommitted(c *gin.Context) bool {
	return c != nil && c.Writer != nil && (c.Writer.Written() || service.IsResponseCommitted(c))
}

func googleSecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	if securityAuditResponseCommitted(c) {
		status := securityAuditStatus(decision)
		googleStatus := googleapi.HTTPStatusToGoogleStatus(status)
		if status == http.StatusServiceUnavailable {
			googleStatus = "UNAVAILABLE"
		}
		requestID := ""
		if c.Request != nil {
			requestID = contentModerationRequestID(c.Request.Context())
		}
		payload, err := json.Marshal(gin.H{"error": gin.H{
			"code": status, "message": securityAuditMessage(decision), "status": googleStatus,
			"details": []gin.H{{
				"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
				"reason": securityAuditErrorCode(decision), "domain": "sub2api.securityaudit",
				"metadata": gin.H{"request_id": requestID},
			}},
		}})
		if err != nil {
			_ = c.Error(err)
			return
		}
		service.MarkOpsStreamError(c, securityAuditStreamErrorType(decision), securityAuditMessage(decision), status)
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
			_ = c.Error(err)
			return
		}
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		googleError(c, securityAuditStatus(decision), securityAuditMessage(decision))
		return
	}
	status := securityAuditStatus(decision)
	googleStatus := googleapi.HTTPStatusToGoogleStatus(status)
	if status == http.StatusServiceUnavailable {
		googleStatus = "UNAVAILABLE"
	}
	requestID := ""
	if c != nil && c.Request != nil {
		requestID = contentModerationRequestID(c.Request.Context())
	}
	c.JSON(status, gin.H{"error": gin.H{
		"code": status, "message": securityAuditMessage(decision), "status": googleStatus,
		"details": []gin.H{{
			"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
			"reason": securityAuditErrorCode(decision), "domain": "sub2api.securityaudit",
			"metadata": gin.H{"request_id": requestID},
		}},
	}})
}

func writeSecurityAuditWSError(ctx context.Context, conn *coderws.Conn, decision *securityaudit.Decision) {
	if conn == nil || decision == nil {
		return
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		legacy := decision.Legacy
		writeContentModerationWSError(ctx, conn, (legacyContentModerationDecision{legacy}).toService())
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := json.Marshal(gin.H{
		"event_id": "evt_prompt_guard_rejected", "type": "error",
		"error": gin.H{"type": "invalid_request_error", "code": securityAuditErrorCode(decision), "message": securityAuditMessage(decision)},
	})
	if err != nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = conn.Write(writeCtx, coderws.MessageText, payload)
}

type legacyContentModerationDecision struct{ value *securityaudit.LegacyDecision }

func (d legacyContentModerationDecision) toService() *service.ContentModerationDecision {
	if d.value == nil {
		return nil
	}
	return &service.ContentModerationDecision{Allowed: d.value.Allowed, Blocked: d.value.Blocked, Flagged: d.value.Flagged, Message: d.value.Message, StatusCode: d.value.StatusCode, Action: d.value.Action}
}

func securityAuditWSCloseStatus(decision *securityaudit.Decision) coderws.StatusCode {
	if decision == nil {
		return coderws.StatusInternalError
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		if decision.Legacy.Action == service.ContentModerationActionUnavailable || decision.Legacy.ErrorCode == service.ContentModerationErrorCodeUnavailable {
			return coderws.StatusTryAgainLater
		}
		return coderws.StatusPolicyViolation
	}
	if decision.Kind == securityaudit.DecisionBlock {
		return coderws.StatusCode(4403)
	}
	return coderws.StatusTryAgainLater
}

func securityAuditWSCloseReason(decision *securityaudit.Decision) string {
	if decision == nil {
		return securityaudit.ErrorCodeUnavailable
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		if decision.Legacy.Action == service.ContentModerationActionUnavailable || decision.Legacy.ErrorCode == service.ContentModerationErrorCodeUnavailable {
			return service.ContentModerationErrorCodeUnavailable
		}
		message := strings.TrimSpace(decision.Legacy.Message)
		if message != "" {
			return message
		}
		return "content_policy_violation"
	}
	code := securityAuditErrorCode(decision)
	if code == "" {
		return securityaudit.ErrorCodeUnavailable
	}
	return code
}
