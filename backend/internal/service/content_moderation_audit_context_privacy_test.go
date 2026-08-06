package service

import (
	"context"
	"strings"
	"testing"
	"time"

	voteaiauditcontext "github.com/Wei-Shaw/sub2api/internal/custom/voteai/auditcontext"
	"github.com/stretchr/testify/require"
)

type contentModerationAuditPrivacyCache struct {
	*contentModerationGuardCache
	lastEvent voteaiauditcontext.AuditEvent
}

func (c *contentModerationAuditPrivacyCache) UpdateContentModerationAuditContextForUser(
	ctx context.Context,
	userID int64,
	key string,
	event voteaiauditcontext.AuditEvent,
	cfg voteaiauditcontext.Config,
	ttl time.Duration,
) (voteaiauditcontext.State, error) {
	c.lastEvent = event
	return c.contentModerationGuardCache.UpdateContentModerationAuditContextForUser(ctx, userID, key, event, cfg, ttl)
}

func TestContentModerationAuditContext_StatelessActorDoesNotCrossConversationBoundary(t *testing.T) {
	cfg := contentModerationGuardConfig("http://127.0.0.1")
	cfg.AIChat.IncrementalAuditEnabled = true
	cache := &contentModerationAuditPrivacyCache{contentModerationGuardCache: newContentModerationGuardCache()}
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	firstInput := ContentModerationCheckInput{
		UserID: 501, APIKeyID: 601, RequestID: "stateless-conversation-one",
		Protocol: ContentModerationProtocolOpenAIChat,
	}
	_, actorKey, _ := contentModerationRiskIdentity(firstInput)
	cache.auditStates[actorKey] = voteaiauditcontext.State{
		Version:             voteaiauditcontext.StateVersion,
		PolicyVersion:       contentModerationAuditPolicyVersion(cfg),
		TurnCount:           12,
		CurrentScore:        0.60,
		MaxScore:            0.80,
		Tier:                voteaiauditcontext.TierObserve,
		Trend:               voteaiauditcontext.TrendRising,
		Categories:          []string{"old_category_canary"},
		Signals:             []string{"old_signal_canary"},
		RecentReasons:       []string{"old-reason-canary"},
		CanonicalPrefixHash: "old-prefix-canary",
	}

	firstPlan, err := svc.prepareIncrementalAudit(context.Background(), firstInput, cfg, contentModerationGuardInput("First independent request."))
	require.NoError(t, err)
	require.False(t, firstPlan.stableSession)
	require.InDelta(t, 0.15, firstPlan.state.CurrentScore, 0.0001, "numeric actor risk should still inform this request")
	for _, providerInput := range []string{firstPlan.fastInput.Text, firstPlan.fullInput} {
		require.NotContains(t, providerInput, "old_category_canary")
		require.NotContains(t, providerInput, "old_signal_canary")
		require.NotContains(t, providerInput, "old-reason-canary")
	}

	svc.updateContentModerationAuditContext(context.Background(), firstInput, cfg, firstPlan, &moderationAPIResult{
		CategoryScores: map[string]float64{"ai_risk": 0.76},
		Categories:     []string{"new_category_canary"},
		Signals:        []string{"new_signal_canary"},
		Reason:         "new-reason-canary",
	}, true)
	require.True(t, cache.lastEvent.NumericRiskOnly)
	require.Empty(t, cache.lastEvent.Categories, "stateless content must be removed before the storage boundary")
	require.Empty(t, cache.lastEvent.Signals, "stateless content must be removed before the storage boundary")
	require.Empty(t, cache.lastEvent.Reason, "stateless content must be removed before the storage boundary")
	stored := cache.auditStates[actorKey]
	require.InDelta(t, 0.76, stored.CurrentScore, 0.0001)
	require.InDelta(t, 0.80, stored.MaxScore, 0.0001)
	require.Empty(t, stored.Categories)
	require.Empty(t, stored.Signals)
	require.Empty(t, stored.RecentReasons)
	require.Empty(t, stored.CanonicalPrefixHash)
	require.Zero(t, stored.TurnCount)

	secondInput := firstInput
	secondInput.RequestID = "stateless-conversation-two"
	secondPlan, err := svc.prepareIncrementalAudit(context.Background(), secondInput, cfg, contentModerationGuardInput("Second independent request."))
	require.NoError(t, err)
	require.False(t, secondPlan.stableSession)
	require.InDelta(t, 0.19, secondPlan.state.CurrentScore, 0.0001, "only damped numeric actor risk may cross requests")
	providerInput := secondPlan.fastInput.Text + "\n" + secondPlan.fullInput
	for _, forbidden := range []string{
		"old_category_canary", "old_signal_canary", "old-reason-canary",
		"new_category_canary", "new_signal_canary", "new-reason-canary",
	} {
		require.False(t, strings.Contains(providerInput, forbidden), "stateless provider input leaked %q", forbidden)
	}
}
