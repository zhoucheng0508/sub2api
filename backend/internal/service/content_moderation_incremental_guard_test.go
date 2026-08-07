package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	voteaiauditcontext "github.com/Wei-Shaw/sub2api/internal/custom/voteai/auditcontext"
	voteaideterministicrisk "github.com/Wei-Shaw/sub2api/internal/custom/voteai/deterministicrisk"
	voteaiinputprovenance "github.com/Wei-Shaw/sub2api/internal/custom/voteai/inputprovenance"
	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
	voteairiskstate "github.com/Wei-Shaw/sub2api/internal/custom/voteai/riskstate"
	"github.com/stretchr/testify/require"
)

type contentModerationGuardCache struct {
	*contentModerationTestHashCache

	auditMu        sync.Mutex
	auditStates    map[string]voteaiauditcontext.State
	auditUpdates   int
	prefixUpdates  int
	sessionUpdates int
	claimMu        sync.Mutex
	claims         map[string]string
}

type contentModerationGuardClaimCache struct {
	*contentModerationGuardCache

	claimMu  sync.Mutex
	claims   map[string]string
	claimErr error
}

type contentModerationGuardSuppressionErrorCache struct {
	*contentModerationGuardCache
	err error
}

func (c *contentModerationGuardSuppressionErrorCache) IsFlaggedInputHashSuppressed(context.Context, string) (bool, error) {
	return false, c.err
}

func newContentModerationGuardClaimCache() *contentModerationGuardClaimCache {
	return &contentModerationGuardClaimCache{
		contentModerationGuardCache: newContentModerationGuardCache(),
		claims:                      make(map[string]string),
	}
}

func (c *contentModerationGuardClaimCache) TryClaimContentModerationRequestVerdict(
	_ context.Context,
	key string,
	owner string,
	_ time.Duration,
) (bool, error) {
	c.claimMu.Lock()
	defer c.claimMu.Unlock()
	if c.claimErr != nil {
		return false, c.claimErr
	}
	if _, exists := c.claims[key]; exists {
		return false, nil
	}
	c.claims[key] = owner
	return true, nil
}

func (c *contentModerationGuardClaimCache) ReleaseContentModerationRequestVerdictClaim(_ context.Context, key, owner string) error {
	c.claimMu.Lock()
	defer c.claimMu.Unlock()
	if c.claims[key] == owner {
		delete(c.claims, key)
	}
	return nil
}

func newContentModerationGuardCache() *contentModerationGuardCache {
	return &contentModerationGuardCache{
		contentModerationTestHashCache: &contentModerationTestHashCache{},
		auditStates:                    make(map[string]voteaiauditcontext.State),
		claims:                         make(map[string]string),
	}
}

func (c *contentModerationGuardCache) TryClaimContentModerationRequestVerdict(
	_ context.Context,
	key string,
	owner string,
	_ time.Duration,
) (bool, error) {
	c.claimMu.Lock()
	defer c.claimMu.Unlock()
	if _, exists := c.claims[key]; exists {
		return false, nil
	}
	c.claims[key] = owner
	return true, nil
}

func (c *contentModerationGuardCache) ReleaseContentModerationRequestVerdictClaim(_ context.Context, key, owner string) error {
	c.claimMu.Lock()
	defer c.claimMu.Unlock()
	if c.claims[key] == owner {
		delete(c.claims, key)
	}
	return nil
}

func (c *contentModerationGuardCache) GetContentModerationAuditContext(_ context.Context, key string) (voteaiauditcontext.State, bool, error) {
	c.auditMu.Lock()
	defer c.auditMu.Unlock()
	state, ok := c.auditStates[key]
	return state, ok, nil
}

func (c *contentModerationGuardCache) UpdateContentModerationAuditContextForUser(
	_ context.Context,
	_ int64,
	key string,
	event voteaiauditcontext.AuditEvent,
	cfg voteaiauditcontext.Config,
	_ time.Duration,
) (voteaiauditcontext.State, error) {
	c.auditMu.Lock()
	defer c.auditMu.Unlock()
	c.auditUpdates++
	state := voteaiauditcontext.Apply(c.auditStates[key], event, cfg)
	c.auditStates[key] = state
	return state, nil
}

func (c *contentModerationGuardCache) UpdateContentModerationAuditPrefixForUser(
	_ context.Context,
	_ int64,
	key string,
	observation voteaiauditcontext.PrefixObservation,
	_ time.Duration,
) (voteaiauditcontext.State, error) {
	c.auditMu.Lock()
	defer c.auditMu.Unlock()
	c.prefixUpdates++
	state := voteaiauditcontext.UpdatePrefix(c.auditStates[key], observation)
	c.auditStates[key] = state
	return state, nil
}

func (c *contentModerationGuardCache) UpdateContentModerationSessionRisk(
	ctx context.Context,
	key string,
	event voteairiskstate.Event,
	cfg voteairiskstate.Config,
) (voteairiskstate.State, error) {
	c.auditMu.Lock()
	c.sessionUpdates++
	c.auditMu.Unlock()
	return c.contentModerationTestHashCache.UpdateContentModerationSessionRisk(ctx, key, event, cfg)
}

func (c *contentModerationGuardCache) updateCounts() (audit, prefix, session int) {
	c.auditMu.Lock()
	defer c.auditMu.Unlock()
	return c.auditUpdates, c.prefixUpdates, c.sessionUpdates
}

func (c *contentModerationGuardCache) snapshotAuditStates() map[string]voteaiauditcontext.State {
	c.auditMu.Lock()
	defer c.auditMu.Unlock()
	out := make(map[string]voteaiauditcontext.State, len(c.auditStates))
	for key, state := range c.auditStates {
		out[key] = state
	}
	return out
}

func contentModerationGuardConfig(baseURL string) *ContentModerationConfig {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.BaseURL = strings.TrimRight(baseURL, "/") + "/v1"
	cfg.AIChat.APIKeys = []string{"deepseek-test-key"}
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.RetryCount = 0
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyBlock
	cfg.AIChat.RiskLevelsEnabled = false
	cfg.RecordNonHits = true
	cfg.normalize()
	return cfg
}

func newContentModerationGuardService(
	t *testing.T,
	cfg *ContentModerationConfig,
	server *httptest.Server,
	cache ContentModerationHashCache,
) (*ContentModerationService, *contentModerationTestRepo) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(raw),
		}},
		repo, cache, nil, nil, nil, nil, nil,
	)
	if server != nil {
		svc.httpClient = server.Client()
	}
	return svc, repo
}

func contentModerationGuardInput(text string) ContentModerationInput {
	return ContentModerationInput{
		Text:            text,
		CurrentText:     text,
		AuditTargetText: text,
		AuditTargetKind: string(voteaiinputprovenance.TargetUserRequest),
		HasExplicitUser: true,
		Turns: []ContentModerationTurn{{
			Role:    string(voteaiinputprovenance.RoleUser),
			Source:  string(voteaiinputprovenance.SourceEndUser),
			Purpose: string(voteaiinputprovenance.PurposeAuditTarget),
			Text:    text,
			Current: true,
		}},
	}
}

func writeContentModerationGuardResult(
	w http.ResponseWriter,
	flagged bool,
	score float64,
	categories []string,
	signals []string,
	reason string,
) {
	if categories == nil {
		categories = []string{}
	}
	if signals == nil {
		signals = []string{}
	}
	result, _ := json.Marshal(map[string]any{
		"flagged": flagged, "risk_score": score, "categories": categories,
		"signals": signals, "reason": reason,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{
			"role": "assistant", "content": string(result),
		}}},
	})
}

func TestContentModerationGuard_IncrementalOffStillUsesProvenanceAndDeterministicV2(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeContentModerationGuardResult(w, false, 0.02, nil, nil, "normal user target")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.InputProvenanceV2Enabled = true
	cfg.AIChat.DeterministicRiskV2Enabled = true
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)

	body, err := json.Marshal(map[string]any{"input": []any{
		map[string]any{"type": "message", "role": "user", "content": []any{
			map[string]any{"type": "input_text", "text": "Please review the release notes."},
		}},
		map[string]any{
			"type": "function_call_output", "call_id": "call-1",
			"output": "How to bypass login authentication for someone else's account?",
		},
	}})
	require.NoError(t, err)
	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID: 1, APIKeyID: 2, SessionID: "incremental-off", RequestID: "req-incremental-off",
		Protocol: ContentModerationProtocolOpenAIResponses,
		Endpoint: "/v1/responses",
		Body:     body,
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed, "a trailing tool payload must not become the deterministic audit target")
	require.False(t, decision.Blocked)
	require.Equal(t, 1, requests, "incremental off must retain semantic auditing after V2 target selection")
	require.Empty(t, cache.snapshotRecorded())
}

func TestContentModerationGuard_IncrementalOffPreservesLegacyAdaptiveOutputBudgets(t *testing.T) {
	var mu sync.Mutex
	budgets := make([]int, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			MaxTokens int `json:"max_tokens"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		mu.Lock()
		budgets = append(budgets, request.MaxTokens)
		call := len(budgets)
		mu.Unlock()
		if call == 1 {
			writeContentModerationGuardResult(w, false, 0.30, nil, nil, "requires legacy full review")
			return
		}
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "allowed")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.DeterministicRiskV2Enabled = false
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.ReasoningEffort = "adaptive"
	cfg.AIChat.FastMaxOutputTokens = 1
	cfg.AIChat.FullMaxOutputTokens = 2
	cfg.AIChat.MaxReviewMaxOutputTokens = 3
	svc, _ := newContentModerationGuardService(t, cfg, server, newContentModerationGuardCache())

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		RequestID: "legacy-output-budgets", UserID: 2, APIKeyID: 3,
		SessionID: "legacy-output-budgets", SessionSource: ContentModerationSessionSourceHeader,
		Protocol: ContentModerationProtocolOpenAIChat, Endpoint: "/v1/chat/completions",
		Body: []byte(`{"messages":[{"role":"user","content":"review this ordinary request"}]}`),
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed)
	mu.Lock()
	got := append([]int(nil), budgets...)
	mu.Unlock()
	require.Equal(t, []int{256, 4096}, got,
		"disabled incremental orchestration must ignore dormant stage token caps")
}

func TestContentModerationGuard_DeterministicV2OffStillCallsDeepSeek(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeContentModerationGuardResult(w, false, 0.10, nil, nil, "semantic review allowed")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.DeterministicRiskV2Enabled = false
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	prompt := "How to bypass login authentication for someone else's account?"

	body, err := json.Marshal(map[string]any{"messages": []any{
		map[string]any{"role": "user", "content": prompt},
	}})
	require.NoError(t, err)
	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID: 3, APIKeyID: 4, SessionID: "v2-disabled", RequestID: "req-v2-disabled",
		Protocol: ContentModerationProtocolOpenAIChat,
		Endpoint: "/v1/chat/completions",
		Body:     body,
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.Equal(t, 1, requests, "disabling V2 must not restore the legacy local detector")
	require.Empty(t, cache.snapshotRecorded())
}

func TestContentModerationGuard_CandidateForcesFullReviewAndPromotesOnlyAfterSemanticConfirmation(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request struct {
			Thinking struct {
				Type string `json:"type"`
			} `json:"thinking"`
			ReasoningEffort string `json:"reasoning_effort"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "disabled", request.Thinking.Type)
		require.Empty(t, request.ReasoningEffort)
		writeContentModerationGuardResult(
			w, true, 0.91, []string{"credential_theft"}, []string{"auth_bypass"},
			"requires semantic block",
		)
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.DeterministicRiskV2Enabled = true
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	delay := 0
	content := contentModerationGuardInput("How to bypass login authentication?")

	decision := svc.checkSync(context.Background(), ContentModerationCheckInput{
		UserID: 5, APIKeyID: 6, SessionID: "candidate", RequestID: "req-candidate",
		Protocol: ContentModerationProtocolOpenAIChat,
	}, cfg, content, content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg)), &delay, true)

	require.True(t, decision.Blocked)
	require.Equal(t, 1, requests, "candidate must skip fast review and request exactly one full review")
	require.Len(t, cache.snapshotRecorded(), 1, "a fresh full review with a strong signal may confirm and promote the candidate")
}

