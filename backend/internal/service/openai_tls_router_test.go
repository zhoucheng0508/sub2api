package service

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type tlsRouterRepoStub struct{ rows []*model.TLSFingerprintRouter }

func (r *tlsRouterRepoStub) List(context.Context) ([]*model.TLSFingerprintRouter, error) {
	return r.rows, nil
}
func (r *tlsRouterRepoStub) GetByID(_ context.Context, id int64) (*model.TLSFingerprintRouter, error) {
	for _, row := range r.rows {
		if row.ID == id {
			return row, nil
		}
	}
	return nil, nil
}
func (r *tlsRouterRepoStub) Create(_ context.Context, row *model.TLSFingerprintRouter) (*model.TLSFingerprintRouter, error) {
	return row, nil
}
func (r *tlsRouterRepoStub) Update(_ context.Context, row *model.TLSFingerprintRouter) (*model.TLSFingerprintRouter, error) {
	return row, nil
}
func (r *tlsRouterRepoStub) Delete(context.Context, int64) error { return nil }

type tlsRecordingUpstream struct {
	proxy   string
	profile *tlsfingerprint.Profile
	headers http.Header
}

func (u *tlsRecordingUpstream) Do(req *http.Request, proxy string, _ int64, _ int) (*http.Response, error) {
	return u.DoWithTLS(req, proxy, 0, 0, nil)
}
func (u *tlsRecordingUpstream) DoWithTLS(req *http.Request, proxy string, _ int64, _ int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.proxy, u.profile, u.headers = proxy, profile, req.Header.Clone()
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
}

func TestTLSFingerprintRouterMatchAndIdentity(t *testing.T) {
	router := NewTLSFingerprintRouterService(&tlsRouterRepoStub{rows: []*model.TLSFingerprintRouter{{
		ID: 7, Name: "clients", Enabled: true,
		Rules: []model.TLSFingerprintRouterRule{{
			Name: "codex desktop", Enabled: true, MatchType: model.TLSRouterMatchPrefix,
			Pattern: "codex_app/", TLSFingerprintProfileID: 3,
			UpstreamUserAgent: "codex_app/2.0", UpstreamOriginator: "codex_app",
		}},
	}}}, nil)
	match := router.MatchUserAgent(7, "Codex_App/2.3 Windows")
	require.True(t, match.Matched)
	require.Equal(t, int64(3), match.TLSFingerprintProfileID)
	require.Equal(t, "codex_app", match.UpstreamOriginator)
}

func TestOpenAITLSInvalidRouterProfileFallsBackAsIdentityPairAndKeepsProxy(t *testing.T) {
	router := NewTLSFingerprintRouterService(&tlsRouterRepoStub{rows: []*model.TLSFingerprintRouter{{
		ID: 9, Name: "invalid-target", Enabled: true,
		Rules: []model.TLSFingerprintRouterRule{{
			Name: "match", Enabled: true, MatchType: model.TLSRouterMatchContains,
			Pattern: "client-x", TLSFingerprintProfileID: 999,
			UpstreamUserAgent: "custom/1", UpstreamOriginator: "custom",
		}},
	}}}, nil)
	profiles := &TLSFingerprintProfileService{localCache: map[int64]*model.TLSFingerprintProfile{}}
	upstream := &tlsRecordingUpstream{}
	svc := &OpenAIGatewayService{httpUpstream: upstream, tlsFPProfileService: profiles, tlsFPRouterService: router}
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Extra: map[string]any{
		"enable_tls_fingerprint": true, "tls_fingerprint_router_id": float64(9),
	}}
	req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request, _ = http.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "client-x/4")

	resp, err := svc.doOpenAIUpstream(c, req, account, "socks5://127.0.0.1:1080")
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "socks5://127.0.0.1:1080", upstream.proxy)
	require.NotNil(t, upstream.profile)
	require.Equal(t, "Built-in Default (Node.js 24.x)", upstream.profile.Name)
	require.Equal(t, codexCLIUserAgent, upstream.headers.Get("User-Agent"))
	require.Equal(t, openai.CodexDefaultOriginator, upstream.headers.Get("originator"))
}

func TestOpenAITLSIdentityPassesThroughOfficialCodexNormalization(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	require.NoError(t, err)

	applyOpenAITLSIdentity(req, &tlsfingerprint.Profile{Name: "test"}, TLSFingerprintRouterMatchResult{
		Matched:            true,
		UpstreamUserAgent:  "unrecognized-client/1.0",
		UpstreamOriginator: "unrecognized-client",
	})

	require.Equal(t, codexCLIUserAgent, req.Header.Get("User-Agent"))
	require.Equal(t, openai.CodexDefaultOriginator, req.Header.Get("originator"))
}

func TestOpenAIProductionPathsDoNotBypassTLSRouter(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var violations []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "openai") || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		require.NoErrorf(t, parseErr, "parse %s", name)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || function.Name.Name == "doOpenAIUpstream" {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				method, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (method.Sel.Name != "Do" && method.Sel.Name != "DoWithTLS") {
					return true
				}
				upstream, ok := method.X.(*ast.SelectorExpr)
				if !ok || upstream.Sel.Name != "httpUpstream" {
					return true
				}
				receiver, ok := upstream.X.(*ast.Ident)
				// API key capability probing intentionally uses the account's fixed
				// TLS profile and never participates in OpenAI OAuth UA routing.
				allowedAPIKeyProbe := name == "openai_apikey_responses_probe.go" && function.Name.Name == "ProbeOpenAIAPIKeyResponsesSupport"
				if ok && receiver.Name == "s" && !allowedAPIKeyProbe {
					violations = append(violations, name+":"+function.Name.Name)
				}
				return true
			})
		}
	}

	require.Empty(t, violations, "OpenAI production paths must use doOpenAIUpstream so account proxy, TLS routing, and identity normalization cannot be bypassed")
}
