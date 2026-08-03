package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountScopedAuditBlockUsesResponsesSSEAfterWaitPing(t *testing.T) {
	c, recorder := committedSecurityAuditTestContext(t, "/v1/responses")
	(&OpenAIGatewayHandler{}).openAISecurityAuditError(c, blockedSecurityAuditTestDecision())

	require.Equal(t, http.StatusOK, recorder.Code, "an already committed stream cannot change HTTP status")
	require.Contains(t, recorder.Body.String(), "event: response.failed")
	require.Contains(t, recorder.Body.String(), "blocked by scope test")
	require.NotContains(t, recorder.Body.String(), `}{"error"`, "must not append a JSON response to an SSE stream")
}

func TestAccountScopedAuditBlockUsesAnthropicSSEAfterWaitPing(t *testing.T) {
	c, recorder := committedSecurityAuditTestContext(t, "/v1/messages")
	(&GatewayHandler{}).anthropicSecurityAuditError(c, blockedSecurityAuditTestDecision())

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `data: {"type":"error"`)
	require.Contains(t, recorder.Body.String(), "blocked by scope test")
}

func TestAccountScopedAuditBlockUsesGeminiSSEAfterCommittedResponse(t *testing.T) {
	c, recorder := committedSecurityAuditTestContext(t, "/v1beta/models/gemini:streamGenerateContent")
	googleSecurityAuditError(c, blockedSecurityAuditTestDecision())

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `data: {"error":{`)
	require.Contains(t, recorder.Body.String(), `"code":403`)
	require.Contains(t, recorder.Body.String(), `"status":"PERMISSION_DENIED"`)
	require.Contains(t, recorder.Body.String(), "blocked by scope test")
	require.NotContains(t, recorder.Body.String(), `}{"error"`, "must not append a JSON response to an SSE stream")

	streamErr, ok := service.GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, http.StatusForbidden, streamErr.IntendedStatus)
}

func committedSecurityAuditTestContext(t *testing.T, path string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	c.Status(http.StatusOK)
	_, err := c.Writer.Write([]byte(": wait ping\n\n"))
	require.NoError(t, err)
	c.Writer.Flush()
	require.True(t, c.Writer.Written())
	return c, recorder
}

func blockedSecurityAuditTestDecision() *securityaudit.Decision {
	return &securityaudit.Decision{
		Kind:           securityaudit.DecisionBlock,
		HTTPStatus:     http.StatusForbidden,
		ErrorCode:      securityaudit.ErrorCodeBlocked,
		ClientMessage:  "blocked by scope test",
		AllowNextStage: false,
	}
}
