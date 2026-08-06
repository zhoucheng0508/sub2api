package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newContentModerationAsyncRuntimeTestService(
	t *testing.T,
	cfg *ContentModerationConfig,
	server *httptest.Server,
	cache ContentModerationHashCache,
) (*ContentModerationService, *contentModerationTestRepo, *contentModerationTestSettingRepo) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	settings := &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: string(raw),
	}}
	repo := &contentModerationTestRepo{}
	// Construct without a repository so background workers cannot consume the
	// task before this test explicitly changes the live runtime configuration.
	svc := NewContentModerationService(settings, nil, cache, nil, nil, nil, nil, nil)
	svc.repo = repo
	if server != nil {
		svc.httpClient = server.Client()
	}
	return svc, repo, settings
}

func setContentModerationAsyncRuntimeTestConfig(t *testing.T, settings *contentModerationTestSettingRepo, cfg *ContentModerationConfig) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	settings.values[SettingKeyContentModerationConfig] = string(raw)
}

type contentModerationEpochReadErrorCache struct {
	*contentModerationGuardClaimCache
	err error
}

func (c *contentModerationEpochReadErrorCache) GetContentModerationUserEpoch(context.Context, int64) (int64, error) {
	return -1, c.err
}

func TestContentModerationProcessAsyncTask_FreshRuntimeShutdownSkipsProviderAndSideEffects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ContentModerationConfig, *contentModerationTestSettingRepo)
	}{
		{
			name: "global risk control disabled",
			mutate: func(_ *ContentModerationConfig, settings *contentModerationTestSettingRepo) {
				settings.values[SettingKeyRiskControlEnabled] = "false"
			},
		},
		{
			name: "moderation disabled",
			mutate: func(cfg *ContentModerationConfig, _ *contentModerationTestSettingRepo) {
				cfg.Enabled = false
			},
		},
		{
			name: "moderation mode off",
			mutate: func(cfg *ContentModerationConfig, _ *contentModerationTestSettingRepo) {
				cfg.Mode = ContentModerationModeOff
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				writeContentModerationGuardResult(w, true, 0.95, []string{"credential_theft"}, []string{"credential_access"}, "blocked")
			}))
			defer server.Close()

			cfg := contentModerationGuardConfig(server.URL)
			cfg.Mode = ContentModerationModeObserve
			cfg.AIChat.IncrementalAuditEnabled = false
			cfg.AIChat.DeterministicRiskV2Enabled = false
			cfg.AIChat.CacheEnabled = false
			cache := newContentModerationGuardCache()
			svc, repo, settings := newContentModerationAsyncRuntimeTestService(t, cfg, server, cache)
			content := contentModerationGuardInput("queued before shutdown")
			require.True(t, svc.enqueueAsync(ContentModerationCheckInput{UserID: 71, Model: "deepseek-chat"}, cfg, content, content.Hash(), false))
			task := <-svc.asyncQueue

			liveCfg := cloneContentModerationConfig(cfg)
			tt.mutate(liveCfg, settings)
			setContentModerationAsyncRuntimeTestConfig(t, settings, liveCfg)
			svc.processAsyncTask(context.Background(), cfg, 0, task)

			require.Zero(t, requests)
			require.Empty(t, repo.snapshotLogs())
			require.Empty(t, cache.snapshotRecorded())
			_, _, sessionUpdates := cache.updateCounts()
			require.Zero(t, sessionUpdates)
		})
	}
}

func TestContentModerationProcessAsyncTask_UsesOnlyFreshRuntimeCredentials(t *testing.T) {
	var mu sync.Mutex
	var authorizations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		mu.Unlock()
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "allowed")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.Mode = ContentModerationModeObserve
	cfg.RecordNonHits = true
	cfg.AIChat.APIKeys = []string{"old-runtime-key"}
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.DeterministicRiskV2Enabled = false
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.ReasoningEffort = "high"
	cfg.normalize()
	cache := newContentModerationGuardCache()
	svc, _, settings := newContentModerationAsyncRuntimeTestService(t, cfg, server, cache)
	content := contentModerationGuardInput("credential rotation request")
	require.True(t, svc.enqueueAsync(ContentModerationCheckInput{UserID: 72, Model: "deepseek-chat"}, cfg, content, content.Hash(), false))
	task := <-svc.asyncQueue

	liveCfg := cloneContentModerationConfig(cfg)
	liveCfg.AIChat.APIKeys = []string{"new-runtime-key"}
	setContentModerationAsyncRuntimeTestConfig(t, settings, liveCfg)
	svc.processAsyncTask(context.Background(), cfg, 0, task)

	mu.Lock()
	got := append([]string(nil), authorizations...)
	mu.Unlock()
	require.Equal(t, []string{"Bearer new-runtime-key"}, got)
	for _, authorization := range got {
		require.NotContains(t, authorization, "old-runtime-key")
	}
}

