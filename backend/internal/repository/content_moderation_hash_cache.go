package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/voteai/auditcontext"
	"github.com/Wei-Shaw/sub2api/internal/custom/voteai/riskstate"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const contentModerationFlaggedHashSetKey = "content_moderation:flagged_hashes"
const contentModerationFlaggedHashSuppressionSetKey = "content_moderation:flagged_hash_suppressions:v1"
const contentModerationResultCachePrefix = "content_moderation:result:v1:"
const contentModerationRequestVerdictClaimPrefix = "content_moderation:request_verdict_claim:v1:"
const contentModerationSessionRiskPrefix = "content_moderation:session_risk:v2:"
const contentModerationSessionRiskIndexPrefix = "content_moderation:session_risk_index:v2:"
const contentModerationUserEpochPrefix = "content_moderation:user_epoch:v1:"
const contentModerationAuditContextPrefix = "content_moderation:audit_context:v1:"
const contentModerationAuditContextIndexPrefix = "content_moderation:audit_context_index:v1:"

var (
	contentModerationFlaggedHashRecordScript = redis.NewScript(`
if redis.call('SISMEMBER', KEYS[2], ARGV[1]) == 1 then
  return -1
end
return redis.call('SADD', KEYS[1], ARGV[1])
`)
	contentModerationFlaggedHashDeleteScript = redis.NewScript(`
local deleted = redis.call('SREM', KEYS[1], ARGV[1])
redis.call('SADD', KEYS[2], ARGV[1])
return deleted
`)
	contentModerationFlaggedHashClearScript = redis.NewScript(`
local deleted = redis.call('SCARD', KEYS[1])
if deleted == 0 then
  return 0
end
redis.call('SUNIONSTORE', KEYS[2], KEYS[2], KEYS[1])
redis.call('DEL', KEYS[1])
return deleted
`)
	contentModerationRequestVerdictClaimReleaseScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`)
)

type contentModerationHashCache struct {
	rdb *redis.Client
}

var _ service.ContentModerationSessionRiskStore = (*contentModerationHashCache)(nil)
var _ service.ContentModerationIndexedSessionRiskStore = (*contentModerationHashCache)(nil)
var _ service.ContentModerationUserStateCleaner = (*contentModerationHashCache)(nil)
var _ service.ContentModerationUserStateEpochStore = (*contentModerationHashCache)(nil)
var _ service.ContentModerationAuditContextStore = (*contentModerationHashCache)(nil)
var _ service.ContentModerationRequestVerdictClaimStore = (*contentModerationHashCache)(nil)
var _ service.ContentModerationFlaggedHashLifecycleStore = (*contentModerationHashCache)(nil)

func NewContentModerationHashCache(rdb *redis.Client) service.ContentModerationHashCache {
	return &contentModerationHashCache{rdb: rdb}
}

func (c *contentModerationHashCache) RecordFlaggedInputHash(ctx context.Context, inputHash string) error {
	_, err := c.RecordFlaggedInputHashIfAllowed(ctx, inputHash)
	return err
}

func (c *contentModerationHashCache) RecordFlaggedInputHashIfAllowed(ctx context.Context, inputHash string) (bool, error) {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return true, nil
	}
	// A suppression entry fences delayed async writers after an administrator has
	// removed a false-positive hash. The caller owns digest normalization and
	// policy/version namespacing; Redis stores that opaque value unchanged.
	result, err := contentModerationFlaggedHashRecordScript.Run(ctx, c.rdb, []string{
		contentModerationFlaggedHashSetKey,
		contentModerationFlaggedHashSuppressionSetKey,
	}, inputHash).Int64()
	if err != nil {
		return false, err
	}
	return result >= 0, nil
}

func (c *contentModerationHashCache) IsFlaggedInputHashSuppressed(ctx context.Context, inputHash string) (bool, error) {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return false, nil
	}
	return c.rdb.SIsMember(ctx, contentModerationFlaggedHashSuppressionSetKey, inputHash).Result()
}

func (c *contentModerationHashCache) HasFlaggedInputHash(ctx context.Context, inputHash string) (bool, error) {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return false, nil
	}
	return c.rdb.SIsMember(ctx, contentModerationFlaggedHashSetKey, inputHash).Result()
}

func (c *contentModerationHashCache) DeleteFlaggedInputHash(ctx context.Context, inputHash string) (bool, error) {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return false, nil
	}
	deleted, err := contentModerationFlaggedHashDeleteScript.Run(ctx, c.rdb, []string{
		contentModerationFlaggedHashSetKey,
		contentModerationFlaggedHashSuppressionSetKey,
	}, inputHash).Int64()
	if err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func (c *contentModerationHashCache) ClearFlaggedInputHashes(ctx context.Context) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	// Clearing means "remove every currently confirmed hash and suppress its
	// stale resurrection". New hashes that were not part of the clear remain
	// recordable, and the returned count still means confirmed hashes removed.
	return contentModerationFlaggedHashClearScript.Run(ctx, c.rdb, []string{
		contentModerationFlaggedHashSetKey,
		contentModerationFlaggedHashSuppressionSetKey,
	}).Int64()
}

func (c *contentModerationHashCache) CountFlaggedInputHashes(ctx context.Context) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	return c.rdb.SCard(ctx, contentModerationFlaggedHashSetKey).Result()
}

// CUSTOM(VOTE-AI-AI-AUDIT): cache only normalized verdicts, never raw user input.
func (c *contentModerationHashCache) GetContentModerationResult(ctx context.Context, key string) ([]byte, bool, error) {
	key = strings.TrimSpace(key)
	if c == nil || c.rdb == nil || key == "" {
		return nil, false, nil
	}
	value, err := c.rdb.Get(ctx, contentModerationResultCachePrefix+key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (c *contentModerationHashCache) SetContentModerationResult(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	key = strings.TrimSpace(key)
	if c == nil || c.rdb == nil || key == "" || len(value) == 0 || ttl <= 0 {
		return nil
	}
	return c.rdb.Set(ctx, contentModerationResultCachePrefix+key, value, ttl).Err()
}

// CUSTOM(VOTE-AI-AUDIT-IDEMPOTENCY): a short Redis lease coalesces the same
// normalized request across application instances. The final verdict remains
// the source of truth; this claim contains only opaque key and owner values.
func (c *contentModerationHashCache) TryClaimContentModerationRequestVerdict(
	ctx context.Context,
	key string,
	owner string,
	ttl time.Duration,
) (bool, error) {
	key = strings.TrimSpace(key)
	owner = strings.TrimSpace(owner)
	if c == nil || c.rdb == nil || key == "" || owner == "" || ttl <= 0 {
		return false, nil
	}
	return c.rdb.SetNX(ctx, contentModerationRequestVerdictClaimPrefix+key, owner, ttl).Result()
}

func (c *contentModerationHashCache) ReleaseContentModerationRequestVerdictClaim(ctx context.Context, key, owner string) error {
	key = strings.TrimSpace(key)
	owner = strings.TrimSpace(owner)
	if c == nil || c.rdb == nil || key == "" || owner == "" {
		return nil
	}
	return contentModerationRequestVerdictClaimReleaseScript.Run(
		ctx,
		c.rdb,
		[]string{contentModerationRequestVerdictClaimPrefix + key},
		owner,
	).Err()
}

// CUSTOM(VOTE-AI-AUDIT-CONTEXT): only structured, redacted state is stored.
func (c *contentModerationHashCache) GetContentModerationAuditContext(ctx context.Context, key string) (auditcontext.State, bool, error) {
	key = strings.TrimSpace(key)
	if c == nil || c.rdb == nil || key == "" {
		return auditcontext.State{}, false, nil
	}
	raw, err := c.rdb.Get(ctx, contentModerationAuditContextPrefix+key).Bytes()
	if err == redis.Nil {
		return auditcontext.State{}, false, nil
	}
	if err != nil {
		return auditcontext.State{}, false, err
	}
	var state auditcontext.State
	if err := json.Unmarshal(raw, &state); err != nil {
		return auditcontext.State{}, false, err
	}
	return state, true, nil
}

func (c *contentModerationHashCache) UpdateContentModerationAuditContextForUser(
	ctx context.Context,
	userID int64,
	key string,
	event auditcontext.AuditEvent,
	cfg auditcontext.Config,
	ttl time.Duration,
) (auditcontext.State, error) {
	return c.updateContentModerationAuditContext(ctx, userID, key, ttl, func(previous auditcontext.State) auditcontext.State {
		return auditcontext.Apply(previous, event, cfg)
	})
}

func (c *contentModerationHashCache) UpdateContentModerationAuditPrefixForUser(
	ctx context.Context,
	userID int64,
	key string,
	observation auditcontext.PrefixObservation,
	ttl time.Duration,
) (auditcontext.State, error) {
	return c.updateContentModerationAuditContext(ctx, userID, key, ttl, func(previous auditcontext.State) auditcontext.State {
		return auditcontext.UpdatePrefix(previous, observation)
	})
}

func (c *contentModerationHashCache) updateContentModerationAuditContext(
	ctx context.Context,
	userID int64,
	key string,
	ttl time.Duration,
	mutate func(auditcontext.State) auditcontext.State,
) (auditcontext.State, error) {
	key = strings.TrimSpace(key)
	if c == nil || c.rdb == nil || key == "" || mutate == nil {
		return auditcontext.State{}, nil
	}
	if ttl <= 0 {
		ttl = 120 * time.Minute
	}
	redisKey := contentModerationAuditContextPrefix + key
	indexKey := ""
	if userID > 0 {
		indexKey = contentModerationAuditContextIndexKey(userID)
	}
	var updated auditcontext.State
	for attempt := 0; attempt < 5; attempt++ {
		err := c.rdb.Watch(ctx, func(tx *redis.Tx) error {
			var previous auditcontext.State
			raw, err := tx.Get(ctx, redisKey).Bytes()
			if err != nil && err != redis.Nil {
				return err
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &previous); err != nil {
					return err
				}
			}
			updated = mutate(previous)
			encoded, err := json.Marshal(updated)
			if err != nil {
				return err
			}
			var indexTTL time.Duration
			if indexKey != "" {
				indexTTL, err = tx.PTTL(ctx, indexKey).Result()
				if err != nil {
					return err
				}
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, redisKey, encoded, ttl)
				if indexKey != "" {
					pipe.SAdd(ctx, indexKey, redisKey)
					if indexTTL < 0 || indexTTL < ttl {
						pipe.Expire(ctx, indexKey, ttl)
					}
				}
				return nil
			})
			return err
		}, contentModerationAuditContextWatchKeys(redisKey, indexKey)...)
		if err == nil {
			return updated, nil
		}
		if err != redis.TxFailedErr {
			return auditcontext.State{}, err
		}
	}
	return auditcontext.State{}, redis.TxFailedErr
}

func contentModerationAuditContextIndexKey(userID int64) string {
	return fmt.Sprintf("%s{%d}", contentModerationAuditContextIndexPrefix, userID)
}

func contentModerationAuditContextWatchKeys(redisKey, indexKey string) []string {
	if indexKey == "" {
		return []string{redisKey}
	}
	return []string{redisKey, indexKey}
}

// CUSTOM(VOTE-AI-SESSION-RISK): keep only structured risk state under an opaque identity hash.
func (c *contentModerationHashCache) GetContentModerationSessionRisk(ctx context.Context, key string) (riskstate.State, bool, error) {
	key = strings.TrimSpace(key)
	if c == nil || c.rdb == nil || key == "" {
		return riskstate.State{}, false, nil
	}
	raw, err := c.rdb.Get(ctx, contentModerationSessionRiskPrefix+key).Bytes()
	if err == redis.Nil {
		return riskstate.State{}, false, nil
	}
	if err != nil {
		return riskstate.State{}, false, err
	}
	var state riskstate.State
	if err := json.Unmarshal(raw, &state); err != nil {
		return riskstate.State{}, false, err
	}
	return state, true, nil
}

func (c *contentModerationHashCache) UpdateContentModerationSessionRisk(ctx context.Context, key string, event riskstate.Event, cfg riskstate.Config) (riskstate.State, error) {
	return c.updateContentModerationSessionRisk(ctx, 0, key, event, cfg)
}

func (c *contentModerationHashCache) UpdateContentModerationSessionRiskForUser(ctx context.Context, userID int64, key string, event riskstate.Event, cfg riskstate.Config) (riskstate.State, error) {
	return c.updateContentModerationSessionRisk(ctx, userID, key, event, cfg)
}

func (c *contentModerationHashCache) updateContentModerationSessionRisk(ctx context.Context, userID int64, key string, event riskstate.Event, cfg riskstate.Config) (riskstate.State, error) {
	key = strings.TrimSpace(key)
	if c == nil || c.rdb == nil || key == "" {
		return riskstate.Apply(riskstate.State{}, event, cfg), nil
	}
	redisKey := contentModerationSessionRiskPrefix + key
	indexKey := ""
	if userID > 0 {
		indexKey = contentModerationSessionRiskIndexKey(userID)
	}
	normalizedTTL := riskstate.NormalizeConfig(cfg).TTL
	var updated riskstate.State
	for attempt := 0; attempt < 5; attempt++ {
		err := c.rdb.Watch(ctx, func(tx *redis.Tx) error {
			var previous riskstate.State
			raw, err := tx.Get(ctx, redisKey).Bytes()
			if err != nil && err != redis.Nil {
				return err
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &previous); err != nil {
					return err
				}
			}
			updated = riskstate.Apply(previous, event, cfg)
			encoded, err := json.Marshal(updated)
			if err != nil {
				return err
			}
			var indexTTL time.Duration
			if indexKey != "" {
				indexTTL, err = tx.PTTL(ctx, indexKey).Result()
				if err != nil {
					return err
				}
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, redisKey, encoded, normalizedTTL)
				if indexKey != "" {
					pipe.SAdd(ctx, indexKey, redisKey)
					if indexTTL < 0 || indexTTL < normalizedTTL {
						pipe.Expire(ctx, indexKey, normalizedTTL)
					}
				}
				return nil
			})
			return err
		}, contentModerationSessionRiskWatchKeys(redisKey, indexKey)...)
		if err == nil {
			return updated, nil
		}
		if err != redis.TxFailedErr {
			return riskstate.State{}, err
		}
	}
	return riskstate.State{}, redis.TxFailedErr
}

func contentModerationSessionRiskIndexKey(userID int64) string {
	return fmt.Sprintf("%s{%d}", contentModerationSessionRiskIndexPrefix, userID)
}

func (c *contentModerationHashCache) GetContentModerationUserEpoch(ctx context.Context, userID int64) (int64, error) {
	if c == nil || c.rdb == nil || userID <= 0 {
		return 0, nil
	}
	epoch, err := c.rdb.Get(ctx, contentModerationUserEpochKey(userID)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return epoch, err
}

func (c *contentModerationHashCache) AdvanceContentModerationUserEpoch(ctx context.Context, userID int64) (int64, error) {
	if c == nil || c.rdb == nil || userID <= 0 {
		return 0, nil
	}
	return c.rdb.Incr(ctx, contentModerationUserEpochKey(userID)).Result()
}

func contentModerationUserEpochKey(userID int64) string {
	return fmt.Sprintf("%s{%d}", contentModerationUserEpochPrefix, userID)
}

func contentModerationSessionRiskWatchKeys(redisKey, indexKey string) []string {
	if indexKey == "" {
		return []string{redisKey}
	}
	return []string{redisKey, indexKey}
}
