package auditcontext

import (
	"net/netip"
	"regexp"
	"strings"
)

type redactionPattern struct {
	replacement string
	pattern     *regexp.Regexp
}

var credentialPatterns = []redactionPattern{
	{"[REDACTED_PRIVATE_KEY]", regexp.MustCompile(`(?is)-----BEGIN(?: [A-Z0-9]+)? PRIVATE KEY-----.*?-----END(?: [A-Z0-9]+)? PRIVATE KEY-----`)},
	{"[REDACTED_PRIVATE_KEY]", regexp.MustCompile(`(?is)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*$`)},
	{"${1}[REDACTED]", regexp.MustCompile(`(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=-]{8,}`)},
	{"[REDACTED_JWT]", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)},
	{"[REDACTED_API_KEY]", regexp.MustCompile(`(?i)\b(?:sk|sk-proj|sk-ant|sess|rk|pk|ak|api|key|token|secret)[-_][A-Za-z0-9._~+/=-]{8,}\b`)},
	{"[REDACTED_API_KEY]", regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|(?:AKIA|ASIA)[A-Z0-9]{16}|AIza[A-Za-z0-9_-]{20,})\b`)},
	{"[REDACTED_API_KEY]", regexp.MustCompile(`(?i)\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)},
	{"${1}[REDACTED]", regexp.MustCompile(`(?i)((?:authorization|bearer|password|passwd|pwd|api[_ -]?key|apikey|access[_ -]?token|refresh[_ -]?token|id[_ -]?token|token|cookie|set[_ -]?cookie|session(?:[_ -]?(?:id|token))?|client[_ -]?secret|private[_ -]?key|secret|otp|verification[_ -]?code|密码|口令|密钥|令牌|验证码|校验码|动态码|一次性密码)\s*(?:[:：=]|\bis\b|是|为)\s*)(?:"[^"]*"|'[^']*')`)},
	{"${1}[REDACTED]", regexp.MustCompile(`(?i)((?:authorization|bearer|password|passwd|pwd|api[_ -]?key|apikey|access[_ -]?token|refresh[_ -]?token|id[_ -]?token|token|cookie|set[_ -]?cookie|session(?:[_ -]?(?:id|token))?|client[_ -]?secret|private[_ -]?key|secret|otp|verification[_ -]?code|密码|口令|密钥|令牌|验证码|校验码|动态码|一次性密码)\s*(?:[:：=]|\bis\b|是|为)\s*)[^\s"',;，。；、&?#<>]+`)},
	{"${1}[REDACTED_OTP]", regexp.MustCompile(`(?i)((?:\botp\b|\b(?:one[-_ ]?time|verification|security|authentication|auth)[-_ ]?(?:password|passcode|code)\b|验证码|校验码|动态码|一次性密码)\s*(?:[:：=]|\bis\b|是|为)?\s*)["']?\d{4,8}["']?\b`)},
	{"[REDACTED_SECRET]", regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`)},
	{"[REDACTED_SECRET]", regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)},
}

var piiPatterns = []redactionPattern{
	{"${1}[REDACTED_EMAIL]${2}", regexp.MustCompile(`(?i)(^|[^A-Z0-9.!#$%&'*+/=?^_{}|~-])[A-Z0-9.!#$%&'*+/=?^_{}|~-]+(@(?:[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?\.)+[A-Z]{2,63})\b`)},
	{"${1}${2}[REDACTED]", regexp.MustCompile(`(^|[^0-9])((?:(?:25[0-5]|2[0-4][0-9]|1?[0-9]{1,2})\.){3})(?:25[0-5]|2[0-4][0-9]|1?[0-9]{1,2})\b`)},
	{"${1}[REDACTED_PHONE]", regexp.MustCompile(`(^|[^0-9])(?:\+?86[ -]?)?1[3-9][0-9]{9}\b`)},
}

var ipv6CandidatePattern = regexp.MustCompile(`(?i)\[[0-9a-f:.%]+\]|(?:[0-9a-f]{0,4}:){2,7}[0-9a-f]{0,4}`)

// RedactSecrets removes common credentials and direct PII before data is
// persisted, sent to an audit provider, or included in a reusable risk summary.
// URL structure remains visible while sensitive query values and address data
// are replaced.
func RedactSecrets(value string) string {
	redacted := value
	for _, item := range credentialPatterns {
		redacted = item.pattern.ReplaceAllString(redacted, item.replacement)
	}
	for _, item := range piiPatterns {
		redacted = item.pattern.ReplaceAllString(redacted, item.replacement)
	}
	return redactIPv6Addresses(redacted)
}

func redactIPv6Addresses(value string) string {
	return ipv6CandidatePattern.ReplaceAllStringFunc(value, func(candidate string) string {
		bracketed := strings.HasPrefix(candidate, "[") && strings.HasSuffix(candidate, "]")
		address := strings.TrimSuffix(strings.TrimPrefix(candidate, "["), "]")
		parsed, err := netip.ParseAddr(address)
		if err != nil || !parsed.Is6() {
			return candidate
		}
		if bracketed {
			return "[REDACTED_IPV6]"
		}
		return "[REDACTED_IPV6]"
	})
}

func SanitizeReason(reason string, maxChars int) string {
	if maxChars <= 0 {
		maxChars = DefaultConfig().ReasonMaxChars
	}
	reason = RedactSecrets(reason)
	reason = strings.Join(strings.Fields(reason), " ")
	return truncateRunes(reason, maxChars, "[TRUNCATED]")
}
