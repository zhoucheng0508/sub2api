package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type contentModerationEmailDedupeTestCall struct {
	userID   int64
	scopes   []ContentModerationEmailDedupeScope
	riskRank int
}

type contentModerationEmailDedupeTestStore struct {
	mu        sync.Mutex
	markers   map[string]string
	calls     []contentModerationEmailDedupeTestCall
	nextToken int
	err       error
}

type contentModerationEmailIntegrationCache struct {
	*contentModerationTestHashCache
	*contentModerationEmailDedupeTestStore
}

type contentModerationEmailLifecycleTestRepo struct {
	contentModerationTestRepo
	mu      sync.Mutex
	patches []ContentModerationLogEffectsPatch
}

func (r *contentModerationEmailLifecycleTestRepo) UpdateLogEffects(_ context.Context, _ int64, patch ContentModerationLogEffectsPatch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.patches = append(r.patches, patch)
	return nil
}

func (r *contentModerationEmailLifecycleTestRepo) GetModerationUserState(_ context.Context, userID int64) (*ContentModerationUserState, error) {
	return &ContentModerationUserState{UserID: userID}, nil
}

func (r *contentModerationEmailLifecycleTestRepo) TryApplyModerationOwnedBan(_ context.Context, _, _ int64, _ time.Time) (string, error) {
	return ContentModerationBanOutcomeIneligible, nil
}

func (r *contentModerationEmailLifecycleTestRepo) RestoreModerationOwnedBan(_ context.Context, _ int64) (bool, error) {
	return false, nil
}

func (r *contentModerationEmailLifecycleTestRepo) snapshotPatches() []ContentModerationLogEffectsPatch {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ContentModerationLogEffectsPatch(nil), r.patches...)
}

func (s *contentModerationEmailDedupeTestStore) TryReserveContentModerationEmail(_ context.Context, userID int64, scopes []ContentModerationEmailDedupeScope, riskRank int) (ContentModerationEmailDedupeReserveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	empty := ContentModerationEmailDedupeReserveResult{ConflictScopeIndex: -1}
	if s.err != nil {
		return empty, s.err
	}
	s.calls = append(s.calls, contentModerationEmailDedupeTestCall{
		userID: userID, scopes: append([]ContentModerationEmailDedupeScope(nil), scopes...), riskRank: riskRank,
	})
	if s.markers == nil {
		s.markers = make(map[string]string)
	}
	for scopeIndex, scope := range scopes {
		for rank := riskRank; rank <= int(contentModerationEmailRiskCritical); rank++ {
			if _, exists := s.markers[contentModerationEmailDedupeTestMarker(userID, scope.Hash, rank)]; exists {
				empty.ConflictScopeIndex = scopeIndex
				return empty, nil
			}
		}
	}
	s.nextToken++
	token := fmt.Sprintf("lease-%d", s.nextToken)
	for _, scope := range scopes {
		for rank := 0; rank <= riskRank; rank++ {
			marker := contentModerationEmailDedupeTestMarker(userID, scope.Hash, rank)
			if _, exists := s.markers[marker]; !exists {
				s.markers[marker] = token
			}
		}
	}
	lease := &ContentModerationEmailDedupeLease{
		UserID: userID, Token: token, Scopes: append([]ContentModerationEmailDedupeScope(nil), scopes...), RiskRank: riskRank,
	}
	return ContentModerationEmailDedupeReserveResult{Acquired: true, ConflictScopeIndex: -1, Lease: lease}, nil
}

func (s *contentModerationEmailDedupeTestStore) ReleaseContentModerationEmail(_ context.Context, lease ContentModerationEmailDedupeLease) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var deleted int64
	for _, scope := range lease.Scopes {
		for rank := 0; rank <= lease.RiskRank; rank++ {
			marker := contentModerationEmailDedupeTestMarker(lease.UserID, scope.Hash, rank)
			if s.markers[marker] == lease.Token {
				delete(s.markers, marker)
				deleted++
			}
		}
	}
	return deleted, nil
}

