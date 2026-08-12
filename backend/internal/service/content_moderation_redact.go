package service

import (
	"strings"

	voteaiauditcontext "github.com/Wei-Shaw/sub2api/internal/custom/voteai/auditcontext"
)

func redactContentModerationSecrets(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return voteaiauditcontext.RedactSecrets(text)
}
