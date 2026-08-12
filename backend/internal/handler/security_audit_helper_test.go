package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/internalprobe"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCachesSecurityAuditCompletionSkipsWebSocketStages(t *testing.T) {
	require.True(t, cachesSecurityAuditCompletion("http"))
	require.True(t, cachesSecurityAuditCompletion(""))
	require.False(t, cachesSecurityAuditCompletion("first_turn"))
	require.False(t, cachesSecurityAuditCompletion("subsequent_turn"))
}

func TestBuildSecurityAuditRequestPreservesClientSessionID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("X-Session-Id", "conversation-123")
	c.Request.Header.Set("User-Agent", "test-client/1.0")
	c.Request.Header.Set("X-Codex-Signal", "bounded-observation")
	service.SetOpsCyberPolicyEpochSnapshot(c, 9, true)

	req := buildSecurityAuditRequest(c, nil, middleware2.AuthSubject{UserID: 7}, "openai_chat_completions", "gpt-test", []byte(`{}`), "http")
	require.Equal(t, "conversation-123", req.SessionID)
	require.Equal(t, service.ContentModerationSessionSourceHeader, req.SessionSource)
	require.Equal(t, "test-client/1.0", req.ClientHeaders.Get("User-Agent"))
	require.Equal(t, "bounded-observation", req.ClientHeaders.Get("X-Codex-Signal"))
	require.False(t, req.TrustedMetadataProvenance)
	require.True(t, req.ModerationEpochSet)
	require.EqualValues(t, 9, req.ModerationEpoch)
	legacy := buildContentModerationInput(c, nil, middleware2.AuthSubject{UserID: 7}, "openai_chat_completions", "gpt-test", []byte(`{}`))
	require.True(t, legacy.ModerationEpochSet)
	require.EqualValues(t, 9, legacy.ModerationEpoch)
}

func TestRunSecurityAuditDoesNotSkipSubsequentWebSocketTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{mode: securityaudit.ModeAsync}
	coordinator := securityaudit.NewCoordinator(nil, engine)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	subject := middleware2.AuthSubject{UserID: 7, Concurrency: 1}
	first := runSecurityAudit(c, nil, coordinator, nil, nil, subject, "openai_responses", "gpt-test",
		[]byte(`{"type":"response.create","response":{"input":"benign"}}`), "first_turn")
	require.NotNil(t, first)
	require.True(t, first.AllowNextStage)
	require.Equal(t, int64(1), engine.enqueues.Load())
	_, cached := c.Get(securityAuditCompletedContextKey)
	require.False(t, cached, "WebSocket stages must not set the HTTP completion cache")

	// Even if an HTTP path previously cached completion on this Context, WS turns
	// must still audit every response.create payload.
	c.Set(securityAuditCompletedContextKey, true)

	second := runSecurityAudit(c, nil, coordinator, nil, nil, subject, "openai_responses", "gpt-test",
		[]byte(`{"type":"response.create","response":{"input":"malicious follow-up"}}`), "subsequent_turn")
	require.NotNil(t, second)
	require.Equal(t, int64(2), engine.enqueues.Load(), "subsequent WebSocket turns must be audited again")
}

func TestRunSecurityAuditDefersUntilProtectedAccountAndCachesSuccessfulAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newHandlerScopeModerationService(t,
		service.ContentModerationUserFilter{Type: service.ContentModerationScopeFilterAll},
		service.ContentModerationAccountFilter{Type: service.ContentModerationScopeFilterInclude, AccountIDs: []int64{202}},
	)
	engine := &handlerPromptEngine{mode: securityaudit.ModeBlocking, decision: &securityaudit.PromptDecision{
		Kind: securityaudit.DecisionAllow, AllowNextStage: true,
	}}
	coordinator := securityaudit.NewCoordinator(nil, engine)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hello"}`))
	subject := middleware2.AuthSubject{UserID: 7}

	decision := runSecurityAudit(c, nil, coordinator, svc, nil, subject, service.ContentModerationProtocolOpenAIResponses, "gpt-test", []byte(`{"input":"hello"}`), "http")
	require.Nil(t, decision, "account-scoped audit must wait for routing")
	_, _, epochCaptured := service.GetOpsCyberPolicyEpochSnapshot(c)
	require.True(t, epochCaptured, "the user epoch must be captured before account routing reaches the upstream pool")
	require.Zero(t, engine.evaluated)
	_, completed := c.Get(securityAuditCompletedContextKey)
	require.False(t, completed)

	plus := &service.Account{ID: 101, Name: "plus"}
	decision = runSecurityAuditForAccount(c, nil, coordinator, svc, nil, subject, plus, service.ContentModerationProtocolOpenAIResponses, "gpt-test", []byte(`{"input":"hello"}`), "http")
	require.Nil(t, decision, "unprotected account must skip without completing the request audit")
	require.Zero(t, engine.evaluated)
	_, completed = c.Get(securityAuditCompletedContextKey)
	require.False(t, completed)

	proParentID := int64(202)
	pro := &service.Account{ID: 302, Name: "pro-spark-shadow", ParentAccountID: &proParentID}
	decision = runSecurityAuditForAccount(c, nil, coordinator, svc, nil, subject, pro, service.ContentModerationProtocolOpenAIResponses, "gpt-test", []byte(`{"input":"hello"}`), "http")
	require.NotNil(t, decision)
	require.True(t, decision.AllowNextStage)
	evaluated, _, requests := engine.snapshot()
	require.Equal(t, 1, evaluated)
	require.Len(t, requests, 1)
	require.Equal(t, pro.ID, requests[0].AccountID)
	require.Equal(t, pro.Name, requests[0].AccountName)

	decision = runSecurityAuditForAccount(c, nil, coordinator, svc, nil, subject, &service.Account{ID: 203, Name: "fallback-pro"}, service.ContentModerationProtocolOpenAIResponses, "gpt-test", []byte(`{"input":"hello"}`), "http")
	require.Nil(t, decision, "a passed request must not be re-audited during failover")
	evaluated, _, _ = engine.snapshot()
	require.Equal(t, 1, evaluated)
}

