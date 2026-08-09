package service

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"log/slog"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
	"golang.org/x/sync/singleflight"
)

const (
	contentModerationRequestVerdictVersion         = 2
	contentModerationRequestVerdictTTL             = 10 * time.Minute
	contentModerationRequestVerdictQueuedTTL       = contentModerationRequestVerdictTTL
	contentModerationRequestVerdictQueuedLease     = 60 * time.Second
	contentModerationRequestVerdictClockSkew       = 5 * time.Second
	contentModerationRequestVerdictRedisTimeout    = 75 * time.Millisecond
	contentModerationRequestVerdictPollInterval    = 40 * time.Millisecond
	contentModerationRequestVerdictSyncWaitGrace   = 500 * time.Millisecond
	contentModerationRequestVerdictMinWait         = time.Second
	contentModerationRequestVerdictMaxSyncWait     = 6 * time.Second
	contentModerationRequestVerdictMaxAsyncWait    = 30 * time.Second
	contentModerationRequestVerdictClaimLeaseGrace = 5 * time.Second
	contentModerationRequestVerdictMinimumClaimTTL = 10 * time.Second
)

// contentModerationRequestVerdictFlight prevents concurrent retries in this
// process from reaching the audit provider before the first verdict is saved.
type contentModerationRequestVerdictFlight struct {
	group singleflight.Group
}

// ContentModerationRequestVerdictClaimStore is an optional cross-instance
// lease. Implementations must make acquisition atomic and release only when
// owner still holds the claim.
type ContentModerationRequestVerdictClaimStore interface {
	TryClaimContentModerationRequestVerdict(ctx context.Context, key, owner string, ttl time.Duration) (bool, error)
	ReleaseContentModerationRequestVerdictClaim(ctx context.Context, key, owner string) error
}

type contentModerationRequestVerdictEntry struct {
	Version           int                       `json:"version"`
	State             string                    `json:"state,omitempty"`
	QueuedAtUnixMilli int64                     `json:"queued_at_unix_milli,omitempty"`
	ReviewComplete    bool                      `json:"review_complete"`
	Decision          ContentModerationDecision `json:"decision"`
}

const (
	contentModerationRequestVerdictStateQueued   = "queued"
	contentModerationRequestVerdictStateComplete = "complete"
)

func contentModerationRequestVerdictCacheKey(
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	content ContentModerationInput,
	targetHash string,
	executionMode string,
) string {
	requestID := strings.TrimSpace(input.RequestID)
	targetHash = strings.TrimSpace(targetHash)
	executionMode = strings.ToLower(strings.TrimSpace(executionMode))
	if cfg == nil || cfg.AuditProvider != ContentModerationProviderAIChat || requestID == "" || targetHash == "" || executionMode == "" {
		return ""
	}
	// A positive user must have a successfully captured epoch. Otherwise an
	// administrator unban could not fence a verdict written before the reset.
	if input.UserID > 0 && (!input.ModerationEpochSet || input.ModerationEpoch < 0) {
		return ""
	}
	sessionKey, actorKey, _ := contentModerationRiskIdentity(input)
	if actorKey == "" {
		return ""
	}
	conversationDigest := contentModerationRequestVerdictInputDigest(content)
	if conversationDigest == "" {
		return ""
	}
	groupID := int64(0)
	if input.GroupID != nil {
		groupID = *input.GroupID
	}
	return voteaimoderation.CacheKey(
		"request-verdict:v2",
		fmt.Sprintf("actor=%s", actorKey),
		fmt.Sprintf("session=%s", defaultContentModerationString(sessionKey, "none")),
		fmt.Sprintf("request=%s", requestID),
		fmt.Sprintf("policy=%s", contentModerationAuditPolicyVersion(cfg)),
		fmt.Sprintf("local_policy=%s", contentModerationRequestVerdictLocalPolicyFingerprint(cfg)),
		fmt.Sprintf("mode=%s", executionMode),
		fmt.Sprintf("target=%s", targetHash),
		fmt.Sprintf("conversation=%s", conversationDigest),
		fmt.Sprintf("epoch=%d", input.ModerationEpoch),
		fmt.Sprintf("session_source=%s", normalizeContentModerationSessionSource(input.SessionSource, input.SessionID)),
		fmt.Sprintf("group=%d", groupID),
		fmt.Sprintf("endpoint=%s", strings.TrimSpace(input.Endpoint)),
		fmt.Sprintf("protocol=%s", strings.TrimSpace(input.Protocol)),
		fmt.Sprintf("provider=%s", strings.TrimSpace(input.Provider)),
		fmt.Sprintf("model=%s", strings.TrimSpace(input.Model)),
	)
}

