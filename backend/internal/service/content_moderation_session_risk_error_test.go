package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentModerationSessionRiskStoreFailurePolicy(t *testing.T) {
	tests := []struct {
		name             string
		operation        string
		failurePolicy    string
		wantAction       string
		wantAllowed      bool
		wantStatus       int
		wantProviderCall int64
	}{
		{
			name: "get error blocks before provider when policy blocks", operation: "get",
			failurePolicy: ContentModerationFailurePolicyBlock,
			wantAction:    ContentModerationActionUnavailable, wantStatus: http.StatusServiceUnavailable,
		},
		{
			name: "get error continues when policy allows", operation: "get",
			failurePolicy: ContentModerationFailurePolicyAllow,
			wantAction:    ContentModerationActionAllow, wantAllowed: true, wantProviderCall: 1,
		},
		{
			name: "update error returns unavailable when policy blocks", operation: "update",
			failurePolicy: ContentModerationFailurePolicyBlock,
			wantAction:    ContentModerationActionUnavailable, wantStatus: http.StatusServiceUnavailable, wantProviderCall: 1,
		},
		{
			name: "update error preserves current high risk decision when policy allows", operation: "update",
			failurePolicy: ContentModerationFailurePolicyAllow,
			wantAction:    ContentModerationActionBlock, wantStatus: defaultContentModerationBlockHTTPStatus, wantProviderCall: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var providerCalls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				providerCalls.Add(1)
				if tt.operation == "update" {
					writeContentModerationGuardResult(w, true, 0.91, []string{"credential_theft"}, []string{"credential_access"}, "high risk")
					return
				}
				writeContentModerationGuardResult(w, false, 0.05, nil, []string{"defensive_context"}, "benign")
			}))
			defer server.Close()

			cfg := contentModerationGuardConfig(server.URL)
			cfg.AIChat.IncrementalAuditEnabled = false
			cfg.AIChat.CacheEnabled = false
			cfg.AIChat.RiskLevelsEnabled = true
			cfg.AIChat.SessionRiskEnabled = true
			cfg.AIChat.ActorRiskEnabled = false
			cfg.AIChat.FailurePolicy = tt.failurePolicy
			cfg.normalize()

			cache := newContentModerationGuardCache()
			if tt.operation == "get" {
				cache.sessionGetErr = errors.New("redis get failed")
			} else {
				cache.sessionPutErr = errors.New("redis update failed")
			}
			svc, _ := newContentModerationGuardService(t, cfg, server, cache)
			decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
				RequestID: "session-risk-" + tt.operation + "-" + tt.failurePolicy,
				UserID:    401, APIKeyID: 501,
				SessionID: "session-risk-store-failure", SessionSource: ContentModerationSessionSourceHeader,
				Protocol: ContentModerationProtocolOpenAIChat, Endpoint: "/v1/chat/completions",
				Body: []byte(`{"messages":[{"role":"user","content":"review this request"}]}`),
			})

			require.NoError(t, err)
			require.NotNil(t, decision)
			require.Equal(t, tt.wantAction, decision.Action)
			require.Equal(t, tt.wantAllowed, decision.Allowed)
			require.Equal(t, tt.wantStatus, decision.StatusCode)
			require.Equal(t, tt.wantProviderCall, providerCalls.Load())
		})
	}
}

func TestContentModerationSessionRiskStoreCapabilityIsRequiredOnlyForStatefulIdentity(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.AIChat.RiskLevelsEnabled = true
	cfg.AIChat.SessionRiskEnabled = true
	svc := &ContentModerationService{}

	_, found, err := svc.getSessionRisk(context.Background(), ContentModerationCheckInput{
		UserID: 1, APIKeyID: 2,
	}, cfg)
	require.NoError(t, err)
	require.False(t, found)

	_, found, err = svc.getSessionRisk(context.Background(), ContentModerationCheckInput{
		UserID: 1, APIKeyID: 2, SessionID: "stateful",
	}, cfg)
	require.Error(t, err)
	require.False(t, found)
}
