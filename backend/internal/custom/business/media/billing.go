package media

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"go.uber.org/zap"
)

// 媒体任务的两阶段计费：创建时冻结，终态成功时确认扣除，失败或过期时退回。
//
// 为什么不走 GatewayService.RecordUsage：RecordUsageInput 没有传入预算金额的
// 字段，成本在 recordUsageCore 内部计算，而 calculateRecordUsageCost 只按
// ImageCount / AudioUsage 分派，没有视频分支——媒体任务 token 全为 0，走通用
// 路径会算出 0 元完全不扣费。
const (
	// mediaBillingModel 是计价用的兜底模型 id。
	//
	// 定价按「真实模型优先、此 id 兜底」两级解析（见 resolveUnitPrice）：
	// 管理端可为单个 Seedance 模型（seedance2.5 / 2.0fast 等）配差异化
	// per_request 价，未单独配价的模型回落到这一个 id 的通用价，因此不会
	// 漏配某个模型导致该模型被拒。usage_log.model 记真实模型；对账时若按真实
	// 模型查不到价，说明它走了兜底，需回落到此 id 再查。
	mediaBillingModel = "seedance"

	// mediaHoldBatchIDPrefix 复用既有余额冻结原语时的 BatchID 前缀。
	//
	// Reserve/Capture/Release 三个方法名带 BatchImage，但底层是 users.balance
	// 与 frozen_balance 的通用两阶段扣费，与批量图片业务无耦合。前缀保证与真实
	// 批量作业的 BatchID 空间不相交。
	mediaHoldBatchIDPrefix = "media:"
)

// MediaTaskBilling 实现 MediaBilling。
//
// apiKeyService 单独注入而非从 GatewayService 取：后者没有该字段，而配额
// 耗尽后必须失效鉴权缓存，否则该 Key 在缓存 TTL 内仍可继续用。
type MediaTaskBilling struct {
	gateway       *GatewayService
	apiKeyService APIKeyQuotaUpdater
}

func NewMediaTaskBilling(gateway *GatewayService, apiKeyService APIKeyQuotaUpdater) *MediaTaskBilling {
	return &MediaTaskBilling{gateway: gateway, apiKeyService: apiKeyService}
}

func mediaHoldBatchID(taskID string) string {
	return mediaHoldBatchIDPrefix + strings.TrimSpace(taskID)
}

// resolveUnitPrice 从管理端配置的渠道/分组定价取按条单价。
//
// 两级解析：先按任务的真实模型（seedance2.5 / 2.0fast 等）查独立定价，查不到
// 再回落到通用计价 id "seedance"。这样管理端既能给单个模型配差异化 per_request
// 价，又不必为每个模型都配一遍——未单独配价的模型走通用兜底价。
//
// 解析不到一律 fail-closed 拒绝创建，绝不按 0 元或默认值放行出片：
// 免费出片的损失不可追回，而拒绝创建只需管理员补一条配置。
func (b *MediaTaskBilling) resolveUnitPrice(ctx context.Context, model string, apiKey *APIKey) (float64, error) {
	if b == nil || b.gateway == nil {
		return 0, ErrMediaTaskUnavailable
	}
	// 真实模型名与通用兜底 id 不同才先试模型级定价，避免对同一 id 查两遍。
	if model = strings.TrimSpace(model); model != "" && !strings.EqualFold(model, mediaBillingModel) {
		if price, ok := b.perRequestPriceFor(ctx, model, apiKey); ok {
			return price, nil
		}
	}
	if price, ok := b.perRequestPriceFor(ctx, mediaBillingModel, apiKey); ok {
		return price, nil
	}
	return 0, errMediaPriceMissing()
}

// perRequestPriceFor 取某个计价 id 的按条单价；未配置或非按条模式返回 (0,false)。
//
// 单一阶梯允许 0 价（与历史行为一致），由 Reserve 的 unitPrice<=0 兜底拦截；
// 多阶梯不认——媒体按条计价不做上下文分层。
func (b *MediaTaskBilling) perRequestPriceFor(ctx context.Context, billingModel string, apiKey *APIKey) (float64, bool) {
	resolved := service.ResolveChannelPricingForMedia(b.gateway, ctx, billingModel, apiKey)
	if resolved == nil {
		return 0, false
	}
	switch resolved.Mode {
	case BillingModePerRequest, BillingModeImage, BillingModeVideo:
		if resolved.DefaultPerRequestPrice > 0 {
			return resolved.DefaultPerRequestPrice, true
		}
		if len(resolved.RequestTiers) == 1 && resolved.RequestTiers[0].PerRequestPrice != nil && *resolved.RequestTiers[0].PerRequestPrice >= 0 {
			return *resolved.RequestTiers[0].PerRequestPrice, true
		}
	}
	return 0, false
}

