package admin

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestContentModerationRequestsDecodeAIThinkingSettings(t *testing.T) {
	t.Run("config update", func(t *testing.T) {
		var req contentModerationConfigRequest
		require.NoError(t, json.Unmarshal([]byte(`{"ai_thinking_mode":"enabled","ai_reasoning_effort":"max"}`), &req))
		require.NotNil(t, req.AIThinkingMode)
		require.NotNil(t, req.AIReasoningEffort)
		require.Equal(t, "enabled", *req.AIThinkingMode)
		require.Equal(t, "max", *req.AIReasoningEffort)
	})

	t.Run("api key test", func(t *testing.T) {
		var req contentModerationAPIKeyTestRequest
		require.NoError(t, json.Unmarshal([]byte(`{"ai_thinking_mode":"disabled","ai_reasoning_effort":"high","ai_synchronous_budget_ms":4500,"ai_fast_input_chars":16000,"ai_fallback_input_chars":6000,"ai_risk_levels_enabled":false,"ai_observe_threshold":0.42}`), &req))
		require.Equal(t, "disabled", req.AIThinkingMode)
		require.Equal(t, "high", req.AIReasoningEffort)
		require.Equal(t, 4500, req.AISynchronousBudgetMS)
		require.Equal(t, 16000, req.AIFastInputChars)
		require.Equal(t, 6000, req.AIFallbackInputChars)
		require.NotNil(t, req.AIRiskLevelsEnabled)
		require.False(t, *req.AIRiskLevelsEnabled)
		require.Equal(t, 0.42, req.AIObserveThreshold)
	})
}

func TestContentModerationConfigRequestDecodesSessionRiskSettings(t *testing.T) {
	var req contentModerationConfigRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"ai_risk_levels_enabled":true,
		"ai_observe_threshold":0.35,
		"ai_session_risk_enabled":true,
		"ai_session_risk_ttl_minutes":120,
		"ai_session_risk_half_life_minutes":30,
		"ai_session_risk_block_cooldown_minutes":45,
		"ai_actor_risk_enabled":true
	}`), &req))
	require.Equal(t, true, *req.AIRiskLevelsEnabled)
	require.InDelta(t, 0.35, *req.AIObserveThreshold, 0.0001)
	require.Equal(t, true, *req.AISessionRiskEnabled)
	require.Equal(t, 120, *req.AISessionRiskTTLMinutes)
	require.Equal(t, 30, *req.AISessionRiskHalfLifeMinutes)
	require.Equal(t, 45, *req.AISessionRiskBlockCooldownMinutes)
	require.Equal(t, true, *req.AIActorRiskEnabled)
}

func TestContentModerationConfigRequestDecodesScopeFilters(t *testing.T) {
	var req contentModerationConfigRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"user_filter":{"type":"exclude","user_ids":[3,7]},
		"account_filter":{"type":"include","account_ids":[11,13]}
	}`), &req))
	require.NotNil(t, req.UserFilter)
	require.Equal(t, "exclude", req.UserFilter.Type)
	require.Equal(t, []int64{3, 7}, req.UserFilter.UserIDs)
	require.NotNil(t, req.AccountFilter)
	require.Equal(t, "include", req.AccountFilter.Type)
	require.Equal(t, []int64{11, 13}, req.AccountFilter.AccountIDs)
}