func TestContentModerationProcessAsyncTask_DropsStaleObserveTaskBeforeProvider(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "allowed")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.Mode = ContentModerationModeObserve
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.CacheEnabled = false
	cache := newContentModerationGuardCache()
	svc, repo, _ := newContentModerationAsyncRuntimeTestService(t, cfg, server, cache)
	content := contentModerationGuardInput("stale queued request")
	require.True(t, svc.enqueueAsync(ContentModerationCheckInput{
		RequestID: "stale-observe", UserID: 75, APIKeyID: 85, Model: "deepseek-chat",
	}, cfg, content, content.Hash(), false))
	task := <-svc.asyncQueue
	maxQueueWait := contentModerationObserveMaxQueueWait(cfg)
	require.Positive(t, maxQueueWait)
	task.enqueuedAt = time.Now().Add(-maxQueueWait - time.Second)

	svc.processAsyncTask(context.Background(), cfg, 0, task)

	require.Zero(t, requests)
	require.Empty(t, repo.snapshotLogs())
	require.EqualValues(t, 1, svc.asyncDropped.Load())
	require.EqualValues(t, 1, svc.asyncProcessed.Load())
}

func TestContentModerationRequestVerdictQueuedMarkerHasShortRecoveryLease(t *testing.T) {
	cache := &contentModerationTestHashCache{}
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	const key = "queued-recovery-lease"

	require.True(t, svc.setContentModerationRequestVerdictQueued(context.Background(), key))
	queued, err := svc.contentModerationRequestVerdictQueued(context.Background(), key)
	require.NoError(t, err)
	require.True(t, queued)
	cache.mu.Lock()
	require.Equal(t, contentModerationRequestVerdictQueuedTTL, cache.resultTTLs[key])
	var entry contentModerationRequestVerdictEntry
	require.NoError(t, json.Unmarshal(cache.results[key], &entry))
	entry.QueuedAtUnixMilli = time.Now().Add(-contentModerationRequestVerdictQueuedLease - time.Second).UnixMilli()
	raw, err := json.Marshal(entry)
	require.NoError(t, err)
	cache.results[key] = raw
	cache.mu.Unlock()

	require.Less(t, contentModerationRequestVerdictQueuedLease, contentModerationRequestVerdictQueuedTTL)
	queued, err = svc.contentModerationRequestVerdictQueued(context.Background(), key)
	require.NoError(t, err)
	require.False(t, queued,
		"an orphaned queued marker must stop suppressing retries before its storage TTL expires")

	cache.mu.Lock()
	entry.QueuedAtUnixMilli = time.Now().Add(contentModerationRequestVerdictClockSkew - time.Second).UnixMilli()
	raw, err = json.Marshal(entry)
	require.NoError(t, err)
	cache.results[key] = raw
	cache.mu.Unlock()
	queued, err = svc.contentModerationRequestVerdictQueued(context.Background(), key)
	require.NoError(t, err)
	require.True(t, queued, "normal cross-instance clock skew must keep a fresh marker active")

	cache.mu.Lock()
	entry.QueuedAtUnixMilli = time.Now().Add(contentModerationRequestVerdictClockSkew + time.Second).UnixMilli()
	raw, err = json.Marshal(entry)
	require.NoError(t, err)
	cache.results[key] = raw
	cache.mu.Unlock()
	queued, err = svc.contentModerationRequestVerdictQueued(context.Background(), key)
	require.NoError(t, err)
	require.False(t, queued, "a far-future timestamp must be recoverable as a damaged marker")
}

