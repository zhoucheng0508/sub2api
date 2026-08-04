package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/voteai/riskstate"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const contentModerationFlaggedHashSetKey = "content_moderation:flagged_hashes"
const contentModerationResultCachePrefix = "content_moderation:result:v1:"
const contentModerationSessionRiskPrefix = "content_moderation:session_risk:v2:"
const contentModerationSessionRiskIndexPrefix = "content_moderation:session_risk_index:v2:"
const contentModerationUserEpochPrefix = "content_moderation:user_epoch:v1:"

type contentModerationHashCache struct {
	rdb *redis.Client
}

var _ service.ContentModerationSessionRiskStore = (*contentModerationHashCache)(nil)
var _ service.ContentModerationIndexedSessionRiskStore = (*contentModerationHashCache)(nil)
var _ service.ContentModerationUserStateCleaner = (*contentModerationHashCache)(nil)
var _ service.ContentModerationUserStateEpochStore = (*contentModerationHashCache)(nil)

func NewContentModerationHashCache(rdb *redis.Client) service.ContentModerationHashCache {
	return &contentModerationHashCache{rdb: rdb}
}

func (c *contentModerationHashCache) RecordFlaggedInputHash(ctx context.Context, inputHash string) error {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return nil
	}
	return c.rdb.SAdd(ctx, contentModerationFlaggedHashSetKey, inputHash).Err()
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
	deleted, err := c.rdb.SRem(ctx, contentModerationFlaggedHashSetKey, inputHash).Result()
	if err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func (c *contentModerationHashCache) ClearFlaggedInputHashes(ctx context.Context) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	deleted, err := c.rdb.SCard(ctx, contentModerationFlaggedHashSetKey).Result()
	if err != nil {
		return 0, err
	}
	if deleted == 0 {
		return 0, nil
	}
	if err := c.rdb.Del(ctx, contentModerationFlaggedHashSetKey).Err(); err != nil {
		return 0, err
	}
	return deleted, nil
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