func errMediaPriceMissing() error {
	return infraerrors.New(http.StatusServiceUnavailable, "MEDIA_PRICE_NOT_CONFIGURED",
		"per-request price for media tasks is not configured")
}

// Reserve 在向上游建单之前冻结余额，返回冻结所用的单价快照。
//
// 顺序上先解析价格、再做订阅护栏、最后冻结：三步都在建单之前，任一步失败
// 都可以安全地把错误返回给客户端。
func (b *MediaTaskBilling) Reserve(ctx context.Context, task *MediaTaskRecord, apiKey *APIKey, user *User, account *Account) (float64, error) {
	if b == nil || b.gateway == nil || task == nil || apiKey == nil {
		return 0, ErrMediaTaskUnavailable
	}

	// 订阅护栏：两阶段冻结只作用于 users.balance，订阅额度没有冻结语义。
	// 静默按余额扣等于对已付费的订阅用户双重收费，因此 fail-closed 拒绝。
	if apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		return 0, ErrMediaSubscriptionUnsupported
	}

	unitPrice, err := b.resolveUnitPrice(ctx, task.Model, apiKey)
	if err != nil {
		return 0, err
	}
	if unitPrice <= 0 {
		// 单价为 0 视为未配置而非免费：免费出片必须是显式的产品决策，
		// 不能由一条缺失或写错的配置静默产生。
		return 0, errMediaPriceMissing()
	}

	cmd := &BatchImageBalanceHoldCommand{
		// RequestID 必须经既有助手生成：Release 内部会用
		// BatchImageHoldRequestID(cmd.BatchID) 反查 hold claim，
		// 自定义前缀会让释放被静默跳过、冻结额永久悬挂。
		RequestID:  BatchImageHoldRequestID(mediaHoldBatchID(task.ID)),
		APIKeyID:   apiKey.ID,
		UserID:     apiKey.UserID,
		BatchID:    mediaHoldBatchID(task.ID),
		HoldAmount: unitPrice,
	}
	if _, err := service.MediaBillingDepsOf(b.gateway).UsageBilling.ReserveBatchImageBalance(ctx, cmd); err != nil {
		return 0, mapMediaBillingError(err)
	}
	b.invalidateBalanceCache(ctx, apiKey.UserID)
	return unitPrice, nil
}

// Capture 在首次观察到终态成功且结果可下载时确认扣除。
//
// 落库顺序固定为「先装配 UsageLog → 再扣费 → 最后写库」。
func (b *MediaTaskBilling) Capture(ctx context.Context, task *MediaTaskRecord, apiKey *APIKey, user *User, account *Account) error {
	if b == nil || b.gateway == nil || task == nil || apiKey == nil || account == nil {
		return ErrMediaTaskUnavailable
	}
	// 用创建时的单价快照，不重新解析价格：capture 要求实扣不超过冻结额，
	// 管理员在任务在途期间调高价格会让重新解析的结果卡死结算。
	amount := task.UnitPrice
	if amount <= 0 {
		return errMediaPriceMissing()
	}

	usageLog := b.buildUsageLog(task, apiKey, account, amount)

	captureCmd := &BatchImageBalanceHoldCommand{
		RequestID:    BatchImageCaptureRequestID(mediaHoldBatchID(task.ID)),
		APIKeyID:     apiKey.ID,
		UserID:       apiKey.UserID,
		BatchID:      mediaHoldBatchID(task.ID),
		HoldAmount:   amount,
		ActualAmount: amount,
	}
	if _, err := service.MediaBillingDepsOf(b.gateway).UsageBilling.CaptureBatchImageBalance(ctx, captureCmd); err != nil {
		// 同一 RequestID 的指纹冲突说明此前已成功提交过一次确认，
		// 视为幂等成功，避免卡在毒消息循环里。
		if !errors.Is(err, ErrUsageBillingRequestConflict) {
			return mapMediaBillingError(err)
		}
	}

	b.applyNonBalanceDimensions(ctx, task, apiKey, account, amount, usageLog)
	service.WriteUsageLogForMedia(ctx, b.gateway, usageLog, "media.task_billing")
	return nil
}

