package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestSub2APIBalance(t *testing.T) {
	for _, tc := range []struct {
		name, body, scope string
		amount            float64
	}{
		{"wallet", `{"mode":"unrestricted","isValid":true,"balance":12.5,"remaining":12.5,"unit":"USD"}`, "wallet", 12.5},
		{"zero", `{"mode":"unrestricted","isValid":true,"balance":0,"unit":"USD"}`, "wallet", 0},
		{"negative", `{"mode":"unrestricted","isValid":true,"balance":-1,"unit":"USD"}`, "wallet", -1},
		{"key", `{"mode":"quota_limited","isValid":true,"quota":{"remaining":3},"remaining":3,"unit":"USD"}`, "key", 3},
		{"subscription", `{"mode":"unrestricted","isValid":true,"subscription":{},"remaining":7,"unit":"USD"}`, "subscription", 7},
		{"missing", `{"mode":"unrestricted","isValid":true,"unit":"USD"}`, "", 0},
		{"null", `{"mode":"unrestricted","isValid":true,"balance":null,"unit":"USD"}`, "", 0},
		{"invalid", `{"mode":"unrestricted","isValid":false,"balance":4,"unit":"USD"}`, "", 0},
		{"unlimited subscription", `{"mode":"unrestricted","isValid":true,"subscription":{},"remaining":-1,"unit":"USD"}`, "", 0},
		{"unknown", `{"balance":5}`, "", 0},
		{"html", `<html>Login</html>`, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSub2APIBalance([]byte(tc.body))
			if tc.scope == "" {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, tc.scope, got.Scope)
			require.Equal(t, tc.amount, *got.Amount)
		})
	}
}

func TestNewAPIBalance(t *testing.T) {
	status := []byte(`{"success":true,"data":{"quota_display_type":"CNY"}}`)
	sub := []byte(`{"object":"billing_subscription","hard_limit_usd":100}`)
	usage := []byte(`{"object":"list","total_usage":1234}`)
	got := parseNewAPIBalance(status, sub, usage, false)
	require.NotNil(t, got)
	require.Equal(t, "CNY", got.Unit)
	require.InDelta(t, 87.66, *got.Amount, 0.00001)
	require.Equal(t, "upstream", got.Scope)
	for _, bad := range []string{`{}`, `{"object":"list"}`, `{"object":"list","total_usage":null}`, `{"object":"list","total_usage":-1}`, `{"object":"list","total_usage":2,"error":{"message":"oops"}}`} {
		require.Nil(t, parseNewAPIBalance(status, sub, []byte(bad), false))
	}
	require.Nil(t, parseNewAPIBalance([]byte(`{"success":true,"data":{}}`), sub, usage, false))
	unlimited := parseNewAPIBalance(status, []byte(`{"object":"billing_subscription","hard_limit_usd":100000000}`), usage, true)
	require.True(t, unlimited.Unlimited)
	require.Nil(t, unlimited.Amount)
	// An unlimited key can still return a finite user wallet under the upstream's settings.
	got = parseNewAPIBalance(status, sub, usage, true)
	require.False(t, got.Unlimited)
	require.InDelta(t, 87.66, *got.Amount, 0.00001)
}

type balanceHTTPStub struct {
	HTTPUpstream
	t      *testing.T
	bodies map[string]string
	paths  []string
}

func (s *balanceHTTPStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	s.paths = append(s.paths, req.URL.Path)
	if strings.HasSuffix(req.URL.Path, "/api/status") {
		require.Empty(s.t, req.Header.Get("Authorization"))
	} else {
		require.Equal(s.t, "Bearer secret", req.Header.Get("Authorization"))
	}
	status := http.StatusOK
	body, ok := s.bodies[req.URL.Path]
	if !ok {
		status = http.StatusNotFound
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestUpstreamBalanceQuery(t *testing.T) {
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{1: {
		ID: 1, Type: AccountTypeAPIKey, Platform: PlatformOpenAI,
		Credentials: map[string]any{"api_key": "secret", "base_url": "https://relay.example/prefix/v1"},
	}}}
	upstream := &balanceHTTPStub{t: t, bodies: map[string]string{
		"/prefix/api/usage/token/":                  `{"success":true,"data":{"object":"token_usage","unlimited_quota":false}}`,
		"/prefix/api/status":                        `{"success":true,"data":{"quota_display_type":"USD"}}`,
		"/prefix/v1/dashboard/billing/subscription": `{"object":"billing_subscription","hard_limit_usd":10}`,
		"/prefix/v1/dashboard/billing/usage":        `{"object":"list","total_usage":100}`,
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	got, err := svc.QueryUpstreamBalance(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "newapi", got.Provider)
	require.Equal(t, 9.0, *got.Amount)
	require.Len(t, upstream.paths, 5)
	require.Empty(t, repo.updates) // querying must not mutate quota/rates

	upstream.paths = nil
	upstream.bodies["/prefix/v1/usage"] = `{"mode":"unrestricted","isValid":true,"unit":"USD","balance":0}`
	got, err = svc.QueryUpstreamBalance(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "sub2api", got.Provider)
	require.Equal(t, 0.0, *got.Amount)
	require.Len(t, upstream.paths, 1)

	upstream.bodies = nil
	got, err = svc.QueryUpstreamBalance(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "unavailable", got.Status)
	require.Nil(t, got.Amount)

	upstream.paths = nil
	repo.accounts[1].Credentials["base_url"] = "https://api.openai.com"
	got, err = svc.QueryUpstreamBalance(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "unavailable", got.Status)
	require.Empty(t, upstream.paths)
}