func contentModerationRequestVerdictEpochFallbackRequired(
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	key string,
) bool {
	return strings.TrimSpace(key) == "" &&
		strings.TrimSpace(input.RequestID) != "" &&
		cfg != nil &&
		cfg.AuditProvider == ContentModerationProviderAIChat &&
		input.UserID > 0 &&
		(!input.ModerationEpochSet || input.ModerationEpoch < 0)
}

func contentModerationRequestVerdictLocalPolicyFingerprint(cfg *ContentModerationConfig) string {
	if cfg == nil {
		return "unknown"
	}
	keywords := make([]string, 0, len(cfg.BlockedKeywords))
	for _, keyword := range cfg.BlockedKeywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" {
			keywords = append(keywords, keyword)
		}
	}
	sort.Strings(keywords)
	unique := keywords[:0]
	for _, keyword := range keywords {
		if len(unique) == 0 || unique[len(unique)-1] != keyword {
			unique = append(unique, keyword)
		}
	}
	payload := strings.Join([]string{
		"request-verdict-local-policy:v1",
		strings.TrimSpace(cfg.KeywordBlockingMode),
		strings.Join(unique, "\x00"),
		fmt.Sprintf("block_status=%d", cfg.BlockStatus),
		fmt.Sprintf("block_message=%s", strings.TrimSpace(cfg.BlockMessage)),
		fmt.Sprintf("api_key_count=%d", len(cfg.apiKeys())),
	}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// contentModerationRequestVerdictInputDigest fingerprints the complete,
// provenance-normalized input without persisting raw conversation content.
// AuditTargetHash is intentionally separate: the digest fences retries whose
// final target is unchanged but whose earlier conversation was rewritten.
func contentModerationRequestVerdictInputDigest(content ContentModerationInput) string {
	content.Normalize()
	h := sha256.New()
	writeContentModerationVerdictDigestField(h, "request-verdict-input:v1")
	writeContentModerationVerdictDigestField(h, strings.TrimSpace(content.AuditTargetKind))
	writeContentModerationVerdictDigestField(h, contentModerationVerdictNormalizedText(content.AuditTargetText))
	writeContentModerationVerdictDigestField(h, contentModerationVerdictNormalizedText(content.Text))
	writeContentModerationVerdictDigestField(h, contentModerationVerdictNormalizedText(content.CurrentText))
	writeContentModerationVerdictDigestBool(h, content.HasExplicitUser)
	writeContentModerationVerdictDigestBool(h, content.TrustedClient)
	for _, value := range normalizeContentModerationStringValues(content.TrustedSignals) {
		writeContentModerationVerdictDigestField(h, "trusted_signal:"+value)
	}
	for _, value := range normalizeContentModerationStringValues(content.IgnoredMetadata) {
		writeContentModerationVerdictDigestField(h, "ignored_metadata:"+value)
	}
	for _, image := range content.Images {
		imageDigest := sha256.Sum256([]byte(strings.TrimSpace(image)))
		writeContentModerationVerdictDigestField(h, "image:"+hex.EncodeToString(imageDigest[:]))
	}
	for _, turn := range content.Turns {
		writeContentModerationVerdictDigestField(h, "turn")
		writeContentModerationVerdictDigestField(h, strings.ToLower(strings.TrimSpace(turn.Role)))
		writeContentModerationVerdictDigestField(h, strings.ToLower(strings.TrimSpace(turn.Source)))
		writeContentModerationVerdictDigestField(h, strings.ToLower(strings.TrimSpace(turn.Purpose)))
		writeContentModerationVerdictDigestField(h, strings.ToLower(strings.TrimSpace(turn.MetadataKind)))
		writeContentModerationVerdictDigestField(h, contentModerationVerdictNormalizedText(turn.Text))
		writeContentModerationVerdictDigestBool(h, turn.ToolCall)
		writeContentModerationVerdictDigestBool(h, turn.Truncated)
		writeContentModerationVerdictDigestBool(h, turn.Current)
		writeContentModerationVerdictDigestBool(h, turn.LinkedToUserIntent)
		writeContentModerationVerdictDigestBool(h, turn.MetadataEnvelope)
		writeContentModerationVerdictDigestField(h, strings.ToLower(strings.TrimSpace(turn.MetadataHint)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func contentModerationVerdictNormalizedText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSpace(value)
}

func writeContentModerationVerdictDigestField(h hash.Hash, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = h.Write(size[:])
	_, _ = h.Write([]byte(value))
}

func writeContentModerationVerdictDigestBool(h hash.Hash, value bool) {
	if value {
		writeContentModerationVerdictDigestField(h, "1")
		return
	}
	writeContentModerationVerdictDigestField(h, "0")
}

func contentModerationRequestVerdictExecutionMode(cfg *ContentModerationConfig, queueDelay *int, allowBlock bool) string {
	if queueDelay != nil || (cfg != nil && cfg.Mode == ContentModerationModeObserve) {
		if cfg != nil && cfg.AIChat.supplementalReview {
			return "async_supplemental"
		}
		return "async_observe"
	}
	if cfg != nil && cfg.Mode == ContentModerationModePreBlock && allowBlock {
		return "preblock_sync"
	}
	return "sync_observe"
}

func (s *ContentModerationService) contentModerationRequestVerdictQueued(ctx context.Context, key string) (bool, error) {
	if s == nil || strings.TrimSpace(key) == "" {
		return false, nil
	}
	cache, ok := s.hashCache.(ContentModerationResultCache)
	if !ok {
		return false, nil
	}
	operationCtx, cancel := contentModerationRequestVerdictRedisContext(ctx)
	defer cancel()
	raw, hit, err := cache.GetContentModerationResult(operationCtx, key)
	if err != nil {
		return false, fmt.Errorf("get queued content moderation request verdict: %w", err)
	}
	if !hit {
		return false, nil
	}
	var entry contentModerationRequestVerdictEntry
	if json.Unmarshal(raw, &entry) != nil ||
		entry.Version != contentModerationRequestVerdictVersion ||
		entry.ReviewComplete ||
		entry.State != contentModerationRequestVerdictStateQueued ||
		entry.QueuedAtUnixMilli <= 0 {
		return false, nil
	}
	queuedAt := time.UnixMilli(entry.QueuedAtUnixMilli)
	age := time.Since(queuedAt)
	return age >= -contentModerationRequestVerdictClockSkew && age <= contentModerationRequestVerdictQueuedLease, nil
}

func (s *ContentModerationService) setContentModerationRequestVerdictQueued(ctx context.Context, key string) bool {
	if s == nil || strings.TrimSpace(key) == "" {
		return false
	}
	cache, ok := s.hashCache.(ContentModerationResultCache)
	if !ok {
		return false
	}
	raw, err := json.Marshal(contentModerationRequestVerdictEntry{
		Version:           contentModerationRequestVerdictVersion,
		State:             contentModerationRequestVerdictStateQueued,
		QueuedAtUnixMilli: time.Now().UnixMilli(),
		ReviewComplete:    false,
		Decision: ContentModerationDecision{
			Allowed: true,
			Action:  ContentModerationActionAllow,
		},
	})
	if err != nil {
		return false
	}
	operationCtx, cancel := contentModerationRequestVerdictRedisContext(ctx)
	defer cancel()
	if err := cache.SetContentModerationResult(operationCtx, key, raw, contentModerationRequestVerdictQueuedTTL); err != nil {
		slog.Warn("content_moderation.request_verdict_queue_set_failed", "error", err)
		return false
	}
	return true
}

func (s *ContentModerationService) clearContentModerationRequestVerdictQueued(ctx context.Context, key string) {
	if s == nil || strings.TrimSpace(key) == "" {
		return
	}
	cache, ok := s.hashCache.(ContentModerationResultCache)
	if !ok {
		return
	}
	operationCtx, cancel := contentModerationRequestVerdictRedisContext(ctx)
	defer cancel()
	// The cache contract has no delete operation. An invalid, immediately
	// expiring value is a miss for both queued and complete readers.
	if err := cache.SetContentModerationResult(operationCtx, key, []byte(`{}`), time.Nanosecond); err != nil {
		slog.Warn("content_moderation.request_verdict_queue_clear_failed", "error", err)
	}
}

func (s *ContentModerationService) getContentModerationRequestVerdict(ctx context.Context, key string) (*ContentModerationDecision, bool, error) {
	if s == nil || strings.TrimSpace(key) == "" {
		return nil, false, nil
	}
	cache, ok := s.hashCache.(ContentModerationResultCache)
	if !ok {
		return nil, false, nil
	}
	operationCtx, cancel := contentModerationRequestVerdictRedisContext(ctx)
	defer cancel()
	raw, hit, err := cache.GetContentModerationResult(operationCtx, key)
	if err != nil {
		return nil, false, fmt.Errorf("get content moderation request verdict: %w", err)
	}
	if !hit {
		return nil, false, nil
	}
	var entry contentModerationRequestVerdictEntry
	if json.Unmarshal(raw, &entry) != nil || entry.Version != contentModerationRequestVerdictVersion || !entry.ReviewComplete || strings.TrimSpace(entry.Decision.Action) == "" {
		return nil, false, nil
	}
	entry.Decision.requestVerdictCacheable = false
	return cloneContentModerationDecision(&entry.Decision), true, nil
}

func (s *ContentModerationService) setContentModerationRequestVerdict(ctx context.Context, key string, decision *ContentModerationDecision) error {
	if s == nil || strings.TrimSpace(key) == "" || decision == nil || !decision.requestVerdictCacheable {
		return nil
	}
	cache, ok := s.hashCache.(ContentModerationResultCache)
	if !ok {
		return fmt.Errorf("content moderation result cache unavailable")
	}
	copyDecision := cloneContentModerationDecision(decision)
	copyDecision.requestVerdictCacheable = false
	raw, err := json.Marshal(contentModerationRequestVerdictEntry{
		Version:        contentModerationRequestVerdictVersion,
		State:          contentModerationRequestVerdictStateComplete,
		ReviewComplete: true,
		Decision:       *copyDecision,
	})
	if err != nil {
		return fmt.Errorf("marshal content moderation request verdict: %w", err)
	}
	operationCtx, cancel := contentModerationRequestVerdictRedisContext(ctx)
	defer cancel()
	if err := cache.SetContentModerationResult(operationCtx, key, raw, contentModerationRequestVerdictTTL); err != nil {
		return fmt.Errorf("set content moderation request verdict: %w", err)
	}
	return nil
}

// enqueueContentModerationObserveIdempotent records an explicit queued state,
// not a terminal allow verdict. The worker replaces it with the final audit
// decision. The marker is retained long enough to cover the queue, but only a
// short timestamped lease suppresses retries after a process exit.
func (s *ContentModerationService) enqueueContentModerationObserveIdempotent(
	ctx context.Context,
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	content ContentModerationInput,
	targetHash string,
	key string,
) *ContentModerationDecision {
	allow := &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow}
	executionMode := contentModerationRequestVerdictExecutionMode(cfg, nil, true)
	if contentModerationRequestVerdictEpochFallbackRequired(input, cfg, key) {
		return contentModerationRequestVerdictFallbackDecision(
			contentModerationRequestVerdictWaitFallback(cfg, executionMode),
		)
	}
	if strings.TrimSpace(key) == "" {
		if s.enqueueAsync(input, cfg, content, targetHash, false) {
			return allow
		}
		return nil
	}
	queued, queueErr := s.contentModerationRequestVerdictQueued(ctx, key)
	if queueErr != nil {
		slog.Warn("content_moderation.request_verdict_queue_get_failed", "error", queueErr)
		return allow
	}
	if queued {
		return allow
	}
	return s.evaluateContentModerationRequestVerdictIdempotentWithFallback(ctx, input, cfg, key, executionMode, allow, func(workCtx context.Context) *ContentModerationDecision {
		queued, queueErr := s.contentModerationRequestVerdictQueued(workCtx, key)
		if queueErr != nil {
			slog.Warn("content_moderation.request_verdict_queue_get_failed", "error", queueErr)
			return allow
		}
		if queued {
			return allow
		}
		markerSet := s.setContentModerationRequestVerdictQueued(workCtx, key)
		if !markerSet {
			slog.Warn("content_moderation.request_verdict_queue_set_required")
			return allow
		}
		if !s.enqueueAsync(input, cfg, content, targetHash, false) {
			s.clearContentModerationRequestVerdictQueued(workCtx, key)
			return nil
		}
		return allow
	})
}

type contentModerationRequestVerdictEvaluator func(context.Context) *ContentModerationDecision

// localContentModerationVerdictIdempotent runs a local terminal decision and
// all of its side effects under the same process and Redis coordination used
// by semantic audits. With no RequestID it invokes evaluate directly. A
// RequestID whose user epoch could not be captured must use the supplied
// fallback because direct execution cannot be fenced from an administrator
// reset.
func (s *ContentModerationService) localContentModerationVerdictIdempotent(
	ctx context.Context,
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	key string,
	fallback *ContentModerationDecision,
	evaluate contentModerationRequestVerdictEvaluator,
) *ContentModerationDecision {
	if evaluate == nil {
		return nil
	}
	if strings.TrimSpace(key) == "" {
		if contentModerationRequestVerdictEpochFallbackRequired(input, cfg, key) {
			return contentModerationRequestVerdictFallbackDecision(fallback)
		}
		return evaluate(ctx)
	}
	executionMode := contentModerationRequestVerdictExecutionMode(cfg, nil, true)
	return s.evaluateContentModerationRequestVerdictIdempotentWithFallback(ctx, input, cfg, key, executionMode, fallback, func(workCtx context.Context) *ContentModerationDecision {
		decision := cloneContentModerationDecision(evaluate(workCtx))
		if decision != nil {
			decision.requestVerdictCacheable = true
		}
		return decision
	})
}

func (s *ContentModerationService) checkSyncIdempotent(
	ctx context.Context,
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	content ContentModerationInput,
	targetHash string,
	queueDelay *int,
	allowBlock bool,
) *ContentModerationDecision {
	executionMode := contentModerationRequestVerdictExecutionMode(cfg, queueDelay, allowBlock)
	key := contentModerationRequestVerdictCacheKey(input, cfg, content, targetHash, executionMode)
	if key == "" {
		if contentModerationRequestVerdictEpochFallbackRequired(input, cfg, key) {
			return contentModerationRequestVerdictFallbackDecision(
				contentModerationRequestVerdictWaitFallback(cfg, executionMode),
			)
		}
		return s.checkSync(ctx, input, cfg, content, targetHash, queueDelay, allowBlock)
	}
	if !s.contentModerationCachedVerdictAllowed(ctx, targetHash) {
		// An administrator suppression invalidates both the request ledger and the
		// ordinary AI-result cache for this target. Do not write a replacement
		// ledger entry while the suppression remains active.
		return s.checkSync(ctx, input, cfg, content, targetHash, queueDelay, allowBlock)
	}
	return s.evaluateContentModerationRequestVerdictIdempotent(ctx, input, cfg, key, executionMode, func(workCtx context.Context) *ContentModerationDecision {
		return s.checkSync(workCtx, input, cfg, content, targetHash, queueDelay, allowBlock)
	})
}

func (s *ContentModerationService) evaluateContentModerationRequestVerdictIdempotent(
	ctx context.Context,
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	key string,
	executionMode string,
	evaluate contentModerationRequestVerdictEvaluator,
) *ContentModerationDecision {
	return s.evaluateContentModerationRequestVerdictIdempotentWithFallback(
		ctx,
		input,
		cfg,
		key,
		executionMode,
		contentModerationRequestVerdictWaitFallback(cfg, executionMode),
		evaluate,
	)
}

func (s *ContentModerationService) evaluateContentModerationRequestVerdictIdempotentWithFallback(
	ctx context.Context,
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	key string,
	executionMode string,
	fallback *ContentModerationDecision,
	evaluate contentModerationRequestVerdictEvaluator,
) *ContentModerationDecision {
	if evaluate == nil {
		return nil
	}
	if cached, hit, cacheErr := s.getContentModerationRequestVerdict(ctx, key); cacheErr != nil {
		slog.Warn("content_moderation.request_verdict_cache_get_failed", "error", cacheErr)
		return contentModerationRequestVerdictFallbackDecision(fallback)
	} else if hit {
		slog.Info("content_moderation.request_verdict_cache_hit", "user_id", input.UserID, "api_key_id", input.APIKeyID, "request_id", input.RequestID)
		return cached
	}

	result := s.requestVerdictFlight.group.DoChan(key, func() (any, error) {
		return s.runContentModerationRequestVerdictSafely(ctx, input, cfg, key, executionMode, fallback, evaluate)
	})
	select {
	case outcome := <-result:
		decision, _ := outcome.Val.(*ContentModerationDecision)
		return cloneContentModerationDecision(decision)
	case <-ctx.Done():
		return contentModerationRequestVerdictFallbackDecision(fallback)
	}
}

func (s *ContentModerationService) runContentModerationRequestVerdictSafely(
	ctx context.Context,
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	key string,
	executionMode string,
	fallback *ContentModerationDecision,
	evaluate contentModerationRequestVerdictEvaluator,
) (decision any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("content_moderation.request_verdict_panic",
				"request_id", input.RequestID,
				"execution_mode", executionMode,
				"recover", recovered,
				"stack", string(debug.Stack()))
			decision = contentModerationRequestVerdictFallbackDecision(fallback)
			err = nil
		}
	}()
	workCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		contentModerationRequestVerdictWorkTimeout(cfg, executionMode),
	)
	defer cancel()
	if cached, hit, cacheErr := s.getContentModerationRequestVerdict(workCtx, key); cacheErr != nil {
		slog.Warn("content_moderation.request_verdict_cache_get_failed", "error", cacheErr)
		return contentModerationRequestVerdictFallbackDecision(fallback), nil
	} else if hit {
		return cached, nil
	}
	return s.runContentModerationRequestVerdict(workCtx, input, cfg, key, executionMode, fallback, evaluate), nil
}

