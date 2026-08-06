package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	voteaiauditcontext "github.com/Wei-Shaw/sub2api/internal/custom/voteai/auditcontext"
	voteaiinputprovenance "github.com/Wei-Shaw/sub2api/internal/custom/voteai/inputprovenance"
	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
	"github.com/stretchr/testify/require"
)

func TestAggregateContentModerationStageUsage(t *testing.T) {
	tests := []struct {
		name       string
		stages     []*moderationAPIResult
		prompt     int
		cached     int
		completion int
		incomplete bool
		nilUsage   bool
		failed     bool
	}{
		{
			name: "both stages called upstream",
			stages: []*moderationAPIResult{
				{Stage: voteaimoderation.StageFast, Usage: completeModerationUsage(20, 5, 15, 2)},
				{Stage: voteaimoderation.StageFull, Usage: completeModerationUsage(100, 80, 20, 4)},
			},
			prompt: 120, cached: 85, completion: 6,
		},
		{
			name: "fast cache and full upstream",
			stages: []*moderationAPIResult{
				{Stage: voteaimoderation.StageFast, ResultCacheHit: true},
				{Stage: voteaimoderation.StageFull, Usage: completeModerationUsage(100, 80, 20, 4)},
			},
			prompt: 100, cached: 80, completion: 4,
		},
		{
			name: "fast upstream and full cache",
			stages: []*moderationAPIResult{
				{Stage: voteaimoderation.StageFast, Usage: completeModerationUsage(20, 5, 15, 2)},
				{Stage: voteaimoderation.StageFull, ResultCacheHit: true},
			},
			prompt: 20, cached: 5, completion: 2,
		},
		{
			name: "full attempt missing usage",
			stages: []*moderationAPIResult{
				{Stage: voteaimoderation.StageFast, Usage: completeModerationUsage(20, 5, 15, 2)},
				{Stage: voteaimoderation.StageFull},
			},
			prompt: 20, cached: 5, completion: 2, incomplete: true,
		},
		{
			name: "fast upstream and full failed",
			stages: []*moderationAPIResult{
				{Stage: voteaimoderation.StageFast, Usage: completeModerationUsage(20, 5, 15, 2)},
			},
			prompt: 20, cached: 5, completion: 2, incomplete: true, failed: true,
		},
		{
			name: "all required stages served from result cache",
			stages: []*moderationAPIResult{
				{Stage: voteaimoderation.StageFast, ResultCacheHit: true},
				{Stage: voteaimoderation.StageFull, ResultCacheHit: true},
			},
			nilUsage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := contentModerationMergeCalledStageUsage(tt.failed, tt.stages...)
			if tt.nilUsage {
				require.Nil(t, usage)
				return
			}
			require.NotNil(t, usage)
			require.Equal(t, tt.incomplete, usage.Incomplete)
			require.Equal(t, tt.prompt, valueOrNegativeOne(usage.PromptTokens))
			require.Equal(t, tt.cached, valueOrNegativeOne(usage.CachedPromptTokens))
			require.Equal(t, tt.completion, valueOrNegativeOne(usage.CompletionTokens))
		})
	}
}