// Release 在创建失败、任务失败或过期时退回冻结额。全额退回，不留手续费。
func (b *MediaTaskBilling) Release(ctx context.Context, task *MediaTaskRecord, apiKey *APIKey) error {
	if b == nil || b.gateway == nil || task == nil || apiKey == nil {
		return nil
	}
	if task.UnitPrice <= 0 {
		return nil
	}
	return b.ReleaseHold(ctx, &MediaPendingHold{
		TaskID:   task.ID,
		UserID:   apiKey.UserID,
		APIKeyID: apiKey.ID,
		Amount:   task.UnitPrice,
	})
}

// ReleaseHold 按冻结索引退回，供正常失败路径与过期兜底扫描共用。
//
// 单独一层是因为 sweeper 手里只有索引记录：任务过期时任务记录早已从 Redis
// 消失，拿不到完整的 MediaTaskRecord 与 APIKey。
func (b *MediaTaskBilling) ReleaseHold(ctx context.Context, hold *MediaPendingHold) error {
	if b == nil || b.gateway == nil || hold == nil || hold.Amount <= 0 {
		return nil
	}
	cmd := &BatchImageBalanceHoldCommand{
		RequestID:  BatchImageReleaseRequestID(mediaHoldBatchID(hold.TaskID)),
		APIKeyID:   hold.APIKeyID,
		UserID:     hold.UserID,
		BatchID:    mediaHoldBatchID(hold.TaskID),
		HoldAmount: hold.Amount,
	}
	if _, err := service.MediaBillingDepsOf(b.gateway).UsageBilling.ReleaseBatchImageBalance(ctx, cmd); err != nil {
		if errors.Is(err, ErrUsageBillingRequestConflict) {
			return nil
		}
		return mapMediaBillingError(err)
	}
	b.invalidateBalanceCache(ctx, hold.UserID)
	return nil
}

// buildUsageLog 自行装配用量行。
//
// 因为自行装配，video_resolution 与 video_duration_seconds 两列可以正常写入
// （service.UsageLog 本就有这两个字段），无需触碰共享的 ForwardResult。
func (b *MediaTaskBilling) buildUsageLog(task *MediaTaskRecord, apiKey *APIKey, account *Account, amount float64) *UsageLog {
	billingMode := string(BillingModeVideo)
	inbound := "/v1/media/videos"
	upstream := "seedance:/v1/videos"
	multiplier := 1.0

	log := &UsageLog{
		UserID:   apiKey.UserID,
		APIKeyID: apiKey.ID,
		// 账号固定为创建时绑定的那个：上游资源按账号隔离，用别的账号对账会错位。
		AccountID:        account.ID,
		RequestID:        mediaHoldBatchID(task.ID),
		Model:            task.Model,
		RequestedModel:   task.Model,
		InboundEndpoint:  &inbound,
		UpstreamEndpoint: &upstream,
		BillingType:      BillingTypeBalance,
		RequestType:      RequestTypeSync,
		BillingMode:      &billingMode,
		RateMultiplier:   multiplier,
		TotalCost:        amount,
		ActualCost:       amount,
		GroupID:          apiKey.GroupID,
		CreatedAt:        time.Now(),
	}
	if task.Resolution != "" {
		resolution := task.Resolution
		log.VideoResolution = &resolution
	}
	if task.DurationSeconds > 0 {
		// 真实时长直接落库，不经 NormalizeVideoBillingDurationSecondsOrDefault：
		// 那里的 15 秒上限会把 seedance2.5 的 30 秒截半。
		duration := task.DurationSeconds
		log.VideoDurationSeconds = &duration
	}
	return log
}

