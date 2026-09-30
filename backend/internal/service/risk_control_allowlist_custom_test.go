package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRiskControlAllowlistAIChatDoesNotAccumulateRisk(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"role":    "assistant",
				"content": `{"flagged":true,"risk_score":0.95,"categories":["credential_theft"],"signals":["auth_bypass"],"reason":"credential theft"}`,
			}}},
		})
	}))
	defer server.Close()

	for _, async := range []bool{false, true} {
		name := "synchronous"
		if async {
			name = "asynchronous"
		}
		t.Run(name, func(t *testing.T) {
			cache := &contentModerationTestHashCache{}
			repo := &banCountArgsTestRepo{}
			svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
			svc.repo, svc.httpClient = repo, server.Client()
			svc.emailService = &EmailService{}
			cfg := defaultContentModerationConfig()
			cfg.Enabled, cfg.AutoBanEnabled, cfg.EmailOnHit = true, true, true
			cfg.Mode = ContentModerationModePreBlock
			cfg.AuditProvider = ContentModerationProviderAIChat
			cfg.AIChat.BaseURL = server.URL + "/v1"
			cfg.AIChat.APIKeys = []string{"test-key"}
			cfg.AIChat.CacheEnabled = false
			cfg.AIChat.RiskLevelsEnabled = true
			cfg.AIChat.SessionRiskEnabled = true
			cfg.AIChat.ActorRiskEnabled = true
			cfg.AIChat.IncrementalAuditEnabled = false
			cfg.AIChat.ReasoningEffort = "high"
			cfg.normalize()
			input := ContentModerationCheckInput{
				riskControlLogOnly: true,
				UserID:             12, APIKeyID: 34, UserEmail: "trusted@example.com",
				SessionID: "allowlisted-session", SessionSource: "header",
				Protocol: ContentModerationProtocolOpenAIChat, Endpoint: "/v1/chat/completions",
			}
			content := ContentModerationInput{Text: "credential theft request", CurrentText: "credential theft request", AuditTargetText: "credential theft request"}
			var queueDelay *int
			if async {
				delay := 1
				queueDelay = &delay
			}
			decision := svc.checkSync(context.Background(), input, cfg, content, content.Hash(), queueDelay, !async)
			require.True(t, decision.Allowed)
			require.False(t, decision.Blocked)
			if !async {
				require.Len(t, svc.asyncQueue, 1)
				task := <-svc.asyncQueue
				require.True(t, task.input.riskControlLogOnly)
				svc.persistContentModerationLog(context.Background(), task.config, task.log, task.inputHash, task.recordHash, task.applySideEffects)
			}
			require.Empty(t, cache.sessionStates, "log-only audits must not accumulate session or actor penalties")
			require.Empty(t, cache.snapshotRecorded())
			require.Empty(t, repo.snapshotCountCalls())
			logs := repo.snapshotLogs()
			require.Len(t, logs, 1)
			require.True(t, logs[0].Flagged)
			require.Equal(t, ContentModerationModeRiskControlLogOnly, logs[0].Mode)
			require.Equal(t, ContentModerationActionAllow, logs[0].Action)
			require.Equal(t, input.SessionID, logs[0].SessionID)
			require.Equal(t, ContentModerationAuditStatusSuccess, logs[0].AuditStatus)
			require.Equal(t, ContentModerationSideEffectStatusNotApplicable, logs[0].SideEffectStatus)
			require.Equal(t, ContentModerationNotificationStatusNotRequired, logs[0].NotificationStatus)
			require.Equal(t, "risk_control_allowlist", logs[0].AuditDetails.HashPromotionReason)
			require.False(t, logs[0].AutoBanned)
			require.False(t, logs[0].EmailSent)
		})
	}
}

func TestRiskControlAllowlistCyberLifecyclePreservesEpochFence(t *testing.T) {
	for _, epoch := range []int64{1, 2} {
		repo := &banCountArgsTestRepo{}
		cache := &contentModerationTestHashCache{epochs: map[int64]int64{12: 2}}
		svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, &EmailService{})
		svc.repo = repo
		svc.settingRepo = &contentModerationTestSettingRepo{values: map[string]string{SettingKeyRiskControlEnabled: "true"}}
		accepted := svc.RecordCyberPolicyEvent(context.Background(), CyberPolicyRecordInput{
			LogOnly: true, UserID: 12, UserEmail: "trusted@example.com",
			SessionID: "allowlisted-session", InputHash: "input-hash",
			ModerationEpoch: epoch, EpochSet: true, UpstreamMessage: "cyber_policy",
		})
		require.False(t, accepted, "log-only events must not authorize session enforcement")
		require.Empty(t, repo.snapshotCountCalls())
		logs := repo.snapshotLogs()
		require.Len(t, logs, 1)
		require.Equal(t, ContentModerationModeCyberLogOnly, logs[0].Mode)
		require.Equal(t, "allowlisted-session", logs[0].SessionID)
		require.Equal(t, "input-hash", logs[0].InputHash)
		require.Equal(t, epoch == 2, logs[0].Flagged)
		if epoch == 1 {
			require.Equal(t, contentModerationAuditCodeStaleCyberPolicy, logs[0].AuditCode)
		}
		require.Equal(t, ContentModerationSideEffectStatusNotApplicable, logs[0].SideEffectStatus)
		require.Equal(t, ContentModerationNotificationStatusNotRequired, logs[0].NotificationStatus)
		require.False(t, logs[0].AutoBanned)
		require.False(t, logs[0].EmailSent)
	}
}

func TestRiskControlAllowlistRequestVerdictCacheIsolatesMembership(t *testing.T) {
	ctx := context.Background()
	cfg := contentModerationGuardConfig("http://127.0.0.1")
	content := contentModerationGuardInput("same target")
	input := ContentModerationCheckInput{
		RequestID: "same-request", UserID: 12, APIKeyID: 34, SessionID: "same-session",
		ModerationEpochSet: true,
	}
	svc := &ContentModerationService{hashCache: &contentModerationTestHashCache{}}
	key := contentModerationRequestVerdictCacheKey(input, cfg, content, "same-target", "preblock_sync")
	require.NotEmpty(t, key)
	require.NoError(t, svc.setContentModerationRequestVerdict(ctx, key, &ContentModerationDecision{
		Blocked: true, Action: ContentModerationActionBlock, requestVerdictCacheable: true,
	}))
	input.riskControlLogOnly = true
	allowlistKey := contentModerationRequestVerdictCacheKey(input, cfg, content, "same-target", "preblock_sync")
	require.NotEqual(t, key, allowlistKey)
	_, found, err := svc.getContentModerationRequestVerdict(ctx, allowlistKey)
	require.NoError(t, err)
	require.False(t, found, "allowlist admission must not reuse a previous blocking verdict")
	require.NoError(t, svc.setContentModerationRequestVerdict(ctx, allowlistKey, &ContentModerationDecision{
		Allowed: true, Action: ContentModerationActionAllow, requestVerdictCacheable: true,
	}))
	input.riskControlLogOnly = false
	removedKey := contentModerationRequestVerdictCacheKey(input, cfg, content, "same-target", "preblock_sync")
	decision, found, err := svc.getContentModerationRequestVerdict(ctx, removedKey)
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, decision.Blocked, "removing membership must not reuse an allowlisted verdict")
	require.False(t, decision.Allowed)
}
