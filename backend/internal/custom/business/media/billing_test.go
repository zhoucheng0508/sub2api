package media

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"net/http"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	"github.com/stretchr/testify/require"
)

// fakeBillingRepo 记录三个冻结原语与通用 Apply 的调用参数。
type fakeBillingRepo struct {
	reserved []*BatchImageBalanceHoldCommand
	captured []*BatchImageBalanceHoldCommand
	released []*BatchImageBalanceHoldCommand
	applied  []*UsageBillingCommand

	reserveErr error
	captureErr error
	releaseErr error
}

func (f *fakeBillingRepo) Apply(_ context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	f.applied = append(f.applied, cmd)
	return &UsageBillingApplyResult{Applied: true}, nil
}

func (f *fakeBillingRepo) ReserveBatchImageBalance(_ context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	if f.reserveErr != nil {
		return nil, f.reserveErr
	}
	f.reserved = append(f.reserved, cmd)
	return &BatchImageBalanceHoldResult{}, nil
}

func (f *fakeBillingRepo) CaptureBatchImageBalance(_ context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	if f.captureErr != nil {
		return nil, f.captureErr
	}
	f.captured = append(f.captured, cmd)
	return &BatchImageBalanceHoldResult{}, nil
}

func (f *fakeBillingRepo) ReleaseBatchImageBalance(_ context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	if f.releaseErr != nil {
		return nil, f.releaseErr
	}
	f.released = append(f.released, cmd)
	return &BatchImageBalanceHoldResult{}, nil
}

// newBillingEnv 只装配扣费所需的最小依赖。usageLogRepo 留空：
// writeUsageLogBestEffort 对 nil repo 直接返回，用量行的字段断言改为
// 直接测 buildUsageLog，避免为一个断言实现整个 UsageLogRepository。
func newBillingEnv(t *testing.T) (*MediaTaskBilling, *fakeBillingRepo) {
	t.Helper()
	repo := &fakeBillingRepo{}
	gw := service.NewGatewayForMediaBillingTest(repo)
	return NewMediaTaskBilling(gw, nil), repo
}

func billingFixtures() (*MediaTaskRecord, *APIKey, *Account) {
	groupID := int64(9)
	task := &MediaTaskRecord{
		ID:              "task-1",
		UserID:          10,
		APIKeyID:        100,
		AccountID:       7,
		Model:           SeedanceModel25,
		DurationSeconds: 30,
		Resolution:      SeedanceResolution720,
		UnitPrice:       2.0,
	}
	apiKey := &APIKey{ID: 100, UserID: 10, GroupID: &groupID,
		Group: &Group{ID: 9, Platform: PlatformSeedance}}
	account := &Account{ID: 7, Platform: PlatformSeedance, Type: AccountTypeAPIKey}
	return task, apiKey, account
}

// newPricingBillingEnv 在计费仓储之外再装配一个真实的定价解析器，用于验证
// 「按真实模型解析、查不到回落通用 id」的分模型定价。billingService 必须非
// nil：未单独配价的模型在回落解析时会触碰 GetModelPricing（nil 会 panic）；
// channelService 留 nil，分组定价直接读 Group.ModelPricing 不经过它。
func newPricingBillingEnv(t *testing.T) (*MediaTaskBilling, *fakeBillingRepo) {
	t.Helper()
	repo := &fakeBillingRepo{}
	resolver := service.NewModelPricingResolver(nil, service.NewBillingService(nil, nil))
	gw := service.NewGatewayForMediaPricingTest(repo, resolver)
	return NewMediaTaskBilling(gw, nil), repo
}