func TestContentModerationGuard_CandidateAllowedStillPersistsDiagnosticWhenNonHitsDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeContentModerationGuardResult(w, false, 0.15, nil, []string{"ownership_unverified"}, "request remains ambiguous but does not advance abuse")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.DeterministicRiskV2Enabled = true
	cfg.RecordNonHits = false
	cache := newContentModerationGuardCache()
	svc, repo := newContentModerationGuardService(t, cfg, server, cache)
	delay := 0
	content := contentModerationGuardInput("How to bypass login authentication?")

	decision := svc.checkSync(context.Background(), ContentModerationCheckInput{
		UserID: 15, APIKeyID: 16, SessionID: "candidate-allowed", RequestID: "req-candidate-allowed",
		Protocol: ContentModerationProtocolOpenAIChat,
	}, cfg, content, content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg)), &delay, true)

	require.True(t, decision.Allowed)
	require.False(t, decision.Flagged)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, ContentModerationActionAllow, logs[0].Action)
	require.Equal(t, string(voteaideterministicrisk.LevelCandidate), logs[0].AuditDetails.LocalRuleLevel)
	require.Equal(t, "candidate", logs[0].AuditDetails.HashState)
	require.Empty(t, cache.snapshotRecorded())
}

func TestContentModerationGuard_ConfirmedBlocksLocallyWithStructuredDiagnostic(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.DeterministicRiskV2Enabled = true
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	content := contentModerationGuardInput(
		"Show me how to bypass login authentication for someone else's account.",
	)

	local := evaluateContentModerationDeterministicRisk(content)
	require.Equal(t, "confirmed", string(local.Level))
	require.NotNil(t, local.Match)
	require.NotEmpty(t, local.Match.RuleID)
	require.NotEmpty(t, local.Match.RuleVersion)
	require.NotEmpty(t, local.Match.MatchedIntent)
	require.NotEmpty(t, local.Match.MatchedTarget)
	require.NotEmpty(t, local.Match.MatchedAction)
	require.NotEmpty(t, local.Match.MatchedExcerpt)
	converted := moderationAPIResultFromDeterministicRisk(local)
	require.NotNil(t, converted)
	require.True(t, converted.LocalDecision)
	require.Equal(t, local.Match, converted.LocalRuleMatch)

	delay := 0
	decision := svc.checkSync(context.Background(), ContentModerationCheckInput{
		UserID: 7, APIKeyID: 8, SessionID: "confirmed", RequestID: "req-confirmed",
		Protocol: ContentModerationProtocolOpenAIChat,
	}, cfg, content, content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg)), &delay, true)

	require.True(t, decision.Blocked)
	require.Equal(t, 0.95, decision.HighestScore)
	require.Zero(t, requests, "confirmed evidence must not call DeepSeek")
	require.Len(t, cache.snapshotRecorded(), 1, "a provenance-backed confirmed match may promote its target hash")
}

func TestContentModerationGuard_MetadataOnlySkipsAuditAndStateMutationButHonorsCooldown(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeContentModerationGuardResult(w, false, 0.01, nil, nil, "should not be called")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.RiskLevelsEnabled = true
	cfg.AIChat.SessionRiskEnabled = true
	cfg.AIChat.ActorRiskEnabled = true
	body := []byte(`{"messages":[{"role":"developer","content":"<environment_context>trusted machine state</environment_context>"}]}`)
	baseInput := ContentModerationCheckInput{
		UserID: 9, APIKeyID: 10, SessionID: "metadata-only", RequestID: "req-metadata-only",
		Protocol:                  ContentModerationProtocolOpenAIChat,
		Endpoint:                  "/v1/chat/completions",
		TrustedMetadataProvenance: true,
		ClientHeaders: http.Header{
			"User-Agent":              {"codex_cli_rs/0.141.0 (x)"},
			"Originator":              {"codex_cli_rs"},
			"X-Codex-Installation-Id": {"installation-1"},
		},
		Body: body,
	}

	t.Run("allow without cooldown", func(t *testing.T) {
		cache := newContentModerationGuardCache()
		svc, _ := newContentModerationGuardService(t, cfg, server, cache)
		decision, err := svc.Check(context.Background(), baseInput)

		require.NoError(t, err)
		require.True(t, decision.Allowed)
		require.False(t, decision.Blocked)
		require.Empty(t, cache.snapshotChecked(), "metadata-only input must skip permanent hash lookup")
		require.Empty(t, cache.snapshotRecorded())
		audit, prefix, session := cache.updateCounts()
		require.Zero(t, audit)
		require.Zero(t, prefix)
		require.Zero(t, session)
	})

	t.Run("block during existing cooldown", func(t *testing.T) {
		cache := newContentModerationGuardCache()
		sessionKey, _, _ := contentModerationRiskIdentity(baseInput)
		cache.sessionStates = map[string]voteairiskstate.State{
			sessionKey: {
				Version: 1, Score: 0.93, Tier: voteairiskstate.TierHigh,
				BlockedUntilUnix: time.Now().Add(5 * time.Minute).Unix(),
			},
		}
		svc, _ := newContentModerationGuardService(t, cfg, server, cache)
		input := baseInput
		input.RequestID = "req-metadata-cooldown"
		decision, err := svc.Check(context.Background(), input)

		require.NoError(t, err)
		require.True(t, decision.Blocked)
		require.Equal(t, contentModerationSessionRiskCategory, decision.HighestCategory)
		require.Empty(t, cache.snapshotChecked())
		require.Empty(t, cache.snapshotRecorded())
		audit, prefix, session := cache.updateCounts()
		require.Zero(t, audit)
		require.Zero(t, prefix)
		require.Zero(t, session)
	})

	require.Zero(t, requests, "metadata-only requests must never call DeepSeek")
}

func TestContentModerationGuard_ResultCacheIsolatedByAuditStage(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "stage-specific result")
	}))
	defer server.Close()

	cache := newContentModerationGuardCache()
	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.CacheEnabled = true
	cfg.AIChat.CacheTTLSeconds = 60
	cfg.normalize()
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	svc.httpClient = server.Client()

	keys := make(map[string]string)
	for _, stage := range []voteaimoderation.ReviewStage{
		voteaimoderation.StageFast,
		voteaimoderation.StageFull,
		voteaimoderation.StageMax,
	} {
		stageCfg := cloneContentModerationConfig(cfg)
		stageCfg.AIChat.auditStage = string(stage)
		keys[string(stage)] = contentModerationAIResultCacheKey(stageCfg, "identical audit input")
		first, err := svc.callModeration(context.Background(), stageCfg, "identical audit input")
		require.NoError(t, err)
		require.Equal(t, stage, first.Stage)
		second, err := svc.callModeration(context.Background(), stageCfg, "identical audit input")
		require.NoError(t, err)
		require.Equal(t, stage, second.Stage)
		require.True(t, second.ResultCacheHit)
	}

	require.NotEqual(t, keys[string(voteaimoderation.StageFast)], keys[string(voteaimoderation.StageFull)])
	require.NotEqual(t, keys[string(voteaimoderation.StageFast)], keys[string(voteaimoderation.StageMax)])
	require.NotEqual(t, keys[string(voteaimoderation.StageFull)], keys[string(voteaimoderation.StageMax)])
	require.Equal(t, 3, requests, "each audit stage must populate its own result-cache entry")
	require.Len(t, cache.results, 3)
}

func TestContentModerationGuard_ProvenanceOffDisablesPermanentHashPromotion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeContentModerationGuardResult(
			w, true, 0.94, []string{"credential_theft"}, []string{"auth_bypass"},
			"semantic full-review block",
		)
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.InputProvenanceV2Enabled = false
	cfg.AIChat.DeterministicRiskV2Enabled = false
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.auditStage = string(voteaimoderation.StageFull)
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	content := contentModerationGuardInput("ambiguous high-risk request")
	delay := 0

	decision := svc.checkSync(context.Background(), ContentModerationCheckInput{
		UserID: 11, APIKeyID: 12, SessionID: "provenance-off", RequestID: "req-provenance-off",
		Protocol: ContentModerationProtocolOpenAIChat,
	}, cfg, content, content.Hash(), &delay, true)

	require.True(t, decision.Blocked)
	require.Empty(t, cache.snapshotRecorded(), "semantic results cannot enter the permanent hash set without provenance V2")
}