func TestContentModerationExpiredQueueLeaseDuplicatesConvergeOnOneProviderAndSideEffectRun(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeContentModerationGuardResult(w, true, 0.91, []string{"credential_theft"}, []string{"credential_access"}, "high risk")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.Mode = ContentModerationModeObserve
	cfg.RecordNonHits = true
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.RiskLevelsEnabled = true
	cfg.AIChat.SessionRiskEnabled = true
	cfg.AIChat.ActorRiskEnabled = false
	cache := newContentModerationGuardClaimCache()
	svc, repo, _ := newContentModerationAsyncRuntimeTestService(t, cfg, server, cache)
	content := contentModerationGuardInput("duplicate queue convergence")
	input := ContentModerationCheckInput{
		RequestID: "duplicate-queue", UserID: 79, APIKeyID: 89,
		SessionID: "duplicate-queue", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpochSet: true, Model: "deepseek-chat",
	}
	targetHash := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))
	key := contentModerationRequestVerdictCacheKey(input, cfg, content, targetHash, "async_observe")
	require.NotEmpty(t, key)

	require.True(t, svc.enqueueContentModerationObserveIdempotent(context.Background(), input, cfg, content, targetHash, key).Allowed)
	cache.mu.Lock()
	var entry contentModerationRequestVerdictEntry
	require.NoError(t, json.Unmarshal(cache.results[key], &entry))
	entry.QueuedAtUnixMilli = time.Now().Add(-contentModerationRequestVerdictQueuedLease - time.Second).UnixMilli()
	raw, err := json.Marshal(entry)
	require.NoError(t, err)
	cache.results[key] = raw
	cache.mu.Unlock()
	require.True(t, svc.enqueueContentModerationObserveIdempotent(context.Background(), input, cfg, content, targetHash, key).Allowed)
	require.Len(t, svc.asyncQueue, 2)

	first := <-svc.asyncQueue
	second := <-svc.asyncQueue
	svc.processAsyncTask(context.Background(), cfg, 0, first)
	svc.processAsyncTask(context.Background(), cfg, 0, second)

	require.Equal(t, 1, requests)
	require.Len(t, repo.snapshotLogs(), 1)
	_, _, sessionUpdates := cache.updateCounts()
	require.Equal(t, 1, sessionUpdates)
}

func TestContentModerationRequestVerdictReadFailureDoesNotExecuteEvaluator(t *testing.T) {
	for _, failurePolicy := range []string{ContentModerationFailurePolicyAllow, ContentModerationFailurePolicyBlock} {
		t.Run(failurePolicy, func(t *testing.T) {
			cache := newContentModerationGuardClaimCache()
			cache.resultGetErr = errors.New("redis read failed")
			svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
			cfg := contentModerationGuardConfig("https://audit.invalid")
			cfg.AIChat.FailurePolicy = failurePolicy
			fallback := contentModerationRequestVerdictWaitFallback(cfg, "preblock_sync")
			calls := 0

			decision := svc.evaluateContentModerationRequestVerdictIdempotentWithFallback(
				context.Background(),
				ContentModerationCheckInput{RequestID: "read-failure", UserID: 76, APIKeyID: 86},
				cfg, "read-failure-key", "preblock_sync", fallback,
				func(context.Context) *ContentModerationDecision {
					calls++
					return &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow, requestVerdictCacheable: true}
				},
			)

			require.NotNil(t, decision)
			require.Zero(t, calls)
			if failurePolicy == ContentModerationFailurePolicyBlock {
				require.Equal(t, ContentModerationActionUnavailable, decision.Action)
			} else {
				require.True(t, decision.Allowed)
				require.Equal(t, ContentModerationActionAllow, decision.Action)
			}
			cache.claimMu.Lock()
			require.Empty(t, cache.claims)
			cache.claimMu.Unlock()
		})
	}
}

func TestContentModerationObserveQueueWriteFailureDoesNotEnqueue(t *testing.T) {
	cache := newContentModerationGuardClaimCache()
	cache.resultSetErr = errors.New("redis write failed")
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	svc.asyncQueue = make(chan contentModerationTask, 1)
	cfg := contentModerationGuardConfig("https://audit.invalid")
	cfg.Mode = ContentModerationModeObserve
	content := contentModerationGuardInput("queue write failure")
	input := ContentModerationCheckInput{
		RequestID: "queue-write-failure", UserID: 77, APIKeyID: 87,
		SessionID: "queue-write-failure", ModerationEpochSet: true,
	}
	targetHash := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))
	key := contentModerationRequestVerdictCacheKey(input, cfg, content, targetHash, "async_observe")

	decision := svc.enqueueContentModerationObserveIdempotent(context.Background(), input, cfg, content, targetHash, key)

	require.NotNil(t, decision)
	require.True(t, decision.Allowed)
	require.Empty(t, svc.asyncQueue)
	require.Zero(t, svc.asyncEnqueued.Load())
	cache.claimMu.Lock()
	require.Empty(t, cache.claims)
	cache.claimMu.Unlock()
}

