package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	voteaiauditcontext "github.com/Wei-Shaw/sub2api/internal/custom/voteai/auditcontext"
	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
	"github.com/stretchr/testify/require"
)

func TestPopulateContentModerationAuditDetails_IncludesLatencyBreakdown(t *testing.T) {
	extraction, provenance, deterministic := 3, 2, 4
	verdictCache, contextLoad, fastBuild := 5, 6, 7
	reviewBuild, provider := 8, 99
	content := ContentModerationInput{auditTimings: &contentModerationLatencyBreakdown{
		startedAt:              time.Now().Add(-150 * time.Millisecond),
		postprocessStartedAt:   time.Now().Add(-12 * time.Millisecond),
		extractionLatencyMS:    &extraction,
		provenanceLatencyMS:    &provenance,
		deterministicLatencyMS: &deterministic,
		verdictCacheLatencyMS:  &verdictCache,
		contextLoadLatencyMS:   &contextLoad,
		fastBuildLatencyMS:     &fastBuild,
		reviewBuildLatencyMS:   &reviewBuild,
		providerLatencyMS:      &provider,
	}}
	result := &moderationAPIResult{StageDetails: []ContentModerationAuditStageDetails{
		{Stage: string(voteaimoderation.StageFast), ProviderCalled: true, LatencyMS: auditIntPtr(21)},
		{Stage: string(voteaimoderation.StageFull), ResultCacheHit: true, LatencyMS: auditIntPtr(1)},
		{Stage: string(voteaimoderation.StageMax), ProviderCalled: true, LatencyMS: auditIntPtr(34)},
	}}
	log := &ContentModerationLog{}
	cfg := &ContentModerationConfig{AuditProvider: ContentModerationProviderAIChat}

	populateContentModerationAuditDetails(log, cfg, content, result, nil, false)

	details := log.AuditDetails
	require.NotNil(t, details.TotalLatencyMS)
	require.GreaterOrEqual(t, *details.TotalLatencyMS, 140)
	require.Equal(t, 3, *details.ExtractionLatencyMS)
	require.Equal(t, 2, *details.ProvenanceLatencyMS)
	require.Equal(t, 4, *details.DeterministicLatencyMS)
	require.Equal(t, 5, *details.VerdictCacheLatencyMS)
	require.Equal(t, 6, *details.ContextLoadLatencyMS)
	require.Equal(t, 7, *details.FastBuildLatencyMS)
	require.Equal(t, 8, *details.ReviewBuildLatencyMS)
	require.Equal(t, 55, *details.ProviderLatencyMS)
	require.NotNil(t, details.PostprocessLatencyMS)
	require.GreaterOrEqual(t, *details.PostprocessLatencyMS, 10)
}

