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

var releaseImageTaskSlotScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
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

func (s *imageTaskStore) Acquire(ctx context.Context, apiKeyID int64, taskID string, ttl time.Duration) (bool, error) {
	return s.rdb.SetNX(ctx, imageTaskActiveKey(apiKeyID), strings.TrimSpace(taskID), ttl).Result()
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