func TestContentModerationTerminalVerdictWriteFailureRetainsClaimLease(t *testing.T) {
	cache := newContentModerationGuardClaimCache()
	cache.resultSetErr = errors.New("redis write failed")
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	cfg := contentModerationGuardConfig("https://audit.invalid")
	cfg.AIChat.FailurePolicy = ContentModerationFailurePolicyAllow
	input := ContentModerationCheckInput{RequestID: "write-failure", UserID: 78, APIKeyID: 88}
	const key = "terminal-write-failure"
	calls := 0
	evaluate := func(context.Context) *ContentModerationDecision {
		calls++
		return &ContentModerationDecision{
			Allowed: false, Blocked: true, Action: ContentModerationActionBlock,
			requestVerdictCacheable: true,
		}
	}

	first := svc.evaluateContentModerationRequestVerdictIdempotentWithFallback(
		context.Background(), input, cfg, key, "preblock_sync",
		&ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow}, evaluate,
	)
	require.True(t, first.Blocked)
	require.Equal(t, 1, calls)
	cache.claimMu.Lock()
	require.Contains(t, cache.claims, key)
	cache.claimMu.Unlock()

	retryCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	second := svc.evaluateContentModerationRequestVerdictIdempotentWithFallback(
		retryCtx, input, cfg, key, "preblock_sync",
		&ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow}, evaluate,
	)
	require.True(t, second.Allowed)
	require.Equal(t, 1, calls, "an immediate retry must not repeat side effects while the uncertain claim is leased")

	cache.claimMu.Lock()
	delete(cache.claims, key) // Simulate lease expiry.
	cache.claimMu.Unlock()
	cache.resultSetErr = nil
	third := svc.evaluateContentModerationRequestVerdictIdempotentWithFallback(
		context.Background(), input, cfg, key, "preblock_sync",
		&ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow}, evaluate,
	)
	require.True(t, third.Blocked)
	require.Equal(t, 2, calls)
}

func TestContentModerationProcessAsyncTask_ChangedGenerationPreservesPolicyAndDisablesSideEffects(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		mu.Lock()
		for _, message := range payload.Messages {
			if message.Role == "system" {
				prompts = append(prompts, message.Content)
			}
		}
		mu.Unlock()
		result, err := json.Marshal(map[string]any{
			"flagged":    true,
			"risk_score": 0.95,
			"categories": []string{"credential_theft"},
			"signals":    []string{"credential_access"},
			"reason":     "high risk",
		})
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"role": "assistant", "content": string(result),
			}}},
			"usage": map[string]any{
				"prompt_tokens": 10, "prompt_cache_hit_tokens": 4,
				"prompt_cache_miss_tokens": 6, "completion_tokens": 2,
				"total_tokens": 12,
			},
		}))
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.Mode = ContentModerationModeObserve
	cfg.RecordNonHits = true
	cfg.EmailOnHit = true
	cfg.AutoBanEnabled = true
	cfg.BanThreshold = 1
	cfg.ViolationWindowHours = 0
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.DeterministicRiskV2Enabled = false
	cfg.AIChat.RiskLevelsEnabled = true
	cfg.AIChat.CacheEnabled = false
	cfg.AIChat.ReasoningEffort = "high"
	cfg.AIChat.SystemPrompt = "queued-policy-v1"
	cfg.AIChat.PricingVersion = "queued-rates-v1"
	cfg.AIChat.UncachedInputUSDPerMillionTokens = asyncRuntimeFloat64Ptr(1)
	cfg.AIChat.CachedInputUSDPerMillionTokens = asyncRuntimeFloat64Ptr(0.1)
	cfg.AIChat.OutputUSDPerMillionTokens = asyncRuntimeFloat64Ptr(2)
	cfg.normalize()
	baseCache := &contentModerationTestHashCache{}
	dedupe := &contentModerationEmailDedupeTestStore{}
	cache := &contentModerationEmailIntegrationCache{
		contentModerationTestHashCache:        baseCache,
		contentModerationEmailDedupeTestStore: dedupe,
	}
	svc, repo, settings := newContentModerationAsyncRuntimeTestService(t, cfg, server, cache)
	svc.emailService = &EmailService{}
	repo.banOutcome = ContentModerationBanOutcomeApplied
	content := contentModerationGuardInput("high risk queued under policy v1")
	input := ContentModerationCheckInput{
		UserID: 73, UserEmail: "queued@example.com", APIKeyID: 83,
		SessionID: "changed-generation", Model: "deepseek-chat",
	}
	require.True(t, svc.enqueueAsync(input, cfg, content, content.Hash(), false))
	task := <-svc.asyncQueue

	liveCfg := cloneContentModerationConfig(cfg)
	liveCfg.AIChat.SystemPrompt = "live-policy-v2"
	liveCfg.AIChat.PricingVersion = "live-rates-v2"
	liveCfg.AIChat.UncachedInputUSDPerMillionTokens = asyncRuntimeFloat64Ptr(10)
	liveCfg.AIChat.CachedInputUSDPerMillionTokens = asyncRuntimeFloat64Ptr(1)
	liveCfg.AIChat.OutputUSDPerMillionTokens = asyncRuntimeFloat64Ptr(20)
	require.NotEqual(t, task.configGeneration, svc.contentModerationAsyncTaskGeneration(liveCfg))
	setContentModerationAsyncRuntimeTestConfig(t, settings, liveCfg)
	svc.processAsyncTask(context.Background(), cfg, 0, task)

	mu.Lock()
	gotPrompts := append([]string(nil), prompts...)
	mu.Unlock()
	require.Equal(t, []string{"queued-policy-v1"}, gotPrompts)
	cost := svc.contentModerationAuditCostSnapshot()
	require.Contains(t, cost.byVersionUSD, "queued-rates-v1")
	require.NotContains(t, cost.byVersionUSD, "live-rates-v2")
	require.Empty(t, baseCache.snapshotRecorded())
	require.Empty(t, baseCache.sessionStates)
	require.Nil(t, repo.moderationState)
	require.Empty(t, dedupe.snapshotCalls())
	logs := repo.snapshotLogs()
	require.Len(t, logs, 1)
	require.False(t, logs[0].EmailSent)
	require.Equal(t, ContentModerationNotificationStatusNotRequired, logs[0].NotificationStatus)
	require.Equal(t, "configuration_generation_changed", logs[0].AuditDetails.HashPromotionReason)
}

