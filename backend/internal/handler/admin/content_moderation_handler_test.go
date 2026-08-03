package admin

import (
	"encoding/json"
	"testing"

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
		require.NoError(t, json.Unmarshal([]byte(`{"ai_thinking_mode":"disabled","ai_reasoning_effort":"high"}`), &req))
		require.Equal(t, "disabled", req.AIThinkingMode)
		require.Equal(t, "high", req.AIReasoningEffort)
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
