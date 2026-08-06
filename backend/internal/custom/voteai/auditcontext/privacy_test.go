package auditcontext

import (
	"strings"
	"testing"
	"time"
)

func TestRedactSecrets(t *testing.T) {
	t.Parallel()
	input := "password=topsecret client_secret='two word secret' Bearer abcdefghijklmnopqrstuv sk-1234567890abcdefgh " +
		"验证码: 123456 eyJabcdefghijk.abcdefghijkl.abcdefghijkl"
	got := RedactSecrets(input)
	for _, secret := range []string{"topsecret", "two word secret", "abcdefghijklmnopqrstuv", "1234567890abcdefgh", "123456", "eyJabcdefghijk"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret %q remains in %q", secret, got)
		}
	}
}

func TestRedactSecrets_CredentialsPIIAndURLStructure(t *testing.T) {
	t.Parallel()
	const (
		passwordWordOne = "correct-horse-canary"
		passwordWordTwo = "battery-staple-canary"
		bearerToken     = "BearerCanaryToken123456789"
		jwtToken        = "eyJcanaryheader.canarypayload.canarysignature"
		privateKeyBody  = "PrivateKeyCanaryBody1234567890"
		emailAddress    = "private.person+audit@example.test"
		ipv4Address     = "203.0.113.42"
		ipv6Address     = "2001:db8:abcd::42"
		phoneNumber     = "13800138000"
		urlSecret       = "url-query-secret-canary"
	)
	openAIKey := strings.Join([]string{"sk", "proj", "canary1234567890abcdef"}, "-")
	githubKey := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz123456"
	input := strings.Join([]string{
		`password="` + passwordWordOne + " " + passwordWordTwo + `"`,
		"otp=4821 verification code: 729104",
		"Authorization: Bearer " + bearerToken,
		jwtToken,
		openAIKey,
		githubKey,
		"-----BEGIN PRIVATE KEY-----\n" + privateKeyBody + "\n-----END PRIVATE KEY-----",
		"email=" + emailAddress,
		"client_ip=" + ipv4Address,
		"peer=[" + ipv6Address + "]:8443",
		"phone=" + phoneNumber,
		"https://api.example.test/v1/audit?token=" + urlSecret + "&mode=debug",
	}, "\n")

	got := RedactSecrets(input)
	for _, secret := range []string{
		passwordWordOne,
		passwordWordTwo,
		"4821",
		"729104",
		bearerToken,
		jwtToken,
		openAIKey,
		githubKey,
		privateKeyBody,
		emailAddress,
		ipv4Address,
		ipv6Address,
		phoneNumber,
		urlSecret,
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("credential or PII %q remains in %q", secret, got)
		}
	}
	for _, diagnostic := range []string{
		"https://api.example.test/v1/audit?token=[REDACTED]&mode=debug",
		"[REDACTED_EMAIL]@example.test",
		"203.0.113.[REDACTED]",
		"[REDACTED_IPV6]:8443",
	} {
		if !strings.Contains(got, diagnostic) {
			t.Fatalf("expected diagnostic structure %q in %q", diagnostic, got)
		}
	}
}

func TestApplySanitizesPersistedRecentReason(t *testing.T) {
	t.Parallel()
	const (
		passwordWord = "persisted-password-canary"
		otp          = "6842"
		email        = "audit-state-person@example.test"
		ipv6         = "2001:db8:beef::99"
	)
	cfg := DefaultConfig()
	cfg.ReasonMaxChars = 1000
	state := Apply(State{}, AuditEvent{
		Reason: `password="` + passwordWord + ` multi word" otp=` + otp + " email=" + email + " peer=" + ipv6,
		At:     time.Unix(100, 0),
	}, cfg)
	if len(state.RecentReasons) != 1 {
		t.Fatalf("recent reasons=%v", state.RecentReasons)
	}
	persisted := state.RecentReasons[0]
	for _, secret := range []string{passwordWord, "multi word", otp, email, ipv6} {
		if strings.Contains(persisted, secret) {
			t.Fatalf("persisted audit-context reason leaked %q in %q", secret, persisted)
		}
	}
}

func TestSanitizeReasonNormalizesWhitespaceAndLength(t *testing.T) {
	t.Parallel()
	got := SanitizeReason("  first\n\tsecond "+strings.Repeat("z", 100), 40)
	if strings.ContainsAny(got, "\n\t") || runeLen(got) > 40 || !strings.Contains(got, "[TRUNCATED]") {
		t.Fatalf("sanitized reason=%q len=%d", got, runeLen(got))
	}
}