func TestPopulateContentModerationAuditDetails_RedactsAndBoundsPersistedExcerpts(t *testing.T) {
	const (
		sensitiveURL   = "https://sensitive.example.test/private/path?token=url-secret-123456"
		cookieSecret   = "session-cookie-secret-123456789"
		bearerToken    = "bearer-token-secret-1234567890"
		jwtToken       = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhdWRpdC11c2VyIn0.signature123456789"
		privateKeyBody = "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC1234567890"
	)
	longAPIKey := strings.Join([]string{"sk", "abcdefghijklmnopqrstuvwxyz0123456789"}, "-")
	privateKey := "-----BEGIN PRIVATE KEY-----\n" + privateKeyBody + "\n-----END PRIVATE KEY-----"
	targetRaw := strings.Join([]string{
		"inspect " + sensitiveURL,
		"Cookie: " + cookieSecret,
		"Authorization: Bearer " + bearerToken,
		"JWT=" + jwtToken,
		"api_key=" + longAPIKey,
		"private_key: target-private-key-secret-123456",
		privateKey,
		strings.Repeat("目标", 900),
	}, "\n")

	toolLines := []string{
		privateKey,
		"Cookie: " + cookieSecret,
		"Authorization: Bearer " + bearerToken,
		"jwt=" + jwtToken,
		"api_key=" + longAPIKey,
	}
	for i := 0; i < 120; i++ {
		line := fmt.Sprintf("tool-output-line-%03d-%s", i, strings.Repeat("x", 24))
		if i == 60 {
			line += "-MIDDLE-RAW-TOOL-SENTINEL"
		}
		toolLines = append(toolLines, line)
	}
	toolLines = append(toolLines, "tool-tail-line")
	toolRaw := strings.Join(toolLines, "\n")

	content := ContentModerationInput{
		AuditTargetText: targetRaw,
		AuditTargetKind: "user_intent",
		Turns: []ContentModerationTurn{
			{Role: "user", Source: "openai_responses", Purpose: "audit_target", Text: targetRaw},
			{
				Role:    "user",
				Source:  "openai_responses",
				Purpose: "supporting_context",
				Text: strings.Join([]string{
					"referenced " + sensitiveURL,
					"Cookie: " + cookieSecret,
					"Bearer " + bearerToken,
					jwtToken,
					longAPIKey,
					strings.Repeat("上下文", 400),
				}, "\n"),
			},
			{Role: "tool", Source: "openai_responses", Purpose: "supporting_context", Text: toolRaw},
		},
	}
	log := &ContentModerationLog{SessionID: "session-audit-details"}
	cfg := &ContentModerationConfig{
		AuditProvider: ContentModerationProviderAIChat,
		AIChat: ContentModerationAIChatConfig{
			BaseURL:      "https://api.deepseek.example",
			Model:        "deepseek-v4-flash",
			SystemPrompt: "stable policy",
		},
	}

	populateContentModerationAuditDetails(log, cfg, content, nil, nil, false)

	details := log.AuditDetails
	require.Equal(t, "user_intent", details.AuditTargetKind)
	require.Equal(t, "openai_responses", details.AuditTargetSource)
	require.LessOrEqual(t, len([]rune(details.AuditTargetExcerpt)), 1200)
	require.LessOrEqual(t, len([]rune(details.SupportingContextExcerpt)), 1600)
	require.NotEqual(t, targetRaw, details.AuditTargetExcerpt)
	require.NotContains(t, details.SupportingContextExcerpt, toolRaw)
	require.NotContains(t, details.SupportingContextExcerpt, "MIDDLE-RAW-TOOL-SENTINEL")
	require.Contains(t, details.SupportingContextExcerpt, "[CONTEXT OMITTED]")
	require.Contains(t, details.SupportingContextExcerpt, "tool-tail-line")
	require.Contains(t, details.SupportingContextExcerpt, "[REDACTED_PRIVATE_KEY]")

	persisted := details.AuditTargetExcerpt + "\n" + details.SupportingContextExcerpt
	for _, secret := range []string{
		sensitiveURL,
		cookieSecret,
		bearerToken,
		jwtToken,
		longAPIKey,
		privateKeyBody,
		"target-private-key-secret-123456",
	} {
		require.NotContains(t, persisted, secret)
	}
}

func TestPopulateContentModerationAuditDetails_RedactsCanariesAcrossPersistedTextFields(t *testing.T) {
	target, targetSecrets := contentModerationAuditCanaryPayload("target", "4821", "13800138001", "41", "101")
	supporting, supportingSecrets := contentModerationAuditCanaryPayload("support", "5937", "13800138002", "42", "102")
	modelReason, modelSecrets := contentModerationAuditCanaryPayload("model", "6148", "13800138003", "43", "103")
	log := &ContentModerationLog{}
	cfg := &ContentModerationConfig{
		AuditProvider: ContentModerationProviderAIChat,
		AIChat:        ContentModerationAIChatConfig{CacheEnabled: true},
	}

	populateContentModerationAuditDetails(log, cfg, ContentModerationInput{
		AuditTargetText: target,
		AuditTargetKind: "user_intent",
		Turns: []ContentModerationTurn{
			{Role: "user", Source: "openai_responses", Purpose: "audit_target", Text: target},
			{Role: "assistant", Source: "openai_responses", Purpose: "supporting_context", Text: supporting},
		},
	}, &moderationAPIResult{Reason: modelReason}, nil, false)

	assertContentModerationAuditFieldRedacted(t, "audit target", log.AuditDetails.AuditTargetExcerpt, targetSecrets)
	assertContentModerationAuditFieldRedacted(t, "supporting context", log.AuditDetails.SupportingContextExcerpt, supportingSecrets)
	assertContentModerationAuditFieldRedacted(t, "model reason", log.AuditDetails.ModelReason, modelSecrets)
	for name, value := range map[string]string{
		"audit target":       log.AuditDetails.AuditTargetExcerpt,
		"supporting context": log.AuditDetails.SupportingContextExcerpt,
		"model reason":       log.AuditDetails.ModelReason,
	} {
		require.Contains(t, value, "[REDACTED", name)
	}
}

