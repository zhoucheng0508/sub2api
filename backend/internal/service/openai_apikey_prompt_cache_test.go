package service

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func promptCacheTestContext(apiKeyID int64, sessionID string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set("api_key", &APIKey{ID: apiKeyID})
	if sessionID != "" {
		c.Request.Header.Set("X-Session-ID", sessionID)
	}
	return c
}

func promptCacheTestRequest(extraMessages ...apicompat.ChatMessage) *apicompat.ChatCompletionsRequest {
	messages := []apicompat.ChatMessage{
		{Role: "system", Content: json.RawMessage(`"stable-system"`)},
		{Role: "user", Content: json.RawMessage(`"first-user"`)},
	}
	messages = append(messages, extraMessages...)
	return &apicompat.ChatCompletionsRequest{
		Model:    "gpt-5.6-terra",
		Messages: messages,
		Tools: []apicompat.ChatTool{{
			Type:     "function",
			Function: &apicompat.ChatFunction{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)},
		}},
	}
}

func TestResolveOpenAIAPIKeyPromptCacheKeyStabilityAndIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{
		openai_compat.ExtraKeyPromptCacheMode: string(openai_compat.PromptCacheModeKeyOnly),
	}}
	req := promptCacheTestRequest()

	key1, diag1 := resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(20, "conversation-a"), account, nil, req, "gpt-5.6-terra", "")
	key2, _ := resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(20, "conversation-a"), account, nil, promptCacheTestRequest(
		apicompat.ChatMessage{Role: "assistant", Content: json.RawMessage(`"later-answer"`)},
		apicompat.ChatMessage{Role: "user", Content: json.RawMessage(`"later-question"`)},
	), "gpt-5.6-terra", "")
	require.NotEmpty(t, key1)
	require.Equal(t, key1, key2)
	require.Equal(t, "session_header", diag1.KeySource)
	require.NotContains(t, key1, "conversation-a")

	differentAPIKey, _ := resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(21, "conversation-a"), account, nil, req, "gpt-5.6-terra", "")
	differentSession, _ := resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(20, "conversation-b"), account, nil, req, "gpt-5.6-terra", "")
	differentModel, _ := resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(20, "conversation-a"), account, nil, req, "gpt-5.6-luna", "")
	otherAccount := *account
	otherAccount.ID = 11
	differentAccount, _ := resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(20, "conversation-a"), &otherAccount, nil, req, "gpt-5.6-terra", "")
	require.NotEqual(t, key1, differentAPIKey)
	require.NotEqual(t, key1, differentSession)
	require.NotEqual(t, key1, differentModel)
	require.NotEqual(t, key1, differentAccount)
}

func TestResolveOpenAIAPIKeyPromptCacheKeyClientAndOffPrecedence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{
		openai_compat.ExtraKeyPromptCacheMode: string(openai_compat.PromptCacheModeGPT56Explicit),
	}}
	req := promptCacheTestRequest()
	req.PromptCacheKey = "client-key-exact"
	key, diag := resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(20, "header-session"), account, nil, req, "gpt-5.6-terra", "legacy")
	require.Equal(t, "client-key-exact", key)
	require.Equal(t, "client_body", diag.KeySource)
	require.False(t, diag.AutoKeyInjected)

	account.Extra = nil
	req.PromptCacheKey = ""
	key, diag = resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(20, "header-session"), account, nil, req, "gpt-5.6-terra", "header-session")
	require.Equal(t, "header-session", key, "off mode preserves existing explicit-session behavior")
	require.Equal(t, openai_compat.PromptCacheModeOff, diag.Mode)
	require.False(t, diag.CacheFieldsAutoInjected)
}

func TestResolveOpenAIAPIKeyPromptCacheKeyFallbackIgnoresAppendedTurns(t *testing.T) {
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{
		openai_compat.ExtraKeyPromptCacheMode: string(openai_compat.PromptCacheModeKeyOnly),
	}}
	key1, diag := resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(20, ""), account, nil, promptCacheTestRequest(), "gpt-5.6-terra", "")
	key2, _ := resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(20, ""), account, nil, promptCacheTestRequest(
		apicompat.ChatMessage{Role: "user", Content: json.RawMessage(`"newest-changing-turn"`)},
	), "gpt-5.6-terra", "")
	require.NotEmpty(t, key1)
	require.Equal(t, key1, key2)
	require.Equal(t, "auto_derived", diag.KeySource)
	require.NotContains(t, key1, "stable-system")
	require.NotContains(t, key1, "first-user")
}

