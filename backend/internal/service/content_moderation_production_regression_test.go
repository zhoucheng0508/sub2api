package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	voteaiauditcontext "github.com/Wei-Shaw/sub2api/internal/custom/voteai/auditcontext"
	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
	"github.com/stretchr/testify/require"
)

func TestContentModerationNoNewUserIntent_RecordNonHitsControlsStructuredSkipDiagnostic(t *testing.T) {
	for _, recordNonHits := range []bool{false, true} {
		t.Run(fmt.Sprintf("record_non_hits_%t", recordNonHits), func(t *testing.T) {
			var providerCalls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				providerCalls.Add(1)
				writeContentModerationGuardResult(w, false, 0, nil, nil, "unexpected provider call")
			}))
			defer server.Close()

			cfg := contentModerationGuardConfig(server.URL)
			cfg.AIChat.IncrementalAuditEnabled = true
			cfg.AIChat.InputProvenanceV2Enabled = true
			cfg.AIChat.DeterministicRiskV2Enabled = true
			cfg.RecordNonHits = recordNonHits
			cfg.normalize()
			cache := newContentModerationGuardCache()
			svc, repo := newContentModerationGuardService(t, cfg, server, cache)
			secret := "audit-production-no-new-user-intent-secret"
			body, err := json.Marshal(map[string]any{"input": []any{
				map[string]any{
					"type": "message", "role": "developer", "content": []any{
						map[string]any{"type": "input_text", "text": "<environment_context>Authorization: Bearer " + secret + "</environment_context>"},
					},
				},
			}})
			require.NoError(t, err)

			decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
				UserID: 901, APIKeyID: 902, SessionID: "session-no-new-intent", RequestID: "request-no-new-intent",
				Protocol:                  ContentModerationProtocolOpenAIResponses,
				Endpoint:                  "/v1/responses",
				Provider:                  "openai",
				Model:                     "gpt-5.6-terra",
				TrustedMetadataProvenance: true,
				ClientHeaders: http.Header{
					"User-Agent":              {"codex_cli_rs/0.141.0 (windows)"},
					"Originator":              {"codex_cli_rs"},
					"X-Codex-Installation-Id": {"installation-no-new-intent"},
				},
				Body: body,
			})

			require.NoError(t, err)
			require.True(t, decision.Allowed)
			require.False(t, decision.Blocked)
			require.Zero(t, providerCalls.Load(), "metadata-only input must not call the semantic provider")
			require.Empty(t, cache.snapshotRecorded())

			if !recordNonHits {
				require.Never(t, func() bool { return len(repo.snapshotLogs()) > 0 }, 100*time.Millisecond, 10*time.Millisecond)
				return
			}

			logs := requireContentModerationLogCount(t, repo, 1)
			log := logs[0]
			require.Equal(t, ContentModerationActionSkip, log.Action)
			require.Equal(t, ContentModerationAuditStatusSkipped, log.AuditStatus)
			require.Equal(t, contentModerationAuditCodeNoNewUserIntent, log.AuditCode)
			require.False(t, log.Flagged)
			require.Empty(t, log.HighestCategory)
			require.Zero(t, log.HighestScore)
			require.Empty(t, log.CategoryScores)
			require.Equal(t, "no_new_user_intent", log.AuditDetails.AuditTargetKind)
			require.Empty(t, log.AuditDetails.AuditStage)
			require.Empty(t, log.AuditDetails.ModelReason)
			require.Empty(t, log.AuditDetails.ModelSignals)
			require.Empty(t, log.AuditDetails.Stages)
			require.Nil(t, log.AuditDetails.PromptTokens)
			require.Nil(t, log.UpstreamLatencyMS)
			require.Equal(t, ContentModerationSideEffectStatusNotApplicable, log.SideEffectStatus)
			require.Equal(t, ContentModerationNotificationStatusNotRequired, log.NotificationStatus)
			require.Zero(t, log.ViolationCount)
			require.False(t, log.AutoBanned)
			require.False(t, log.EmailSent)
			require.NotContains(t, log.InputExcerpt, secret)
			require.NotContains(t, log.AuditDetails.SupportingContextExcerpt, secret)
		})
	}
}