func TestRunSecurityAuditRunsBeforeRoutingWhenEntireGroupIsProtected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const groupID int64 = 8
	settingRepo := &contentModerationHandlerSettingRepo{values: map[string]string{}}
	groupRepo := &handlerScopeGroupRepo{accountIDs: []int64{202, 203}}
	svc := service.NewContentModerationService(settingRepo, nil, nil, groupRepo, nil, nil, nil, nil)
	accountFilter := service.ContentModerationAccountFilter{Type: service.ContentModerationScopeFilterInclude, AccountIDs: []int64{202, 203}}
	_, err := svc.UpdateConfig(context.Background(), service.UpdateContentModerationConfigInput{AccountFilter: &accountFilter})
	require.NoError(t, err)
	engine := &handlerPromptEngine{mode: securityaudit.ModeBlocking, decision: &securityaudit.PromptDecision{
		Kind: securityaudit.DecisionAllow, AllowNextStage: true,
	}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-2","prompt":"hello"}`))
	apiKey := &service.APIKey{ID: 9, UserID: 7, GroupID: contentModerationHandlerInt64Ptr(groupID)}

	decision := runSecurityAudit(c, nil, securityaudit.NewCoordinator(nil, engine), svc, apiKey, middleware2.AuthSubject{UserID: 7}, service.ContentModerationProtocolOpenAIImages, "gpt-image-2", []byte(`{"prompt":"hello"}`), "http")
	require.NotNil(t, decision)
	require.True(t, decision.AllowNextStage)
	require.Equal(t, 1, engine.evaluated)
	completed, exists := c.Get(securityAuditCompletedContextKey)
	require.True(t, exists)
	require.Equal(t, true, completed)
}

func TestRunSecurityAuditAccountExcludeUsesShadowParentIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newHandlerScopeModerationService(t,
		service.ContentModerationUserFilter{Type: service.ContentModerationScopeFilterAll},
		service.ContentModerationAccountFilter{Type: service.ContentModerationScopeFilterExclude, AccountIDs: []int64{202}},
	)
	engine := &handlerPromptEngine{mode: securityaudit.ModeBlocking, decision: &securityaudit.PromptDecision{
		Kind: securityaudit.DecisionAllow, AllowNextStage: true,
	}}
	coordinator := securityaudit.NewCoordinator(nil, engine)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	subject := middleware2.AuthSubject{UserID: 7}

	require.Nil(t, runSecurityAudit(c, nil, coordinator, svc, nil, subject, service.ContentModerationProtocolOpenAIResponses, "gpt-test", []byte(`{}`), "http"))
	excludedParentID := int64(202)
	excludedShadow := &service.Account{ID: 302, Name: "excluded-shadow", ParentAccountID: &excludedParentID}
	require.Nil(t, runSecurityAuditForAccount(c, nil, coordinator, svc, nil, subject, excludedShadow, service.ContentModerationProtocolOpenAIResponses, "gpt-test", []byte(`{}`), "http"))
	evaluated, _, _ := engine.snapshot()
	require.Zero(t, evaluated)

	includedParentID := int64(203)
	includedShadow := &service.Account{ID: 303, Name: "included-shadow", ParentAccountID: &includedParentID}
	decision := runSecurityAuditForAccount(c, nil, coordinator, svc, nil, subject, includedShadow, service.ContentModerationProtocolOpenAIResponses, "gpt-test", []byte(`{}`), "http")
	require.NotNil(t, decision)
	require.True(t, decision.AllowNextStage)
	evaluated, _, requests := engine.snapshot()
	require.Equal(t, 1, evaluated)
	require.Len(t, requests, 1)
	require.Equal(t, includedShadow.ID, requests[0].AccountID, "audit request logging keeps the selected child identity")
	require.Equal(t, includedShadow.Name, requests[0].AccountName)
}