func (s *ContentModerationService) runContentModerationRequestVerdict(
	ctx context.Context,
	input ContentModerationCheckInput,
	cfg *ContentModerationConfig,
	key string,
	executionMode string,
	fallback *ContentModerationDecision,
	evaluate contentModerationRequestVerdictEvaluator,
) *ContentModerationDecision {
	claimStore, ok := s.hashCache.(ContentModerationRequestVerdictClaimStore)
	if !ok {
		slog.Warn("content_moderation.request_verdict_claim_store_unavailable",
			"request_id", input.RequestID,
			"execution_mode", executionMode)
		return contentModerationRequestVerdictFallbackDecision(fallback)
	}
	owner := newContentModerationRequestVerdictClaimOwner()
	if owner == "" {
		return contentModerationRequestVerdictFallbackDecision(fallback)
	}

	wait := contentModerationRequestVerdictWait(cfg, executionMode)
	claimTTL := contentModerationRequestVerdictClaimTTL(cfg, executionMode)
	acquired, err := s.tryClaimContentModerationRequestVerdict(ctx, claimStore, key, owner, claimTTL)
	if err != nil {
		slog.Warn("content_moderation.request_verdict_claim_failed", "error", err)
		// Without the cross-instance lease we cannot prove that this process owns
		// the non-idempotent log, risk, ban, and notification side effects.
		return contentModerationRequestVerdictFallbackDecision(fallback)
	}
	if acquired {
		decision, releaseSafe := s.executeClaimedContentModerationRequestVerdict(ctx, key, fallback, evaluate)
		if releaseSafe {
			s.releaseContentModerationRequestVerdictClaim(ctx, claimStore, key, owner)
		}
		return decision
	}

	timer := time.NewTimer(wait)
	ticker := time.NewTicker(contentModerationRequestVerdictPollInterval)
	defer timer.Stop()
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return contentModerationRequestVerdictFallbackDecision(fallback)
		case <-timer.C:
			slog.Warn("content_moderation.request_verdict_claim_wait_timeout", "request_id", input.RequestID, "execution_mode", executionMode)
			return contentModerationRequestVerdictFallbackDecision(fallback)
		case <-ticker.C:
			if cached, hit, cacheErr := s.getContentModerationRequestVerdict(ctx, key); cacheErr != nil {
				slog.Warn("content_moderation.request_verdict_cache_get_failed", "error", cacheErr)
				return contentModerationRequestVerdictFallbackDecision(fallback)
			} else if hit {
				return cached
			}
			acquired, err = s.tryClaimContentModerationRequestVerdict(ctx, claimStore, key, owner, claimTTL)
			if err != nil {
				slog.Warn("content_moderation.request_verdict_claim_wait_failed", "error", err)
				return contentModerationRequestVerdictFallbackDecision(fallback)
			}
			if acquired {
				decision, releaseSafe := s.executeClaimedContentModerationRequestVerdict(ctx, key, fallback, evaluate)
				if releaseSafe {
					s.releaseContentModerationRequestVerdictClaim(ctx, claimStore, key, owner)
				}
				return decision
			}
		}
	}
}

