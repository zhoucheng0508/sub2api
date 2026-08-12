package handler

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	contentModerationIdentityHeaderMaxNames      = 24
	contentModerationIdentityHeaderMaxNameBytes  = 128
	contentModerationIdentityHeaderMaxValues     = 4
	contentModerationIdentityHeaderMaxValueBytes = 1024
	contentModerationIdentityHeaderMaxTotalBytes = 8 * 1024
)

var contentModerationIdentityHeaderPriority = []string{
	"user-agent",
	"originator",
}

func contentModerationStatus(decision *service.ContentModerationDecision) int {
	if decision == nil || decision.StatusCode < 400 || decision.StatusCode > 599 {
		return http.StatusForbidden
	}
	return decision.StatusCode
}

func contentModerationErrorCode(decision *service.ContentModerationDecision) string {
	if decision != nil && decision.Action == service.ContentModerationActionUnavailable {
		return service.ContentModerationErrorCodeUnavailable
	}
	return "content_policy_violation"
}

func clientRequestedModel(c *gin.Context, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if c == nil || c.Request == nil {
		return fallback
	}
	if model, ok := service.RequestedPublicModelFromContext(c.Request.Context()); ok {
		return model
	}
	return fallback
}

func clientRequestedUsageFields(c *gin.Context, mapping service.ChannelMappingResult, fallbackModel, upstreamModel string) service.ChannelUsageFields {
	return mapping.ToUsageFields(clientRequestedModel(c, fallbackModel), upstreamModel)
}

func runContentModeration(c *gin.Context, reqLog *zap.Logger, svc *service.ContentModerationService, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol string, model string, body []byte) *service.ContentModerationDecision {
	if svc == nil || c == nil || c.Request == nil {
		return nil
	}
	input := buildContentModerationInput(c, apiKey, subject, protocol, model, body)
	if reqLog != nil {
		reqLog.Info("content_moderation.gateway_check_start",
			zap.String("moderation_event_id", input.RequestID),
			zap.Int64("user_id", input.UserID),
			zap.Int64("api_key_id", input.APIKeyID),
			zap.String("api_key_name", input.APIKeyName),
			zap.Int64p("group_id", input.GroupID),
			zap.String("group_name", input.GroupName),
			zap.String("endpoint", input.Endpoint),
			zap.String("provider", input.Provider),
			zap.String("protocol", input.Protocol),
			zap.String("model", input.Model),
			zap.Int("body_bytes", len(body)),
		)
	}
	decision, err := svc.Check(c.Request.Context(), input)
	if err != nil {
		if reqLog != nil {
			reqLog.Warn("content_moderation.check_failed", zap.Error(err))
		}
		return nil
	}
	if reqLog != nil && decision != nil {
		reqLog.Info("content_moderation.gateway_check_done",
			zap.String("moderation_event_id", input.RequestID),
			zap.Bool("allowed", decision.Allowed),
			zap.Bool("blocked", decision.Blocked),
			zap.Bool("flagged", decision.Flagged),
			zap.String("action", decision.Action),
			zap.Int("status_code", decision.StatusCode),
			zap.String("highest_category", decision.HighestCategory),
			zap.Float64("highest_score", decision.HighestScore),
		)
	}
	return decision
}

func buildContentModerationInput(c *gin.Context, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol string, model string, body []byte) service.ContentModerationCheckInput {
	sessionID, sessionSource := service.ExtractContentModerationSessionIdentity(c, body)
	input := service.ContentModerationCheckInput{
		RequestID:     newContentModerationEventID(),
		UserID:        subject.UserID,
		Endpoint:      GetInboundEndpoint(c),
		Provider:      contentModerationProvider(apiKey),
		Model:         clientRequestedModel(c, model),
		Protocol:      protocol,
		Body:          body,
		SessionID:     sessionID,
		SessionSource: sessionSource,
		ClientHeaders: contentModerationClientHeaders(c.Request.Header),
		// Transport identity values remain available as bounded, untrusted
		// observations. A network client cannot attest metadata provenance.
		TrustedMetadataProvenance: false,
	}
	if epoch, epochSet, captured := service.GetOpsCyberPolicyEpochSnapshot(c); captured {
		input.ModerationEpoch = epoch
		input.ModerationEpochSet = epochSet
	}
	if resolvedPlatform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context()); ok {
		input.Provider = resolvedPlatform
	}
	if forcedPlatform, ok := middleware2.GetForcePlatformFromContext(c); ok {
		input.Provider = strings.TrimSpace(forcedPlatform)
	}
	if apiKey != nil {
		input.APIKeyID = apiKey.ID
		input.APIKeyName = apiKey.Name
		if apiKey.User != nil {
			input.UserEmail = apiKey.User.Email
		}
		if apiKey.GroupID != nil {
			groupID := *apiKey.GroupID
			input.GroupID = &groupID
		}
		if apiKey.Group != nil {
			input.GroupName = apiKey.Group.Name
		}
	}
	if input.Endpoint == "" && c.Request != nil && c.Request.URL != nil {
		input.Endpoint = c.Request.URL.Path
	}
	return input
}

