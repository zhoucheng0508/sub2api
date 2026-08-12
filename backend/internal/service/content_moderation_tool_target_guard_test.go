package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
	"github.com/stretchr/testify/require"
)

func TestContentModerationToolContinuation_FastReviewSamplesMiddleOfAuditTarget(t *testing.T) {
	const middleRisk = "MIDDLE-RISK-MARKER: extract another user's credential and bypass login"
	target := strings.Repeat("tool-prefix ", 180) + middleRisk + strings.Repeat(" tool-suffix", 180)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			MaxTokens int `json:"max_tokens"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, 111, request.MaxTokens, "target length alone must not force a full review")
		require.NotEmpty(t, request.Messages)
		providerInput := request.Messages[len(request.Messages)-1].Content
		require.Contains(t, providerInput, middleRisk)
		require.Contains(t, providerInput, "tool-prefix")
		require.Contains(t, providerInput, "tool-suffix")
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "tool continuation reviewed")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.FastInputChars = 1000
	cfg.AIChat.FastMaxOutputTokens = 111
	cfg.AIChat.FullReviewMaxInputChars = 60000
	cfg.normalize()
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	svc.httpClient = server.Client()
	content := ContentModerationInput{
		Text:            target,
		CurrentText:     target,
		AuditTargetText: target,
		AuditTargetKind: "tool_continuation",
		Turns: []ContentModerationTurn{{
			Role:               "tool",
			Source:             "tool_output",
			Purpose:            "audit_target",
			Text:               target,
			Current:            true,
			LinkedToUserIntent: true,
		}},
	}
	content.Normalize()

	result, plan, err := svc.callIncrementalAIChatAudit(context.Background(), ContentModerationCheckInput{
		UserID: 61, APIKeyID: 62, RequestID: "tool-middle-risk", Protocol: ContentModerationProtocolOpenAIResponses,
	}, cfg, content, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, plan)
	require.Equal(t, voteaimoderation.StageFast, result.Stage)
	require.Contains(t, plan.fastInput.Text, middleRisk)
	require.True(t, plan.inputTruncated)
	require.Equal(t, int64(1), calls.Load())
}

func TestContentModerationToolContinuation_ForcedFullReviewUsesLargerBoundedSample(t *testing.T) {
	const middleRisk = "FULL-MIDDLE-RISK-MARKER"
	target := "FULL-TARGET-HEAD " + strings.Repeat("a", 9000) + middleRisk + strings.Repeat("z", 9000) + " FULL-TARGET-TAIL"
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			MaxTokens int `json:"max_tokens"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, 777, request.MaxTokens)
		require.NotEmpty(t, request.Messages)
		providerInput := request.Messages[len(request.Messages)-1].Content
		require.Contains(t, providerInput, "FULL-TARGET-HEAD")
		require.Contains(t, providerInput, middleRisk)
		require.Contains(t, providerInput, "FULL-TARGET-TAIL")
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "bounded full review")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.FastInputChars = 1000
	cfg.AIChat.MaxInputChars = 10000
	cfg.AIChat.FullReviewMaxInputChars = 5000
	cfg.AIChat.FullMaxOutputTokens = 777
	cfg.normalize()
	cfg.AIChat.forceFullReview = true
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	svc.httpClient = server.Client()
	content := ContentModerationInput{
		Text:            target,
		CurrentText:     target,
		AuditTargetText: target,
		AuditTargetKind: "tool_continuation",
		Turns: []ContentModerationTurn{{
			Role:               "tool",
			Source:             "tool_output",
			Purpose:            "audit_target",
			Text:               target,
			Current:            true,
			LinkedToUserIntent: true,
		}},
	}
	content.Normalize()

	result, plan, err := svc.callIncrementalAIChatAudit(context.Background(), ContentModerationCheckInput{
		UserID: 71, APIKeyID: 72, RequestID: "tool-full-bound", Protocol: ContentModerationProtocolOpenAIResponses,
	}, cfg, content, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, plan)
	require.Equal(t, voteaimoderation.StageFull, result.Stage)
	require.LessOrEqual(t, len([]rune(plan.fullInput)), cfg.AIChat.FullReviewMaxInputChars)
	require.Greater(t, len([]rune(plan.fullInput)), len([]rune(plan.fastInput.Text)))
	require.Contains(t, plan.fullInput, middleRisk)
	require.True(t, plan.fullHistoryTruncated)
	require.Equal(t, int64(1), calls.Load())
}

func TestContentModerationToolContinuation_OversizedFullSampleIsStableAndCacheable(t *testing.T) {
	target := "CACHE-TARGET-HEAD " + strings.Repeat("a", 8000) + " CACHE-TARGET-MIDDLE " + strings.Repeat("z", 8000) + " CACHE-TARGET-TAIL"
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeContentModerationGuardResult(w, false, 0.03, nil, nil, "stable cached sample")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.CacheEnabled = true
	cfg.AIChat.CacheTTLSeconds = 60
	cfg.AIChat.MaxInputChars = 10000
	cfg.AIChat.FullReviewMaxInputChars = 4000
	cfg.normalize()
	cfg.AIChat.forceFullReview = true
	cache := newContentModerationGuardCache()
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	svc.httpClient = server.Client()
	content := ContentModerationInput{
		Text: target, CurrentText: target, AuditTargetText: target, AuditTargetKind: "tool_continuation",
		Turns: []ContentModerationTurn{{
			Role: "tool", Source: "tool_output", Purpose: "audit_target", Text: target,
			Current: true, LinkedToUserIntent: true,
		}},
	}
	content.Normalize()
	input := ContentModerationCheckInput{Protocol: ContentModerationProtocolOpenAIResponses}

	first, firstPlan, err := svc.callIncrementalAIChatAudit(context.Background(), input, cfg, content, false)
	require.NoError(t, err)
	second, secondPlan, err := svc.callIncrementalAIChatAudit(context.Background(), input, cfg, content, false)
	require.NoError(t, err)

	require.False(t, first.ResultCacheHit)
	require.True(t, second.ResultCacheHit)
	require.Equal(t, firstPlan.fastInput.Text, secondPlan.fastInput.Text)
	require.Equal(t, firstPlan.fullInput, secondPlan.fullInput)
	require.LessOrEqual(t, len([]rune(secondPlan.fullInput)), cfg.AIChat.FullReviewMaxInputChars)
	require.Equal(t, int64(1), calls.Load())
}