func TestContentModerationGuard_LargeTrailingToolOutputRemainsBoundedSupportingContext(t *testing.T) {
	const userTarget = "新增这个功能，注意不要影响到旧功能的使用。"
	toolNoise := strings.Repeat("scripts/auth/account credential secret defensive-review \ufffd ", 14000)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.NotEmpty(t, request.Messages)
		auditText := request.Messages[len(request.Messages)-1].Content
		require.Contains(t, auditText, userTarget)
		require.Contains(t, auditText, "[AUDIT-TARGET")
		require.Contains(t, auditText, "kind=user_request")
		require.Less(t, len([]rune(auditText)), 10000, "tool output must be bounded before provider dispatch")
		require.LessOrEqual(t, strings.Count(auditText, "scripts/auth/account"), 50, "repeated tool noise must not scale provider input")
		writeContentModerationGuardResult(w, false, 0, nil, nil, "正常功能开发请求")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.InputProvenanceV2Enabled = true
	cfg.AIChat.DeterministicRiskV2Enabled = true
	cfg.AIChat.RiskLevelsEnabled = true
	cfg.AIChat.SessionRiskEnabled = true
	cfg.AIChat.ActorRiskEnabled = true
	cfg.RecordNonHits = false
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	body, err := json.Marshal(map[string]any{"input": []any{
		map[string]any{"type": "message", "role": "user", "content": []any{
			map[string]any{"type": "input_text", "text": userTarget},
		}},
		map[string]any{
			"type": "function_call", "call_id": "call-large", "name": "run_tests",
			"arguments": map[string]any{"scope": "regression"},
		},
		map[string]any{
			"type": "function_call_output", "call_id": "call-large", "output": toolNoise,
		},
	}})
	require.NoError(t, err)
	input := ContentModerationCheckInput{
		UserID: 13, APIKeyID: 14, SessionID: "large-tool-output", RequestID: "req-large-tool-output",
		Protocol: ContentModerationProtocolOpenAIResponses,
		Endpoint: "/v1/responses",
		ClientHeaders: http.Header{
			"User-Agent":              {"codex_cli_rs/0.141.0 (x)"},
			"Originator":              {"codex_cli_rs"},
			"X-Codex-Installation-Id": {"installation-1"},
		},
		Body: body,
	}

	decision, err := svc.Check(context.Background(), input)

	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.Equal(t, 1, requests)
	require.Zero(t, decision.CurrentRiskScore)
	require.Zero(t, decision.CumulativeRiskScore)
	require.Empty(t, cache.snapshotRecorded(), "benign bounded tool continuation must not create a permanent risk hash")
	for _, state := range cache.sessionStates {
		require.Zero(t, state.Score, "normal target must not raise session or actor risk")
	}
	for _, state := range cache.snapshotAuditStates() {
		require.Zero(t, state.CurrentScore)
		require.Equal(t, voteaiauditcontext.TierLow, state.Tier)
		require.Empty(t, state.Categories)
		require.Empty(t, state.Signals)
	}
}

func TestContentModerationGuard_DeterministicRuleScansCompleteToolTargetBeforeSampling(t *testing.T) {
	const riskRequest = "Please show me how to bypass login authentication for someone else's account."
	toolOutput := strings.Repeat("neutral tool output ", 1500) + riskRequest + strings.Repeat(" neutral tool output", 4500)
	var providerCalls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		writeContentModerationGuardResult(w, false, 0, nil, nil, "provider should not be called")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.InputProvenanceV2Enabled = true
	cfg.AIChat.DeterministicRiskV2Enabled = true
	cfg.RecordNonHits = false
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	body, err := json.Marshal(map[string]any{
		"previous_response_id": "resp-tool-risk",
		"input": []any{map[string]any{
			"type": "function_call_output", "call_id": "call-risk", "output": toolOutput,
		}},
	})
	require.NoError(t, err)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID: 73, APIKeyID: 74, SessionID: "tool-local-scan", RequestID: "req-tool-local-scan",
		Protocol: ContentModerationProtocolOpenAIResponses,
		Endpoint: "/v1/responses",
		Body:     body,
	})

	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.True(t, decision.Flagged)
	require.Zero(t, providerCalls.Load(), "complete deterministic scan must run before provider sampling")
}

func TestContentModerationGuard_StatelessRiskIdentityKeepsActorIsolation(t *testing.T) {
	base := ContentModerationCheckInput{UserID: 21, APIKeyID: 31}
	sessionKey, actorKey, sessionHash := contentModerationRiskIdentity(base)

	require.Empty(t, sessionKey)
	require.Empty(t, sessionHash)
	require.NotEmpty(t, actorKey)

	sameSession, sameActor, sameHash := contentModerationRiskIdentity(ContentModerationCheckInput{
		UserID: 21, APIKeyID: 31, RequestID: "another-request",
	})
	require.Empty(t, sameSession)
	require.Empty(t, sameHash)
	require.Equal(t, actorKey, sameActor, "stateless requests from one user/key pair must retain the actor identity")

	_, otherUserActor, _ := contentModerationRiskIdentity(ContentModerationCheckInput{UserID: 22, APIKeyID: 31})
	_, otherKeyActor, _ := contentModerationRiskIdentity(ContentModerationCheckInput{UserID: 21, APIKeyID: 32})
	require.NotEqual(t, actorKey, otherUserActor)
	require.NotEqual(t, actorKey, otherKeyActor)
	require.NotEqual(t, otherUserActor, otherKeyActor)

	for _, invalid := range []ContentModerationCheckInput{
		{APIKeyID: 31},
		{UserID: 21},
	} {
		gotSession, gotActor, gotHash := contentModerationRiskIdentity(invalid)
		require.Empty(t, gotSession)
		require.Empty(t, gotActor)
		require.Empty(t, gotHash)
	}
}

func TestContentModerationGuard_StatelessIncrementalPlanUsesLowWeightActorStateWithoutFalsePeriodicReview(t *testing.T) {
	cfg := contentModerationGuardConfig("http://127.0.0.1")
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.PeriodicFullReviewTurns = 10
	cache := newContentModerationGuardCache()
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	input := ContentModerationCheckInput{
		UserID: 41, APIKeyID: 51, RequestID: "stateless-current",
		Protocol: ContentModerationProtocolOpenAIChat,
	}
	_, actorKey, _ := contentModerationRiskIdentity(input)
	cache.auditStates[actorKey] = voteaiauditcontext.State{
		Version:             voteaiauditcontext.StateVersion,
		PolicyVersion:       contentModerationAuditPolicyVersion(cfg),
		TurnCount:           99,
		CurrentScore:        0.80,
		MaxScore:            0.90,
		Trend:               voteaiauditcontext.TrendRising,
		Tier:                voteaiauditcontext.TierHigh,
		LastFullReviewTurn:  90,
		LastFullReviewAt:    time.Now().Add(-time.Minute).Unix(),
		PrefixEpoch:         8,
		CanonicalPrefixHash: "must-not-carry",
	}
	content := contentModerationGuardInput("Summarize the release notes.")

	plan, err := svc.prepareIncrementalAudit(context.Background(), input, cfg, content)

	require.NoError(t, err)
	require.False(t, plan.stableSession)
	require.Equal(t, "none", plan.sessionSource)
	require.Equal(t, actorKey, plan.stateKey)
	require.False(t, plan.fullHistoryAvailable)
	require.InDelta(t, 0.20, plan.state.CurrentScore, 0.0001)
	require.InDelta(t, 0.225, plan.state.MaxScore, 0.0001)
	require.Equal(t, voteaiauditcontext.TierLow, plan.state.Tier)
	require.Zero(t, plan.state.TurnCount)
	require.Zero(t, plan.state.LastFullReviewTurn)
	require.Zero(t, plan.state.LastFullReviewAt)
	require.Zero(t, plan.state.PrefixEpoch)
	require.Empty(t, plan.state.CanonicalPrefixHash)

	review := voteaiauditcontext.DecideFullReview(plan.state, voteaiauditcontext.ReviewInput{
		FastScore:            0,
		LatestUserText:       plan.latestUserText,
		StableSession:        plan.stableSession,
		FullHistoryAvailable: plan.fullHistoryAvailable,
		At:                   time.Now().UTC(),
	}, cfg.auditContextConfig())
	require.False(t, review.Required, "actor history must not manufacture a periodic full review for a single stateless turn")

	replay := input
	replay.RequestID = "stateless-next-request"
	nextPlan, err := svc.prepareIncrementalAudit(context.Background(), replay, cfg, content)
	require.NoError(t, err)
	require.Equal(t, actorKey, nextPlan.stateKey)
	require.Equal(t, plan.state.CurrentScore, nextPlan.state.CurrentScore)
	require.Zero(t, nextPlan.state.TurnCount)
}

func TestContentModerationGuard_FullReviewContainsExactlyOneAuditTarget(t *testing.T) {
	cfg := contentModerationGuardConfig("http://127.0.0.1")
	cfg.AIChat.IncrementalAuditEnabled = true
	cache := newContentModerationGuardCache()
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	const target = "新增这个功能，注意不要影响到旧功能的使用。"
	content := ContentModerationInput{
		Text:            "conversation",
		CurrentText:     target,
		AuditTargetText: target,
		AuditTargetKind: string(voteaiinputprovenance.TargetUserRequest),
		HasExplicitUser: true,
		Turns: []ContentModerationTurn{
			{
				Role: "user", Source: string(voteaiinputprovenance.SourceEndUser),
				Purpose: string(voteaiinputprovenance.PurposeSupportingContext),
				Text:    "Earlier request.", Current: true,
			},
			{
				Role: "assistant", Source: string(voteaiinputprovenance.SourceAssistantResponse),
				Purpose: string(voteaiinputprovenance.PurposeSupportingContext),
				Text:    "Earlier answer.", Current: true,
			},
			{
				Role: "user", Source: string(voteaiinputprovenance.SourceEndUser),
				Purpose: string(voteaiinputprovenance.PurposeAuditTarget),
				Text:    target, Current: true,
			},
		},
	}

	plan, err := svc.prepareIncrementalAudit(context.Background(), ContentModerationCheckInput{
		UserID: 61, APIKeyID: 71, SessionID: "unique-target",
	}, cfg, content)

	require.NoError(t, err)
	plan.ensureReviewInput(cfg, false, nil)
	require.Equal(t, 1, strings.Count(plan.fullInput, "[AUDIT-TARGET-LOCATOR"))
	require.Equal(t, 1, strings.Count(plan.fullInput, target))
	require.Contains(t, plan.fullInput, "[CONVERSATION-HISTORY]")
	require.Contains(t, plan.fullInput, "Earlier request.")
	require.Contains(t, plan.fullInput, "Earlier answer.")
	require.Zero(t, strings.Count(plan.canonicalFullPrefix, "[AUDIT-TARGET-LOCATOR"))
	require.Equal(t, 1, strings.Count(plan.canonicalFullPrefix, target))
}