func (s *contentModerationEmailDedupeTestStore) ClearContentModerationEmailDedupe(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

func (s *contentModerationEmailDedupeTestStore) snapshotCalls() []contentModerationEmailDedupeTestCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]contentModerationEmailDedupeTestCall(nil), s.calls...)
}

func contentModerationEmailDedupeTestMarker(userID int64, scopeHash string, riskRank int) string {
	return fmt.Sprintf("%d:%s:%d", userID, scopeHash, riskRank)
}

func TestReserveContentModerationEmailNotification_DeduplicatesSameInput(t *testing.T) {
	store := &contentModerationEmailDedupeTestStore{}
	request := contentModerationEmailDedupeRequest{
		UserID: 11, InputHash: "input-a", RiskLevel: contentModerationEmailRiskHigh,
	}

	first := reserveContentModerationEmailNotification(context.Background(), store, request)
	second := reserveContentModerationEmailNotification(context.Background(), store, request)

	require.True(t, first.ShouldSend)
	require.Equal(t, contentModerationEmailDedupeStateAcquired, first.State)
	require.False(t, second.ShouldSend)
	require.Equal(t, contentModerationEmailDedupeStateDeduplicated, second.State)
	require.Equal(t, contentModerationEmailDedupeScopeInput, second.Scope)
	calls := store.snapshotCalls()
	require.Len(t, calls, 2)
	require.Equal(t, contentModerationEmailInputDedupeTTL, calls[0].scopes[0].TTL)
}

func TestReserveContentModerationEmailNotification_DeduplicatesConversation(t *testing.T) {
	store := &contentModerationEmailDedupeTestStore{}
	first := reserveContentModerationEmailNotification(context.Background(), store, contentModerationEmailDedupeRequest{
		UserID: 11, APIKeyID: 22, SessionID: "conversation-a", InputHash: "input-a", RiskLevel: contentModerationEmailRiskHigh,
	})
	second := reserveContentModerationEmailNotification(context.Background(), store, contentModerationEmailDedupeRequest{
		UserID: 11, APIKeyID: 22, SessionID: "conversation-a", InputHash: "input-b", RiskLevel: contentModerationEmailRiskHigh,
	})

	require.True(t, first.ShouldSend)
	require.False(t, second.ShouldSend)
	require.Equal(t, contentModerationEmailDedupeScopeSession, second.Scope)
	calls := store.snapshotCalls()
	require.Len(t, calls, 2)
	require.Len(t, calls[0].scopes, 2)
	require.Equal(t, contentModerationEmailSessionDedupeTTL, calls[0].scopes[0].TTL)
	require.Equal(t, contentModerationEmailInputDedupeTTL, calls[0].scopes[1].TTL)
	require.Equal(t, contentModerationEmailSessionDedupeTTL, calls[1].scopes[0].TTL)
}

func TestReserveContentModerationEmailNotification_DoesNotPartiallyReserveScopes(t *testing.T) {
	store := &contentModerationEmailDedupeTestStore{}
	repeatedInput := contentModerationEmailDedupeRequest{
		UserID: 11, InputHash: "input-a", RiskLevel: contentModerationEmailRiskHigh,
	}
	require.True(t, reserveContentModerationEmailNotification(context.Background(), store, repeatedInput).ShouldSend)

	conflict := reserveContentModerationEmailNotification(context.Background(), store, contentModerationEmailDedupeRequest{
		UserID: 11, APIKeyID: 22, SessionID: "conversation-b", InputHash: "input-a", RiskLevel: contentModerationEmailRiskHigh,
	})
	require.False(t, conflict.ShouldSend)
	require.Equal(t, contentModerationEmailDedupeScopeInput, conflict.Scope)

	novelInput := reserveContentModerationEmailNotification(context.Background(), store, contentModerationEmailDedupeRequest{
		UserID: 11, APIKeyID: 22, SessionID: "conversation-b", InputHash: "input-b", RiskLevel: contentModerationEmailRiskHigh,
	})
	require.True(t, novelInput.ShouldSend)
	require.NotNil(t, novelInput.Lease)
}