func TestContentModerationFullReviewPayloadPreservesAppendStableCanonicalPrefix(t *testing.T) {
	var mu sync.Mutex
	payloads := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Len(t, body.Messages, 2)
		mu.Lock()
		payloads = append(payloads, body.Messages[1].Content)
		call := len(payloads)
		mu.Unlock()
		writeModerationResultWithUsage(w, false, 0.05, nil, nil, "benign", 100+call*20, 80+call*20, 20, 3)
	}))
	defer server.Close()

	cache := newContentModerationGuardCache()
	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.InputProvenanceV2Enabled = true
	cfg.AIChat.forceFullReview = true
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.FullReviewMaxInputChars = 60000
	cfg.normalize()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)

	firstContent := appendStableReviewContent("implement the service", false)
	firstResult, firstPlan, err := svc.callIncrementalAIChatAudit(context.Background(), ContentModerationCheckInput{
		UserID: 91, APIKeyID: 92, SessionID: "stable-prefix", RequestID: "prefix-1",
	}, cfg, firstContent, false)
	require.NoError(t, err)
	require.NotNil(t, firstResult)
	require.NotNil(t, firstPlan)

	secondContent := appendStableReviewContent("add regression tests", true)
	secondResult, secondPlan, err := svc.callIncrementalAIChatAudit(context.Background(), ContentModerationCheckInput{
		UserID: 91, APIKeyID: 92, SessionID: "stable-prefix", RequestID: "prefix-2",
	}, cfg, secondContent, false)
	require.NoError(t, err)
	require.NotNil(t, secondResult)
	require.NotNil(t, secondPlan)

	mu.Lock()
	captured := append([]string(nil), payloads...)
	mu.Unlock()
	require.Len(t, captured, 2)
	require.True(t, firstPlan.state.PrefixBaseline)
	require.False(t, firstPlan.state.PrefixContinuity)
	require.True(t, strings.HasPrefix(secondPlan.canonicalFullPrefix, firstPlan.canonicalFullPrefix))
	require.Contains(t, captured[0], firstPlan.canonicalFullPrefix)
	require.Contains(t, captured[1], secondPlan.canonicalFullPrefix)
	require.Zero(t, strings.Count(firstPlan.canonicalFullPrefix, "[AUDIT-TARGET-LOCATOR"))
	require.Zero(t, strings.Count(secondPlan.canonicalFullPrefix, "[AUDIT-TARGET-LOCATOR"))
	require.Equal(t, 1, strings.Count(captured[1], "add regression tests"))
	require.Equal(t, 1, strings.Count(captured[1], "[AUDIT-TARGET-LOCATOR kind="))
	require.Equal(t, firstPlan.state.PrefixEpoch, secondPlan.state.PrefixEpoch)
	require.False(t, secondPlan.state.PrefixBaseline)
	require.True(t, secondPlan.state.PrefixContinuity)
}

func TestContentModerationCachedFullReviewDoesNotAdvanceProviderPrefixState(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeModerationResultWithUsage(w, false, 0.05, nil, nil, "benign", 120, 80, 40, 3)
	}))
	defer server.Close()

	cache := newContentModerationGuardCache()
	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.InputProvenanceV2Enabled = true
	cfg.AIChat.forceFullReview = true
	cfg.AIChat.CacheEnabled = true
	cfg.AIChat.CacheTTLSeconds = 60
	cfg.normalize()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	content := appendStableReviewContent("cacheable full review", false)
	input := ContentModerationCheckInput{
		UserID: 111, APIKeyID: 112, SessionID: "cached-full-prefix", RequestID: "cached-full-1",
	}
	plan, err := svc.prepareIncrementalAudit(context.Background(), input, cfg, content)
	require.NoError(t, err)
	fullCfg := cloneContentModerationConfig(cfg)
	fullCfg.AIChat.auditStage = string(voteaimoderation.StageFull)
	fullCfg.AIChat.riskStateDigest = contentModerationAuditRiskDigest(plan.state, plan.policyVersion, cfg)
	fullCfg.AIChat.MaxInputChars = max(fullCfg.AIChat.MaxInputChars, len([]rune(plan.fullInput)))

	first, err := svc.callModeration(context.Background(), fullCfg, plan.fullInput)
	require.NoError(t, err)
	require.False(t, first.ResultCacheHit)
	contentModerationSetStageDetails(first, contentModerationSuccessfulStageDetails(voteaimoderation.StageFull, first, 10))
	svc.updateContentModerationAuditPrefix(context.Background(), input, cfg, plan, first)
	require.EqualValues(t, 120, plan.state.LastPrefixTokens)

	second, err := svc.callModeration(context.Background(), fullCfg, plan.fullInput)
	require.NoError(t, err)
	require.True(t, second.ResultCacheHit)
	contentModerationSetStageDetails(second, contentModerationSuccessfulStageDetails(voteaimoderation.StageFull, second, 1))
	svc.updateContentModerationAuditPrefix(context.Background(), input, cfg, plan, second)
	require.Equal(t, 1, requests)
	_, prefixUpdates, _ := cache.updateCounts()
	require.Equal(t, 1, prefixUpdates)
	require.EqualValues(t, 120, plan.state.LastPrefixTokens)
	require.True(t, plan.state.PrefixBaseline)
}

