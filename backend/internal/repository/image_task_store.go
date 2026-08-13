package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	imageTaskKeyPrefix       = "image_task:"
	imageTaskActiveKeyPrefix = "image_task_active:"
)

var acquireImageTaskSlotScript = redis.NewScript(`
local key = KEYS[1]
local task_id = ARGV[1]
local now_ms = tonumber(ARGV[2])
local expires_ms = tonumber(ARGV[3])
local max_active = tonumber(ARGV[4])
local key_type = redis.call("TYPE", key)["ok"]
if key_type == "string" then
  return 0
end
redis.call("ZREMRANGEBYSCORE", key, "-inf", now_ms)
if redis.call("ZSCORE", key, task_id) then
  return 0
end
if redis.call("ZCARD", key) >= max_active then
  return 0
end
redis.call("ZADD", key, expires_ms, task_id)
redis.call("PEXPIREAT", key, expires_ms)
return 1
`)

var releaseImageTaskSlotScript = redis.NewScript(`
local key_type = redis.call("TYPE", KEYS[1])["ok"]
if key_type == "string" then
  if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
  end
  return 0
end
return redis.call("ZREM", KEYS[1], ARGV[1])
`)

type imageTaskStore struct {
	rdb *redis.Client
}

func NewImageTaskStore(rdb *redis.Client) service.ImageTaskStore {
	return &imageTaskStore{rdb: rdb}
}

func (s *imageTaskStore) Save(ctx context.Context, task *service.ImageTaskRecord, ttl time.Duration) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, imageTaskKey(task.ID), data, ttl).Err()
}

func (s *imageTaskStore) Get(ctx context.Context, id string) (*service.ImageTaskRecord, error) {
	data, err := s.rdb.Get(ctx, imageTaskKey(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrImageTaskNotFound
		}
		return nil, err
	}
	var task service.ImageTaskRecord
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *imageTaskStore) Acquire(ctx context.Context, apiKeyID int64, taskID string, maxActive int, ttl time.Duration) (bool, error) {
	if maxActive < 1 {
		maxActive = 1
	}
	now := time.Now()
	result, err := acquireImageTaskSlotScript.Run(ctx, s.rdb, []string{imageTaskActiveKey(apiKeyID)},
		strings.TrimSpace(taskID), now.UnixMilli(), now.Add(ttl).UnixMilli(), maxActive).Int()
	return result == 1, err
}

func (s *imageTaskStore) Release(ctx context.Context, apiKeyID int64, taskID string) error {
	return releaseImageTaskSlotScript.Run(ctx, s.rdb, []string{imageTaskActiveKey(apiKeyID)}, strings.TrimSpace(taskID)).Err()
}

func imageTaskKey(id string) string {
	return imageTaskKeyPrefix + strings.TrimSpace(id)
}

func imageTaskActiveKey(apiKeyID int64) string {
	return imageTaskActiveKeyPrefix + strconv.FormatInt(apiKeyID, 10)
}
