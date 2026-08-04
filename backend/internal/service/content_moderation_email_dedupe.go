package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	contentModerationEmailInputDedupeTTL   = 10 * time.Minute
	contentModerationEmailSessionDedupeTTL = 30 * time.Minute
)

const (
	contentModerationEmailDedupeStateAcquired     = "acquired"
	contentModerationEmailDedupeStateDeduplicated = "deduplicated"
	contentModerationEmailDedupeStateFailOpen     = "fail_open"
	contentModerationEmailDedupeStateNoScope      = "no_scope"
)

const (
	contentModerationEmailDedupeScopeInput   = "input"
	contentModerationEmailDedupeScopeSession = "session"
)

type contentModerationEmailRiskLevel int

const (
	contentModerationEmailRiskLow contentModerationEmailRiskLevel = iota
	contentModerationEmailRiskObserve
	contentModerationEmailRiskHigh
	contentModerationEmailRiskCritical
)

var errContentModerationEmailDedupeStoreUnavailable = errors.New("content moderation email dedupe store is unavailable")

// ContentModerationEmailDedupeStore is an optional Redis-backed capability of
// ContentModerationHashCache. Implementations atomically reserve a risk level
// and keep a per-user index so an administrative unban can clear only that
// user's short-lived notification state.
type ContentModerationEmailDedupeStore interface {
	TryReserveContentModerationEmail(ctx context.Context, userID int64, scopes []ContentModerationEmailDedupeScope, riskRank int) (ContentModerationEmailDedupeReserveResult, error)
	ReleaseContentModerationEmail(ctx context.Context, lease ContentModerationEmailDedupeLease) (int64, error)
	ClearContentModerationEmailDedupe(ctx context.Context, userID int64) (int64, error)
}

type ContentModerationEmailDedupeScope struct {
	Hash string
	TTL  time.Duration
}

type ContentModerationEmailDedupeLease struct {
	UserID   int64
	Token    string
	Scopes   []ContentModerationEmailDedupeScope
	RiskRank int
}

type ContentModerationEmailDedupeReserveResult struct {
	Acquired           bool
	ConflictScopeIndex int
	Lease              *ContentModerationEmailDedupeLease
}

type contentModerationEmailDedupeRequest struct {
	UserID    int64
	APIKeyID  int64
	SessionID string
	InputHash string
	RiskLevel contentModerationEmailRiskLevel
}

type contentModerationEmailDedupeDecision struct {
	ShouldSend bool
	State      string
	Scope      string
	RiskLevel  string
	FailOpen   bool
	Error      string
	Lease      *ContentModerationEmailDedupeLease
}

func (s *ContentModerationService) reserveContentModerationEmailNotification(ctx context.Context, request contentModerationEmailDedupeRequest) contentModerationEmailDedupeDecision {
	store, ok := contentModerationEmailDedupeStoreFromService(s)
	if !ok {
		return contentModerationEmailDedupeDecision{
			ShouldSend: true,
			State:      contentModerationEmailDedupeStateFailOpen,
			RiskLevel:  request.RiskLevel.String(),
			FailOpen:   true,
			Error:      errContentModerationEmailDedupeStoreUnavailable.Error(),
		}
	}
	return reserveContentModerationEmailNotification(ctx, store, request)
}

func (s *ContentModerationService) reserveContentModerationEmailForLog(ctx context.Context, log *ContentModerationLog) contentModerationEmailDedupeDecision {
	request := contentModerationEmailDedupeRequest{}
	if log != nil {
		request.UserID = contentModerationEmailUserID(log)
		if log.APIKeyID != nil {
			request.APIKeyID = *log.APIKeyID
		}
		request.SessionID = log.SessionID
		request.InputHash = log.InputHash
		request.RiskLevel = contentModerationEmailRiskLevelForLog(log)
	}
	return s.reserveContentModerationEmailNotification(ctx, request)
}

func contentModerationEmailDedupeDecisionError(decision contentModerationEmailDedupeDecision) error {
	if !decision.FailOpen || strings.TrimSpace(decision.Error) == "" {
		return nil
	}
	return fmt.Errorf("content moderation email dedupe %s failed open: %s", defaultContentModerationString(decision.Scope, "unknown"), decision.Error)
}