func TestContentModerationRequestVerdict_DoChanEvaluatorPanicReturnsUncachedFallbackAndHealthyRetryCaches(t *testing.T) {
	cache := newContentModerationGuardClaimCache()
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	cfg := contentModerationGuardConfig("https://audit.invalid")
	fallback := &ContentModerationDecision{
		Allowed: true, Action: ContentModerationActionAllow, requestVerdictCacheable: true,
	}
	input := ContentModerationCheckInput{RequestID: "panic-retry", UserID: 74, APIKeyID: 84}
	const key = "request-verdict-panic-retry"
	calls := 0
	evaluate := func(context.Context) *ContentModerationDecision {
		calls++
		if calls == 1 {
			panic("test evaluator panic")
		}
		return &ContentModerationDecision{
			Allowed: false, Blocked: true, Flagged: true,
			Action: ContentModerationActionBlock, requestVerdictCacheable: true,
		}
	}

	first := svc.evaluateContentModerationRequestVerdictIdempotentWithFallback(
		context.Background(), input, cfg, key, "preblock_sync", fallback, evaluate,
	)
	require.NotNil(t, first)
	require.True(t, first.Allowed)
	require.False(t, first.requestVerdictCacheable)
	_, hit, cacheErr := svc.getContentModerationRequestVerdict(context.Background(), key)
	require.NoError(t, cacheErr)
	require.False(t, hit, "panic fallback must not enter the request-verdict cache")
	cache.claimMu.Lock()
	require.Contains(t, cache.claims, key, "uncertain panic completion must retain the claim until its lease expires")
	delete(cache.claims, key) // Simulate Redis lease expiry before a healthy retry.
	cache.claimMu.Unlock()

	second := svc.evaluateContentModerationRequestVerdictIdempotentWithFallback(
		context.Background(), input, cfg, key, "preblock_sync", fallback, evaluate,
	)
	require.NotNil(t, second)
	require.True(t, second.Blocked)
	cached, hit, cacheErr := svc.getContentModerationRequestVerdict(context.Background(), key)
	require.NoError(t, cacheErr)
	require.True(t, hit)
	require.True(t, cached.Blocked)

	third := svc.evaluateContentModerationRequestVerdictIdempotentWithFallback(
		context.Background(), input, cfg, key, "preblock_sync", fallback, evaluate,
	)
	require.True(t, third.Blocked)
	require.Equal(t, 2, calls, "the healthy decision must satisfy later retries from cache")
}