func TestRunSecurityAuditExcludedUserBypassesEveryAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newHandlerScopeModerationService(t,
		service.ContentModerationUserFilter{Type: service.ContentModerationScopeFilterExclude, UserIDs: []int64{7}},
		service.ContentModerationAccountFilter{Type: service.ContentModerationScopeFilterInclude, AccountIDs: []int64{202}},
	)
	engine := &handlerPromptEngine{mode: securityaudit.ModeBlocking, decision: &securityaudit.PromptDecision{Kind: securityaudit.DecisionAllow, AllowNextStage: true}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	subject := middleware2.AuthSubject{UserID: 7}

	require.Nil(t, runSecurityAudit(c, nil, securityaudit.NewCoordinator(nil, engine), svc, nil, subject, service.ContentModerationProtocolOpenAIResponses, "gpt-test", []byte(`{}`), "http"))
	require.Nil(t, runSecurityAuditForAccount(c, nil, securityaudit.NewCoordinator(nil, engine), svc, nil, subject, &service.Account{ID: 202}, service.ContentModerationProtocolOpenAIResponses, "gpt-test", []byte(`{}`), "http"))
	require.Zero(t, engine.evaluated)
	_, completed := c.Get(securityAuditCompletedContextKey)
	require.False(t, completed)
}

func TestRunSecurityAuditInternalProbeBypassesCoordinator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := internalprobe.NewAuthenticator("unit-test-secret")
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test"}`))
	require.NoError(t, authenticator.SignRequest(req))
	require.True(t, authenticator.VerifyAndMarkRequest(req))
	engine := &handlerPromptEngine{mode: securityaudit.ModeBlocking, decision: &securityaudit.PromptDecision{Kind: securityaudit.DecisionAllow, AllowNextStage: true}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	decision := runSecurityAudit(c, nil, securityaudit.NewCoordinator(nil, engine), nil, nil, middleware2.AuthSubject{UserID: 1}, service.ContentModerationProtocolOpenAIChat, "gpt-test", []byte(`{}`), "http")
	require.Nil(t, decision)
	require.Zero(t, engine.evaluated)
	_, completed := c.Get(securityAuditCompletedContextKey)
	require.False(t, completed)
}

func TestLegacySecurityAuditUnavailableUsesRetryableErrorAcrossProtocols(t *testing.T) {
	legacy := &service.ContentModerationDecision{
		Allowed:    false,
		Blocked:    true,
		Message:    "Content audit service is temporarily unavailable; please retry later",
		StatusCode: http.StatusServiceUnavailable,
		Action:     service.ContentModerationActionUnavailable,
	}
	decision := legacySecurityAuditDecision(legacy)
	require.NotNil(t, decision)
	require.Equal(t, securityaudit.DecisionUnavailable, decision.Kind)
	require.Equal(t, service.ContentModerationErrorCodeUnavailable, decision.ErrorCode)
	require.Equal(t, service.ContentModerationErrorCodeUnavailable, decision.Legacy.ErrorCode)
	require.False(t, decision.AllowNextStage)

	c, recorder := securityAuditErrorTestContext(t)
	(&OpenAIGatewayHandler{}).openAISecurityAuditError(c, decision)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), service.ContentModerationErrorCodeUnavailable)
	require.NotContains(t, recorder.Body.String(), "content_policy_violation")

	cSSE, recorderSSE := securityAuditErrorTestContext(t)
	_, err := cSSE.Writer.Write([]byte("data: start\n\n"))
	require.NoError(t, err)
	(&OpenAIGatewayHandler{}).openAISecurityAuditError(cSSE, decision)
	require.Contains(t, recorderSSE.Body.String(), service.ContentModerationErrorCodeUnavailable)
	require.NotContains(t, recorderSSE.Body.String(), "content_policy_violation")

	require.Equal(t, "api_error", securityAuditStreamErrorType(decision))
	require.Equal(t, coderws.StatusTryAgainLater, securityAuditWSCloseStatus(decision))
	require.Equal(t, service.ContentModerationErrorCodeUnavailable, securityAuditWSCloseReason(decision))
}