func TestPopulateContentModerationAuditDetails_RedactsBeforeTruncating(t *testing.T) {
	targetSecret := "target-secret-after-long-prefix"
	supportingSecret := "support-secret-after-long-prefix"
	modelSecret := "model-secret-after-long-prefix"
	log := &ContentModerationLog{}
	cfg := &ContentModerationConfig{AuditProvider: ContentModerationProviderAIChat}

	populateContentModerationAuditDetails(log, cfg, ContentModerationInput{
		AuditTargetText: strings.Repeat("t", 1175) + ` password="` + targetSecret + ` extra words"`,
		Turns: []ContentModerationTurn{{
			Role: "assistant", Purpose: "supporting_context",
			Text: strings.Repeat("s", 475) + ` password="` + supportingSecret + ` extra words"`,
		}},
	}, &moderationAPIResult{
		Reason: strings.Repeat("m", 475) + ` password="` + modelSecret + ` extra words"`,
	}, nil, false)

	require.NotContains(t, log.AuditDetails.AuditTargetExcerpt, targetSecret)
	require.NotContains(t, log.AuditDetails.SupportingContextExcerpt, supportingSecret)
	require.NotContains(t, log.AuditDetails.ModelReason, modelSecret)
	require.Contains(t, log.AuditDetails.AuditTargetExcerpt, "[REDACTED]")
	require.Contains(t, log.AuditDetails.SupportingContextExcerpt, "[REDACTED]")
	require.Contains(t, log.AuditDetails.ModelReason, "[REDACTED]")
}

func contentModerationAuditCanaryPayload(scope, otp, phone, ipv4Last, ipv6Last string) (string, []string) {
	passwordOne := scope + "-password-alpha"
	passwordTwo := scope + "-password-beta"
	bearer := scope + "BearerCanary123456789"
	jwt := "eyJ" + scope + "header." + scope + "payload12." + scope + "signature12"
	apiKey := strings.Join([]string{"sk", "proj", scope + "ApiKeyCanary123456"}, "-")
	pemBody := strings.ToUpper(scope) + "PEMPRIVATEKEYCANARY123456789"
	email := scope + ".person@example.test"
	ipv4 := "203.0.113." + ipv4Last
	ipv6 := "2001:db8:abcd::" + ipv6Last
	payload := strings.Join([]string{
		`password="` + passwordOne + " " + passwordTwo + `"`,
		"otp=" + otp,
		"Authorization: Bearer " + bearer,
		jwt,
		apiKey,
		"-----BEGIN PRIVATE KEY-----\n" + pemBody + "\n-----END PRIVATE KEY-----",
		"email=" + email,
		"ip=" + ipv4,
		"peer=[" + ipv6 + "]",
		"phone=" + phone,
	}, "\n")
	return payload, []string{
		passwordOne,
		passwordTwo,
		otp,
		bearer,
		jwt,
		apiKey,
		pemBody,
		email,
		ipv4,
		ipv6,
		phone,
	}
}

func assertContentModerationAuditFieldRedacted(t *testing.T, name, value string, secrets []string) {
	t.Helper()
	for _, secret := range secrets {
		require.NotContains(t, value, secret, "%s leaked %q", name, secret)
	}
}

func TestPopulateContentModerationAuditDetails_BoundsSupportingContextAndKeepsOnlyFourTurns(t *testing.T) {
	turns := []ContentModerationTurn{
		{Role: "user", Source: "chat", Purpose: "audit_target", Text: "current request"},
	}
	for i := 1; i <= 5; i++ {
		turns = append(turns, ContentModerationTurn{
			Role:    "assistant",
			Source:  fmt.Sprintf("support-%d", i),
			Purpose: "supporting_context",
			Text:    fmt.Sprintf("supporting-turn-%d-%s", i, strings.Repeat("界", 700)),
		})
	}
	log := &ContentModerationLog{}
	cfg := &ContentModerationConfig{
		AuditProvider: ContentModerationProviderAIChat,
		AIChat: ContentModerationAIChatConfig{
			Model:        "deepseek-v4-flash",
			SystemPrompt: "stable policy",
		},
	}

	populateContentModerationAuditDetails(log, cfg, ContentModerationInput{
		AuditTargetText: "current request",
		AuditTargetKind: "user_intent",
		Turns:           turns,
	}, nil, nil, false)

	excerpt := log.AuditDetails.SupportingContextExcerpt
	require.LessOrEqual(t, len([]rune(excerpt)), 1600)
	require.Contains(t, excerpt, "supporting-turn-1")
	require.Contains(t, excerpt, "supporting-turn-2")
	require.Contains(t, excerpt, "supporting-turn-3")
	require.NotContains(t, excerpt, "supporting-turn-5")
}

