package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/voteai/riskstate"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const contentModerationFlaggedHashSetKey = "content_moderation:flagged_hashes"
const contentModerationResultCachePrefix = "content_moderation:result:v1:"
const contentModerationSessionRiskPrefix = "content_moderation:session_risk:v1:"

type contentModerationHashCache struct {
	rdb *redis.Client
}

var _ service.ContentModerationSessionRiskStore = (*contentModerationHashCache)(nil)

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
	key = strings.TrimSpace(key)
	if c == nil || c.rdb == nil || key == "" {
		return riskstate.Apply(riskstate.State{}, event, cfg), nil
	}
	redisKey := contentModerationSessionRiskPrefix + key
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
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, redisKey, encoded, riskstate.NormalizeConfig(cfg).TTL)
				return nil
			})
			return err
		}, redisKey)
		if err == nil {
			return updated, nil
		}
		if err != redis.TxFailedErr {
			return riskstate.State{}, err
		}
	}
	return riskstate.State{}, redis.TxFailedErr
}
