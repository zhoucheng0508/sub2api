//go:build unit

package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/internalprobe"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInternalProbeMiddlewareMarksOnlyValidRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth := internalprobe.NewAuthenticator("middleware-test-secret-that-is-long-enough")
	body := `{"input":"health check"}`
	signed := httptest.NewRequest(http.MethodPost, "http://example.test/v1/responses", strings.NewReader(body))
	require.NoError(t, auth.SignRequest(signed))

	assertRequest := func(t *testing.T, marker string, wantMarked bool) {
		t.Helper()
		router := gin.New()
		router.Use(InternalProbe(auth))
		router.POST("/v1/responses", func(c *gin.Context) {
			readBody, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			require.Equal(t, body, string(readBody))
			require.Empty(t, c.GetHeader(internalprobe.HeaderName))
			require.Equal(t, wantMarked, internalprobe.IsMarked(c.Request.Context()))
			c.Status(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		req.Header.Set(internalprobe.HeaderName, marker)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusNoContent, recorder.Code)
	}

	t.Run("valid", func(t *testing.T) {
		assertRequest(t, signed.Header.Get(internalprobe.HeaderName), true)
	})
	t.Run("forged", func(t *testing.T) {
		assertRequest(t, "v1:1:Zm9yZ2Vk:Zm9yZ2Vk", false)
	})
}

func TestInternalProbeMiddlewareMarksGeminiV1BetaRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth := internalprobe.NewAuthenticator("middleware-test-secret-that-is-long-enough")
	body := `{"contents":[{"parts":[{"text":"health check"}]}]}`
	path := "/v1beta/models/gemini-test:generateContent"
	signed := httptest.NewRequest(http.MethodPost, "http://example.test"+path, strings.NewReader(body))
	require.NoError(t, auth.SignRequest(signed))

	router := gin.New()
	router.Use(InternalProbe(auth))
	router.POST("/v1beta/models/*modelAction", func(c *gin.Context) {
		readBody, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Equal(t, body, string(readBody))
		require.Empty(t, c.GetHeader(internalprobe.HeaderName))
		require.True(t, internalprobe.IsMarked(c.Request.Context()))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set(internalprobe.HeaderName, signed.Header.Get(internalprobe.HeaderName))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}