func (s *ContentModerationService) executeClaimedContentModerationRequestVerdict(
	ctx context.Context,
	key string,
	fallback *ContentModerationDecision,
	evaluate contentModerationRequestVerdictEvaluator,
) (*ContentModerationDecision, bool) {
	// The cache read that preceded lease acquisition may have raced with the
	// previous owner publishing its verdict and releasing the lease. Recheck
	// while holding the lease before executing non-idempotent side effects.
	if cached, hit, err := s.getContentModerationRequestVerdict(ctx, key); err != nil {
		slog.Warn("content_moderation.request_verdict_claimed_cache_get_failed", "error", err)
		return contentModerationRequestVerdictFallbackDecision(fallback), true
	} else if hit {
		return cached, true
	}
	return s.executeContentModerationRequestVerdict(ctx, key, evaluate)
}

func (s *ContentModerationService) executeContentModerationRequestVerdict(
	ctx context.Context,
	key string,
	evaluate contentModerationRequestVerdictEvaluator,
) (*ContentModerationDecision, bool) {
	decision := evaluate(ctx)
	releaseSafe := true
	if err := s.setContentModerationRequestVerdict(ctx, key, decision); err != nil {
		slog.Warn("content_moderation.request_verdict_cache_set_failed", "error", err)
		releaseSafe = false
	}
	if decision != nil {
		decision.requestVerdictCacheable = false
	}
	return cloneContentModerationDecision(decision), releaseSafe
}