func TestContentModerationInvalidEpochWithRequestIDReturnsFallbackWithoutEvaluation(t *testing.T) {
	for _, failurePolicy := range []string{ContentModerationFailurePolicyAllow, ContentModerationFailurePolicyBlock} {
		for _, epochCase := range []string{"read_error", "negative"} {
			t.Run(failurePolicy+"/"+epochCase, func(t *testing.T) {
				requests := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requests++
					writeContentModerationGuardResult(w, true, 0.99, []string{"credential_theft"}, []string{"auth_bypass"}, "must not run")
				}))
				defer server.Close()

				cfg := contentModerationGuardConfig(server.URL)
				cfg.AIChat.FailurePolicy = failurePolicy
				cache := &contentModerationEpochReadErrorCache{
					contentModerationGuardClaimCache: newContentModerationGuardClaimCache(),
					err:                              errors.New("redis epoch read failed"),
				}
				svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
				svc.httpClient = server.Client()
				input := ContentModerationCheckInput{
					RequestID: "invalid-epoch", UserID: 501, APIKeyID: 601,
					SessionID: "invalid-epoch", Model: "deepseek-chat",
				}
				if epochCase == "read_error" {
					svc.captureContentModerationEpoch(context.Background(), &input)
					require.True(t, input.ModerationEpochSet)
					require.EqualValues(t, -1, input.ModerationEpoch)
				} else {
					input.ModerationEpochSet = true
					input.ModerationEpoch = -7
				}
				content := contentModerationGuardInput("invalid epoch must not reach the provider")
				targetHash := content.AuditTargetHash(contentModerationAuditPolicyVersion(cfg))
				key := contentModerationRequestVerdictCacheKey(input, cfg, content, targetHash, "preblock_sync")
				require.Empty(t, key)

				decision := svc.checkSyncIdempotent(context.Background(), input, cfg, content, targetHash, nil, true)

				require.NotNil(t, decision)
				require.Zero(t, requests)
				if failurePolicy == ContentModerationFailurePolicyBlock {
					require.Equal(t, ContentModerationActionUnavailable, decision.Action)
				} else {
					require.True(t, decision.Allowed)
					require.Equal(t, ContentModerationActionAllow, decision.Action)
				}
			})
		}
	}
}

func TestContentModerationInvalidEpochDoesNotRunLocalEvaluatorOrObserveEnqueue(t *testing.T) {
	cfg := contentModerationGuardConfig("https://audit.invalid")
	cache := newContentModerationGuardClaimCache()
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	input := ContentModerationCheckInput{
		RequestID: "invalid-epoch-local", UserID: 502, APIKeyID: 602,
		ModerationEpochSet: true, ModerationEpoch: -1,
	}
	fallback := &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow}
	calls := 0

	decision := svc.localContentModerationVerdictIdempotent(
		context.Background(), input, cfg, "", fallback,
		func(context.Context) *ContentModerationDecision {
			calls++
			return &ContentModerationDecision{Blocked: true, Action: ContentModerationActionKeywordBlock}
		},
	)
	require.NotNil(t, decision)
	require.True(t, decision.Allowed)
	require.Zero(t, calls)

	cfg.Mode = ContentModerationModeObserve
	content := contentModerationGuardInput("invalid epoch observe")
	decision = svc.enqueueContentModerationObserveIdempotent(
		context.Background(), input, cfg, content, content.Hash(), "",
	)
	require.NotNil(t, decision)
	require.True(t, decision.Allowed)
	require.Empty(t, svc.asyncQueue)
}

func TestContentModerationRequestVerdictMissingClaimCapabilityReturnsFallback(t *testing.T) {
	cache := &contentModerationTestHashCache{}
	svc := NewContentModerationService(nil, nil, cache, nil, nil, nil, nil, nil)
	cfg := contentModerationGuardConfig("https://audit.invalid")
	fallback := &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow}
	calls := 0

	decision := svc.evaluateContentModerationRequestVerdictIdempotentWithFallback(
		context.Background(),
		ContentModerationCheckInput{RequestID: "missing-claim", UserID: 503, APIKeyID: 603},
		cfg, "missing-claim-key", "preblock_sync", fallback,
		func(context.Context) *ContentModerationDecision {
			calls++
			return &ContentModerationDecision{Blocked: true, Action: ContentModerationActionBlock, requestVerdictCacheable: true}
		},
	)

	require.NotNil(t, decision)
	require.True(t, decision.Allowed)
	require.Zero(t, calls)
}

