package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func promptCacheForwardContext(body []byte) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Conversation-ID", "conversation-123")
	c.Set("api_key", &APIKey{ID: 99})
	return c
}

func promptCacheForwardAccount(mode openai_compat.PromptCacheMode) *Account {
	return &Account{
		ID:          42,
		Name:        "responses-compatible",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesSupported: true,
			openai_compat.ExtraKeyPromptCacheMode:    string(mode),
		},
	}
}

func terminalPromptCacheTestResponse(message string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"` + message + `"}}`)),
	}
}

func TestForwardAsChatCompletionsAPIKeyPromptCacheModes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-terra","messages":[{"role":"system","content":"stable"},{"role":"user","content":"question"}],"stream":false}`)

	t.Run("key_only injects stable key without explicit fields", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{resp: terminalPromptCacheTestResponse("stop")}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		_, err := svc.ForwardAsChatCompletions(context.Background(), promptCacheForwardContext(body), promptCacheForwardAccount(openai_compat.PromptCacheModeKeyOnly), body, "", "gpt-5.6-terra")
		require.Error(t, err)
		require.True(t, strings.HasPrefix(gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String(), openAIPromptCacheKeyPrefix))
		require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_options").Exists())
		require.Equal(t, 0, countPromptCacheBreakpointsInBody(upstream.lastBody))
	})

	t.Run("gpt56_explicit injects options and bounded breakpoints", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{resp: terminalPromptCacheTestResponse("stop")}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		_, err := svc.ForwardAsChatCompletions(context.Background(), promptCacheForwardContext(body), promptCacheForwardAccount(openai_compat.PromptCacheModeGPT56Explicit), body, "", "gpt-5.6-terra")
		require.Error(t, err)
		require.Equal(t, "explicit", gjson.GetBytes(upstream.lastBody, "prompt_cache_options.mode").String())
		require.Equal(t, 2, countPromptCacheBreakpointsInBody(upstream.lastBody))
		require.LessOrEqual(t, countPromptCacheBreakpointsInBody(upstream.lastBody), maxOpenAIAutoCacheBreakpoints)
	})

	t.Run("off does not auto inject", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{resp: terminalPromptCacheTestResponse("stop")}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		c := promptCacheForwardContext(body)
		c.Request.Header.Del("X-Conversation-ID")
		_, err := svc.ForwardAsChatCompletions(context.Background(), c, promptCacheForwardAccount(openai_compat.PromptCacheModeOff), body, "", "gpt-5.6-terra")
		require.Error(t, err)
		require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").Exists())
		require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_options").Exists())
		require.Equal(t, 0, countPromptCacheBreakpointsInBody(upstream.lastBody))
	})
}

func TestForwardAsChatCompletionsPromptCacheRejectionRetriesOnceThroughSameProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-terra","messages":[{"role":"system","content":"stable"},{"role":"user","content":"question"}],"service_tier":"fast","stream":false}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"unsupported_parameter","param":"prompt_cache_options","message":"Unsupported parameter: prompt_cache_options"}}`)),
		},
		terminalPromptCacheTestResponse("terminal"),
	}}
	proxyID := int64(7)
	account := promptCacheForwardAccount(openai_compat.PromptCacheModeGPT56Explicit)
	account.ProxyID = &proxyID
	account.Proxy = &Proxy{ID: proxyID, Protocol: "http", Host: "127.0.0.1", Port: 7890}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	_, err := svc.ForwardAsChatCompletions(context.Background(), promptCacheForwardContext(body), account, body, "", "gpt-5.6-terra")
	require.Error(t, err)
	require.Len(t, upstream.bodies, 2)
	require.True(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_options").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_options").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").Exists())
	require.Equal(t, 0, countPromptCacheBreakpointsInBody(upstream.bodies[1]))
	require.Equal(t, "priority", gjson.GetBytes(upstream.bodies[0], "service_tier").String())
	require.Equal(t, "priority", gjson.GetBytes(upstream.bodies[1], "service_tier").String())
	require.Equal(t, "http://127.0.0.1:7890", upstream.lastProxyURL)
}

func TestForwardAsChatCompletionsPromptCacheRetryPreservesClientFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-5.6-terra",
		"prompt_cache_key":"client-key-exact",
		"prompt_cache_options":{"mode":"explicit"},
		"messages":[
			{"role":"system","content":[{"type":"text","text":"stable","prompt_cache_breakpoint":{"mode":"explicit"}}]},
			{"role":"user","content":"question"}
		],
		"stream":false
	}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"unsupported_parameter","message":"Unsupported parameter: input[1].content[0].prompt_cache_breakpoint"}}`)),
		},
		terminalPromptCacheTestResponse("terminal"),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	_, err := svc.ForwardAsChatCompletions(context.Background(), promptCacheForwardContext(body), promptCacheForwardAccount(openai_compat.PromptCacheModeGPT56Explicit), body, "", "gpt-5.6-terra")
	require.Error(t, err)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "client-key-exact", gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").String())
	require.Equal(t, "explicit", gjson.GetBytes(upstream.bodies[1], "prompt_cache_options.mode").String())
	require.Equal(t, 1, countPromptCacheBreakpointsInBody(upstream.bodies[1]))
	require.Equal(t, "stable", gjson.GetBytes(upstream.bodies[1], "input.0.content.0.text").String())
}

func TestForwardAsRawChatCompletionsNeverReceivesResponsesPromptCacheFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-terra","messages":[{"role":"user","content":"question"}],"stream":false}`)
	upstream := &httpUpstreamRecorder{resp: terminalPromptCacheTestResponse("terminal")}
	account := promptCacheForwardAccount(openai_compat.PromptCacheModeGPT56Explicit)
	account.Extra[openai_compat.ExtraKeyResponsesSupported] = false
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	_, err := svc.ForwardAsChatCompletions(context.Background(), promptCacheForwardContext(body), account, body, "", "gpt-5.6-terra")
	require.Error(t, err)
	require.Equal(t, "https://api.openai.com/v1/chat/completions", upstream.lastReq.URL.String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_options").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "messages.0.content.0.prompt_cache_breakpoint").Exists())
}

func countPromptCacheBreakpointsInBody(body []byte) int {
	count := 0
	gjson.GetBytes(body, "input.#.content.#.prompt_cache_breakpoint").ForEach(func(_, value gjson.Result) bool {
		if value.Exists() {
			count++
		}
		return true
	})
	return count
}
