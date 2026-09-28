package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentModerationEngineProfilesKeepAIChatIndependent(t *testing.T) {
	repo := &contentModerationTestSettingRepo{values: map[string]string{SettingKeyContentModerationConfig: `{"engine":"typesafe","audit_provider":"openai_moderations","base_url":"https://openai.example","api_keys":["openai-test-key"],"typesafe":{"base_url":"https://typesafe.example","model":"jev-test","api_keys":["typesafe-test-key"],"timeout_ms":3000,"thresholds":{"sexual":0.92}}}`}}
	s := &ContentModerationService{settingRepo: repo}
	provider, base, model := ContentModerationProviderAIChat, "https://deepseek.example", "deepseek-v4-flash"
	keys := []string{"ai-chat-test-key"}
	view, err := s.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{AuditProvider: &provider, BaseURL: &base, Model: &model, APIKeys: &keys})
	require.NoError(t, err)
	require.Equal(t, base, view.BaseURL)
	require.Equal(t, "typesafe", view.Engine)
	require.Equal(t, "https://openai.example", view.EngineConfigs["openai"].BaseURL)
	require.Equal(t, "https://typesafe.example", view.EngineConfigs["typesafe"].BaseURL)
	var stored ContentModerationConfig
	require.NoError(t, json.Unmarshal([]byte(repo.values[SettingKeyContentModerationConfig]), &stored))
	require.Equal(t, keys, stored.AIChat.APIKeys)
	require.Equal(t, []string{"openai-test-key"}, stored.APIKeys)
	require.Equal(t, []string{"typesafe-test-key"}, stored.TypeSafe.APIKeys)
	runtime := stored.effectiveEngine(stored.Engine)
	require.Equal(t, ContentModerationProviderAIChat, runtime.AuditProvider)
	require.Equal(t, keys, runtime.apiKeys())
	require.Equal(t, base, runtime.activeBaseURL())
	provider = ContentModerationProviderOpenAIModerations
	view, err = s.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{AuditProvider: &provider})
	require.NoError(t, err)
	require.Equal(t, "https://typesafe.example", view.BaseURL)
	require.Equal(t, 0.92, view.Thresholds["sexual"])
	require.Equal(t, base, view.AIChat.BaseURL)
	require.Equal(t, 1, view.AIChat.APIKeyCount)
}