// TestMediaBillingReserveResolvesPerModelPriceThenFallsBack 覆盖两级定价解析：
// 单独配价的模型走模型级价，未配价的模型回落到通用兜底 id `seedance` 的价。
func TestMediaBillingReserveResolvesPerModelPriceThenFallsBack(t *testing.T) {
	price := func(v float64) *float64 { return &v }
	group := &Group{
		ID: 9, Platform: PlatformSeedance,
		ModelPricing: []service.ChannelModelPricing{
			// seedance2.5 单独配价 5；通用 id seedance 兜底价 2。
			{Platform: "seedance", Models: []string{SeedanceModel25}, BillingMode: service.BillingModePerRequest, PerRequestPrice: price(5)},
			{Platform: "seedance", Models: []string{mediaBillingModel}, BillingMode: service.BillingModePerRequest, PerRequestPrice: price(2)},
		},
	}
	billing, repo := newPricingBillingEnv(t)
	apiKey := &APIKey{ID: 100, UserID: 10, GroupID: &group.ID, Group: group}
	account := &Account{ID: 7, Platform: PlatformSeedance, Type: AccountTypeAPIKey}

	// 单独配价的模型走模型级价 5。
	task25 := &MediaTaskRecord{ID: "t-25", UserID: 10, APIKeyID: 100, AccountID: 7, Model: SeedanceModel25}
	got25, err := billing.Reserve(context.Background(), task25, apiKey, nil, account)
	require.NoError(t, err)
	require.EqualValues(t, 5, got25)

	// 未单独配价的模型回落到通用 id seedance 的兜底价 2。
	task20 := &MediaTaskRecord{ID: "t-20", UserID: 10, APIKeyID: 100, AccountID: 7, Model: SeedanceModel20}
	got20, err := billing.Reserve(context.Background(), task20, apiKey, nil, account)
	require.NoError(t, err)
	require.EqualValues(t, 2, got20)

	// 冻结额与解析出的单价一致，且模型级价先于兜底价命中。
	require.Len(t, repo.reserved, 2)
	require.EqualValues(t, 5, repo.reserved[0].HoldAmount)
	require.EqualValues(t, 2, repo.reserved[1].HoldAmount)
}

func TestMediaBillingReserveRejectsSubscriptionGroups(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, account := billingFixtures()
	apiKey.Group.SubscriptionType = SubscriptionTypeSubscription

	_, err := billing.Reserve(context.Background(), task, apiKey, nil, account)

	// 订阅额度没有冻结语义，静默按余额扣等于对已付费用户双重收费。
	require.Error(t, err)
	require.Equal(t, "MEDIA_SUBSCRIPTION_UNSUPPORTED", infraerrors.Reason(err))
	require.Empty(t, repo.reserved, "订阅护栏必须在冻结之前生效")
}

func TestMediaBillingReserveFailsClosedWithoutConfiguredPrice(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, account := billingFixtures()

	// resolver 为 nil ⇒ 解析不到价格。绝不能按 0 元放行出片。
	_, err := billing.Reserve(context.Background(), task, apiKey, nil, account)
	require.Error(t, err)
	require.Equal(t, "MEDIA_PRICE_NOT_CONFIGURED", infraerrors.Reason(err))
	require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(err))
	require.Empty(t, repo.reserved)
}

func TestMediaBillingCaptureUsesSnapshotPriceAndWritesVideoColumns(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, account := billingFixtures()

	require.NoError(t, billing.Capture(context.Background(), task, apiKey, nil, account))

	require.Len(t, repo.captured, 1)
	cap := repo.captured[0]
	// 用创建时的单价快照，实扣不超过冻结额。
	require.EqualValues(t, 2.0, cap.HoldAmount)
	require.EqualValues(t, 2.0, cap.ActualAmount)

	log := billing.buildUsageLog(task, apiKey, account, 2.0)
	require.Equal(t, string(BillingModeVideo), *log.BillingMode)
	require.EqualValues(t, 2.0, log.ActualCost)
	// 30 秒真实落库，不被 VideoBillingMaxDurationSeconds 的 15 秒上限截半。
	require.NotNil(t, log.VideoDurationSeconds)
	require.Equal(t, 30, *log.VideoDurationSeconds)
	require.NotNil(t, log.VideoResolution)
	require.Equal(t, SeedanceResolution720, *log.VideoResolution)
	// 记账模型是真实模型，与计价用的固定 id 不同。
	require.Equal(t, SeedanceModel25, log.Model)
}