func TestReserveContentModerationEmailNotification_AllowsRiskEscalation(t *testing.T) {
	store := &contentModerationEmailDedupeTestStore{}
	request := contentModerationEmailDedupeRequest{
		UserID: 11, APIKeyID: 22, SessionID: "conversation-a", RiskLevel: contentModerationEmailRiskHigh,
	}

	high := reserveContentModerationEmailNotification(context.Background(), store, request)
	request.RiskLevel = contentModerationEmailRiskCritical
	critical := reserveContentModerationEmailNotification(context.Background(), store, request)
	request.RiskLevel = contentModerationEmailRiskHigh
	downgrade := reserveContentModerationEmailNotification(context.Background(), store, request)

	require.True(t, high.ShouldSend)
	require.True(t, critical.ShouldSend)
	require.Equal(t, "critical", critical.RiskLevel)
	require.False(t, downgrade.ShouldSend)
	require.Equal(t, "high", downgrade.RiskLevel)
}

func TestReserveContentModerationEmailNotification_FailsOpenOnStoreError(t *testing.T) {
	store := &contentModerationEmailDedupeTestStore{err: errors.New("redis unavailable")}
	decision := reserveContentModerationEmailNotification(context.Background(), store, contentModerationEmailDedupeRequest{
		UserID: 11, InputHash: "input-a", RiskLevel: contentModerationEmailRiskHigh,
	})

	require.True(t, decision.ShouldSend)
	require.True(t, decision.FailOpen)
	require.Equal(t, contentModerationEmailDedupeStateFailOpen, decision.State)
	require.Equal(t, "redis unavailable", decision.Error)
}

func TestReserveContentModerationEmailNotification_AllowsWithoutScope(t *testing.T) {
	store := &contentModerationEmailDedupeTestStore{}
	decision := reserveContentModerationEmailNotification(context.Background(), store, contentModerationEmailDedupeRequest{
		RiskLevel: contentModerationEmailRiskHigh,
	})

	require.True(t, decision.ShouldSend)
	require.False(t, decision.FailOpen)
	require.Equal(t, contentModerationEmailDedupeStateNoScope, decision.State)
	require.Empty(t, store.snapshotCalls())
}

func TestSendFlaggedNotificationSideEffects_UsesLogIdentityAndDeduplicates(t *testing.T) {
	dedupe := &contentModerationEmailDedupeTestStore{}
	cache := &contentModerationEmailIntegrationCache{
		contentModerationTestHashCache:        &contentModerationTestHashCache{},
		contentModerationEmailDedupeTestStore: dedupe,
	}
	svc := &ContentModerationService{hashCache: cache, emailService: &EmailService{}}
	userID, apiKeyID := int64(11), int64(22)
	log := &ContentModerationLog{
		UserID: &userID, UserEmail: "user@example.com", APIKeyID: &apiKeyID,
		SessionID: "conversation-a", InputHash: "input-a", Flagged: true, HighestScore: 0.8,
	}
	cfg := defaultContentModerationConfig()
	cfg.EmailOnHit = true

	first := svc.reserveContentModerationEmailForLog(context.Background(), log)
	require.True(t, first.ShouldSend)
	status, sent, err := svc.sendFlaggedNotificationSideEffects(context.Background(), cfg, log, false)

	require.NoError(t, err)
	require.False(t, sent)
	require.Equal(t, ContentModerationNotificationStatusDeduplicated, status)
	calls := dedupe.snapshotCalls()
	require.Len(t, calls, 2)
	require.Equal(t, userID, calls[0].userID)
	require.Equal(t, contentModerationEmailSessionDedupeTTL, calls[0].scopes[0].TTL)
}