func TestPopulateContentModerationAuditDetails_CompleteProviderUsageAndReviewFlags(t *testing.T) {
	prompt, cached, uncached, output := 100, 80, 20, 7
	log := &ContentModerationLog{
		SessionID:     "cache-session",
		SessionSource: ContentModerationSessionSourcePromptCacheKey,
	}
	cfg := &ContentModerationConfig{
		AuditProvider: ContentModerationProviderAIChat,
		AIChat:        ContentModerationAIChatConfig{CacheEnabled: true},
	}
	result := &moderationAPIResult{
		InputChars: 4321,
		Usage: &voteaimoderation.Usage{
			PromptTokens:         &prompt,
			CachedPromptTokens:   &cached,
			UncachedPromptTokens: &uncached,
			CompletionTokens:     &output,
		},
	}
	content := ContentModerationInput{HasExplicitUser: true, TrustedClient: true}

	populateContentModerationAuditDetails(log, cfg, content, result, nil, false)

	details := log.AuditDetails
	require.Equal(t, ContentModerationSessionSourcePromptCacheKey, details.SessionSource)
	require.Equal(t, 4321, details.InputChars)
	require.False(t, details.ResultCacheHit)
	require.NotNil(t, details.ProviderApplicable)
	require.True(t, *details.ProviderApplicable)
	require.NotNil(t, details.ResultCacheApplicable)
	require.True(t, *details.ResultCacheApplicable)
	require.NotNil(t, details.Sub2APIResultCacheHit)
	require.False(t, *details.Sub2APIResultCacheHit)
	require.NotNil(t, details.ReviewComplete)
	require.True(t, *details.ReviewComplete)
	require.NotNil(t, details.HasExplicitUserTurn)
	require.True(t, *details.HasExplicitUserTurn)
	require.NotNil(t, details.TrustedClient)
	require.True(t, *details.TrustedClient)
	require.False(t, details.UsageUnknown)
	require.NotNil(t, details.ProviderPrefixCacheRatio)
	require.InDelta(t, 0.8, *details.ProviderPrefixCacheRatio, 0.000001)
	encoded, err := json.Marshal(details)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"sub2api_result_cache_hit":false`)
}

func TestPopulateContentModerationAuditDetails_IncompleteUsageKeepsRatioUnknown(t *testing.T) {
	prompt, cached, uncached := 100, 80, 30
	log := &ContentModerationLog{}
	cfg := &ContentModerationConfig{AuditProvider: ContentModerationProviderAIChat}
	result := &moderationAPIResult{
		ReviewIncomplete: true,
		Usage: &voteaimoderation.Usage{
			PromptTokens:         &prompt,
			CachedPromptTokens:   &cached,
			UncachedPromptTokens: &uncached,
		},
	}

	populateContentModerationAuditDetails(log, cfg, ContentModerationInput{}, result, &contentModerationIncrementalPlan{}, false)

	details := log.AuditDetails
	require.Nil(t, details.ProviderPrefixCacheRatio)
	require.True(t, details.UsageUnknown)
	require.NotNil(t, details.ReviewComplete)
	require.False(t, *details.ReviewComplete)
	require.NotNil(t, details.HasExplicitUserTurn)
	require.False(t, *details.HasExplicitUserTurn)
	require.NotNil(t, details.TrustedClient)
	require.False(t, *details.TrustedClient)
	require.NotNil(t, details.InputTruncated)
	require.False(t, *details.InputTruncated)
	encoded, err := json.Marshal(details)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"has_explicit_user_turn":false`)
	require.Contains(t, string(encoded), `"trusted_client":false`)
	require.Contains(t, string(encoded), `"input_truncated":false`)
}