func TestContentModerationGuard_CleanFastPathDoesNotBuildReviewInput(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "benign")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.PeriodicFullReviewTurns = 25
	svc, _ := newContentModerationGuardService(t, cfg, server, newContentModerationGuardCache())
	timings := newContentModerationLatencyBreakdown()
	content := contentModerationGuardInput("ordinary project update")
	content.auditTimings = timings

	result, plan, err := svc.callIncrementalAIChatAudit(context.Background(), ContentModerationCheckInput{
		UserID: 611, APIKeyID: 711, SessionID: "clean-fast", RequestID: "clean-fast-1",
	}, cfg, content, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, voteaimoderation.StageFast, result.Stage)
	require.Equal(t, int64(1), calls.Load())
	require.False(t, plan.reviewInputBuilt)
	require.Empty(t, plan.fullInput)
	require.Empty(t, plan.periodicInput)
	require.Nil(t, timings.reviewBuildLatencyMS)
}

func TestContentModerationGuard_FastStageUsesBoundedBudgetWithoutSameKeyRetry(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		time.Sleep(3 * time.Second)
	}))
	defer func() {
		server.CloseClientConnections()
		server.Close()
	}()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.RetryCount = 2
	cfg.AIChat.FastStageBudgetMS = 1500
	svc, _ := newContentModerationGuardService(t, cfg, server, newContentModerationGuardCache())
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	startedAt := time.Now()

	result, _, err := svc.callIncrementalAIChatAudit(ctx, ContentModerationCheckInput{
		UserID: 612, APIKeyID: 712, SessionID: "bounded-fast", RequestID: "bounded-fast-1",
	}, cfg, contentModerationGuardInput("ordinary project update"), false)

	require.Error(t, err)
	require.Nil(t, result)
	require.Less(t, time.Since(startedAt), 2200*time.Millisecond)
	require.Equal(t, int64(1), calls.Load())
}

func TestContentModerationGuard_PeriodicTrajectoryKeepsTenTurnsAndSamplesContent(t *testing.T) {
	turns := make([]voteaiauditcontext.Turn, 0, 24)
	for round := 1; round <= 12; round++ {
		turns = append(turns,
			voteaiauditcontext.Turn{Role: voteaiauditcontext.RoleUser, Text: fmt.Sprintf("user-%02d-%s", round, strings.Repeat("u", 240))},
			voteaiauditcontext.Turn{Role: voteaiauditcontext.RoleAssistant, Text: fmt.Sprintf("assistant-%02d-%s", round, strings.Repeat("a", 160))},
		)
	}
	targetIndex := len(turns) - 2
	target := turns[targetIndex].Text
	canonical, input := buildContentModerationPeriodicReviewInputForTurns(
		turns, targetIndex, "user_request", target, voteaiauditcontext.State{TurnCount: 9},
	)

	require.Contains(t, canonical, "[PERIODIC-RISK-TRAJECTORY last_user_turns=10]")
	require.NotContains(t, canonical, "user-02-")
	require.Contains(t, canonical, "user-03-")
	require.Contains(t, canonical, "user-12-")
	require.Contains(t, canonical, "[CONTEXT OMITTED]")
	require.Equal(t, 1, strings.Count(input, "[AUDIT-TARGET-LOCATOR"))
	require.Less(t, len([]rune(input)), 3000)
}

func TestContentModerationGuard_PeriodicTrajectoryOnlyAppliesToSolePeriodicReason(t *testing.T) {
	require.True(t, contentModerationUsesPeriodicTrajectory([]string{voteaiauditcontext.ReviewReasonPeriodic}))
	require.False(t, contentModerationUsesPeriodicTrajectory([]string{
		voteaiauditcontext.ReviewReasonPeriodic,
		voteaiauditcontext.ReviewReasonRiskRise,
	}))
	require.False(t, contentModerationUsesPeriodicTrajectory([]string{voteaiauditcontext.ReviewReasonStrongSignal}))

	plan := &contentModerationIncrementalPlan{
		reviewTargetKind: "user_request",
		reviewTargetText: "current target",
		reviewSourceTurns: []ContentModerationTurn{
			{Role: "user", Purpose: "supporting_context", Text: "earlier target"},
			{Role: "user", Purpose: "audit_target", Text: "current target"},
		},
	}
	plan.ensureReviewInput(defaultContentModerationConfig(), true, nil)
	require.Contains(t, plan.fullInput, "[PERIODIC-RISK-TRAJECTORY")
	require.Equal(t, plan.periodicInput, plan.fullInput)
	require.Equal(t, plan.canonicalPeriodic, plan.canonicalFullPrefix)
	require.True(t, plan.fullHistoryTruncated)
	require.True(t, plan.prefixCompacted)
	require.False(t, plan.prefixHistoryRewrite)
}

func TestContentModerationGuard_PersistedAuditReasonDropsCleanLowRiskExplanation(t *testing.T) {
	cfg := defaultContentModerationConfig()
	result := &moderationAPIResult{
		CategoryScores: map[string]float64{"ai_risk": 0.05},
		Reason:         "请求为正常项目进度讨论",
	}
	require.Empty(t, contentModerationPersistedAuditReason(result, cfg))
}

func TestContentModerationGuard_PersistedAuditReasonKeepsRiskEvidence(t *testing.T) {
	cfg := defaultContentModerationConfig()
	tests := []moderationAPIResult{
		{CategoryScores: map[string]float64{"ai_risk": 0.25}, Reason: "风险分达到历史阈值"},
		{CategoryScores: map[string]float64{"ai_risk": 0.05}, Categories: []string{"cyber_abuse"}, Reason: "存在风险类别"},
		{CategoryScores: map[string]float64{"ai_risk": 0.05}, Signals: []string{"defensive_context"}, Reason: "存在语义信号"},
	}
	for index := range tests {
		require.Equal(t, tests[index].Reason, contentModerationPersistedAuditReason(&tests[index], cfg))
	}
}

func TestContentModerationGuard_PeriodicOnlyReviewUsesCompactTrajectory(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "benign")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.PeriodicFullReviewTurns = 1
	cfg.AIChat.CacheEnabled = false
	svc, _ := newContentModerationGuardService(t, cfg, server, newContentModerationGuardCache())

	result, plan, err := svc.callIncrementalAIChatAudit(context.Background(), ContentModerationCheckInput{
		UserID: 81, APIKeyID: 91, SessionID: "periodic-compact", RequestID: "periodic-compact-1",
	}, cfg, contentModerationGuardInput("ordinary project update"), false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, voteaimoderation.StageFull, result.Stage)
	require.Equal(t, int64(2), calls.Load())
	require.Equal(t, []string{voteaiauditcontext.ReviewReasonPeriodic}, plan.escalationReasons)
	require.Contains(t, plan.fullInput, "[PERIODIC-RISK-TRAJECTORY")
	require.NotContains(t, plan.fullInput, "[CONVERSATION-HISTORY]")
}

func TestContentModerationGuard_StrongSignalKeepsFullHistoryEvenWhenPeriodic(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeContentModerationGuardResult(w, true, 0.95, []string{"policy_evasion"}, []string{"policy_evasion"}, "high risk")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.PeriodicFullReviewTurns = 1
	cfg.AIChat.CacheEnabled = false
	svc, _ := newContentModerationGuardService(t, cfg, server, newContentModerationGuardCache())
	content := contentModerationGuardInput("ordinary project update")
	content.Turns = []ContentModerationTurn{
		{Role: "user", Purpose: string(voteaiinputprovenance.PurposeSupportingContext), Text: strings.Repeat("historical context ", 80)},
		{Role: "assistant", Purpose: string(voteaiinputprovenance.PurposeSupportingContext), Text: "ordinary answer"},
		{Role: "user", Purpose: string(voteaiinputprovenance.PurposeAuditTarget), Text: "ordinary project update"},
	}

	result, plan, err := svc.callIncrementalAIChatAudit(context.Background(), ContentModerationCheckInput{
		UserID: 82, APIKeyID: 92, SessionID: "periodic-strong", RequestID: "periodic-strong-1",
	}, cfg, content, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, voteaimoderation.StageMax, result.Stage)
	require.Equal(t, int64(2), calls.Load())
	require.Contains(t, plan.escalationReasons, voteaiauditcontext.ReviewReasonPeriodic)
	require.Contains(t, plan.escalationReasons, voteaiauditcontext.ReviewReasonStrongSignal)
	require.Contains(t, plan.fullInput, "[CONVERSATION-HISTORY]")
	require.NotContains(t, plan.fullInput, "[PERIODIC-RISK-TRAJECTORY")
}

func TestContentModerationGuard_FullReviewRedactsSecretsFromEveryHistoricalRole(t *testing.T) {
	cfg := contentModerationGuardConfig("http://127.0.0.1")
	cfg.AIChat.IncrementalAuditEnabled = true
	svc := NewContentModerationService(nil, nil, newContentModerationGuardCache(), nil, nil, nil, nil, nil)
	content := ContentModerationInput{
		Text:            "conversation",
		CurrentText:     "Review the latest deployment result.",
		AuditTargetText: "Review the latest deployment result.",
		AuditTargetKind: string(voteaiinputprovenance.TargetUserRequest),
		HasExplicitUser: true,
		Turns: []ContentModerationTurn{
			{Role: "system", Source: string(voteaiinputprovenance.SourceClientInstruction), Purpose: string(voteaiinputprovenance.PurposeSupportingContext), Text: "api_key=audit-system-secret-canary-1234567890"},
			{Role: "user", Source: string(voteaiinputprovenance.SourceEndUser), Purpose: string(voteaiinputprovenance.PurposeSupportingContext), Text: "password=historical-user-password"},
			{Role: "assistant", Source: string(voteaiinputprovenance.SourceAssistantResponse), Purpose: string(voteaiinputprovenance.PurposeSupportingContext), Text: "Authorization: Bearer historical-assistant-token"},
			{Role: "user", Source: string(voteaiinputprovenance.SourceEndUser), Purpose: string(voteaiinputprovenance.PurposeAuditTarget), Text: "Review the latest deployment result."},
		},
	}

	plan, err := svc.prepareIncrementalAudit(context.Background(), ContentModerationCheckInput{
		UserID: 62, APIKeyID: 72, SessionID: "redacted-history",
	}, cfg, content)

	require.NoError(t, err)
	plan.ensureReviewInput(cfg, false, nil)
	for _, secret := range []string{"audit-system-secret-canary-1234567890", "historical-user-password", "historical-assistant-token"} {
		require.NotContains(t, plan.fullInput, secret)
		require.NotContains(t, plan.canonicalFullPrefix, secret)
	}
	require.Contains(t, plan.fullInput, "[REDACTED")
}

