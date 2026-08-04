package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatCompletionsToResponsesPreservesPromptCacheFieldsAndSemantics(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model:              "gpt-5.6-terra",
		PromptCacheKey:     "client-key",
		PromptCacheOptions: json.RawMessage(`{"mode":"explicit"}`),
		Messages: []ChatMessage{
			{Role: "developer", Content: json.RawMessage(`[{"type":"text","text":"stable","prompt_cache_breakpoint":{"mode":"explicit"}}]`)},
			{Role: "user", Content: json.RawMessage(`"question"`)},
		},
		Tools: []ChatTool{{Type: "function", Function: &ChatFunction{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}}},
	}

	got, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)
	require.Equal(t, "client-key", got.PromptCacheKey)
	require.JSONEq(t, `{"mode":"explicit"}`, string(got.PromptCacheOptions))
	require.Equal(t, "developer", gjson.GetBytes(got.Input, "0.role").String())
	require.Equal(t, "stable", gjson.GetBytes(got.Input, "0.content.0.text").String())
	require.Equal(t, "explicit", gjson.GetBytes(got.Input, "0.content.0.prompt_cache_breakpoint.mode").String())
	require.Equal(t, "user", gjson.GetBytes(got.Input, "1.role").String())
	require.Equal(t, "question", gjson.GetBytes(got.Input, "1.content").String())
	require.Len(t, got.Tools, 1)
	require.Equal(t, "lookup", got.Tools[0].Name)
}