func contentModerationRequestVerdictWorkTimeout(cfg *ContentModerationConfig, executionMode string) time.Duration {
	wait := contentModerationRequestVerdictWait(cfg, executionMode)
	return maxContentModerationRequestVerdictDuration(
		wait+contentModerationRequestVerdictClaimLeaseGrace,
		contentModerationRequestVerdictMinimumClaimTTL,
	)
}

func contentModerationRequestVerdictClaimTTL(cfg *ContentModerationConfig, executionMode string) time.Duration {
	return contentModerationRequestVerdictWorkTimeout(cfg, executionMode) + contentModerationRequestVerdictClaimLeaseGrace
}

func contentModerationObserveMaxQueueWait(cfg *ContentModerationConfig) time.Duration {
	reserved := contentModerationRequestVerdictClaimTTL(cfg, "async_observe")
	if reserved >= contentModerationRequestVerdictQueuedTTL {
		return 0
	}
	return contentModerationRequestVerdictQueuedTTL - reserved
}

func contentModerationRequestVerdictWaitFallback(cfg *ContentModerationConfig, executionMode string) *ContentModerationDecision {
	if executionMode == "preblock_sync" && cfg != nil && cfg.AIChat.FailurePolicy == ContentModerationFailurePolicyBlock {
		return contentModerationUnavailableDecision()
	}
	return &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow}
}