func reserveContentModerationEmailNotification(ctx context.Context, store ContentModerationEmailDedupeStore, request contentModerationEmailDedupeRequest) contentModerationEmailDedupeDecision {
	request.RiskLevel = request.RiskLevel.normalized()
	base := contentModerationEmailDedupeDecision{
		ShouldSend: true,
		State:      contentModerationEmailDedupeStateNoScope,
		RiskLevel:  request.RiskLevel.String(),
	}
	if store == nil {
		base.State = contentModerationEmailDedupeStateFailOpen
		base.FailOpen = true
		base.Error = errContentModerationEmailDedupeStoreUnavailable.Error()
		return base
	}

	type namedScope struct {
		name  string
		scope ContentModerationEmailDedupeScope
	}
	scopes := make([]namedScope, 0, 2)
	if request.UserID > 0 && request.APIKeyID > 0 && strings.TrimSpace(request.SessionID) != "" {
		scopes = append(scopes, namedScope{
			name: contentModerationEmailDedupeScopeSession,
			scope: ContentModerationEmailDedupeScope{
				Hash: contentModerationEmailScopeHash(
					contentModerationEmailDedupeScopeSession,
					fmt.Sprintf("%d", request.UserID),
					fmt.Sprintf("%d", request.APIKeyID),
					strings.TrimSpace(request.SessionID),
				),
				TTL: contentModerationEmailSessionDedupeTTL,
			},
		})
	}
	if request.UserID > 0 && strings.TrimSpace(request.InputHash) != "" {
		scopes = append(scopes, namedScope{
			name: contentModerationEmailDedupeScopeInput,
			scope: ContentModerationEmailDedupeScope{
				Hash: contentModerationEmailScopeHash(
					contentModerationEmailDedupeScopeInput,
					fmt.Sprintf("%d", request.UserID),
					strings.TrimSpace(request.InputHash),
				),
				TTL: contentModerationEmailInputDedupeTTL,
			},
		})
	}
	if len(scopes) == 0 {
		return base
	}

	storeScopes := make([]ContentModerationEmailDedupeScope, len(scopes))
	for i := range scopes {
		storeScopes[i] = scopes[i].scope
	}
	result, err := store.TryReserveContentModerationEmail(ctx, request.UserID, storeScopes, int(request.RiskLevel))
	if err != nil {
		return contentModerationEmailDedupeDecision{
			ShouldSend: true,
			State:      contentModerationEmailDedupeStateFailOpen,
			RiskLevel:  request.RiskLevel.String(),
			FailOpen:   true,
			Error:      trimRunes(err.Error(), 500),
		}
	}
	if !result.Acquired {
		conflictScope := ""
		if result.ConflictScopeIndex >= 0 && result.ConflictScopeIndex < len(scopes) {
			conflictScope = scopes[result.ConflictScopeIndex].name
		}
		return contentModerationEmailDedupeDecision{
			ShouldSend: false,
			State:      contentModerationEmailDedupeStateDeduplicated,
			Scope:      conflictScope,
			RiskLevel:  request.RiskLevel.String(),
		}
	}

	base.State = contentModerationEmailDedupeStateAcquired
	base.Scope = scopes[len(scopes)-1].name
	base.Lease = result.Lease
	return base
}

func (s *ContentModerationService) releaseContentModerationEmailReservation(ctx context.Context, decision contentModerationEmailDedupeDecision) error {
	if decision.Lease == nil {
		return nil
	}
	store, ok := contentModerationEmailDedupeStoreFromService(s)
	if !ok {
		return errContentModerationEmailDedupeStoreUnavailable
	}
	_, err := store.ReleaseContentModerationEmail(ctx, *decision.Lease)
	return err
}

func contentModerationEmailDedupeStoreFromService(s *ContentModerationService) (ContentModerationEmailDedupeStore, bool) {
	if s == nil || s.hashCache == nil {
		return nil, false
	}
	store, ok := s.hashCache.(ContentModerationEmailDedupeStore)
	return store, ok && store != nil
}

func contentModerationEmailRiskLevelForLog(log *ContentModerationLog) contentModerationEmailRiskLevel {
	if log == nil {
		return contentModerationEmailRiskLow
	}
	if log.Action == ContentModerationActionCyberPolicy || log.HighestScore >= 0.90 || log.AutoBanned {
		return contentModerationEmailRiskCritical
	}
	if log.Flagged || log.HighestScore >= 0.70 {
		return contentModerationEmailRiskHigh
	}
	if log.HighestScore >= 0.35 {
		return contentModerationEmailRiskObserve
	}
	return contentModerationEmailRiskLow
}

func contentModerationEmailScopeHash(kind string, values ...string) string {
	parts := append([]string{"vote-ai-email-dedupe-v1", strings.TrimSpace(kind)}, values...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (level contentModerationEmailRiskLevel) normalized() contentModerationEmailRiskLevel {
	if level < contentModerationEmailRiskLow || level > contentModerationEmailRiskCritical {
		return contentModerationEmailRiskHigh
	}
	return level
}

func (level contentModerationEmailRiskLevel) String() string {
	switch level.normalized() {
	case contentModerationEmailRiskLow:
		return "low"
	case contentModerationEmailRiskObserve:
		return "observe"
	case contentModerationEmailRiskCritical:
		return "critical"
	default:
		return "high"
	}
}