func TestLegacySecurityAuditBlockKeepsPolicyViolationErrorCode(t *testing.T) {
	decision := legacySecurityAuditDecision(&service.ContentModerationDecision{
		Allowed:    false,
		Blocked:    true,
		Message:    "blocked",
		StatusCode: http.StatusForbidden,
		Action:     service.ContentModerationActionBlock,
	})
	require.Equal(t, securityaudit.DecisionBlock, decision.Kind)
	require.Equal(t, "content_policy_violation", decision.ErrorCode)
	require.Equal(t, coderws.StatusPolicyViolation, securityAuditWSCloseStatus(decision))
}

func TestRunSecurityAuditBlockedProtectedAccountDoesNotComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newHandlerScopeModerationService(t,
		service.ContentModerationUserFilter{Type: service.ContentModerationScopeFilterAll},
		service.ContentModerationAccountFilter{Type: service.ContentModerationScopeFilterInclude, AccountIDs: []int64{202}},
	)
	engine := &handlerPromptEngine{mode: securityaudit.ModeBlocking, decision: &securityaudit.PromptDecision{Kind: securityaudit.DecisionBlock, AllowNextStage: false}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	subject := middleware2.AuthSubject{UserID: 7}

	require.Nil(t, runSecurityAudit(c, nil, securityaudit.NewCoordinator(nil, engine), svc, nil, subject, service.ContentModerationProtocolOpenAIResponses, "gpt-test", []byte(`{}`), "http"))
	decision := runSecurityAuditForAccount(c, nil, securityaudit.NewCoordinator(nil, engine), svc, nil, subject, &service.Account{ID: 202}, service.ContentModerationProtocolOpenAIResponses, "gpt-test", []byte(`{}`), "http")
	require.NotNil(t, decision)
	require.False(t, decision.AllowNextStage)
	_, completed := c.Get(securityAuditCompletedContextKey)
	require.False(t, completed)
	upstreamCalls := 0
	if decision.AllowNextStage {
		upstreamCalls++
	}
	require.Zero(t, upstreamCalls)
}

func newHandlerScopeModerationService(t *testing.T, userFilter service.ContentModerationUserFilter, accountFilter service.ContentModerationAccountFilter) *service.ContentModerationService {
	t.Helper()
	repo := &contentModerationHandlerSettingRepo{values: map[string]string{}}
	svc := service.NewContentModerationService(repo, nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.UpdateConfig(context.Background(), service.UpdateContentModerationConfigInput{
		UserFilter: &userFilter, AccountFilter: &accountFilter,
	})
	require.NoError(t, err)
	return svc
}

type handlerScopeGroupRepo struct {
	service.GroupRepository
	accountIDs []int64
}

func (r *handlerScopeGroupRepo) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	return append([]int64(nil), r.accountIDs...), nil
}

func contentModerationHandlerInt64Ptr(value int64) *int64 {
	return &value
}

func TestRunSecurityAuditLogsWebSocketChecksAndCacheHits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{mode: securityaudit.ModeBlocking}
	coordinator := securityaudit.NewCoordinator(nil, engine)
	core, logs := observer.New(zap.InfoLevel)
	reqLog := zap.New(core)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(securityAuditWSTurnContextKey, 2)
	payload := []byte(`{"type":"response.create","response":{"input":"same turn"}}`)

	runSecurityAudit(c, reqLog, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_responses", "gpt-test", payload, "subsequent_turn")
	runSecurityAudit(c, reqLog, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_responses", "gpt-test", payload, "subsequent_turn")

	startLogs := logs.FilterMessage("security_audit.gateway_check_start").All()
	require.Len(t, startLogs, 1)
	require.Equal(t, false, startLogs[0].ContextMap()["cached"])

	doneLogs := logs.FilterMessage("security_audit.gateway_check_done").All()
	require.Len(t, doneLogs, 2)
	require.Equal(t, false, doneLogs[0].ContextMap()["cached"])
	require.Equal(t, true, doneLogs[1].ContextMap()["cached"])
	require.Equal(t, "allow", doneLogs[1].ContextMap()["decision"])
	require.Equal(t, "subsequent_turn", doneLogs[1].ContextMap()["stage"])
	require.Equal(t, int64(1), engine.evaluates.Load())
}

type turnCountingEngine struct {
	mode     securityaudit.Mode
	enqueues atomic.Int64
	evaluates atomic.Int64
}

func (e *turnCountingEngine) EffectiveMode() securityaudit.Mode { return e.mode }
func (e *turnCountingEngine) Enqueue(context.Context, securityaudit.Request) error {
	e.enqueues.Add(1)
	return nil
}
func (e *turnCountingEngine) Evaluate(context.Context, securityaudit.Request) (*securityaudit.PromptDecision, error) {
	e.evaluates.Add(1)
	return &securityaudit.PromptDecision{Kind: securityaudit.DecisionAllow, AllowNextStage: true}, nil
}
