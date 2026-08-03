package internalprobe

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuthenticatorValidSignatureMarksContextAndRestoresBody(t *testing.T) {
	auth := newDeterministicTestAuthenticator()
	body := `{"model":"gpt-test","input":"probe"}`
	outgoing := httptest.NewRequest(http.MethodPost, "https://gateway.example/v1/responses?mode=test", strings.NewReader(body))
	require.NoError(t, auth.SignRequest(outgoing))

	incoming := httptest.NewRequest(http.MethodPost, outgoing.URL.String(), strings.NewReader(body))
	incoming.Header.Set(HeaderName, outgoing.Header.Get(HeaderName))
	require.True(t, auth.VerifyAndMarkRequest(incoming))
	require.Empty(t, incoming.Header.Get(HeaderName))
	require.True(t, IsMarked(incoming.Context()))
	restored, err := io.ReadAll(incoming.Body)
	require.NoError(t, err)
	require.Equal(t, body, string(restored))
}

func TestAuthenticatorInvalidSignatureIsStrippedWithoutChangingRequest(t *testing.T) {
	auth := newDeterministicTestAuthenticator()
	body := `{"input":"original"}`
	outgoing := httptest.NewRequest(http.MethodPost, "https://gateway.example/v1/responses", strings.NewReader(body))
	require.NoError(t, auth.SignRequest(outgoing))

	incoming := httptest.NewRequest(http.MethodPost, outgoing.URL.String(), strings.NewReader(`{"input":"changed"}`))
	incoming.Header.Set(HeaderName, outgoing.Header.Get(HeaderName))
	require.False(t, auth.VerifyAndMarkRequest(incoming))
	require.Empty(t, incoming.Header.Get(HeaderName))
	require.False(t, IsMarked(incoming.Context()))
	restored, err := io.ReadAll(incoming.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"input":"changed"}`, string(restored))
}

func TestAuthenticatorRejectsExpiredAndRouteMismatchedSignatures(t *testing.T) {
	auth := newDeterministicTestAuthenticator()
	body := `{"input":"probe"}`
	outgoing := httptest.NewRequest(http.MethodPost, "https://gateway.example/v1/responses", strings.NewReader(body))
	require.NoError(t, auth.SignRequest(outgoing))
	header := outgoing.Header.Get(HeaderName)

	auth.now = func() time.Time { return time.Unix(1_700_000_061, 0) }
	expired := httptest.NewRequest(http.MethodPost, outgoing.URL.String(), strings.NewReader(body))
	expired.Header.Set(HeaderName, header)
	require.False(t, auth.VerifyAndMarkRequest(expired))
	require.Empty(t, expired.Header.Get(HeaderName))

	auth.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	wrongRoute := httptest.NewRequest(http.MethodPost, "https://gateway.example/v1/chat/completions", strings.NewReader(body))
	wrongRoute.Header.Set(HeaderName, header)
	require.False(t, auth.VerifyAndMarkRequest(wrongRoute))
	require.Empty(t, wrongRoute.Header.Get(HeaderName))
}

func TestAuthenticatorMalformedMarkerDoesNotReadOrRejectRequest(t *testing.T) {
	auth := newDeterministicTestAuthenticator()
	body := &countingReadCloser{Reader: strings.NewReader("ordinary client body")}
	req := httptest.NewRequest(http.MethodPost, "https://gateway.example/v1/responses", nil)
	req.Body = body
	req.ContentLength = int64(len("ordinary client body"))
	req.Header.Set(HeaderName, "not-a-valid-marker")

	require.False(t, auth.VerifyAndMarkRequest(req))
	require.Zero(t, body.reads)
	require.Empty(t, req.Header.Get(HeaderName))
}

func TestAuthenticatorOversizedActualBodyIsFullyRestored(t *testing.T) {
	auth := newDeterministicTestAuthenticator()
	body := bytes.Repeat([]byte("x"), int(maxSignedBodyBytes)+128)
	req := httptest.NewRequest(http.MethodPost, "https://gateway.example/v1/responses", nil)
	req.Body = io.NopCloser(bytes.NewReader(body))
	// Simulate a mismatched framing value to prove verification cannot truncate
	// an otherwise ordinary request body.
	req.ContentLength = 1
	req.Header.Set(HeaderName, "v1:1700000000:QkJCQkJCQkJCQkJCQkJCQg:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")

	require.False(t, auth.VerifyAndMarkRequest(req))
	restored, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, body, restored)
	require.Empty(t, req.Header.Get(HeaderName))
}

func TestNewAuthenticatorEmptySecretDisablesAuthentication(t *testing.T) {
	require.Nil(t, NewAuthenticator(""))
	require.Nil(t, NewAuthenticator("   "))
}

func newDeterministicTestAuthenticator() *Authenticator {
	auth := NewAuthenticator("test-secret-that-is-long-enough-for-tests")
	auth.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	auth.random = bytes.NewReader(bytes.Repeat([]byte{0x42}, 64))
	return auth
}

type countingReadCloser struct {
	io.Reader
	reads int
}

func (r *countingReadCloser) Read(p []byte) (int, error) {
	r.reads++
	return r.Reader.Read(p)
}

func (r *countingReadCloser) Close() error { return nil }