// contentModerationClientHeaders snapshots only bounded, untrusted transport
// identity observations. Credentials and arbitrary request headers must never
// cross this boundary.
func contentModerationClientHeaders(source http.Header) http.Header {
	if len(source) == 0 {
		return nil
	}

	type candidate struct {
		name   string
		values []string
	}
	candidates := make(map[string]*candidate)
	for rawName, values := range source {
		name := strings.TrimSpace(rawName)
		lowerName := strings.ToLower(name)
		if !isContentModerationIdentityHeader(lowerName) || len(name) > contentModerationIdentityHeaderMaxNameBytes {
			continue
		}
		canonicalName := http.CanonicalHeaderKey(name)
		if canonicalName == "" {
			continue
		}
		entry := candidates[lowerName]
		if entry == nil {
			entry = &candidate{name: canonicalName}
			candidates[lowerName] = entry
		}
		entry.values = append(entry.values, values...)
	}

	orderedNames := make([]string, 0, len(candidates))
	for _, name := range contentModerationIdentityHeaderPriority {
		if _, ok := candidates[name]; ok {
			orderedNames = append(orderedNames, name)
		}
	}
	var remaining []string
	for name := range candidates {
		if name != "user-agent" && name != "originator" {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	orderedNames = append(orderedNames, remaining...)

	result := make(http.Header)
	totalBytes := 0
	for _, name := range orderedNames {
		if len(result) >= contentModerationIdentityHeaderMaxNames {
			break
		}
		entry := candidates[name]
		values, valueBytes, ok := boundedContentModerationHeaderValues(entry.values)
		if !ok {
			continue
		}
		entryBytes := len(entry.name) + valueBytes
		if totalBytes+entryBytes > contentModerationIdentityHeaderMaxTotalBytes {
			continue
		}
		result[entry.name] = values
		totalBytes += entryBytes
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func isContentModerationIdentityHeader(lowerName string) bool {
	if lowerName == "user-agent" || lowerName == "originator" {
		return true
	}
	if !strings.HasPrefix(lowerName, "x-codex-") {
		return false
	}
	for _, sensitiveMarker := range []string{
		"authorization",
		"cookie",
		"api-key",
		"apikey",
		"access-token",
		"refresh-token",
		"credential",
		"secret",
	} {
		if strings.Contains(lowerName, sensitiveMarker) {
			return false
		}
	}
	return true
}

func boundedContentModerationHeaderValues(values []string) ([]string, int, bool) {
	if len(values) == 0 || len(values) > contentModerationIdentityHeaderMaxValues {
		return nil, 0, false
	}
	result := make([]string, 0, len(values))
	totalBytes := 0
	for _, value := range values {
		if strings.TrimSpace(value) == "" || len(value) > contentModerationIdentityHeaderMaxValueBytes {
			return nil, 0, false
		}
		result = append(result, value)
		totalBytes += len(value)
	}
	return result, totalBytes, true
}

func contentModerationProvider(apiKey *service.APIKey) string {
	if apiKey == nil || apiKey.Group == nil {
		return ""
	}
	return strings.TrimSpace(apiKey.Group.Platform)
}

func newContentModerationEventID() string {
	return uuid.NewString()
}

func contentModerationRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if requestID, ok := ctx.Value(ctxkey.RequestID).(string); ok {
		return strings.TrimSpace(requestID)
	}
	return ""
}