func TestMediaBillingCaptureNeverDeductsBalanceTwice(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, account := billingFixtures()
	apiKey.Quota = 100 // 触发 API Key 配额维度，确保 Apply 会被调用

	require.NoError(t, billing.Capture(context.Background(), task, apiKey, nil, account))

	require.Len(t, repo.applied, 1)
	cmd := repo.applied[0]
	// 余额已由 Capture 扣过，Apply 只能联动非余额维度。
	require.Zero(t, cmd.BalanceCost, "BalanceCost 必须为 0，否则与冻结机制重复扣费")
	require.Zero(t, cmd.SubscriptionCost)
	require.EqualValues(t, 2.0, cmd.APIKeyQuotaCost, "配额维度不能因 BalanceCost 归零而一起失效")
}

func TestMediaBillingSkipsApplyWhenNoNonBalanceDimensions(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, account := billingFixtures()
	// 无配额、无限流、账号无配额限制 ⇒ 没有任何非余额维度需要联动。
	require.NoError(t, billing.Capture(context.Background(), task, apiKey, nil, account))
	require.Empty(t, repo.applied, "三个维度都为 0 时不应产生空扣费记录")
}

func TestMediaBillingHoldRequestIDsUseExistingHelpers(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, _ := billingFixtures()

	require.NoError(t, billing.Release(context.Background(), task, apiKey))
	require.Len(t, repo.released, 1)

	batchID := mediaHoldBatchID(task.ID)
	require.Equal(t, "media:task-1", batchID)
	// Release 内部会用 BatchImageHoldRequestID(cmd.BatchID) 反查 hold claim，
	// 三个 RequestID 必须由既有助手生成；自定义前缀会让释放被静默跳过、
	// 冻结额永久退不回给用户。
	require.Equal(t, BatchImageReleaseRequestID(batchID), repo.released[0].RequestID)
	require.True(t, strings.HasPrefix(BatchImageHoldRequestID(batchID), "batch_image_hold:"))
	require.Equal(t, batchID, repo.released[0].BatchID)
}

func TestMediaBillingReleaseIsFullRefund(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, _ := billingFixtures()

	require.NoError(t, billing.Release(context.Background(), task, apiKey))
	require.Len(t, repo.released, 1)
	// 全额退回，不留手续费。
	require.EqualValues(t, 2.0, repo.released[0].HoldAmount)
}

func TestMediaBillingTreatsRequestConflictAsIdempotentSuccess(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, account := billingFixtures()

	repo.captureErr = ErrUsageBillingRequestConflict
	require.NoError(t, billing.Capture(context.Background(), task, apiKey, nil, account),
		"同一 RequestID 的指纹冲突说明此前已确认过，应视为幂等成功而非卡在重试循环")

	repo.releaseErr = ErrUsageBillingRequestConflict
	require.NoError(t, billing.Release(context.Background(), task, apiKey))
}

