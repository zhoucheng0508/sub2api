//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/internalprobe"
	"github.com/stretchr/testify/require"
)

func TestRunCheckForModelSignsTrustedInternalProbe(t *testing.T) {
	swapMonitorHTTPClient(t)
	auth := internalprobe.NewAuthenticator("monitor-test-secret-that-is-long-enough")
	marked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		marked = auth.VerifyAndMarkRequest(req)
		require.Empty(t, req.Header.Get(internalprobe.HeaderName))
		var body map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		answer := answerFromOpenAIRequest(body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": answer}}},
		})
	}))
	t.Cleanup(server.Close)

	result := runCheckForModel(
		context.Background(),
		MonitorProviderOpenAI,
		server.URL,
		"local-api-key",
		"gpt-test",
		&CheckOptions{InternalProbeAuthenticator: auth},
	)
	require.True(t, marked)
	require.Equal(t, MonitorStatusOperational, result.Status)
}

func TestChannelMonitorInternalProbeAuthIsSameOriginOnly(t *testing.T) {
	settings := NewSettingService(&internalProbeSettingRepo{values: map[string]string{
		SettingKeyAPIBaseURL: "https://AI.Vote520.com/v1",
	}}, &config.Config{})
	service := NewChannelMonitorService(nil, nil)
	service.setInternalProbeAuthenticator(
		internalprobe.NewAuthenticator("monitor-test-secret-that-is-long-enough"),
		settings,
	)

	require.NotNil(t, service.internalProbeAuthenticatorFor(context.Background(), "https://ai.vote520.com"))
	require.NotNil(t, service.internalProbeAuthenticatorFor(context.Background(), "https://ai.vote520.com:443/custom"))
	require.Nil(t, service.internalProbeAuthenticatorFor(context.Background(), "http://ai.vote520.com"))
	require.Nil(t, service.internalProbeAuthenticatorFor(context.Background(), "https://third-party.example"))
	require.Nil(t, service.internalProbeAuthenticatorFor(context.Background(), "not-a-url"))
}

func TestExternalChannelMonitorDoesNotReceiveInternalProbeHeader(t *testing.T) {
	handler := &openAICaptureHandler{}
	endpoint := setupFakeOpenAI(t, handler)
	settings := NewSettingService(&internalProbeSettingRepo{values: map[string]string{
		SettingKeyAPIBaseURL: "https://ai.vote520.com/v1",
	}}, &config.Config{})
	monitorService := NewChannelMonitorService(nil, nil)
	monitorService.setInternalProbeAuthenticator(
		internalprobe.NewAuthenticator("monitor-test-secret-that-is-long-enough"),
		settings,
	)

	result := runCheckForModel(
		context.Background(),
		MonitorProviderOpenAI,
		endpoint,
		"third-party-key",
		"gpt-test",
		&CheckOptions{InternalProbeAuthenticator: monitorService.internalProbeAuthenticatorFor(context.Background(), endpoint)},
	)

	require.Equal(t, MonitorStatusOperational, result.Status)
	require.Empty(t, handler.lastHeaders.Get(internalprobe.HeaderName))
}

func TestInternalProbeHeaderIsRemovedBeforeRedirect(t *testing.T) {
	swapMonitorHTTPClient(t)
	auth := internalprobe.NewAuthenticator("monitor-test-secret-that-is-long-enough")
	receivedAtRedirectTarget := ""
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		receivedAtRedirectTarget = req.Header.Get(internalprobe.HeaderName)
		var body map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		answer := answerFromOpenAIRequest(body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": answer}}},
		})
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.NotEmpty(t, req.Header.Get(internalprobe.HeaderName))
		http.Redirect(w, req, target.URL+req.URL.RequestURI(), http.StatusTemporaryRedirect)
	}))
	t.Cleanup(source.Close)

	result := runCheckForModel(
		context.Background(),
		MonitorProviderOpenAI,
		source.URL,
		"local-api-key",
		"gpt-test",
		&CheckOptions{InternalProbeAuthenticator: auth},
	)

	require.Equal(t, MonitorStatusOperational, result.Status)
	require.Empty(t, receivedAtRedirectTarget)
}

type internalProbeSettingRepo struct {
	values map[string]string
}

func (r *internalProbeSettingRepo) Get(context.Context, string) (*Setting, error) { return nil, nil }
func (r *internalProbeSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	return r.values[key], nil
}
func (r *internalProbeSettingRepo) Set(context.Context, string, string) error { return nil }
func (r *internalProbeSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *internalProbeSettingRepo) SetMultiple(context.Context, map[string]string) error { return nil }
func (r *internalProbeSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *internalProbeSettingRepo) Delete(context.Context, string) error { return nil }

func TestSameHTTPOriginRejectsLookalikeHosts(t *testing.T) {
	require.True(t, sameHTTPOrigin("https://EXAMPLE.com/a", "https://example.com/b"))
	require.True(t, sameHTTPOrigin("https://example.com:443/a", "https://example.com/b"))
	require.False(t, sameHTTPOrigin("https://example.com.attacker.test", "https://example.com"))
	require.False(t, sameHTTPOrigin("https://example.com@attacker.test", "https://example.com"))
	require.False(t, sameHTTPOrigin("http://example.com", "https://example.com"))
	require.False(t, sameHTTPOrigin("https://example.com", strings.Repeat("x", 10)+"://bad"))
}