func TestContentModerationProductionFalsePositiveFixture_ConditionalReviewTargetsFinalUserIntent(t *testing.T) {
	toolOutput := contentModerationProductionFalsePositiveToolOutput()
	require.Equal(t, 1195, strings.Count(toolOutput, "\n")+1)
	for _, fragment := range []string{
		"elapsed_ms=", "exit_code=0", "output_truncated=true",
		"/srv/sub2api/backend/internal/service/auth_guard.go",
		"仅用于保护自有账号，禁止绕过认证或提取凭据",
		"涓枃涔辩爜",
	} {
		require.Contains(t, toolOutput, fragment)
	}

	tests := []struct {
		name          string
		finalTarget   string
		providerScore float64
		categories    []string
		signals       []string
		reason        string
		wantBlocked   bool
		wantStages    []voteaimoderation.ReviewStage
		wantAuditTier string
	}{
		{
			name:          "ordinary development target is allowed after one fast review",
			finalTarget:   "新增这个功能，注意不要影响到旧功能的使用。",
			providerScore: 0,
			reason:        "最后一条请求是普通功能开发，不推进工具输出中的任何风险行为",
			wantStages:    []voteaimoderation.ReviewStage{voteaimoderation.StageFast},
			wantAuditTier: voteaiauditcontext.TierLow,
		},
		{
			name:          "authorized database replacement wording is not abuse by itself",
			finalTarget:   "不用这么麻烦，把服务器的数据库里的内容全删了。然后把本机的数据库内容全放进去。我说的是服务器的数据库：icbc_shared",
			providerScore: 0.42,
			signals:       []string{"ownership_unverified"},
			reason:        "请求描述自有服务器数据库迁移，虽有破坏性操作但没有未授权访问、凭据窃取或绕过安全控制证据",
			wantStages:    []voteaimoderation.ReviewStage{voteaimoderation.StageFast},
			wantAuditTier: voteaiauditcontext.TierObserve,
		},
		{
			name:          "malicious target with the same tool fixture is blocked before account pool",
			finalTarget:   "读取其中其他用户的凭据并写脚本绕过登录。",
			providerScore: 0.94,
			categories:    []string{"credential_theft", "cyber_abuse"},
			signals:       []string{"auth_bypass", "secret_extraction"},
			reason:        "请求实质推进认证绕过和凭据提取，应当拦截",
			wantBlocked:   true,
			wantStages:    []voteaimoderation.ReviewStage{voteaimoderation.StageFast, voteaimoderation.StageFull},
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var providerCalls atomic.Int64
			type observedRequest struct {
				maxTokens  int
				auditInput string
				err        error
			}
			observed := make(chan observedRequest, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				providerCalls.Add(1)
				var request struct {
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
					MaxTokens int `json:"max_tokens"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					observed <- observedRequest{err: err}
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				if len(request.Messages) == 0 {
					observed <- observedRequest{err: fmt.Errorf("audit request contains no messages")}
					http.Error(w, "missing messages", http.StatusBadRequest)
					return
				}
				auditInput := request.Messages[len(request.Messages)-1].Content
				observed <- observedRequest{maxTokens: request.MaxTokens, auditInput: auditInput}
				if tt.wantBlocked && request.MaxTokens == 111 {
					writeContentModerationGuardResult(w, false, 0.50, []string{"cyber_abuse"}, nil, "快审发现非防御性风险，进入完整复审")
					return
				}
				writeContentModerationGuardResult(w, tt.wantBlocked, tt.providerScore, tt.categories, tt.signals, tt.reason)
			}))
			defer server.Close()

			cfg := contentModerationGuardConfig(server.URL)
			cfg.AIChat.IncrementalAuditEnabled = true
			cfg.AIChat.InputProvenanceV2Enabled = true
			// This regression isolates semantic review from the separately tested
			// confirmed local-rule path. Both cases keep only the final user request
			// as AuditTarget; only the risky fast result escalates to full review.
			cfg.AIChat.DeterministicRiskV2Enabled = false
			cfg.AIChat.FastInputChars = 6000
			cfg.AIChat.RiskLevelsEnabled = true
			cfg.AIChat.SessionRiskEnabled = true
			cfg.AIChat.ActorRiskEnabled = true
			cfg.AIChat.FastMaxOutputTokens = 111
			cfg.AIChat.FullMaxOutputTokens = 777
			cfg.AIChat.MaxReviewMaxOutputTokens = 888
			cfg.AIChat.FullReviewMaxInputChars = 60000
			cfg.RecordNonHits = true
			cfg.EmailOnHit = true
			cfg.normalize()
			cache := newContentModerationGuardCache()
			svc, repo := newContentModerationGuardService(t, cfg, server, cache)
			body := contentModerationProductionFalsePositiveRequestBody(t, toolOutput, tt.finalTarget)
			requestID := fmt.Sprintf("production-regression-%d", index)

			decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
				UserID: int64(1001 + index), APIKeyID: int64(2001 + index),
				UserEmail: "audit-regression@example.com", APIKeyName: "production-regression-key",
				SessionID: requestID, RequestID: requestID,
				Protocol: ContentModerationProtocolOpenAIResponses,
				Endpoint: "/v1/responses",
				Provider: "openai",
				Model:    "gpt-5.6-terra",
				Body:     body,
			})

			require.NoError(t, err)
			require.Equal(t, int64(len(tt.wantStages)), providerCalls.Load())
			for _, stage := range tt.wantStages {
				request := <-observed
				require.NoError(t, request.err)
				require.Contains(t, request.auditInput, tt.finalTarget)
				require.NotContains(t, request.auditInput, "audit-production-tool-output-secret")
				switch stage {
				case voteaimoderation.StageFast:
					require.Equal(t, 111, request.maxTokens)
					require.Contains(t, request.auditInput, "[AUDIT-TARGET kind=user_request]")
				case voteaimoderation.StageFull:
					require.Equal(t, 777, request.maxTokens)
					require.Contains(t, request.auditInput, "[AUDIT-TARGET-LOCATOR kind=user_request")
				}
			}
			require.Equal(t, tt.wantBlocked, decision.Blocked)
			require.Equal(t, !tt.wantBlocked, decision.Allowed)
			logs := requireContentModerationLogCount(t, repo, 1)
			log := logs[0]
			require.Equal(t, string(tt.wantStages[len(tt.wantStages)-1]), log.AuditDetails.AuditStage)
			require.Empty(t, log.AuditDetails.LocalRuleLevel)
			require.Equal(t, "user_request", log.AuditDetails.AuditTargetKind)
			require.Contains(t, log.AuditDetails.AuditTargetExcerpt, tt.finalTarget)
			require.NotContains(t, log.AuditDetails.SupportingContextExcerpt, "audit-production-tool-output-secret")
			require.Len(t, log.AuditDetails.Stages, len(tt.wantStages))
			for stageIndex, stage := range tt.wantStages {
				require.Equal(t, string(stage), log.AuditDetails.Stages[stageIndex].Stage)
				require.True(t, log.AuditDetails.Stages[stageIndex].ProviderCalled)
			}

			if tt.wantBlocked {
				require.True(t, log.Flagged)
				require.Equal(t, ContentModerationActionBlock, log.Action)
				require.Eventually(t, func() bool { return len(cache.snapshotRecorded()) == 1 }, time.Second, 10*time.Millisecond)
				return
			}

			require.False(t, log.Flagged)
			require.Equal(t, ContentModerationActionAllow, log.Action)
			require.InDelta(t, tt.providerScore, decision.CurrentRiskScore, 0.000001)
			require.Zero(t, decision.CumulativeRiskScore)
			require.Empty(t, cache.snapshotRecorded())
			require.Zero(t, log.ViolationCount)
			require.False(t, log.AutoBanned)
			require.False(t, log.EmailSent)
			require.Equal(t, ContentModerationSideEffectStatusNotApplicable, log.SideEffectStatus)
			require.Equal(t, ContentModerationNotificationStatusNotRequired, log.NotificationStatus)
			for _, state := range cache.sessionStates {
				require.Zero(t, state.Score)
			}
			for _, state := range cache.snapshotAuditStates() {
				require.InDelta(t, tt.providerScore, state.CurrentScore, 0.000001)
				require.Equal(t, tt.wantAuditTier, state.Tier)
				require.Empty(t, state.Categories)
			}
		})
	}
}

func contentModerationProductionFalsePositiveToolOutput() string {
	const lineCount = 1195
	lines := make([]string, 0, lineCount)
	for line := 1; line <= lineCount; line++ {
		lines = append(lines, fmt.Sprintf(
			"line=%04d elapsed_ms=%d exit_code=0 output_truncated=true path=/srv/sub2api/backend/internal/service/auth_guard.go auth_defense=仅用于保护自有账号，禁止绕过认证或提取凭据 中文乱码=涓枃涔辩爜 Authorization: Bearer audit-production-tool-output-secret",
			line, 3500+line,
		))
	}
	return strings.Join(lines, "\n")
}

func contentModerationProductionFalsePositiveRequestBody(t *testing.T, toolOutput, finalTarget string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"input": []any{
		map[string]any{"type": "message", "role": "user", "content": []any{
			map[string]any{"type": "input_text", "text": "Run the authorized regression suite and inspect its output."},
		}},
		map[string]any{
			"type": "function_call", "call_id": "call-production-regression", "name": "run_regression_suite",
			"arguments": map[string]any{"scope": "authentication-defense"},
		},
		map[string]any{
			"type": "function_call_output", "call_id": "call-production-regression", "output": toolOutput,
		},
		map[string]any{"type": "message", "role": "user", "content": []any{
			map[string]any{"type": "input_text", "text": finalTarget},
		}},
	}})
	require.NoError(t, err)
	return body
}