// applyNonBalanceDimensions 联动 API Key 配额、限流与账号配额。
//
// 刻意不复用 buildUsageBillingCommand / applyUsageBilling：前者在非订阅分支
// 必然从 ActualCost 推导 BalanceCost，而三个 shouldXxx 谓词同样以
// ActualCost > 0 为前提——无法做到「BalanceCost 为 0 但其余维度非 0」，
// 把 ActualCost 置 0 会让配额、限流、账号配额一起归零。余额已由 Capture 扣过，
// 这里再扣一次就是重复扣费。
//
// 同理不调用 finalizePostUsageBilling：其 syncBalanceCacheAfterDeduction 会
// QueueDeductBalance(ActualCost)，会让余额缓存再少一笔。余额缓存改用失效。
func (b *MediaTaskBilling) applyNonBalanceDimensions(
	ctx context.Context,
	task *MediaTaskRecord,
	apiKey *APIKey,
	account *Account,
	amount float64,
	usageLog *UsageLog,
) {
	repo := service.MediaBillingDepsOf(b.gateway).UsageBilling
	if repo == nil {
		return
	}

	cmd := &UsageBillingCommand{
		RequestID:   BatchImageCaptureRequestID(mediaHoldBatchID(task.ID)) + ":usage",
		APIKeyID:    apiKey.ID,
		UserID:      apiKey.UserID,
		AccountID:   account.ID,
		AccountType: account.Type,
		Model:       usageLog.Model,
		MediaType:   string(BillingModeVideo),
		BillingType: BillingTypeBalance,
		// BalanceCost 与 SubscriptionCost 一律留 0：余额已由 Capture 扣过。
		BalanceCost:      0,
		SubscriptionCost: 0,
	}
	if apiKey.Quota > 0 {
		cmd.APIKeyQuotaCost = amount
	}
	if apiKey.HasRateLimits() {
		cmd.APIKeyRateLimitCost = amount
	}
	if account.IsAPIKeyOrBedrock() && account.HasAnyQuotaLimit() {
		cmd.AccountQuotaCost = amount
	}
	// 三个维度都为 0 时没有必要产生一条扣费记录。
	if cmd.APIKeyQuotaCost == 0 && cmd.APIKeyRateLimitCost == 0 && cmd.AccountQuotaCost == 0 {
		b.invalidateBalanceCache(ctx, apiKey.UserID)
		return
	}
	cmd.Normalize()

	result, err := repo.Apply(ctx, cmd)
	if err != nil {
		logger.L().Error("media_billing.apply_non_balance_dimensions_failed",
			zap.String("task_id", task.ID),
			zap.Int64("user_id", apiKey.UserID),
			zap.Error(err))
		return
	}
	if result != nil && result.APIKeyQuotaExhausted {
		if invalidator := service.AuthCacheInvalidatorOf(b.apiKeyService); invalidator != nil && apiKey.Key != "" {
			invalidator.InvalidateAuthCacheByKey(ctx, apiKey.Key)
		}
	}
	if apiKey.HasRateLimits() && service.MediaBillingDepsOf(b.gateway).BillingCache != nil {
		service.MediaBillingDepsOf(b.gateway).BillingCache.QueueUpdateAPIKeyRateLimitUsage(apiKey.ID, amount)
	}
	if service.MediaBillingDepsOf(b.gateway).Deferred != nil {
		service.MediaBillingDepsOf(b.gateway).Deferred.ScheduleLastUsedUpdate(account.ID)
	}
	b.invalidateBalanceCache(ctx, apiKey.UserID)
}

// invalidateBalanceCache 让余额缓存失效而不是按增量扣减：真实余额由冻结/
// 确认/退回三个原语直接改动，缓存无法通过增量跟上。
func (b *MediaTaskBilling) invalidateBalanceCache(ctx context.Context, userID int64) {
	if b.gateway == nil || service.MediaBillingDepsOf(b.gateway).BillingCache == nil || userID <= 0 {
		return
	}
	if err := service.MediaBillingDepsOf(b.gateway).BillingCache.InvalidateUserBalance(ctx, userID); err != nil {
		logger.L().Warn("media_billing.invalidate_balance_cache_failed",
			zap.Int64("user_id", userID), zap.Error(err))
	}
}

// mapMediaBillingError 把冻结原语的错误转成媒体接口自有错误。
//
// 底层错误值带批量图片语义（ErrBatchImageInsufficientBalance 等），
// 直接透出会让用户看到与自己请求无关的 "batch image" 字样。
func mapMediaBillingError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrBatchImageInsufficientBalance):
		return infraerrors.New(http.StatusPaymentRequired, "MEDIA_INSUFFICIENT_BALANCE",
			"insufficient balance for this media task")
	case errors.Is(err, ErrBatchImageSettlementCostExceedsHold):
		return infraerrors.New(http.StatusInternalServerError, "MEDIA_SETTLEMENT_EXCEEDS_HOLD",
			"settlement amount exceeds the reserved amount")
	case errors.Is(err, ErrUserNotFound):
		return infraerrors.New(http.StatusUnauthorized, "MEDIA_USER_NOT_FOUND", "user not found")
	default:
		return infraerrors.New(http.StatusServiceUnavailable, "MEDIA_BILLING_UNAVAILABLE",
			"media billing is temporarily unavailable").WithCause(err)
	}
}