func TestContentModerationAuditDetails_LegacyMissingBooleansRemainUnknown(t *testing.T) {
	var details ContentModerationAuditDetails
	require.NoError(t, json.Unmarshal([]byte(`{"audit_stage":"fast"}`), &details))

	require.Nil(t, details.HasExplicitUserTurn)
	require.Nil(t, details.TrustedClient)
	require.Nil(t, details.InputTruncated)
	require.Nil(t, details.PrefixContinuity)
	require.Nil(t, details.PrefixBaseline)

	encoded, err := json.Marshal(details)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"has_explicit_user_turn"`)
	require.NotContains(t, string(encoded), `"trusted_client"`)
	require.NotContains(t, string(encoded), `"input_truncated"`)
	require.NotContains(t, string(encoded), `"prefix_continuity"`)
	require.NotContains(t, string(encoded), `"prefix_baseline"`)
}

func TestPopulateContentModerationAuditDetails_EmitsFalsePrefixDiagnostics(t *testing.T) {
	log := &ContentModerationLog{}
	cfg := &ContentModerationConfig{AuditProvider: ContentModerationProviderAIChat}
	plan := &contentModerationIncrementalPlan{
		state: voteaiauditcontext.State{
			PrefixEpoch:       2,
			PrefixContinuity:  false,
			PrefixBaseline:    false,
			PrefixBreakReason: voteaiauditcontext.PrefixBreakHistoryRewritten,
		},
	}

	populateContentModerationAuditDetails(log, cfg, ContentModerationInput{}, &moderationAPIResult{}, plan, false)

	require.NotNil(t, log.AuditDetails.PrefixContinuity)
	require.False(t, *log.AuditDetails.PrefixContinuity)
	require.NotNil(t, log.AuditDetails.PrefixBaseline)
	require.False(t, *log.AuditDetails.PrefixBaseline)
	encoded, err := json.Marshal(log.AuditDetails)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"prefix_continuity":false`)
	require.Contains(t, string(encoded), `"prefix_baseline":false`)
}

func TestRecordContentModerationAuditUsage_CountsIncompleteOrNonConservingUsageAsUnknown(t *testing.T) {
	validPrompt, validCached, validUncached, completion := 100, 75, 25, 4
	missingPromptCached, missingPromptUncached := 10, 5
	badPrompt, badCached, badUncached := 20, 15, 10
	svc := &ContentModerationService{}

	svc.recordContentModerationAuditUsage(&moderationAPIResult{InputChars: 123, Usage: &voteaimoderation.Usage{
		PromptTokens: &validPrompt, CachedPromptTokens: &validCached, UncachedPromptTokens: &validUncached, CompletionTokens: &completion,
	}}, 999)
	svc.recordContentModerationAuditUsage(&moderationAPIResult{InputChars: 45, Usage: &voteaimoderation.Usage{
		CachedPromptTokens: &missingPromptCached, UncachedPromptTokens: &missingPromptUncached, CompletionTokens: &completion,
	}}, 999)
	svc.recordContentModerationAuditUsage(&moderationAPIResult{InputChars: 67, Usage: &voteaimoderation.Usage{
		PromptTokens: &badPrompt, CachedPromptTokens: &badCached, UncachedPromptTokens: &badUncached, CompletionTokens: &completion,
	}}, 999)

	require.Equal(t, int64(2), svc.auditUsageUnknown.Load())
	require.Equal(t, int64(235), svc.auditInputChars.Load(), "actual adapter input must replace the caller estimate")
}

func TestPopulateContentModerationAuditDetails_PartialStageCacheIsNotWholeAuditCacheHit(t *testing.T) {
	prompt, cached, uncached, output := 100, 80, 20, 7
	result := &moderationAPIResult{
		Usage: &voteaimoderation.Usage{
			PromptTokens: &prompt, CachedPromptTokens: &cached,
			UncachedPromptTokens: &uncached, CompletionTokens: &output,
		},
		StageDetails: []ContentModerationAuditStageDetails{
			{
				Stage: string(voteaimoderation.StageFast), ProviderCalled: true, UsageKnown: true,
				InputChars: auditIntPtr(900), PromptTokens: &prompt, CachedInputTokens: &cached,
				UncachedInputTokens: &uncached, OutputTokens: &output,
			},
			{Stage: string(voteaimoderation.StageFull), ResultCacheHit: true},
		},
	}
	log := &ContentModerationLog{}
	populateContentModerationAuditDetails(log, &ContentModerationConfig{
		AuditProvider: ContentModerationProviderAIChat,
		AIChat:        ContentModerationAIChatConfig{CacheEnabled: true},
	}, ContentModerationInput{}, result, nil, false)

	require.NotNil(t, log.AuditDetails.Sub2APIResultCacheHit)
	require.False(t, *log.AuditDetails.Sub2APIResultCacheHit)
	require.False(t, log.AuditDetails.UsageUnknown)
	require.Len(t, log.AuditDetails.Stages, 2)
	require.True(t, log.AuditDetails.Stages[0].ProviderCalled)
	require.True(t, log.AuditDetails.Stages[1].ResultCacheHit)
}