func TestContentModerationLogCarriesEphemeralDedupeIdentity(t *testing.T) {
	repo := &contentModerationTestRepo{}
	svc := &ContentModerationService{repo: repo}
	cfg := defaultContentModerationConfig()
	log := svc.buildLog(ContentModerationCheckInput{
		RequestID: "request-a", UserID: 11, APIKeyID: 22, SessionID: " conversation-a ",
	}, cfg, ContentModerationActionBlock, true, "cyber_abuse", 0.8, nil, "input", nil, nil, "")

	svc.persistContentModerationLog(context.Background(), cfg, log, " input-hash ", false, false)

	require.Equal(t, "conversation-a", log.SessionID)
	require.Equal(t, "input-hash", log.InputHash)
}

func TestRecordCyberPolicyEvent_DeduplicatesBySessionBeforeSending(t *testing.T) {
	dedupe := &contentModerationEmailDedupeTestStore{}
	cache := &contentModerationEmailIntegrationCache{
		contentModerationTestHashCache:        &contentModerationTestHashCache{},
		contentModerationEmailDedupeTestStore: dedupe,
	}
	repo := &contentModerationEmailLifecycleTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{SettingKeyRiskControlEnabled: "true"}},
		repo, cache, nil, nil, nil, nil, &EmailService{},
	)
	userID, apiKeyID := int64(11), int64(22)
	log := &ContentModerationLog{
		UserID: &userID, APIKeyID: &apiKeyID, SessionID: "conversation-a",
		InputHash: "input-a", Action: ContentModerationActionCyberPolicy, HighestScore: 1,
	}
	require.True(t, svc.reserveContentModerationEmailForLog(context.Background(), log).ShouldSend)

	svc.RecordCyberPolicyEvent(context.Background(), CyberPolicyRecordInput{
		UserID: userID, UserEmail: "user@example.com", APIKeyID: apiKeyID,
		SessionID: "conversation-a", InputHash: "input-b", UpstreamMessage: "cyber policy",
	})

	patches := repo.snapshotPatches()
	require.Len(t, patches, 1)
	require.Equal(t, ContentModerationNotificationStatusDeduplicated, patches[0].NotificationStatus)
	require.False(t, patches[0].EmailSent)
}

func TestRecordCyberPolicyEvent_NotificationFailureReleasesDedupeLease(t *testing.T) {
	dedupe := &contentModerationEmailDedupeTestStore{}
	cache := &contentModerationEmailIntegrationCache{
		contentModerationTestHashCache:        &contentModerationTestHashCache{},
		contentModerationEmailDedupeTestStore: dedupe,
	}
	repo := &contentModerationEmailLifecycleTestRepo{}
	settings := &contentModerationTestSettingRepo{values: map[string]string{SettingKeyRiskControlEnabled: "true"}}
	svc := NewContentModerationService(
		settings, repo, cache, nil, nil, nil, nil, NewEmailService(settings, nil),
	)
	userID, apiKeyID := int64(11), int64(22)

	svc.RecordCyberPolicyEvent(context.Background(), CyberPolicyRecordInput{
		UserID: userID, UserEmail: "user@example.com", APIKeyID: apiKeyID,
		SessionID: "conversation-a", InputHash: "input-a", UpstreamMessage: "cyber policy",
	})

	patches := repo.snapshotPatches()
	require.Len(t, patches, 1)
	require.Equal(t, ContentModerationNotificationStatusFailed, patches[0].NotificationStatus)
	require.False(t, patches[0].EmailSent)
	retry := svc.reserveContentModerationEmailForLog(context.Background(), &ContentModerationLog{
		UserID: &userID, APIKeyID: &apiKeyID, SessionID: "conversation-a",
		InputHash: "input-a", Action: ContentModerationActionCyberPolicy, HighestScore: 1,
	})
	require.True(t, retry.ShouldSend, "a failed cyber notification must release its dedupe lease")
}