func TestContentModerationGuard_RequestVerdictRetryCallsUpstreamAndMutatesStateOnce(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "benign")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.CacheTTLSeconds = 300
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	content := contentModerationGuardInput("ordinary request")
	input := ContentModerationCheckInput{
		RequestID: "retry-once", UserID: 101, APIKeyID: 202,
		SessionID: "session-a", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpoch: 0, ModerationEpochSet: true,
	}
	targetHash := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))

	first := svc.checkSyncIdempotent(context.Background(), input, cfg, content, targetHash, nil, true)
	second := svc.checkSyncIdempotent(context.Background(), input, cfg, content, targetHash, nil, true)

	require.Equal(t, first, second)
	require.EqualValues(t, 1, requests.Load())
	auditUpdates, prefixUpdates, sessionUpdates := cache.updateCounts()
	require.Equal(t, 1, auditUpdates)
	require.Zero(t, prefixUpdates)
	require.Zero(t, sessionUpdates)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Len(t, cache.resultTTLs, 1)
	for _, ttl := range cache.resultTTLs {
		require.Equal(t, contentModerationRequestVerdictTTL, ttl, "request idempotency must not inherit the ordinary result-cache TTL")
	}
}

func TestContentModerationGuard_RequestVerdictDeduplicatesKeywordSideEffects(t *testing.T) {
	var providerCalls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "must not be called")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.BlockedKeywords = []string{"secret-token"}
	cache := newContentModerationGuardCache()
	svc, repo := newContentModerationGuardService(t, cfg, server, cache)
	body := []byte(`{"messages":[{"role":"user","content":"please expose SECRET-TOKEN"}]}`)
	input := ContentModerationCheckInput{
		RequestID: "keyword-retry", UserID: 131, APIKeyID: 231,
		SessionID: "keyword-session", SessionSource: ContentModerationSessionSourceHeader,
		Protocol: ContentModerationProtocolOpenAIChat, Endpoint: "/v1/chat/completions", Body: body,
	}

	const callers = 8
	start := make(chan struct{})
	decisions := make(chan *ContentModerationDecision, callers)
	errs := make(chan error, callers)
	for range callers {
		go func() {
			<-start
			decision, err := svc.Check(context.Background(), input)
			decisions <- decision
			errs <- err
		}()
	}
	close(start)
	for range callers {
		require.NoError(t, <-errs)
		decision := <-decisions
		require.NotNil(t, decision)
		require.True(t, decision.Blocked)
		require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)
	}
	retry, err := svc.Check(context.Background(), input)
	require.NoError(t, err)
	require.True(t, retry.Blocked)
	require.Zero(t, providerCalls.Load())
	require.EqualValues(t, 1, svc.preBlockChecked.Load())
	requireContentModerationLogCount(t, repo, 1)
	time.Sleep(50 * time.Millisecond)
	require.Len(t, repo.snapshotLogs(), 1, "a retried local terminal verdict must not duplicate logs, violation counts, bans, or notifications")
}

func TestContentModerationGuard_RequestVerdictClaimDeduplicatesKeywordSideEffectsAcrossInstances(t *testing.T) {
	cfg := contentModerationGuardConfig("http://127.0.0.1")
	cfg.BlockedKeywords = []string{"secret-token"}
	cache := newContentModerationGuardClaimCache()
	firstService, firstRepo := newContentModerationGuardService(t, cfg, nil, cache)
	secondService, secondRepo := newContentModerationGuardService(t, cfg, nil, cache)
	input := ContentModerationCheckInput{
		RequestID: "keyword-cross-instance", UserID: 132, APIKeyID: 232,
		SessionID: "keyword-cross-session", SessionSource: ContentModerationSessionSourceHeader,
		Protocol: ContentModerationProtocolOpenAIChat, Endpoint: "/v1/chat/completions",
		Body: []byte(`{"messages":[{"role":"user","content":"please expose SECRET-TOKEN"}]}`),
	}

	start := make(chan struct{})
	decisions := make(chan *ContentModerationDecision, 2)
	errs := make(chan error, 2)
	for _, svc := range []*ContentModerationService{firstService, secondService} {
		go func(svc *ContentModerationService) {
			<-start
			decision, err := svc.Check(context.Background(), input)
			decisions <- decision
			errs <- err
		}(svc)
	}
	close(start)
	for range 2 {
		require.NoError(t, <-errs)
		decision := <-decisions
		require.NotNil(t, decision)
		require.True(t, decision.Blocked)
		require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)
	}

	require.EqualValues(t, 1, firstService.preBlockChecked.Load()+secondService.preBlockChecked.Load())
	require.Eventually(t, func() bool {
		return len(firstRepo.snapshotLogs())+len(secondRepo.snapshotLogs()) == 1
	}, time.Second, 10*time.Millisecond)
}

func TestContentModerationGuard_RequestVerdictClaimErrorReturnsKeywordFallbackWithoutSideEffects(t *testing.T) {
	var providerCalls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "must not be called")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyAllow
	cfg.BlockedKeywords = []string{"secret-token"}
	cache := newContentModerationGuardClaimCache()
	cache.claimErr = errors.New("redis unavailable")
	svc, repo := newContentModerationGuardService(t, cfg, server, cache)
	input := ContentModerationCheckInput{
		RequestID: "keyword-claim-error", UserID: 134, APIKeyID: 234,
		SessionID: "keyword-claim-error-session", SessionSource: ContentModerationSessionSourceHeader,
		Protocol: ContentModerationProtocolOpenAIChat, Endpoint: "/v1/chat/completions",
		Body: []byte(`{"messages":[{"role":"user","content":"please expose SECRET-TOKEN"}]}`),
	}

	decision, err := svc.Check(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, decision)
	require.True(t, decision.Blocked, "a coordination outage must not turn a deterministic keyword block into fail-open")
	require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)
	require.Zero(t, providerCalls.Load())
	require.Zero(t, svc.preBlockChecked.Load(), "claim failure must not execute the guarded local verdict")
	requireContentModerationLogCount(t, repo, 0)
	auditUpdates, prefixUpdates, sessionUpdates := cache.updateCounts()
	require.Zero(t, auditUpdates)
	require.Zero(t, prefixUpdates)
	require.Zero(t, sessionUpdates)
}

func TestContentModerationGuard_RequestVerdictReadErrorPreservesKeywordFallbackWithoutSideEffects(t *testing.T) {
	var providerCalls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "must not be called")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyAllow
	cfg.BlockedKeywords = []string{"secret-token"}
	cache := newContentModerationGuardClaimCache()
	cache.resultGetErr = errors.New("redis unavailable")
	svc, repo := newContentModerationGuardService(t, cfg, server, cache)
	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		RequestID: "keyword-read-error", UserID: 136, APIKeyID: 236,
		SessionID: "keyword-read-error-session", SessionSource: ContentModerationSessionSourceHeader,
		Protocol: ContentModerationProtocolOpenAIChat, Endpoint: "/v1/chat/completions",
		Body: []byte(`{"messages":[{"role":"user","content":"please expose SECRET-TOKEN"}]}`),
	})

	require.NoError(t, err)
	require.NotNil(t, decision)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)
	require.Zero(t, providerCalls.Load())
	require.Zero(t, svc.preBlockChecked.Load())
	requireContentModerationLogCount(t, repo, 0)
	auditUpdates, prefixUpdates, sessionUpdates := cache.updateCounts()
	require.Zero(t, auditUpdates)
	require.Zero(t, prefixUpdates)
	require.Zero(t, sessionUpdates)
}

func TestContentModerationGuard_RequestVerdictClaimErrorSkipsProviderAndTerminalCache(t *testing.T) {
	var providerCalls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "benign")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyAllow
	cache := newContentModerationGuardClaimCache()
	cache.claimErr = errors.New("redis claim unavailable")
	svc, repo := newContentModerationGuardService(t, cfg, server, cache)
	content := contentModerationGuardInput("ordinary request during a claim outage")
	input := ContentModerationCheckInput{
		RequestID: "provider-claim-error", UserID: 137, APIKeyID: 237,
		SessionID: "provider-claim-error-session", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpochSet: true,
	}
	targetHash := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))

	first := svc.checkSyncIdempotent(context.Background(), input, cfg, content, targetHash, nil, true)
	second := svc.checkSyncIdempotent(context.Background(), input, cfg, content, targetHash, nil, true)

	require.NotNil(t, first)
	require.NotNil(t, second)
	require.True(t, first.Allowed)
	require.Equal(t, first, second)
	require.Zero(t, providerCalls.Load(), "claim failure must not execute a provider evaluation without ownership")
	key := contentModerationRequestVerdictCacheKey(input, cfg, content, targetHash, "preblock_sync")
	_, hit, cacheErr := svc.getContentModerationRequestVerdict(context.Background(), key)
	require.NoError(t, cacheErr)
	require.False(t, hit, "a no-side-effect fallback must not become a terminal verdict")
	requireContentModerationLogCount(t, repo, 0)
	auditUpdates, prefixUpdates, sessionUpdates := cache.updateCounts()
	require.Zero(t, auditUpdates)
	require.Zero(t, prefixUpdates)
	require.Zero(t, sessionUpdates)
}

func TestContentModerationGuard_ResponsesPreviousResponseToolContinuationIsAudited(t *testing.T) {
	var providerCalls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		writeContentModerationGuardResult(w, false, 0.04, nil, nil, "benign tool continuation")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.CacheEnabled = false
	svc, _ := newContentModerationGuardService(t, cfg, server, newContentModerationGuardCache())
	body := []byte(`{
		"previous_response_id":"resp_previous",
		"input":[
			{"type":"function_call_output","call_id":"call_1","output":"release verification completed"}
		]
	}`)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		RequestID: "responses-tool-continuation", UserID: 138, APIKeyID: 238,
		Protocol: ContentModerationProtocolOpenAIResponses,
		Endpoint: "/v1/responses",
		Body:     body,
	})

	require.NoError(t, err)
	require.NotNil(t, decision)
	require.True(t, decision.Allowed)
	require.EqualValues(t, 1, providerCalls.Load(), "linked tool continuation must not be classified as no_new_user_intent")
}

func TestContentModerationGuard_RequestVerdictHeldClaimTimeoutPreservesLocalBlockWithoutEvaluator(t *testing.T) {
	cfg := contentModerationGuardConfig("http://127.0.0.1")
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyAllow
	cache := newContentModerationGuardClaimCache()
	const key = "held-local-verdict"
	cache.claims[key] = "other-instance"
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	fallback := &ContentModerationDecision{
		Allowed: false, Blocked: true, Flagged: true,
		Action: ContentModerationActionKeywordBlock,
	}
	var evaluated atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	decision := svc.runContentModerationRequestVerdict(
		ctx,
		ContentModerationCheckInput{RequestID: "held-local-verdict"},
		cfg,
		key,
		"preblock_sync",
		fallback,
		func(context.Context) *ContentModerationDecision {
			evaluated.Add(1)
			return &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow, requestVerdictCacheable: true}
		},
	)

	require.NotNil(t, decision)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)
	require.Zero(t, evaluated.Load(), "a worker that does not own the claim must never execute guarded side effects")
	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Empty(t, cache.results, "a no-side-effect fallback is not a completed verdict and must not enter the ledger")
}

