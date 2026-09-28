package media

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"

	"go.uber.org/zap"
)

const (
	defaultMediaHoldSweepInterval = 10 * time.Minute
	defaultMediaHoldSweepBatch    = 200
)

// MediaHoldSweeper 兜底退回已过期但从未结清的冻结额。
//
// 为什么必须有它：Release 的另外两条触发路径（轮询到 failed、创建阶段上游
// 失败）都依赖「用户还会再发一次请求」。而任务过期恰恰意味着没人再来——
// 上游卡在 queued/running 永不终结，或用户建完单就不再轮询。任务记录 24h
// 后从 Redis 消失，此时若无独立索引，那笔冻结额将永久悬挂且无任何告警：
// 用户余额少了一笔，却查不到对应账单。
type MediaHoldSweeper struct {
	tasks    *MediaTaskService
	billing  MediaHoldReleaser
	interval time.Duration
	batch    int

	stopOnce sync.Once
	stopCh   chan struct{}
}

// MediaHoldReleaser 是 sweeper 需要的最小结算能力。
//
// 单独定义而不是复用 MediaBilling：sweeper 手里只有索引记录，没有完整的
// 任务记录与 APIKey 对象——任务记录此时已经过期消失了。
type MediaHoldReleaser interface {
	ReleaseHold(ctx context.Context, hold *MediaPendingHold) error
}

func NewMediaHoldSweeper(tasks *MediaTaskService, billing MediaHoldReleaser) *MediaHoldSweeper {
	return &MediaHoldSweeper{
		tasks:    tasks,
		billing:  billing,
		interval: defaultMediaHoldSweepInterval,
		batch:    defaultMediaHoldSweepBatch,
	}
}

// Start 启动后台扫描。tasks 或 billing 缺失时不启动，避免空转。
func (s *MediaHoldSweeper) Start() {
	if s == nil || s.tasks == nil || s.tasks.Store() == nil || s.billing == nil {
		return
	}
	s.stopCh = make(chan struct{})
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.SweepOnce(context.Background())
			}
		}
	}()
}

func (s *MediaHoldSweeper) Stop() {
	if s == nil || s.stopCh == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// SweepOnce 扫描一批到期未结清的冻结额并退回。导出以便测试与手工触发。
func (s *MediaHoldSweeper) SweepOnce(ctx context.Context) (released int, failed int) {
	if s == nil || s.tasks == nil || s.tasks.Store() == nil || s.billing == nil {
		return 0, 0
	}
	store := s.tasks.Store()
	holds, err := store.ListDuePendingHolds(ctx, time.Now(), s.batch)
	if err != nil {
		logger.L().Warn("media_hold_sweeper.list_failed", zap.Error(err))
		return 0, 0
	}
	for _, hold := range holds {
		if hold == nil {
			continue
		}
		// 抢结算 claim：已被 Capture 或 Release 占用说明这笔早已结清，
		// 只需清掉索引，绝不能重复退款。
		won, claimErr := store.ClaimSettlement(ctx, hold.TaskID, s.tasks.ClaimTTL())
		if claimErr != nil {
			failed++
			continue
		}
		if !won {
			_ = store.DeletePendingHold(ctx, hold.TaskID)
			continue
		}
		if err := s.billing.ReleaseHold(ctx, hold); err != nil {
			// 退款失败时归还 claim，下一轮重试；持续失败会在日志里反复出现，
			// 由 oncall 据此人工核对。
			_ = store.ReleaseSettlement(ctx, hold.TaskID)
			failed++
			logger.L().Error("media_hold_sweeper.release_failed",
				zap.String("task_id", hold.TaskID),
				zap.Int64("user_id", hold.UserID),
				zap.Float64("amount", hold.Amount),
				zap.Error(err))
			continue
		}
		_ = store.DeletePendingHold(ctx, hold.TaskID)
		released++
		logger.L().Info("media_hold_sweeper.released_expired_hold",
			zap.String("task_id", hold.TaskID),
			zap.Int64("user_id", hold.UserID),
			zap.Float64("amount", hold.Amount))
	}
	return released, failed
}