func TestMediaBillingErrorsAreMappedAwayFromBatchImageSemantics(t *testing.T) {
	cases := []struct {
		in       error
		reason   string
		httpCode int
	}{
		{ErrBatchImageInsufficientBalance, "MEDIA_INSUFFICIENT_BALANCE", http.StatusPaymentRequired},
		{ErrBatchImageSettlementCostExceedsHold, "MEDIA_SETTLEMENT_EXCEEDS_HOLD", http.StatusInternalServerError},
		{errors.New("boom"), "MEDIA_BILLING_UNAVAILABLE", http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		out := mapMediaBillingError(tc.in)
		require.Equal(t, tc.reason, infraerrors.Reason(out))
		require.Equal(t, tc.httpCode, infraerrors.Code(out))
		// 对外错误体不得出现批量图片语义。
		require.NotContains(t, strings.ToLower(infraerrors.Message(out)), "batch image")
	}
	require.NoError(t, mapMediaBillingError(nil))
}

// ==== 冻结额过期兜底 ====

type memHoldStore struct {
	MediaTaskStore
	holds  map[string]*MediaPendingHold
	claims map[string]bool
}

func newMemHoldStore() *memHoldStore {
	return &memHoldStore{holds: map[string]*MediaPendingHold{}, claims: map[string]bool{}}
}

func (s *memHoldStore) TrackPendingHold(_ context.Context, h *MediaPendingHold, _ time.Duration) error {
	cp := *h
	s.holds[h.TaskID] = &cp
	return nil
}

func (s *memHoldStore) ListDuePendingHolds(_ context.Context, now time.Time, _ int) ([]*MediaPendingHold, error) {
	out := []*MediaPendingHold{}
	for _, h := range s.holds {
		if h.DueAt <= now.Unix() {
			out = append(out, h)
		}
	}
	return out, nil
}

func (s *memHoldStore) DeletePendingHold(_ context.Context, taskID string) error {
	delete(s.holds, taskID)
	return nil
}

func (s *memHoldStore) ClaimSettlement(_ context.Context, taskID string, _ time.Duration) (bool, error) {
	if s.claims[taskID] {
		return false, nil
	}
	s.claims[taskID] = true
	return true, nil
}

func (s *memHoldStore) ReleaseSettlement(_ context.Context, taskID string) error {
	delete(s.claims, taskID)
	return nil
}

type recordingReleaser struct {
	released []*MediaPendingHold
	err      error
}

func (r *recordingReleaser) ReleaseHold(_ context.Context, h *MediaPendingHold) error {
	if r.err != nil {
		return r.err
	}
	r.released = append(r.released, h)
	return nil
}

func newSweeperEnv() (*MediaHoldSweeper, *memHoldStore, *recordingReleaser) {
	store := newMemHoldStore()
	tasks := NewMediaTaskService(store, PlatformSeedance, nil)
	rel := &recordingReleaser{}
	return NewMediaHoldSweeper(tasks, rel), store, rel
}

func TestMediaHoldSweeperReleasesExpiredHolds(t *testing.T) {
	sweeper, store, rel := newSweeperEnv()
	// 到期且从未结清：上游卡在非终态，或用户建单后不再轮询。
	_ = store.TrackPendingHold(context.Background(), &MediaPendingHold{
		TaskID: "t-expired", UserID: 10, APIKeyID: 100, Amount: 2.0,
		DueAt: time.Now().Add(-time.Hour).Unix(),
	}, time.Hour)

	released, failed := sweeper.SweepOnce(context.Background())

	require.Equal(t, 1, released)
	require.Zero(t, failed)
	require.Len(t, rel.released, 1)
	require.EqualValues(t, 2.0, rel.released[0].Amount)
	require.Empty(t, store.holds, "退款后索引必须清除")
}

func TestMediaHoldSweeperSkipsNotYetDue(t *testing.T) {
	sweeper, store, rel := newSweeperEnv()
	_ = store.TrackPendingHold(context.Background(), &MediaPendingHold{
		TaskID: "t-live", UserID: 10, APIKeyID: 100, Amount: 2.0,
		DueAt: time.Now().Add(time.Hour).Unix(),
	}, time.Hour)

	released, _ := sweeper.SweepOnce(context.Background())
	require.Zero(t, released)
	require.Empty(t, rel.released, "未到期的在途任务不得被退款")
	require.Len(t, store.holds, 1)
}

func TestMediaHoldSweeperNeverDoubleRefundsSettledTask(t *testing.T) {
	sweeper, store, rel := newSweeperEnv()
	_ = store.TrackPendingHold(context.Background(), &MediaPendingHold{
		TaskID: "t-settled", UserID: 10, APIKeyID: 100, Amount: 2.0,
		DueAt: time.Now().Add(-time.Hour).Unix(),
	}, time.Hour)
	// 该任务此前已被 Capture 或 Release 占用过结算 claim。
	store.claims["t-settled"] = true

	released, _ := sweeper.SweepOnce(context.Background())

	require.Zero(t, released)
	require.Empty(t, rel.released, "已结清的任务绝不能被重复退款")
	require.Empty(t, store.holds, "但索引应被清掉，避免无限重扫")
}

func TestMediaHoldSweeperReturnsClaimOnReleaseFailure(t *testing.T) {
	sweeper, store, rel := newSweeperEnv()
	rel.err = errors.New("billing down")
	_ = store.TrackPendingHold(context.Background(), &MediaPendingHold{
		TaskID: "t-retry", UserID: 10, APIKeyID: 100, Amount: 2.0,
		DueAt: time.Now().Add(-time.Hour).Unix(),
	}, time.Hour)

	released, failed := sweeper.SweepOnce(context.Background())

	require.Zero(t, released)
	require.Equal(t, 1, failed)
	// 归还 claim 且保留索引，下一轮才能重试。
	require.False(t, store.claims["t-retry"])
	require.Len(t, store.holds, 1)
}
