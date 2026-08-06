package deterministicrisk

import (
	"regexp"
	"strings"
)

const maxMatchedExcerptRunes = 200

var (
	authorizationSecretPattern = regexp.MustCompile(`(?i)(\bauthorization\s*[:=]\s*(?:bearer\s+)?)[^\s,;]+`)
	namedSecretPattern         = regexp.MustCompile(`(?i)\b(api[_ -]?key|password|passwd|pwd|access[_ -]?token|refresh[_ -]?token|token|cookie|secret|client[_ -]?secret|private[_ -]?key|otp|verification[_ -]?code)\b\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
	chineseSecretPattern       = regexp.MustCompile(`(密码|口令|验证码|一次性密码|密钥|令牌)\s*(?:是|为|[:：=])\s*[^\s，。；;]+`)
	jwtSecretPattern           = regexp.MustCompile(`\b[A-Za-z0-9_-]{12,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)
	prefixedSecretPattern      = regexp.MustCompile(`(?i)\b(?:(?:sk|rk|pk)-[A-Za-z0-9_-]{8,}|gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|(?:AKIA|ASIA)[A-Z0-9]{16})\b`)
	longHexSecretPattern       = regexp.MustCompile(`\b[A-Fa-f0-9]{32,}\b`)
	highEntropySecretPattern   = regexp.MustCompile(`\b[A-Za-z0-9_+/=-]{40,}\b`)
	pemPrivateKeyPattern       = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?(?:-----END [A-Z0-9 ]*PRIVATE KEY-----|$)`)
)

func buildMatchedExcerpt(text string, matches []tokenMatch) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	start, end := 0, len(text)
	if len(matches) > 0 {
		start, end = matches[0].start, matches[0].end
		for _, match := range matches[1:] {
			if match.start < start {
				start = match.start
			}
			if match.end > end {
				end = match.end
			}
		}
	}
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	excerpt := runeWindow(text, start, end, maxMatchedExcerptRunes)
	excerpt = redactSecrets(excerpt)
	excerpt = strings.Join(strings.Fields(excerpt), " ")
	return truncateRunes(excerpt, maxMatchedExcerptRunes)
}

func redactSecrets(value string) string {
	value = pemPrivateKeyPattern.ReplaceAllString(value, "[REDACTED PRIVATE KEY]")
	value = authorizationSecretPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = namedSecretPattern.ReplaceAllStringFunc(value, func(match string) string {
		separator := strings.IndexAny(match, ":=")
		if separator < 0 {
			return "[REDACTED]"
		}
		return strings.TrimSpace(match[:separator]) + "=[REDACTED]"
	})
	value = chineseSecretPattern.ReplaceAllString(value, `${1}=[REDACTED]`)
	value = jwtSecretPattern.ReplaceAllString(value, "[REDACTED]")
	value = prefixedSecretPattern.ReplaceAllString(value, "[REDACTED]")
	value = longHexSecretPattern.ReplaceAllString(value, "[REDACTED]")
	value = highEntropySecretPattern.ReplaceAllString(value, "[REDACTED]")
	return value
}

func runeWindow(value string, byteStart, byteEnd, maxRunes int) string {
	if maxRunes <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	startRune := len([]rune(value[:clampByteOffset(value, byteStart)]))
	endRune := len([]rune(value[:clampByteOffset(value, byteEnd)]))
	if endRune < startRune {
		endRune = startRune
	}
	matched := endRune - startRune
	if matched >= maxRunes {
		return string(runes[startRune : startRune+maxRunes])
	}
	remaining := maxRunes - matched
	windowStart := startRune - remaining/2
	if windowStart < 0 {
		windowStart = 0
	}
	windowEnd := windowStart + maxRunes
	if windowEnd > len(runes) {
		windowEnd = len(runes)
		windowStart = windowEnd - maxRunes
	}
	return string(runes[windowStart:windowEnd])
}

func clampByteOffset(value string, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(value) {
		return len(value)
	}
	for offset > 0 && offset < len(value) && (value[offset]&0xc0) == 0x80 {
		offset--
	}
	return offset
}

func truncateRunes(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}