func TestContentModerationProcessAsyncTask_FingerprintPolicyChangeSuppressesIncrementalContext(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "allowed")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.Mode = ContentModerationModeObserve
	cfg.RecordNonHits = true
	cfg.AIChat.IncrementalAuditEnabled = true
	cfg.AIChat.InputProvenanceV2Enabled = true
	cfg.AIChat.DeterministicRiskV2Enabled = false
	cfg.AIChat.RiskLevelsEnabled = false
	cfg.AIChat.CacheEnabled = false
	cache := newContentModerationGuardClaimCache()
	svc, repo, settings := newContentModerationAsyncRuntimeTestService(t, cfg, server, cache)
	settings.values[SettingKeyCodexCLIOnlyEngineFingerprintSignals] = `[{"type":"header_prefix","match":["x-old-engine-"],"required":true}]`
	queuedSnapshot, err := svc.loadFreshRuntimeSnapshot(context.Background())
	require.NoError(t, err)

	content := contentModerationGuardInput("fingerprint generation change")
	input := ContentModerationCheckInput{
		RequestID: "fingerprint-generation", UserID: 504, APIKeyID: 604,
		SessionID: "fingerprint-generation", SessionSource: ContentModerationSessionSourceHeader,
		ModerationEpochSet: true, Model: "deepseek-chat",
	}
	require.True(t, svc.enqueueAsync(input, cfg, content, content.Hash(), false))
	task := <-svc.asyncQueue
	require.Equal(t, queuedSnapshot.runtimeGeneration, task.configGeneration)

	settings.values[SettingKeyCodexCLIOnlyEngineFingerprintSignals] = `[{"type":"header_prefix","match":["x-new-engine-"],"required":true}]`
	svc.processAsyncTask(context.Background(), cfg, 0, task)

	require.Equal(t, 1, requests)
	liveSnapshot := svc.runtimeSnapshot.Load()
	require.NotNil(t, liveSnapshot)
	require.NotEqual(t, task.configGeneration, liveSnapshot.runtimeGeneration)
	auditUpdates, prefixUpdates, sessionUpdates := cache.updateCounts()
	require.Zero(t, auditUpdates)
	require.Zero(t, prefixUpdates)
	require.Zero(t, sessionUpdates)
	logs := repo.snapshotLogs()
	require.Len(t, logs, 1)
	require.Equal(t, "configuration_generation_changed", logs[0].AuditDetails.HashPromotionReason)
}

func TestContentModerationProcessAsyncTask_RevalidatesFreshUserAndAccountScope(t *testing.T) {
	tests := []struct {
		name   string
		input  ContentModerationCheckInput
		mutate func(*ContentModerationConfig)
	}{
		{
			name:  "user excluded",
			input: ContentModerationCheckInput{UserID: 505, AccountID: 605, Model: "deepseek-chat"},
			mutate: func(cfg *ContentModerationConfig) {
				cfg.UserFilter = ContentModerationUserFilter{Type: ContentModerationScopeFilterExclude, UserIDs: []int64{505}}
			},
		},
		{
			name:  "account excluded",
			input: ContentModerationCheckInput{UserID: 506, AccountID: 606, Model: "deepseek-chat"},
			mutate: func(cfg *ContentModerationConfig) {
				cfg.AccountFilter = ContentModerationAccountFilter{Type: ContentModerationScopeFilterExclude, AccountIDs: []int64{606}}
			},
		},
		{
			name:  "restricted account scope lacks identity",
			input: ContentModerationCheckInput{UserID: 507, Model: "deepseek-chat"},
			mutate: func(cfg *ContentModerationConfig) {
				cfg.AccountFilter = ContentModerationAccountFilter{Type: ContentModerationScopeFilterInclude, AccountIDs: []int64{607}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				writeContentModerationGuardResult(w, false, 0.05, nil, nil, "must not run")
			}))
			defer server.Close()

			cfg := contentModerationGuardConfig(server.URL)
			cfg.Mode = ContentModerationModeObserve
			cfg.AIChat.IncrementalAuditEnabled = false
			cfg.AIChat.CacheEnabled = false
			svc, repo, settings := newContentModerationAsyncRuntimeTestService(t, cfg, server, newContentModerationGuardCache())
			content := contentModerationGuardInput("fresh scope revalidation")
			require.True(t, svc.enqueueAsync(tt.input, cfg, content, content.Hash(), false))
			task := <-svc.asyncQueue

			liveCfg := cloneContentModerationConfig(cfg)
			tt.mutate(liveCfg)
			setContentModerationAsyncRuntimeTestConfig(t, settings, liveCfg)
			svc.processAsyncTask(context.Background(), cfg, 0, task)

			require.Zero(t, requests)
			require.Empty(t, repo.snapshotLogs())
		})
	}
}