func TestContentModerationGuard_RequestVerdictNoAuditKeysCreatesOneTerminalLog(t *testing.T) {
	cfg := contentModerationGuardConfig("http://127.0.0.1")
	cfg.AIChat.APIKeys = nil
	cache := newContentModerationGuardClaimCache()
	svc, repo := newContentModerationGuardService(t, cfg, nil, cache)
	input := ContentModerationCheckInput{
		RequestID: "no-audit-key-retry", UserID: 135, APIKeyID: 235,
		SessionID: "no-audit-key-session", SessionSource: ContentModerationSessionSourceHeader,
		Protocol: ContentModerationProtocolOpenAIChat, Endpoint: "/v1/chat/completions",
		Body: []byte(`{"messages":[{"role":"user","content":"ordinary request"}]}`),
	}

	first, err := svc.Check(context.Background(), input)
	require.NoError(t, err)
	second, err := svc.Check(context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, ContentModerationActionUnavailable, first.Action)
	require.Equal(t, first, second)
	require.EqualValues(t, 1, svc.preBlockChecked.Load())
	requireContentModerationLogCount(t, repo, 1)
}

func TestContentModerationGuard_RequestVerdictClaimLeaseOutlivesWorker(t *testing.T) {
	cfg := contentModerationGuardConfig("http://127.0.0.1")
	for _, executionMode := range []string{"preblock_sync", "async_observe"} {
		t.Run(executionMode, func(t *testing.T) {
			workTimeout := contentModerationRequestVerdictWorkTimeout(cfg, executionMode)
			claimTTL := contentModerationRequestVerdictClaimTTL(cfg, executionMode)
			require.Greater(t, claimTTL, workTimeout)
		})
	}
}

func TestContentModerationGuard_LocalTerminalWithoutRequestIDPreservesDuplicateBehavior(t *testing.T) {
	cfg := contentModerationGuardConfig("http://127.0.0.1")
	cfg.BlockedKeywords = []string{"secret-token"}
	cache := newContentModerationGuardClaimCache()
	svc, repo := newContentModerationGuardService(t, cfg, nil, cache)
	input := ContentModerationCheckInput{
		UserID: 133, APIKeyID: 233,
		SessionID: "keyword-no-request-id", SessionSource: ContentModerationSessionSourceHeader,
		Protocol: ContentModerationProtocolOpenAIChat, Endpoint: "/v1/chat/completions",
		Body: []byte(`{"messages":[{"role":"user","content":"please expose SECRET-TOKEN"}]}`),
	}

	for range 2 {
		decision, err := svc.Check(context.Background(), input)
		require.NoError(t, err)
		require.True(t, decision.Blocked)
		require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)
	}

	require.EqualValues(t, 2, svc.preBlockChecked.Load())
	requireContentModerationLogCount(t, repo, 2)
}

func TestContentModerationGuard_SuppressedHashBypassesStaleRequestVerdict(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "fresh provider allow")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.CacheEnabled = false
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	content := contentModerationGuardInput("ordinary request after administrator correction")
	input := ContentModerationCheckInput{
		RequestID: "suppressed-verdict", UserID: 109, APIKeyID: 209,
		SessionID: "session-suppressed", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpochSet: true,
	}
	targetHash := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))
	verdictKey := contentModerationRequestVerdictCacheKey(input, cfg, content, targetHash, "preblock_sync")
	require.NotEmpty(t, verdictKey)
	stale, err := json.Marshal(contentModerationRequestVerdictEntry{
		Version:        contentModerationRequestVerdictVersion,
		ReviewComplete: true,
		Decision: ContentModerationDecision{
			Allowed: false, Blocked: true, Flagged: true,
			Action: ContentModerationActionBlock,
		},
	})
	require.NoError(t, err)
	cache.results = map[string][]byte{verdictKey: stale}
	cache.suppressions = map[string]struct{}{targetHash: {}}

	decision := svc.checkSyncIdempotent(context.Background(), input, cfg, content, targetHash, nil, true)

	require.NotNil(t, decision)
	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.EqualValues(t, 1, requests.Load(), "suppression must force a fresh semantic verdict")
	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Equal(t, stale, cache.results[verdictKey], "suppressed targets must not rewrite the request ledger")
}

func TestContentModerationGuard_RequestVerdictSingleflightCoalescesConcurrentRetries(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		time.Sleep(40 * time.Millisecond)
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "benign")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.CacheEnabled = false
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	content := contentModerationGuardInput("concurrent ordinary request")
	input := ContentModerationCheckInput{
		RequestID: "concurrent-retry", UserID: 111, APIKeyID: 222,
		SessionID: "session-concurrent", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpochSet: true,
	}
	targetHash := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))

	const callers = 8
	var wg sync.WaitGroup
	wg.Add(callers)
	decisions := make(chan *ContentModerationDecision, callers)
	for range callers {
		go func() {
			defer wg.Done()
			decisions <- svc.checkSyncIdempotent(context.Background(), input, cfg, content, targetHash, nil, true)
		}()
	}
	wg.Wait()
	close(decisions)
	for decision := range decisions {
		require.NotNil(t, decision)
		require.True(t, decision.Allowed)
	}
	require.EqualValues(t, 1, requests.Load())
	auditUpdates, _, _ := cache.updateCounts()
	require.Equal(t, 1, auditUpdates)
}

func TestContentModerationGuard_RequestVerdictClaimCoalescesAcrossServiceInstances(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		time.Sleep(100 * time.Millisecond)
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "benign")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.CacheEnabled = false
	cache := newContentModerationGuardClaimCache()
	firstService, _ := newContentModerationGuardService(t, cfg, server, cache)
	secondService, _ := newContentModerationGuardService(t, cfg, server, cache)
	content := contentModerationGuardInput("cross-instance ordinary request")
	input := ContentModerationCheckInput{
		RequestID: "cross-instance-retry", UserID: 112, APIKeyID: 223,
		SessionID: "session-cross-instance", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpochSet: true,
	}
	targetHash := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))

	start := make(chan struct{})
	decisions := make(chan *ContentModerationDecision, 2)
	for _, svc := range []*ContentModerationService{firstService, secondService} {
		go func(svc *ContentModerationService) {
			<-start
			decisions <- svc.checkSyncIdempotent(context.Background(), input, cfg, content, targetHash, nil, true)
		}(svc)
	}
	close(start)
	for range 2 {
		decision := <-decisions
		require.NotNil(t, decision)
		require.True(t, decision.Allowed)
	}

	require.EqualValues(t, 1, requests.Load())
	auditUpdates, _, _ := cache.updateCounts()
	require.Equal(t, 1, auditUpdates)
}

func TestContentModerationGuard_RequestVerdictCanceledLeaderDoesNotPoisonFollower(t *testing.T) {
	var requests atomic.Int64
	var startedOnce sync.Once
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		startedOnce.Do(func() { close(started) })
		<-release
		writeContentModerationGuardResult(w, true, 0.91, []string{"cyber_abuse"}, []string{"auth_bypass"}, "blocked")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyAllow
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	content := contentModerationGuardInput("ordinary request evaluated by the provider")
	input := ContentModerationCheckInput{
		RequestID: "canceled-leader", UserID: 136, APIKeyID: 236,
		SessionID: "canceled-leader-session", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpochSet: true,
	}
	targetHash := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderResult := make(chan *ContentModerationDecision, 1)
	go func() {
		leaderResult <- svc.checkSyncIdempotent(leaderCtx, input, cfg, content, targetHash, nil, true)
	}()
	<-started
	cancelLeader()
	leader := <-leaderResult
	require.NotNil(t, leader)
	require.True(t, leader.Allowed, "the canceled caller receives its fail-open fallback")

	followerResult := make(chan *ContentModerationDecision, 1)
	go func() {
		followerResult <- svc.checkSyncIdempotent(context.Background(), input, cfg, content, targetHash, nil, true)
	}()
	close(release)
	follower := <-followerResult
	require.NotNil(t, follower)
	require.True(t, follower.Blocked, "a healthy follower must receive the real completed verdict")
	require.EqualValues(t, 1, requests.Load())

	replay := svc.checkSyncIdempotent(context.Background(), input, cfg, content, targetHash, nil, true)
	require.True(t, replay.Blocked)
	require.EqualValues(t, 1, requests.Load(), "the completed background verdict must be persisted for later retries")
}