func TestResolveOpenAIAPIKeyPromptCacheKeyUsesBodyConversationID(t *testing.T) {
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{
		openai_compat.ExtraKeyPromptCacheMode: string(openai_compat.PromptCacheModeKeyOnly),
	}}
	req := promptCacheTestRequest()
	key1, diag := resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(20, ""), account, []byte(`{"conversation_id":"body-a"}`), req, "gpt-5.6-terra", "")
	key2, _ := resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(20, ""), account, []byte(`{"conversation_id":"body-b"}`), req, "gpt-5.6-terra", "")
	require.NotEmpty(t, key1)
	require.NotEqual(t, key1, key2)
	require.Equal(t, "session_header", diag.KeySource)
}

func TestResolveOpenAIAPIKeyPromptCacheKeyExplicitModeSkipsOlderModels(t *testing.T) {
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{
		openai_compat.ExtraKeyPromptCacheMode: string(openai_compat.PromptCacheModeGPT56Explicit),
	}}
	key, diag := resolveOpenAIAPIKeyPromptCacheKey(promptCacheTestContext(20, "conversation-a"), account, nil, promptCacheTestRequest(), "gpt-5.5", "")
	require.Empty(t, key)
	require.False(t, diag.CacheFieldsAutoInjected)
}

func TestApplyGPT56ExplicitPromptCache(t *testing.T) {
	input := json.RawMessage(`[
		{"role":"system","content":"stable system"},
		{"role":"developer","content":[{"type":"input_text","text":"stable developer","prompt_cache_breakpoint":{"mode":"explicit"}}]},
		{"role":"user","content":"first"},
		{"role":"assistant","content":[{"type":"output_text","text":"answer"}]},
		{"role":"user","content":"latest"}
	]`)
	req := &apicompat.ResponsesRequest{Model: "gpt-5.6-terra", Input: input}
	diag := &openAIPromptCacheDiagnostics{Mode: openai_compat.PromptCacheModeGPT56Explicit}
	require.NoError(t, applyGPT56ExplicitPromptCache(req, diag))
	require.Equal(t, "explicit", gjson.GetBytes(req.PromptCacheOptions, "mode").String())
	require.Equal(t, 4, diag.BreakpointCount)
	require.Equal(t, 3, diag.AutoBreakpointCount)
	require.True(t, diag.AutoOptionsInjected)
	require.Equal(t, "stable developer", gjson.GetBytes(req.Input, "1.content.0.text").String())
	require.Equal(t, "explicit", gjson.GetBytes(req.Input, "1.content.0.prompt_cache_breakpoint.mode").String())
	require.Equal(t, "answer", gjson.GetBytes(req.Input, "3.content.0.text").String())
	require.False(t, gjson.GetBytes(req.Input, "3.content.0.prompt_cache_breakpoint").Exists())
}

func TestApplyGPT56ExplicitPromptCacheLeavesOtherModelsUntouched(t *testing.T) {
	original := json.RawMessage(`[{"role":"user","content":"hello"}]`)
	req := &apicompat.ResponsesRequest{Model: "gpt-5.5", Input: append(json.RawMessage(nil), original...)}
	diag := &openAIPromptCacheDiagnostics{Mode: openai_compat.PromptCacheModeGPT56Explicit}
	require.NoError(t, applyGPT56ExplicitPromptCache(req, diag))
	require.JSONEq(t, string(original), string(req.Input))
	require.Empty(t, req.PromptCacheOptions)
	require.False(t, diag.CacheFieldsAutoInjected)
}

func TestIsAutoPromptCacheFieldRejection(t *testing.T) {
	require.True(t, isAutoPromptCacheFieldRejection(400, []byte(`{"error":{"code":"unsupported_parameter","param":"prompt_cache_options","message":"unsupported parameter"}}`)))
	require.True(t, isAutoPromptCacheFieldRejection(400, []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: input[0].content[0].prompt_cache_breakpoint"}}`)))
	require.False(t, isAutoPromptCacheFieldRejection(400, []byte(`{"error":{"code":"unknown_parameter","param":"temperature","message":"unsupported parameter"}}`)))
}