func TestContentModerationPrefixTokensUseFullStageUsageOnly(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			writeModerationResultWithUsage(w, true, 0.85, []string{"credential_theft"}, []string{"auth_bypass"}, "review needed", 25, 5, 20, 2)
			return
		}
		writeModerationResultWithUsage(w, false, 0.10, nil, nil, "full review safe", 100, 75, 25, 4)
	}))
	defer server.Close()

	cache := newContentModerationGuardCache()
	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.InputProvenanceV2Enabled = true
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.FullReviewThreshold = 0.40
	cfg.normalize()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)

	result, plan, err := svc.callIncrementalAIChatAudit(context.Background(), ContentModerationCheckInput{
		UserID: 101, APIKeyID: 102, SessionID: "full-stage-tokens", RequestID: "full-stage-1",
	}, cfg, appendStableReviewContent("review this request", true), false)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.NotNil(t, result.Usage)
	require.Equal(t, 125, valueOrNegativeOne(result.Usage.PromptTokens))
	require.EqualValues(t, 100, plan.state.LastPrefixTokens)
}

func TestContentModerationPinnedEarlyContextIsDeterministicallyBounded(t *testing.T) {
	target := "current target must remain complete"
	turns := []voteaiauditcontext.Turn{
		{Role: voteaiauditcontext.RoleSystem, Text: strings.Repeat("S", 10000)},
		{Role: voteaiauditcontext.RoleUser, Text: strings.Repeat("U", 10000)},
		{Role: voteaiauditcontext.RoleAssistant, Text: strings.Repeat("A", 10000)},
		{Role: voteaiauditcontext.RoleUser, Text: target},
	}
	cfg := defaultContentModerationConfig()
	cfg.AIChat.FullReviewMaxInputChars = 6000

	canonical, input, truncated, compacted, _ := buildContentModerationFullReviewInputForTurns(
		turns, len(turns)-1, "user_request", target, voteaiauditcontext.State{}, cfg,
	)

	require.True(t, truncated)
	require.True(t, compacted)
	require.LessOrEqual(t, len([]rune(input)), 6000)
	require.Equal(t, 2, strings.Count(canonical, "[EARLY-CONTEXT-TRUNCATED]"))
	require.Equal(t, 1, strings.Count(canonical, target))
	require.Contains(t, input, "turn_index=3")
	require.Equal(t, 1, strings.Count(input, target))
}

func appendStableReviewContent(target string, includePreviousTarget bool) ContentModerationInput {
	turns := []ContentModerationTurn{
		{Role: "system", Source: "client_instruction", Purpose: "supporting_context", Text: "You are a coding assistant."},
		{Role: "user", Source: "end_user", Purpose: "supporting_context", Text: "Design the API."},
		{Role: "assistant", Source: "assistant_response", Purpose: "supporting_context", Text: "Here is the outline."},
	}
	if includePreviousTarget {
		turns = append(turns,
			ContentModerationTurn{Role: "user", Source: "end_user", Purpose: "supporting_context", Text: "implement the service"},
			ContentModerationTurn{Role: "assistant", Source: "assistant_response", Purpose: "supporting_context", Text: "The service is implemented."},
		)
	}
	turns = append(turns, ContentModerationTurn{
		Role: "user", Source: "end_user", Purpose: "audit_target", Text: target, Current: true,
	})
	return ContentModerationInput{
		Text: target, CurrentText: target, AuditTargetText: target,
		AuditTargetKind: string(voteaiinputprovenance.TargetUserRequest),
		HasExplicitUser: true, Turns: turns,
	}
}

func writeModerationResultWithUsage(
	w http.ResponseWriter,
	flagged bool,
	score float64,
	categories, signals []string,
	reason string,
	prompt, cached, uncached, completion int,
) {
	result, _ := json.Marshal(map[string]any{
		"flagged": flagged, "risk_score": score, "categories": categories,
		"signals": signals, "reason": reason,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{
			"role": "assistant", "content": string(result),
		}}},
		"usage": map[string]any{
			"prompt_tokens": prompt, "prompt_cache_hit_tokens": cached,
			"prompt_cache_miss_tokens": uncached, "completion_tokens": completion,
			"total_tokens": prompt + completion,
		},
	})
}

func completeModerationUsage(prompt, cached, uncached, completion int) *voteaimoderation.Usage {
	total := prompt + completion
	return &voteaimoderation.Usage{
		PromptTokens: &prompt, CachedPromptTokens: &cached,
		UncachedPromptTokens: &uncached, CompletionTokens: &completion,
		TotalTokens: &total,
	}
}

func valueOrNegativeOne(value *int) int {
	if value == nil {
		return -1
	}
	return *value
}