func TestContentModerationGuard_RequestVerdictKeyIsolatesIdentityPolicyContentAndEpoch(t *testing.T) {
	cfg := contentModerationGuardConfig("http://127.0.0.1")
	cfg.AIChat.CacheEnabled = false
	base := ContentModerationCheckInput{
		RequestID: "same", UserID: 1, APIKeyID: 2, SessionID: "session",
		SessionSource:   ContentModerationSessionSourceHeader,
		ModerationEpoch: 3, ModerationEpochSet: true,
	}
	baseContent := contentModerationGuardInput("same target")
	key := contentModerationRequestVerdictCacheKey(base, cfg, baseContent, "target-a", "preblock_sync")
	require.NotEmpty(t, key)

	variants := []struct {
		name       string
		input      ContentModerationCheckInput
		config     *ContentModerationConfig
		content    ContentModerationInput
		targetHash string
		mode       string
	}{
		{name: "user", input: func() ContentModerationCheckInput { value := base; value.UserID++; return value }(), config: cfg, content: baseContent, targetHash: "target-a", mode: "preblock_sync"},
		{name: "api key", input: func() ContentModerationCheckInput { value := base; value.APIKeyID++; return value }(), config: cfg, content: baseContent, targetHash: "target-a", mode: "preblock_sync"},
		{name: "session", input: func() ContentModerationCheckInput { value := base; value.SessionID = "other"; return value }(), config: cfg, content: baseContent, targetHash: "target-a", mode: "preblock_sync"},
		{name: "epoch", input: func() ContentModerationCheckInput { value := base; value.ModerationEpoch++; return value }(), config: cfg, content: baseContent, targetHash: "target-a", mode: "preblock_sync"},
		{name: "target", input: base, config: cfg, content: baseContent, targetHash: "target-b", mode: "preblock_sync"},
		{name: "history", input: base, config: cfg, content: func() ContentModerationInput {
			value := baseContent
			value.Turns = append([]ContentModerationTurn{{Role: "assistant", Source: "model", Purpose: "supporting_context", Text: "rewritten earlier history"}}, value.Turns...)
			return value
		}(), targetHash: "target-a", mode: "preblock_sync"},
		{name: "execution mode", input: base, config: cfg, content: baseContent, targetHash: "target-a", mode: "async_supplemental"},
		{name: "keyword mode", input: base, config: func() *ContentModerationConfig {
			value := cloneContentModerationConfig(cfg)
			value.KeywordBlockingMode = ContentModerationKeywordModeKeywordOnly
			return value
		}(), content: baseContent, targetHash: "target-a", mode: "preblock_sync"},
		{name: "keyword list", input: base, config: func() *ContentModerationConfig {
			value := cloneContentModerationConfig(cfg)
			value.BlockedKeywords = []string{"new-blocked-keyword"}
			return value
		}(), content: baseContent, targetHash: "target-a", mode: "preblock_sync"},
		{name: "audit key count", input: base, config: func() *ContentModerationConfig {
			value := cloneContentModerationConfig(cfg)
			value.AIChat.APIKeys = nil
			return value
		}(), content: baseContent, targetHash: "target-a", mode: "preblock_sync"},
		{name: "policy", input: base, config: func() *ContentModerationConfig {
			value := cloneContentModerationConfig(cfg)
			value.AIChat.FullReviewThreshold = 0.55
			return value
		}(), content: baseContent, targetHash: "target-a", mode: "preblock_sync"},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			require.NotEqual(t, key, contentModerationRequestVerdictCacheKey(variant.input, variant.config, variant.content, variant.targetHash, variant.mode))
		})
	}
}

func TestContentModerationGuard_RequestVerdictDigestIsolatesChangedSecretValues(t *testing.T) {
	cfg := contentModerationGuardConfig("http://127.0.0.1")
	input := ContentModerationCheckInput{
		RequestID: "same-secret-request", UserID: 141, APIKeyID: 241,
		SessionID: "same-secret-session", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpochSet: true,
	}
	first := contentModerationGuardInput("password=first-secret-value")
	second := contentModerationGuardInput("password=second-secret-value")
	policy := contentModerationAuditPolicyVersion(cfg)
	firstTargetHash := first.AuditTargetHash(policy)
	secondTargetHash := second.AuditTargetHash(policy)
	require.Equal(t, firstTargetHash, secondTargetHash, "the permanent risk target remains semantically redacted")

	firstKey := contentModerationRequestVerdictCacheKey(input, cfg, first, firstTargetHash, "preblock_sync")
	secondKey := contentModerationRequestVerdictCacheKey(input, cfg, second, secondTargetHash, "preblock_sync")
	require.NotEqual(t, firstKey, secondKey, "request ledger must hash the complete normalized input before redaction")
}

func TestContentModerationGuard_RequestVerdictSameIDWithRewrittenHistoryDoesNotHit(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "benign")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.CacheEnabled = false
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	firstContent := contentModerationGuardInput("same final request")
	firstContent.Turns = append([]ContentModerationTurn{{
		Role: "assistant", Source: "model", Purpose: "supporting_context", Text: "first history",
	}}, firstContent.Turns...)
	secondContent := firstContent
	secondContent.Turns = append([]ContentModerationTurn(nil), firstContent.Turns...)
	secondContent.Turns[0].Text = "rewritten history"
	input := ContentModerationCheckInput{
		RequestID: "same-id-rewritten-history", UserID: 113, APIKeyID: 224,
		SessionID: "session-rewritten-history", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpochSet: true,
	}
	policy := contentModerationAuditPolicyVersion(cfg)
	firstTargetHash := firstContent.AuditTargetHash(policy)
	secondTargetHash := secondContent.AuditTargetHash(policy)
	require.Equal(t, firstTargetHash, secondTargetHash, "the full conversation digest, not target hash drift, must isolate this retry")

	first := svc.checkSyncIdempotent(context.Background(), input, cfg, firstContent, firstTargetHash, nil, true)
	second := svc.checkSyncIdempotent(context.Background(), input, cfg, secondContent, secondTargetHash, nil, true)

	require.True(t, first.Allowed)
	require.True(t, second.Allowed)
	require.EqualValues(t, 2, requests.Load())
}

func TestContentModerationGuard_RequestVerdictLedgerAcceptsOnlyCompleteReviews(t *testing.T) {
	cache := newContentModerationGuardCache()
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	const key = "opaque-verdict-key"
	incomplete, err := json.Marshal(contentModerationRequestVerdictEntry{
		Version:        contentModerationRequestVerdictVersion,
		ReviewComplete: false,
		Decision: ContentModerationDecision{
			Allowed: true,
			Action:  ContentModerationActionAllow,
		},
	})
	require.NoError(t, err)
	cache.results = map[string][]byte{key: incomplete}

	decision, hit, cacheErr := svc.getContentModerationRequestVerdict(context.Background(), key)
	require.NoError(t, cacheErr)
	require.False(t, hit)
	require.Nil(t, decision)

	require.NoError(t, svc.setContentModerationRequestVerdict(context.Background(), key, &ContentModerationDecision{
		Allowed:                 true,
		Action:                  ContentModerationActionAllow,
		requestVerdictCacheable: true,
	}))
	decision, hit, cacheErr = svc.getContentModerationRequestVerdict(context.Background(), key)
	require.NoError(t, cacheErr)
	require.True(t, hit)
	require.True(t, decision.Allowed)
}

func TestContentModerationGuard_ObserveRequestVerdictStaysQueuedUntilWorkerCompletes(t *testing.T) {
	cache := newContentModerationGuardClaimCache()
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	svc.asyncQueue = make(chan contentModerationTask, 4)
	cfg := contentModerationGuardConfig("https://audit.invalid")
	cfg.Mode = ContentModerationModeObserve
	content := contentModerationGuardInput("ordinary queued request")
	input := ContentModerationCheckInput{
		RequestID: "observe-queued", UserID: 121, APIKeyID: 232,
		SessionID: "observe-queued-session", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpochSet: true,
	}
	targetHash := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))
	key := contentModerationRequestVerdictCacheKey(input, cfg, content, targetHash, "async_observe")
	require.NotEmpty(t, key)

	first := svc.enqueueContentModerationObserveIdempotent(context.Background(), input, cfg, content, targetHash, key)
	require.NotNil(t, first)
	require.True(t, first.Allowed)
	require.Len(t, svc.asyncQueue, 1)
	queued, queueErr := svc.contentModerationRequestVerdictQueued(context.Background(), key)
	require.NoError(t, queueErr)
	require.True(t, queued)
	decision, hit, cacheErr := svc.getContentModerationRequestVerdict(context.Background(), key)
	require.NoError(t, cacheErr)
	require.False(t, hit, "queued work must not masquerade as a completed allow verdict")
	require.Nil(t, decision)

	second := svc.enqueueContentModerationObserveIdempotent(context.Background(), input, cfg, content, targetHash, key)
	require.NotNil(t, second)
	require.True(t, second.Allowed)
	require.Len(t, svc.asyncQueue, 1, "a retry with the same request ID must not enqueue twice")

	require.NoError(t, svc.setContentModerationRequestVerdict(context.Background(), key, &ContentModerationDecision{
		Allowed: true, Action: ContentModerationActionAllow, requestVerdictCacheable: true,
	}))
	queued, queueErr = svc.contentModerationRequestVerdictQueued(context.Background(), key)
	require.NoError(t, queueErr)
	require.False(t, queued)
	decision, hit, cacheErr = svc.getContentModerationRequestVerdict(context.Background(), key)
	require.NoError(t, cacheErr)
	require.True(t, hit)
	require.True(t, decision.Allowed)
}

func TestContentModerationGuard_ObserveQueueCapturesImmutablePolicySnapshot(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	svc.asyncQueue = make(chan contentModerationTask, 1)
	cfg := contentModerationGuardConfig("https://audit.invalid")
	cfg.Mode = ContentModerationModeObserve
	cfg.AIChat.SystemPrompt = "policy-v1"
	cfg.AIChat.PricingVersion = "rates-v1"
	content := contentModerationGuardInput("snapshot request")

	require.True(t, svc.enqueueAsync(ContentModerationCheckInput{UserID: 1}, cfg, content, "hash", false))
	cfg.AIChat.SystemPrompt = "policy-v2"
	cfg.AIChat.PricingVersion = "rates-v2"
	task := <-svc.asyncQueue
	require.NotNil(t, task.config)
	require.Equal(t, "policy-v1", task.config.AIChat.SystemPrompt)
	require.Equal(t, "rates-v1", task.config.AIChat.PricingVersion)
}

func TestContentModerationGuard_SuppressionLookupFailureSkipsRiskStateMutation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeContentModerationGuardResult(w, true, 0.92, []string{"cyber_abuse"}, []string{"auth_bypass"}, "high risk")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.RiskLevelsEnabled = true
	cfg.AIChat.SessionRiskEnabled = true
	base := newContentModerationGuardCache()
	cache := &contentModerationGuardSuppressionErrorCache{contentModerationGuardCache: base, err: errors.New("redis unavailable")}
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	content := contentModerationGuardInput("explicit high-risk request")
	input := ContentModerationCheckInput{
		RequestID: "suppression-error", UserID: 44, APIKeyID: 55,
		SessionID: "suppression-error-session", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpochSet: true,
	}
	targetHash := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))
	decision := svc.checkSync(context.Background(), input, cfg, content, targetHash, nil, true)

	require.NotNil(t, decision)
	require.True(t, decision.Blocked, "the fresh verdict may still protect the upstream account")
	_, _, sessionUpdates := base.updateCounts()
	require.Zero(t, sessionUpdates, "an unavailable suppression fence must prevent persistent risk amplification")
}

func TestContentModerationGuard_RequestVerdictDoesNotCacheAuditFailure(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyAllow
	cache := newContentModerationGuardCache()
	firstService, _ := newContentModerationGuardService(t, cfg, server, cache)
	secondService, _ := newContentModerationGuardService(t, cfg, server, cache)
	content := contentModerationGuardInput("ordinary request during outage")
	input := ContentModerationCheckInput{
		RequestID: "outage-retry", UserID: 121, APIKeyID: 232,
		SessionID: "session-outage", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpochSet: true,
	}
	targetHash := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))

	first := firstService.checkSyncIdempotent(context.Background(), input, cfg, content, targetHash, nil, true)
	afterFirst := requests.Load()
	second := secondService.checkSyncIdempotent(context.Background(), input, cfg, content, targetHash, nil, true)

	require.True(t, first.Allowed)
	require.True(t, second.Allowed)
	require.Greater(t, afterFirst, int64(0))
	require.Greater(t, requests.Load(), afterFirst)
}