func contentModerationRequestVerdictFallbackDecision(fallback *ContentModerationDecision) *ContentModerationDecision {
	decision := cloneContentModerationDecision(fallback)
	if decision != nil {
		decision.requestVerdictCacheable = false
	}
	return decision
}

func contentModerationRequestVerdictWait(cfg *ContentModerationConfig, executionMode string) time.Duration {
	wait := contentModerationRequestVerdictMinWait
	if cfg == nil {
		return wait
	}
	if strings.HasPrefix(executionMode, "async_") {
		attempts := max(1, cfg.AIChat.RetryCount+1)
		wait = time.Duration(max(1, cfg.AIChat.TimeoutMS)) * time.Millisecond * time.Duration(attempts)
		wait += contentModerationRequestVerdictSyncWaitGrace
		return minContentModerationRequestVerdictDuration(
			maxContentModerationRequestVerdictDuration(wait, contentModerationRequestVerdictMinWait),
			contentModerationRequestVerdictMaxAsyncWait,
		)
	}
	wait = time.Duration(max(1, cfg.AIChat.SynchronousBudgetMS))*time.Millisecond + contentModerationRequestVerdictSyncWaitGrace
	return minContentModerationRequestVerdictDuration(
		maxContentModerationRequestVerdictDuration(wait, contentModerationRequestVerdictMinWait),
		contentModerationRequestVerdictMaxSyncWait,
	)
}