func TestContentModerationProcessAsyncTask_ResolvesShadowAccountScopeBeforeProvider(t *testing.T) {
	const (
		parentID int64 = 608
		shadowID int64 = 708
	)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeContentModerationGuardResult(w, false, 0.05, nil, nil, "allowed")
	}))
	defer server.Close()

	cfg := contentModerationGuardConfig(server.URL)
	cfg.Mode = ContentModerationModeObserve
	cfg.RecordNonHits = true
	cfg.AccountFilter = ContentModerationAccountFilter{Type: ContentModerationScopeFilterInclude, AccountIDs: []int64{parentID}}
	cfg.AIChat.IncrementalAuditEnabled = false
	cfg.AIChat.CacheEnabled = false
	svc, repo, _ := newContentModerationAsyncRuntimeTestService(t, cfg, server, newContentModerationGuardCache())
	svc.accountScopeRepo = &contentModerationScopeAccountRepo{accounts: map[int64]*Account{
		parentID: {ID: parentID},
		shadowID: {ID: shadowID, ParentAccountID: contentModerationScopeInt64Ptr(parentID)},
	}}
	content := contentModerationGuardInput("shadow account scope")
	require.True(t, svc.enqueueAsync(ContentModerationCheckInput{
		UserID: 508, AccountID: shadowID, Model: "deepseek-chat",
	}, cfg, content, content.Hash(), false))
	task := <-svc.asyncQueue

	svc.processAsyncTask(context.Background(), cfg, 0, task)

	require.Equal(t, 1, requests)
	require.Len(t, repo.snapshotLogs(), 1)
}

func TestContentModerationProcessAsyncRecord_MissingRestrictedAccountIdentityPersistsWithoutSideEffects(t *testing.T) {
	cfg := contentModerationGuardConfig("https://audit.invalid")
	cfg.Mode = ContentModerationModeObserve
	cfg.AccountFilter = ContentModerationAccountFilter{Type: ContentModerationScopeFilterInclude, AccountIDs: []int64{609}}
	cfg.AutoBanEnabled = true
	cfg.BanThreshold = 1
	cache := newContentModerationGuardCache()
	svc, repo, _ := newContentModerationAsyncRuntimeTestService(t, cfg, nil, cache)
	input := ContentModerationCheckInput{UserID: 509, Model: "deepseek-chat"}
	log := svc.buildLog(input, cfg, ContentModerationActionBlock, true, "credential_theft", 0.99,
		map[string]float64{"credential_theft": 0.99}, "blocked", nil, nil, "")
	svc.enqueueRecord(input, cfg, log, "missing-account-record-hash", true, true)
	task := <-svc.asyncQueue

	svc.processAsyncTask(context.Background(), cfg, 0, task)

	logs := repo.snapshotLogs()
	require.Len(t, logs, 1)
	require.Equal(t, "account_scope_identity_unavailable", logs[0].AuditDetails.HashPromotionReason)
	require.Empty(t, cache.snapshotRecorded())
	require.Nil(t, repo.moderationState)
}

func TestContentModerationUpdateConfig_NonAIChatLeavesIncrementalSettingsDormant(t *testing.T) {
	repo := &contentModerationTestSettingRepo{values: map[string]string{}}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil, nil)
	provider := ContentModerationProviderOpenAIModerations
	incrementalEnabled := true
	provenanceEnabled := false
	pricingVersion := "dormant-rates"
	uncachedRate := 1.25

	_, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
		AuditProvider:                      &provider,
		AIIncrementalAuditEnabled:          &incrementalEnabled,
		AIInputProvenanceV2Enabled:         &provenanceEnabled,
		AIPricingVersion:                   &pricingVersion,
		AIUncachedInputUSDPerMillionTokens: &uncachedRate,
	})
	require.NoError(t, err)

	var saved ContentModerationConfig
	require.NoError(t, json.Unmarshal([]byte(repo.values[SettingKeyContentModerationConfig]), &saved))
	require.Equal(t, ContentModerationProviderOpenAIModerations, saved.AuditProvider)
	require.False(t, saved.AIChat.InputProvenanceV2Enabled)
	require.Equal(t, pricingVersion, saved.AIChat.PricingVersion)
	require.NotNil(t, saved.AIChat.UncachedInputUSDPerMillionTokens)
	require.Equal(t, uncachedRate, *saved.AIChat.UncachedInputUSDPerMillionTokens)
	require.Nil(t, saved.AIChat.CachedInputUSDPerMillionTokens)
	require.Nil(t, saved.AIChat.OutputUSDPerMillionTokens)
}

func asyncRuntimeFloat64Ptr(value float64) *float64 {
	return &value
}