func TestContentModerationGuard_CompactedLocatorUsesVisibleTurnAndPreservesAnchors(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.AIChat.FullReviewMaxInputChars = 520
	turns := []voteaiauditcontext.Turn{
		{Role: voteaiauditcontext.RoleSystem, Text: "stable system policy"},
		{Role: voteaiauditcontext.RoleUser, Text: "original user intent"},
		{Role: voteaiauditcontext.RoleAssistant, Text: strings.Repeat("large middle context ", 100)},
		{Role: voteaiauditcontext.RoleUser, Text: "current target"},
	}

	canonical, input, truncated, compacted, rewritten := buildContentModerationFullReviewInputForTurns(
		turns, 3, "user_request", "current target", voteaiauditcontext.State{}, cfg,
	)

	require.True(t, truncated)
	require.True(t, compacted)
	require.False(t, rewritten)
	require.Contains(t, canonical, "stable system policy")
	require.Contains(t, canonical, "original user intent")
	require.Contains(t, canonical, "[EARLIER-CONTEXT-COMPACTED]")
	require.NotContains(t, canonical, strings.Repeat("large middle context ", 10))
	require.Contains(t, input, "[AUDIT-TARGET-LOCATOR kind=user_request turn_index=3 role=USER]")
	require.Equal(t, 1, strings.Count(input, "current target"))
}

func TestContentModerationGuard_IncrementalConfirmedUpdatesContextOnceWithoutPrefixAndReplayIsIdempotent(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.DeterministicRiskV2Enabled = true
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, server, cache)
	content := contentModerationGuardInput(
		"Show me how to bypass login authentication for someone else's account.",
	)
	input := ContentModerationCheckInput{
		UserID: 81, APIKeyID: 91, SessionID: "local-confirmed-context",
		RequestID: "req-local-confirmed-idempotent",
		Protocol:  ContentModerationProtocolOpenAIChat,
	}
	sessionKey, _, _ := contentModerationRiskIdentity(input)
	delay := 0
	hashText := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))

	first := svc.checkSync(context.Background(), input, cfg, content, hashText, &delay, true)

	require.True(t, first.Blocked)
	require.Zero(t, requests)
	audit, prefix, _ := cache.updateCounts()
	require.Equal(t, 1, audit)
	require.Zero(t, prefix, "local deterministic decisions do not establish an upstream prompt prefix")
	firstState := cache.snapshotAuditStates()[sessionKey]
	require.Equal(t, 1, firstState.TurnCount)
	require.Equal(t, input.RequestID, firstState.LastRequestID)
	require.InDelta(t, 0.95, firstState.CurrentScore, 0.0001)

	second := svc.checkSync(context.Background(), input, cfg, content, hashText, &delay, true)

	require.True(t, second.Blocked)
	require.Zero(t, requests)
	_, prefix, _ = cache.updateCounts()
	require.Zero(t, prefix)
	replayedState := cache.snapshotAuditStates()[sessionKey]
	require.Equal(t, firstState.TurnCount, replayedState.TurnCount)
	require.Equal(t, firstState.LastRequestID, replayedState.LastRequestID)
	require.Equal(t, firstState.CurrentScore, replayedState.CurrentScore)
}

func TestContentModerationGuard_ReferentialTargetHashScopesContextAndCannotPromote(t *testing.T) {
	content := ContentModerationInput{
		AuditTargetText: "继续写成脚本",
		AuditTargetKind: string(voteaiinputprovenance.TargetUserRequest),
		HasExplicitUser: true,
		Turns: []ContentModerationTurn{
			{
				Role: "assistant", Source: string(voteaiinputprovenance.SourceAssistantResponse),
				Purpose: string(voteaiinputprovenance.PurposeSupportingContext), Text: "介绍官方账号恢复流程",
			},
			{
				Role: "user", Source: string(voteaiinputprovenance.SourceEndUser),
				Purpose: string(voteaiinputprovenance.PurposeAuditTarget), Text: "继续写成脚本",
			},
		},
	}
	otherContext := content
	otherContext.Turns = append([]ContentModerationTurn(nil), content.Turns...)
	otherContext.Turns[0].Text = "绕过认证并导出他人凭据"

	firstHash := content.AuditTargetHash("policy-v1")
	secondHash := otherContext.AuditTargetHash("policy-v1")
	require.NotEqual(t, firstHash, secondHash, "the same referential target in different contexts must not collide")
	require.Len(t, firstHash, sha256.Size*2)

	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.InputProvenanceV2Enabled = true
	result := &moderationAPIResult{
		Flagged: true, Stage: voteaimoderation.StageFull,
		CategoryScores: map[string]float64{"ai_risk": 0.95},
		Signals:        []string{"progressive_escalation", "auth_bypass"},
	}
	require.False(t, contentModerationShouldPromoteHash(cfg, content, result, true),
		"context-dependent targets are never safe for a global permanent hash")
}

func TestContentModerationGuard_PolicyVersionCoversDecisionAndInputConfiguration(t *testing.T) {
	base := defaultContentModerationConfig()
	base.AuditProvider = ContentModerationProviderAIChat
	base.AIChat.IncrementalAuditEnabled = true
	base.normalize()
	baseVersion := contentModerationAuditPolicyVersion(base)
	require.Contains(t, baseVersion, "v6-")

	tests := []struct {
		name   string
		mutate func(*ContentModerationConfig)
	}{
		{"confidence threshold", func(cfg *ContentModerationConfig) { cfg.AIChat.ConfidenceThreshold -= 0.01 }},
		{"observe threshold", func(cfg *ContentModerationConfig) { cfg.AIChat.ObserveThreshold += 0.01 }},
		{"incremental switch", func(cfg *ContentModerationConfig) { cfg.AIChat.IncrementalAuditEnabled = false }},
		{"fast input window", func(cfg *ContentModerationConfig) { cfg.AIChat.FastInputChars += 100 }},
		{"summary window", func(cfg *ContentModerationConfig) { cfg.AIChat.SummaryMaxChars += 100 }},
		{"full input window", func(cfg *ContentModerationConfig) { cfg.AIChat.FullReviewMaxInputChars += 100 }},
		{"context ttl", func(cfg *ContentModerationConfig) { cfg.AIChat.AuditContextTTLMinutes++ }},
		{"session risk ttl", func(cfg *ContentModerationConfig) { cfg.AIChat.SessionRiskTTLMinutes++ }},
		{"session cooldown", func(cfg *ContentModerationConfig) { cfg.AIChat.SessionRiskBlockCooldownMinutes++ }},
		{"recent user turns", func(cfg *ContentModerationConfig) { cfg.AIChat.RecentUserTurns++ }},
		{"failure policy", func(cfg *ContentModerationConfig) { cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyBlock }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := cloneContentModerationConfig(base)
			test.mutate(changed)
			require.NotEqual(t, baseVersion, contentModerationAuditPolicyVersion(changed))
		})
	}

	thresholdBase := cloneContentModerationConfig(base)
	thresholdBase.AuditProvider = ContentModerationProviderOpenAIModerations
	thresholdBase.Thresholds = map[string]float64{"violence": 0.8, "hate": 0.7}
	thresholdVersion := contentModerationAuditPolicyVersion(thresholdBase)
	reordered := cloneContentModerationConfig(thresholdBase)
	reordered.Thresholds = map[string]float64{"hate": 0.7, "violence": 0.8}
	require.Equal(t, thresholdVersion, contentModerationAuditPolicyVersion(reordered), "map iteration order must not change the policy version")
	reordered.Thresholds["hate"] = 0.71
	require.NotEqual(t, thresholdVersion, contentModerationAuditPolicyVersion(reordered), "category threshold changes must invalidate policy state")
}

func TestContentModerationGuard_PrepareIncrementalAuditDropsStalePolicyState(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.AuditProvider = ContentModerationProviderAIChat
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.normalize()
	cache := newContentModerationGuardCache()
	svc, _ := newContentModerationGuardService(t, cfg, nil, cache)
	input := ContentModerationCheckInput{UserID: 91, APIKeyID: 92, SessionID: "policy-reset"}
	stateKey, _, _ := contentModerationRiskIdentity(input)
	cache.auditStates[stateKey] = voteaiauditcontext.State{
		Version: 1, PolicyVersion: "stale-policy", TurnCount: 42, CurrentScore: 0.91,
		MaxScore: 0.99, Tier: voteaiauditcontext.TierHigh, Categories: []string{"cyber_abuse"},
		Signals: []string{"auth_bypass"}, PrefixEpoch: 5, CanonicalPrefixHash: "stale-prefix",
	}

	plan, err := svc.prepareIncrementalAudit(context.Background(), input, cfg, contentModerationGuardInput("正常请求"))

	require.NoError(t, err)
	require.Equal(t, contentModerationAuditPolicyVersion(cfg), plan.policyVersion)
	require.Zero(t, plan.state.TurnCount)
	require.Zero(t, plan.state.CurrentScore)
	require.Empty(t, plan.state.Categories)
	require.Empty(t, plan.state.Signals)
	require.Empty(t, plan.state.CanonicalPrefixHash)
}

func TestContentModerationGuard_SupplementalReviewFailureIsTerminal(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			writeContentModerationGuardResult(w, false, 0.10, nil, nil, "fast pass")
			return
		}
		http.Error(w, "full review unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyAllow
	cfg.AIChat.supplementalReview = true
	cache := newContentModerationGuardCache()
	svc, repo := newContentModerationGuardService(t, cfg, server, cache)
	content := contentModerationGuardInput("请解释官方账号恢复流程")
	delay := 5

	decision := svc.checkSync(context.Background(), ContentModerationCheckInput{
		UserID: 93, APIKeyID: 94, SessionID: "supplemental-terminal", RequestID: "req-supplemental-terminal",
	}, cfg, content, content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg)), &delay, false)

	require.True(t, decision.Allowed)
	require.Equal(t, ContentModerationActionError, decision.Action)
	require.Empty(t, svc.asyncQueue, "a supplemental task must never enqueue another supplemental task")
	logs := repo.snapshotLogs()
	require.Len(t, logs, 1)
	require.Contains(t, logs[0].Error, "supplemental_review_final_failure")
	require.Empty(t, cache.snapshotRecorded())
}