func TestPopulateContentModerationAuditDetails_FailedStageMakesAggregateUsageUnknown(t *testing.T) {
	result := &moderationAPIResult{StageDetails: []ContentModerationAuditStageDetails{
		{Stage: string(voteaimoderation.StageFast), ResultCacheHit: true},
		{Stage: string(voteaimoderation.StageFull), ProviderCalled: true, Failed: true, InputChars: auditIntPtr(1200)},
	}}
	log := &ContentModerationLog{}
	populateContentModerationAuditDetails(log, &ContentModerationConfig{
		AuditProvider: ContentModerationProviderAIChat,
		AIChat:        ContentModerationAIChatConfig{CacheEnabled: true},
	}, ContentModerationInput{}, result, nil, false)

	require.True(t, log.AuditDetails.UsageUnknown)
	require.NotNil(t, log.AuditDetails.Sub2APIResultCacheHit)
	require.False(t, *log.AuditDetails.Sub2APIResultCacheHit)
}

func TestPopulateContentModerationAuditDetails_LocalAndSkippedPathsAreNotApplicable(t *testing.T) {
	cfg := &ContentModerationConfig{
		AuditProvider: ContentModerationProviderAIChat,
		AIChat:        ContentModerationAIChatConfig{CacheEnabled: true},
	}
	for _, tt := range []struct {
		name   string
		action string
		result *moderationAPIResult
	}{
		{name: "no new user intent", action: ContentModerationActionSkip},
		{name: "local deterministic decision", action: ContentModerationActionBlock, result: &moderationAPIResult{LocalDecision: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			log := &ContentModerationLog{Action: tt.action}
			populateContentModerationAuditDetails(log, cfg, ContentModerationInput{}, tt.result, nil, false)

			details := log.AuditDetails
			require.NotNil(t, details.ProviderApplicable)
			require.False(t, *details.ProviderApplicable)
			require.NotNil(t, details.ResultCacheApplicable)
			require.False(t, *details.ResultCacheApplicable)
			require.NotNil(t, details.ReviewApplicable)
			require.False(t, *details.ReviewApplicable)
			require.Nil(t, details.Sub2APIResultCacheHit)
			require.Nil(t, details.ReviewComplete)
		})
	}
}

func TestContentModerationStageDetailsPreserveKnownZeroCounters(t *testing.T) {
	details := contentModerationSuccessfulStageDetails(
		voteaimoderation.StageFast,
		&moderationAPIResult{Stage: voteaimoderation.StageFast},
		0,
	)

	require.NotNil(t, details.InputChars)
	require.Zero(t, *details.InputChars)
	require.NotNil(t, details.LatencyMS)
	require.Zero(t, *details.LatencyMS)
	encoded, err := json.Marshal(details)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"input_chars":0`)
	require.Contains(t, string(encoded), `"latency_ms":0`)
}

func TestRecordContentModerationAuditUsage_ResultCacheHitDoesNotCountProviderStageCall(t *testing.T) {
	svc := &ContentModerationService{}
	svc.recordContentModerationAuditUsage(&moderationAPIResult{
		Stage: voteaimoderation.StageFast, ResultCacheHit: true,
	}, 1000)

	require.Equal(t, int64(1), svc.auditResultCacheHits.Load())
	require.Zero(t, svc.auditFastCalls.Load())
	require.Zero(t, svc.auditInputChars.Load())
	require.Zero(t, svc.auditUsageUnknown.Load())
}

func TestRecordContentModerationPrefixContinuity_SkipsFirstBaseline(t *testing.T) {
	svc := &ContentModerationService{}
	svc.recordContentModerationPrefixContinuity(voteaiauditcontext.State{PrefixBaseline: true})
	require.Zero(t, svc.auditPrefixContinuous.Load())
	require.Zero(t, svc.auditPrefixBreaks.Load())

	svc.recordContentModerationPrefixContinuity(voteaiauditcontext.State{PrefixContinuity: true})
	require.Equal(t, int64(1), svc.auditPrefixContinuous.Load())
}