func maxContentModerationRequestVerdictDuration(left, right time.Duration) time.Duration {
	if left > right {
		return left
	}
	return right
}

func minContentModerationRequestVerdictDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

func (s *ContentModerationService) tryClaimContentModerationRequestVerdict(
	ctx context.Context,
	store ContentModerationRequestVerdictClaimStore,
	key string,
	owner string,
	ttl time.Duration,
) (bool, error) {
	operationCtx, cancel := contentModerationRequestVerdictRedisContext(ctx)
	defer cancel()
	return store.TryClaimContentModerationRequestVerdict(operationCtx, key, owner, ttl)
}

func (s *ContentModerationService) releaseContentModerationRequestVerdictClaim(
	ctx context.Context,
	store ContentModerationRequestVerdictClaimStore,
	key string,
	owner string,
) {
	operationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), contentModerationRequestVerdictRedisTimeout)
	defer cancel()
	if err := store.ReleaseContentModerationRequestVerdictClaim(operationCtx, key, owner); err != nil {
		slog.Warn("content_moderation.request_verdict_claim_release_failed", "error", err)
	}
}

func contentModerationRequestVerdictRedisContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, contentModerationRequestVerdictRedisTimeout)
}

func newContentModerationRequestVerdictClaimOwner() string {
	var value [16]byte
	if _, err := cryptorand.Read(value[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(value[:])
}

func cloneContentModerationDecision(decision *ContentModerationDecision) *ContentModerationDecision {
	if decision == nil {
		return nil
	}
	cloned := *decision
	cloned.CategoryScores = cloneFloatMap(decision.CategoryScores)
	return &cloned
}
