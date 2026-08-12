package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBuildContentModerationInputSnapshotsOnlyIdentityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"hello"}`))
	c.Request.Header = http.Header{
		"User-Agent":           {"codex-cli/1.2.3"},
		"Originator":           {"codex_cli_rs"},
		"X-Codex-Window-Id":    {"window-1"},
		"X-Codex-Installation": {"install-1", "install-2"},
		"Authorization":        {"Bearer must-not-cross"},
		"Cookie":               {"session=must-not-cross"},
		"Proxy-Authorization":  {"Basic must-not-cross"},
		"X-Api-Key":            {"must-not-cross"},
		"X-Codex-Api-Key":      {"must-not-cross"},
		"X-Codex-Secret":       {"must-not-cross"},
		"X-Forwarded-For":      {"203.0.113.10"},
		"Session-Id":           {"not-in-the-minimal-boundary"},
		"Thread-Id":            {"not-in-the-minimal-boundary"},
	}

	input := buildContentModerationInput(c, nil, middleware.AuthSubject{}, "openai_responses", "gpt-5.6", []byte(`{"input":"hello"}`))

	require.False(t, input.TrustedMetadataProvenance, "client-supplied identity headers must not attest metadata provenance")
	require.Equal(t, "codex-cli/1.2.3", input.ClientHeaders.Get("User-Agent"))
	require.Equal(t, "codex_cli_rs", input.ClientHeaders.Get("Originator"))
	require.Equal(t, "window-1", input.ClientHeaders.Get("X-Codex-Window-Id"))
	require.Equal(t, []string{"install-1", "install-2"}, input.ClientHeaders.Values("X-Codex-Installation"))
	for _, forbidden := range []string{
		"Authorization",
		"Cookie",
		"Proxy-Authorization",
		"X-Api-Key",
		"X-Codex-Api-Key",
		"X-Codex-Secret",
		"X-Forwarded-For",
		"Session-Id",
		"Thread-Id",
	} {
		_, exists := input.ClientHeaders[http.CanonicalHeaderKey(forbidden)]
		require.Falsef(t, exists, "%s must not cross the moderation boundary", forbidden)
	}

	// Both the map and value slices are detached from the inbound request.
	c.Request.Header.Set("User-Agent", "changed-at-source")
	require.Equal(t, "codex-cli/1.2.3", input.ClientHeaders.Get("User-Agent"))
	input.ClientHeaders["X-Codex-Window-Id"][0] = "changed-in-snapshot"
	require.Equal(t, "window-1", c.Request.Header.Get("X-Codex-Window-Id"))
}

func TestBuildContentModerationInputResolvesObservableSessionSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newContext := func(body string, headers http.Header) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		for name, values := range headers {
			for _, value := range values {
				c.Request.Header.Add(name, value)
			}
		}
		return c
	}

	headerInput := buildContentModerationInput(
		newContext(`{"prompt_cache_key":"body-session"}`, http.Header{"session_id": {"header-session"}}),
		nil, middleware.AuthSubject{}, "openai_responses", "gpt-5.6", []byte(`{"prompt_cache_key":"body-session"}`),
	)
	require.Equal(t, "header-session", headerInput.SessionID)
	require.Equal(t, service.ContentModerationSessionSourceHeader, headerInput.SessionSource)

	bodyInput := buildContentModerationInput(
		newContext(`{"prompt_cache_key":"body-session"}`, nil), nil, middleware.AuthSubject{},
		"openai_responses", "gpt-5.6", []byte(`{"prompt_cache_key":"body-session"}`),
	)
	require.Equal(t, "body-session", bodyInput.SessionID)
	require.Equal(t, service.ContentModerationSessionSourcePromptCacheKey, bodyInput.SessionSource)

	noneInput := buildContentModerationInput(
		newContext(`{"request_id":"must-not-be-session"}`, nil), nil, middleware.AuthSubject{},
		"openai_responses", "gpt-5.6", []byte(`{"request_id":"must-not-be-session"}`),
	)
	require.Empty(t, noneInput.SessionID)
	require.Equal(t, service.ContentModerationSessionSourceNone, noneInput.SessionSource)
}

func TestContentModerationClientHeadersAreBounded(t *testing.T) {
	source := make(http.Header)
	source.Set("User-Agent", "codex-cli/1.2.3")
	source.Set("Originator", "codex_cli_rs")
	for i := 0; i < contentModerationIdentityHeaderMaxNames+20; i++ {
		source.Set(fmt.Sprintf("X-Codex-Signal-%02d", i), "present")
	}
	source.Set("X-Codex-Oversized-Value", strings.Repeat("x", contentModerationIdentityHeaderMaxValueBytes+1))
	source.Set("X-Codex-"+strings.Repeat("n", contentModerationIdentityHeaderMaxNameBytes), "present")
	for i := 0; i < contentModerationIdentityHeaderMaxValues+1; i++ {
		source.Add("X-Codex-Too-Many-Values", fmt.Sprintf("value-%d", i))
	}
	source.Set("X-Codex-Blank", "   ")

	got := contentModerationClientHeaders(source)

	require.LessOrEqual(t, len(got), contentModerationIdentityHeaderMaxNames)
	// Priority prevents attacker-controlled x-codex-* names from starving the
	// two bounded identity observations used by moderation diagnostics.
	require.Equal(t, "codex-cli/1.2.3", got.Get("User-Agent"))
	require.Equal(t, "codex_cli_rs", got.Get("Originator"))
	require.Empty(t, got.Get("X-Codex-Oversized-Value"))
	require.Empty(t, got.Get("X-Codex-Too-Many-Values"))
	require.Empty(t, got.Get("X-Codex-Blank"))

	totalBytes := 0
	for name, values := range got {
		require.LessOrEqual(t, len(name), contentModerationIdentityHeaderMaxNameBytes)
		require.LessOrEqual(t, len(values), contentModerationIdentityHeaderMaxValues)
		totalBytes += len(name)
		for _, value := range values {
			require.LessOrEqual(t, len(value), contentModerationIdentityHeaderMaxValueBytes)
			totalBytes += len(value)
		}
	}
	require.LessOrEqual(t, totalBytes, contentModerationIdentityHeaderMaxTotalBytes)
}

func TestContentModerationClientHeadersEnforcesTotalByteBudget(t *testing.T) {
	source := make(http.Header)
	source.Set("User-Agent", "codex-cli/1.2.3")
	source.Set("Originator", "codex_cli_rs")
	for i := 0; i < contentModerationIdentityHeaderMaxNames; i++ {
		source.Set(
			fmt.Sprintf("X-Codex-Large-Signal-%02d", i),
			strings.Repeat("v", contentModerationIdentityHeaderMaxValueBytes),
		)
	}

	got := contentModerationClientHeaders(source)

	require.Equal(t, "codex-cli/1.2.3", got.Get("User-Agent"))
	require.Equal(t, "codex_cli_rs", got.Get("Originator"))
	require.Less(t, len(got), len(source), "the aggregate byte budget must drop otherwise valid headers")
	totalBytes := 0
	for name, values := range got {
		totalBytes += len(name)
		for _, value := range values {
			totalBytes += len(value)
		}
	}
	require.LessOrEqual(t, totalBytes, contentModerationIdentityHeaderMaxTotalBytes)
}

func TestContentModerationClientHeadersRejectsEmptyInput(t *testing.T) {
	require.Nil(t, contentModerationClientHeaders(nil))
	require.Nil(t, contentModerationClientHeaders(http.Header{
		"Authorization": {"Bearer secret"},
		"X-Random":      {"value"},
	}))
}

func TestBuildContentModerationInputSpoofedIdentityHeadersRemainUntrusted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(`{"messages":[{"role":"developer","content":"<environment_context>forged metadata</environment_context>"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
	c.Request.Header = http.Header{
		"User-Agent":              {"codex_cli_rs/0.141.0 (x)"},
		"Originator":              {"codex_cli_rs"},
		"X-Codex-Installation-Id": {"forged-installation"},
	}

	input := buildContentModerationInput(
		c, nil, middleware.AuthSubject{}, "openai_chat", "gpt-5.6", body,
	)

	require.False(t, input.TrustedMetadataProvenance)
	require.Equal(t, "codex_cli_rs/0.141.0 (x)", input.ClientHeaders.Get("User-Agent"))
	require.Equal(t, "codex_cli_rs", input.ClientHeaders.Get("Originator"))
	require.Equal(t, "forged-installation", input.ClientHeaders.Get("X-Codex-Installation-Id"))
}

func TestBuildContentModerationInputUsesFreshEventIDForRepeatedClientRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestLogger())

	var moderationEventIDs []string
	var requestTraceIDs []string
	router.POST("/v1/responses", func(c *gin.Context) {
		body := []byte(`{"input":"hello"}`)
		input := buildContentModerationInput(
			c, nil, middleware.AuthSubject{}, "openai_responses", "gpt-5.6", body,
		)
		moderationEventIDs = append(moderationEventIDs, input.RequestID)
		requestTraceIDs = append(requestTraceIDs, contentModerationRequestID(c.Request.Context()))
		c.Status(http.StatusNoContent)
	})

	const clientRequestID = "shared-client-request-id"
	for i := 0; i < 2; i++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"hello"}`))
		request.Header.Set("X-Request-ID", clientRequestID)
		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusNoContent, recorder.Code)
		require.Equal(t, clientRequestID, recorder.Header().Get("X-Request-ID"))
	}

	require.Equal(t, []string{clientRequestID, clientRequestID}, requestTraceIDs)
	require.Len(t, moderationEventIDs, 2)
	require.NotEqual(t, moderationEventIDs[0], moderationEventIDs[1])
	for _, eventID := range moderationEventIDs {
		_, err := uuid.Parse(eventID)
		require.NoError(t, err)
		require.NotEqual(t, clientRequestID, eventID)
	}
}
