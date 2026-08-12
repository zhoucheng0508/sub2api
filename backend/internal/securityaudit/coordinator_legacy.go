package securityaudit

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type LegacyModerationAdapter struct {
	service *service.ContentModerationService
}

func NewLegacyModerationAdapter(svc *service.ContentModerationService) LegacyEngine {
	return &LegacyModerationAdapter{service: svc}
}

func (a *LegacyModerationAdapter) Check(ctx context.Context, req Request) (*LegacyDecision, error) {
	if a == nil || a.service == nil {
		return nil, nil
	}
	decision, err := a.service.Check(ctx, service.ContentModerationCheckInput{
		RequestID: req.RequestID, SessionID: req.SessionID, SessionSource: req.SessionSource,
		UserID: req.UserID, UserEmail: req.UserEmail,
		APIKeyID: req.APIKeyID, APIKeyName: req.APIKeyName, AccountID: req.AccountID, GroupID: cloneInt64Ptr(req.GroupID),
		GroupName: req.GroupName, Endpoint: req.Endpoint, Provider: req.Provider,
		Model: req.Model, Protocol: req.Protocol, Body: req.Body,
		ClientHeaders: req.ClientHeaders.Clone(), TrustedMetadataProvenance: req.TrustedMetadataProvenance,
		ModerationEpoch: req.ModerationEpoch, ModerationEpochSet: req.ModerationEpochSet,
	})
	if err != nil || decision == nil {
		return nil, err
	}
	return &LegacyDecision{
		Allowed: decision.Allowed, Blocked: decision.Blocked, Flagged: decision.Flagged,
		Message: decision.Message, StatusCode: decision.StatusCode,
		ErrorCode: moderationErrorCode(decision), Action: decision.Action,
	}, nil
}

func moderationErrorCode(decision *service.ContentModerationDecision) string {
	if decision != nil && decision.Action == service.ContentModerationActionUnavailable {
		return service.ContentModerationErrorCodeUnavailable
	}
	return "content_policy_violation"
}