func TestContentModerationConfigRequestDecodesAndMapsIncrementalSettings(t *testing.T) {
	var req contentModerationConfigRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"ai_incremental_audit_enabled":false,
		"ai_input_provenance_v2_enabled":false,
		"ai_deterministic_risk_v2_enabled":true,
		"ai_recent_user_turns":3,
		"ai_summary_max_chars":1000,
		"ai_full_review_threshold":0.45,
		"ai_full_review_risk_delta":0.2,
		"ai_periodic_full_review_turns":12,
		"ai_full_review_max_input_chars":80000,
		"ai_fast_max_output_tokens":320,
		"ai_full_max_output_tokens":1280,
		"ai_max_review_max_output_tokens":1792,
		"ai_audit_context_ttl_minutes":180,
		"ai_pricing_configured":true,
		"ai_pricing_version":"deepseek-2026-08",
		"ai_uncached_input_usd_per_million_tokens":0.28,
		"ai_cached_input_usd_per_million_tokens":0.028,
		"ai_output_usd_per_million_tokens":0.42
	}`), &req))

	input := service.UpdateContentModerationConfigInput{}
	applyContentModerationIncrementalConfig(&input, req)

	require.NotNil(t, input.AIIncrementalAuditEnabled)
	require.False(t, *input.AIIncrementalAuditEnabled, "an explicit false switch must not be lost as a zero value")
	require.NotNil(t, input.AIInputProvenanceV2Enabled)
	require.False(t, *input.AIInputProvenanceV2Enabled)
	require.NotNil(t, input.AIDeterministicRiskV2Enabled)
	require.True(t, *input.AIDeterministicRiskV2Enabled)
	require.Equal(t, 3, *input.AIRecentUserTurns)
	require.Equal(t, 1000, *input.AISummaryMaxChars)
	require.InDelta(t, 0.45, *input.AIFullReviewThreshold, 0.0001)
	require.InDelta(t, 0.2, *input.AIFullReviewRiskDelta, 0.0001)
	require.Equal(t, 12, *input.AIPeriodicFullReviewTurns)
	require.Equal(t, 80000, *input.AIFullReviewMaxInputChars)
	require.Equal(t, 320, *input.AIFastMaxOutputTokens)
	require.Equal(t, 1280, *input.AIFullMaxOutputTokens)
	require.Equal(t, 1792, *input.AIMaxReviewMaxOutputTokens)
	require.Equal(t, 180, *input.AIAuditContextTTLMinutes)
	require.True(t, *input.AIPricingConfigured)
	require.Equal(t, "deepseek-2026-08", *input.AIPricingVersion)
	require.InDelta(t, 0.28, *input.AIUncachedInputUSDPerMillionTokens, 0.0001)
	require.InDelta(t, 0.028, *input.AICachedInputUSDPerMillionTokens, 0.0001)
	require.InDelta(t, 0.42, *input.AIOutputUSDPerMillionTokens, 0.0001)
}

func TestContentModerationConfigRequestLeavesAbsentIncrementalSettingsUnchanged(t *testing.T) {
	var req contentModerationConfigRequest
	require.NoError(t, json.Unmarshal([]byte(`{"mode":"observe"}`), &req))

	input := service.UpdateContentModerationConfigInput{}
	applyContentModerationIncrementalConfig(&input, req)

	require.Nil(t, input.AIIncrementalAuditEnabled)
	require.Nil(t, input.AIInputProvenanceV2Enabled)
	require.Nil(t, input.AIDeterministicRiskV2Enabled)
	require.Nil(t, input.AIRecentUserTurns)
	require.Nil(t, input.AISummaryMaxChars)
	require.Nil(t, input.AIFullReviewThreshold)
	require.Nil(t, input.AIFullReviewRiskDelta)
	require.Nil(t, input.AIPeriodicFullReviewTurns)
	require.Nil(t, input.AIFullReviewMaxInputChars)
	require.Nil(t, input.AIFastMaxOutputTokens)
	require.Nil(t, input.AIFullMaxOutputTokens)
	require.Nil(t, input.AIMaxReviewMaxOutputTokens)
	require.Nil(t, input.AIAuditContextTTLMinutes)
	require.Nil(t, input.AIPricingConfigured)
	require.Nil(t, input.AIPricingVersion)
	require.Nil(t, input.AIUncachedInputUSDPerMillionTokens)
	require.Nil(t, input.AICachedInputUSDPerMillionTokens)
	require.Nil(t, input.AIOutputUSDPerMillionTokens)
}
