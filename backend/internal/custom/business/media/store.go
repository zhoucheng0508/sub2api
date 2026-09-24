package media

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// 媒体任务的独立 Redis 命名空间。刻意不复用 Grok 的 grok_video_* 前缀，
// 也不并入 GatewayCache：两者的生命周期、TTL 与回滚边界互不相干。
const (
	mediaTaskKeyPrefix       = "media_task:"
	mediaFileKeyPrefix       = "media_file:"
	mediaSettlementKeyPrefix = "media_task_settled:"
)

type mediaTaskStore struct {
	rdb *redis.Client
}

func NewMediaTaskStore(rdb *redis.Client) MediaTaskStore {
	return &mediaTaskStore{rdb: rdb}
}

func (s *mediaTaskStore) SaveTask(ctx context.Context, task *MediaTaskRecord, ttl time.Duration) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, mediaTaskKey(task.ID), data, ttl).Err()
}

func (s *mediaTaskStore) GetTask(ctx context.Context, id string) (*MediaTaskRecord, error) {
	data, err := s.rdb.Get(ctx, mediaTaskKey(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrMediaTaskNotFound
		}
		return nil, err
	}
	var task MediaTaskRecord
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *mediaTaskStore) SaveFile(ctx context.Context, file *MediaFileRecord, ttl time.Duration) error {
	data, err := json.Marshal(file)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, mediaFileKey(file.ID), data, ttl).Err()
}

func (s *mediaTaskStore) GetFile(ctx context.Context, id string) (*MediaFileRecord, error) {
	data, err := s.rdb.Get(ctx, mediaFileKey(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrMediaTaskNotFound
		}
		return nil, err
	}
	var file MediaFileRecord
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	return &file, nil
}

// ClaimSettlement 用 SetNX 原子占位，返回 true 表示本次调用赢得结算权。
// 轮询与下载可能并发观察到同一个终态，claim 保证只有一方进入扣费。
func (s *mediaTaskStore) ClaimSettlement(ctx context.Context, taskID string, ttl time.Duration) (bool, error) {
	if strings.TrimSpace(taskID) == "" {
		return false, ErrMediaTaskNotFound
	}
	return s.rdb.SetNX(ctx, mediaSettlementKey(taskID), "1", ttl).Result()
}

// ReleaseSettlement 清除占位，供扣费或落库失败后重试。
func (s *mediaTaskStore) ReleaseSettlement(ctx context.Context, taskID string) error {
	if strings.TrimSpace(taskID) == "" {
		return nil
	}
	return s.rdb.Del(ctx, mediaSettlementKey(taskID)).Err()
}

func mediaTaskKey(id string) string { return mediaTaskKeyPrefix + strings.TrimSpace(id) }

func mediaFileKey(id string) string { return mediaFileKeyPrefix + strings.TrimSpace(id) }

func mediaSettlementKey(id string) string { return mediaSettlementKeyPrefix + strings.TrimSpace(id) }

// 待结清冻结额的兜底索引。
//
// 用 ZSET 存到期时间、单独的 key 存明细：任务记录 24h 后就从 Redis 消失，
// 届时已无从扫描，而冻结额若未结清必须退回。索引 TTL 与结算 claim 一致（48h），
// 长于任务记录，给清理服务留出兜底窗口。
const (
	mediaPendingHoldZSet      = "media_hold_pending"
	mediaPendingHoldKeyPrefix = "media_hold_pending:"
)

func mediaPendingHoldKey(taskID string) string {
	return mediaPendingHoldKeyPrefix + strings.TrimSpace(taskID)
}

func (s *mediaTaskStore) TrackPendingHold(ctx context.Context, hold *MediaPendingHold, ttl time.Duration) error {
	if hold == nil || strings.TrimSpace(hold.TaskID) == "" {
		return nil
	}
	data, err := json.Marshal(hold)
	if err != nil {
		return err
	}
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, mediaPendingHoldKey(hold.TaskID), data, ttl)
	pipe.ZAdd(ctx, mediaPendingHoldZSet, redis.Z{Score: float64(hold.DueAt), Member: hold.TaskID})
	_, err = pipe.Exec(ctx)
	return err
}

func (s *mediaTaskStore) ListDuePendingHolds(ctx context.Context, now time.Time, limit int) ([]*MediaPendingHold, error) {
	if limit <= 0 {
		limit = 100
	}
	ids, err := s.rdb.ZRangeByScore(ctx, mediaPendingHoldZSet, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   strconv.FormatInt(now.Unix(), 10),
		Count: int64(limit),
	}).Result()
	if err != nil {
		return nil, err
	}
	out := make([]*MediaPendingHold, 0, len(ids))
	for _, id := range ids {
		data, getErr := s.rdb.Get(ctx, mediaPendingHoldKey(id)).Bytes()
		if getErr != nil {
			if getErr == redis.Nil {
				// 明细已过期而 ZSET 成员还在：这是已无法退款的孤儿项，
				// 直接摘掉避免无限重扫。冻结额需人工核对，由调用方告警。
				s.rdb.ZRem(ctx, mediaPendingHoldZSet, id)
			}
			continue
		}
		var hold MediaPendingHold
		if json.Unmarshal(data, &hold) != nil {
			continue
		}
		out = append(out, &hold)
	}
	return out, nil
}

func (s *mediaTaskStore) DeletePendingHold(ctx context.Context, taskID string) error {
	if strings.TrimSpace(taskID) == "" {
		return nil
	}
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, mediaPendingHoldKey(taskID))
	pipe.ZRem(ctx, mediaPendingHoldZSet, taskID)
	_, err := pipe.Exec(ctx)
	return err
}
