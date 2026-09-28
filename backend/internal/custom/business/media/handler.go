package media

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// mediaCreateIdempotencyScopePrefix 是网关侧首个幂等 scope 的前缀。
//
// 调用方身份必须进 Scope 而不是只进 ActorScope：幂等记录的唯一键是
// (scope, idempotency_key_hash)，ActorScope 只参与请求指纹。若 Scope 对所有
// 用户相同，两个用户用同一个 Idempotency-Key 会命中同一条记录，因指纹不同
// 而互相返回 409——既拒绝了合法请求，也泄露了该键已被他人使用。
//
// 同理不能复用 executeUserIdempotentJSON：它的 ActorScope 取面板 JWT 主体，
// 网关请求会全部落到 user:0。
const mediaCreateIdempotencyScopePrefix = "gateway.media.videos.create"

// mediaCreateIdempotencyScope 按调用方隔离幂等作用域。
func mediaCreateIdempotencyScope(userID, apiKeyID int64) string {
	return mediaCreateIdempotencyScopePrefix + ":user:" + strconv.FormatInt(userID, 10) + ":key:" + strconv.FormatInt(apiKeyID, 10)
}

// MediaTaskHandler 提供与既有模型接口完全隔离的媒体任务链路。
//
// 刻意不挂到 GatewayHandler / OpenAIGatewayHandler 上：媒体任务有自己的门禁、
// 账号亲和与并发口径，独立 struct 让 S02 的回滚只需删文件加回退路由。
// mediaAccountResolver 抽出账号选择与取回两件事。
//
// *service.GatewayService 已天然满足本接口；抽成接口是为了让骨架能在没有
// 真实调度器的情况下用假实现完成端到端验收，不影响生产注入。
type mediaAccountResolver interface {
	SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*service.Account, error)
	GetMediaTaskAccount(ctx context.Context, accountID int64) (*service.Account, error)
}

type MediaTaskHandler struct {
	tasks             *MediaTaskService
	gatewayService    mediaAccountResolver
	concurrencyHelper *ConcurrencyHelper
	// 创建请求携带用户 prompt（上游允许 6000 字符），必须与其它带 prompt 的
	// 网关路由一样过内容审核协调器，不得绕开。
	securityAuditCoordinator *securityaudit.Coordinator
	contentModerationService *service.ContentModerationService
}

func NewMediaTaskHandler(
	tasks *MediaTaskService,
	gatewayService *service.GatewayService,
	concurrencyService *service.ConcurrencyService,
	securityAuditCoordinator *securityaudit.Coordinator,
	contentModerationService *service.ContentModerationService,
) *MediaTaskHandler {
	return &MediaTaskHandler{
		tasks:          tasks,
		gatewayService: gatewayService,
		// 媒体任务是短连接的建单/轮询/下载，不需要 SSE 心跳。
		concurrencyHelper:        NewConcurrencyHelper(concurrencyService, SSEPingFormatNone, 0),
		securityAuditCoordinator: securityAuditCoordinator,
		contentModerationService: contentModerationService,
	}
}

// checkSecurityAudit 让创建路径与其它带 prompt 的网关路由走同一个审核协调器。
// routes/prompt_audit_route_coverage_test.go 会强制每条网关 POST 路由要么在
// 此过审、要么显式声明「无 prompt」，本方法即该契约的落点。
func (h *MediaTaskHandler) checkSecurityAudit(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	model string,
	body []byte,
) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	// 复用 openai_images 协议：其抽取器按 prompt + images 取内容，
	// 与媒体创建请求的形状一致，无需新增审核管线。
	return handler.RunSecurityAuditForMedia(c, reqLog, h.securityAuditCoordinator, h.contentModerationService,
		apiKey, subject, service.ContentModerationProtocolOpenAIImages, model, body, "http")
}

// enabled 决定是否接受新建任务与上传。
func (h *MediaTaskHandler) enabled() bool {
	return h != nil && h.tasks != nil && h.tasks.Enabled() && h.gatewayService != nil
}

// pollable 弱于 enabled：总开关关闭后在途任务仍可查询与下载，
// 避免已冻结余额的任务被搁死。
func (h *MediaTaskHandler) pollable() bool {
	return h != nil && h.tasks != nil && h.tasks.Pollable()
}

// resolveMediaAPIKey 取出 API Key 并校验分组平台归属，返回 infraerror。
//
// 抽成返回 error 而非直接写响应，是为了让不同入口复用同一套鉴权判定，
// 各自按自己的信封形状渲染错误。
func (h *MediaTaskHandler) resolveMediaAPIKey(c *gin.Context) (*service.APIKey, error) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.UserID <= 0 || apiKey.ID <= 0 {
		return nil, infraerrors.New(http.StatusUnauthorized, "authentication_error", "invalid API key")
	}
	platform := ""
	if apiKey.Group != nil {
		platform = apiKey.Group.Platform
	}
	// 平台不匹配时的 404 与既有 /v1/videos 对非支持平台的口径一致。
	// 平台取自 service 而非 provider：provider 未接入时仍需允许在途任务轮询。
	if h.tasks == nil || platform != h.tasks.Platform() {
		return nil, infraerrors.New(http.StatusNotFound, "not_found_error", "Media API is not supported for this platform")
	}
	return apiKey, nil
}

// mediaAuth 取出 API Key 并校验分组平台归属，失败时渲染 media 形状错误。
func (h *MediaTaskHandler) mediaAuth(c *gin.Context) (*service.APIKey, bool) {
	apiKey, err := h.resolveMediaAPIKey(c)
	if err != nil {
		mediaError(c, err)
		return nil, false
	}
	return apiKey, true
}

// Models 渲染 provider 声明的能力表。不调用上游，无额外鉴权开销。
func (h *MediaTaskHandler) Models(c *gin.Context) {
	if h == nil || h.tasks == nil || h.tasks.Provider() == nil {
		mediaError(c, ErrMediaTaskDisabled)
		return
	}
	if _, ok := h.mediaAuth(c); !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, h.tasks.Provider().Capabilities())
}

type mediaCreateRequest struct {
	Prompt         string   `json:"prompt"`
	Model          string   `json:"model"`
	Duration       int      `json:"duration"`
	Ratio          string   `json:"ratio"`
	Resolution     string   `json:"resolution"`
	CameraMovement string   `json:"camera_movement"`
	FileIDs        []string `json:"file_ids"`
	ImageURL       string   `json:"image_url"`
	ImageURLs      []string `json:"image_urls"`
}

// CreateVideo 创建视频任务并返回 202。
//
// 执行顺序不得调换：入参校验 → 订阅护栏 → 账号选定（受参考图亲和约束）
// → 冻结 → 上游建单。前四步失败时尚未建单，可安全返回错误；上游建单成功
// 之后，闭包绝不返回 error（见下方注释）。
func (h *MediaTaskHandler) CreateVideo(c *gin.Context) {
	if !h.enabled() {
		mediaError(c, ErrMediaTaskDisabled)
		return
	}
	apiKey, ok := h.mediaAuth(c)
	if !ok {
		return
	}

	// 幂等键在 handler 层强制校验，不依赖 coordinator 的 RequireKey：
	// IdempotencyConfig.ObserveOnly 默认为 true，会让缺 Key 的请求直接放行。
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" {
		mediaJSONError(c, http.StatusBadRequest, "invalid_request_error", "Idempotency-Key header is required")
		return
	}

	// 先取原始 body：内容审核需要原文，ShouldBindJSON 会消费掉 Body。
	rawBody, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil || len(rawBody) == 0 {
		mediaJSONError(c, http.StatusBadRequest, "invalid_request_error", "invalid request body")
		return
	}
	var req mediaCreateRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		mediaJSONError(c, http.StatusBadRequest, "invalid_request_error", "invalid request body")
		return
	}

	subject, _ := middleware2.GetAuthSubjectFromContext(c)

	imageURLs := req.ImageURLs
	if strings.TrimSpace(req.ImageURL) != "" {
		imageURLs = append(imageURLs, req.ImageURL)
	}
	createReq := &MediaVideoCreateRequest{
		Prompt:          req.Prompt,
		Model:           strings.TrimSpace(req.Model),
		DurationSeconds: req.Duration,
		Ratio:           strings.TrimSpace(req.Ratio),
		Resolution:      strings.TrimSpace(req.Resolution),
		CameraMovement:  strings.TrimSpace(req.CameraMovement),
		FileIDs:         req.FileIDs,
		ImageURLs:       imageURLs,
	}

	data, replayed, err := h.submitVideoCreate(c, apiKey, subject, rawBody, createReq, req, idempotencyKey)
	if err != nil {
		if retryAfter := service.RetryAfterSecondsFromError(err); retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		mediaError(c, err)
		return
	}
	if replayed {
		c.Header("X-Idempotency-Replayed", "true")
	}
	// 初次与重放都返回 202，且不套 {code,message,data} 信封——与同类视频接口
	// 的上游语义保持一致。coordinator 记录里的 200 是其内部字段，不对外。
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusAccepted, data)
}

// submitVideoCreate 是创建视频任务的形状无关编排：内容审核 → provider 校验 →
// 参考图解析(账号亲和) → 幂等协调器执行 executeCreate。
//
// 各入口只在请求解析(字段名)与响应渲染(信封形状)上可能不同，编排逻辑共用
// 本方法。所有失败都以 infraerror 返回，由各入口按自己的信封渲染。
//
// payload 由调用方传入用于幂等指纹：两个入口各自传自己解析出的原始请求结构，
// 保证同一入口的重试指纹稳定，不因本次重构而漂移。
// 返回的 data 在初次执行时是 *MediaTask，重放时是 coordinator 从存储反序列化出
// 的 map[string]any（两者 JSON 形状一致）。调用方渲染时直接透传该值，或用
// mediaTaskIdentity 从中取 task_id / status，不得假设它是 *MediaTask。
func (h *MediaTaskHandler) submitVideoCreate(
	c *gin.Context,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	rawBody []byte,
	createReq *MediaVideoCreateRequest,
	payload any,
	idempotencyKey string,
) (data any, replayed bool, err error) {
	reqLog := logger.L().With(
		zap.Int64("user_id", apiKey.UserID),
		zap.Int64("api_key_id", apiKey.ID),
	)
	// 审核在冻结余额与调用上游之前完成：被拦截的请求不应产生任何资金动作。
	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, createReq.Model, rawBody); decision != nil && !decision.AllowNextStage {
		return nil, false, infraerrors.New(
			handler.SecurityAuditStatusForMedia(decision),
			handler.SecurityAuditErrorCodeForMedia(decision),
			handler.SecurityAuditMessageForMedia(decision),
		)
	}
	if verr := h.tasks.Provider().ValidateCreate(createReq); verr != nil {
		return nil, false, verr
	}

	ctx := c.Request.Context()

	// 参考图的上游 image_id 按账号隔离，因此引用参考图的创建必须锁定到
	// 图片所属账号；跨账号引用直接拒绝。
	upstreamImageIDs, pinnedAccountID, resolveErr := h.tasks.ResolveFiles(ctx, createReq.FileIDs, apiKey.UserID, apiKey.ID)
	if resolveErr != nil {
		return nil, false, resolveErr
	}

	coordinator := service.DefaultIdempotencyCoordinator()
	if coordinator == nil {
		// fail-closed：幂等设施不可用时不降级为直接建单，否则重试会产生重复任务。
		return nil, false, ErrMediaTaskUnavailable
	}

	actorScope := "user:" + strconv.FormatInt(apiKey.UserID, 10) + ":key:" + strconv.FormatInt(apiKey.ID, 10)
	result, execErr := coordinator.Execute(ctx, service.IdempotencyExecuteOptions{
		Scope:          mediaCreateIdempotencyScope(apiKey.UserID, apiKey.ID),
		ActorScope:     actorScope,
		Method:         c.Request.Method,
		Route:          c.FullPath(),
		IdempotencyKey: idempotencyKey,
		Payload:        payload,
		RequireKey:     true,
		TTL:            h.tasks.TaskTTL(),
	}, func(execCtx context.Context) (any, error) {
		return h.executeCreate(execCtx, c, apiKey, createReq, upstreamImageIDs, pinnedAccountID, idempotencyKey)
	})
	if execErr != nil {
		return nil, false, execErr
	}
	replayed = result != nil && result.Replayed
	if result != nil {
		data = result.Data
	}
	return data, replayed, nil
}

// mediaTaskIdentity 从 submitVideoCreate 返回的 data 中取 task_id / status，
// 兼容初次执行的 *MediaTask 与重放时的 map[string]any（经 JSON 归一）。
func mediaTaskIdentity(data any) (taskID, status string) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", ""
	}
	var out struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", ""
	}
	return out.TaskID, out.Status
}

// executeCreate 是幂等闭包体。
//
// 硬契约：一旦上游建单成功，本函数绝不返回 error。coordinator 对返回 error 的
// 闭包会 MarkFailedRetryable，过 backoff 后 TryReclaim 会重新执行，产生第二个
// 上游任务。因此建单之后的任何写入失败都降级为成功返回 + 可检索告警。
func (h *MediaTaskHandler) executeCreate(
	ctx context.Context,
	c *gin.Context,
	apiKey *service.APIKey,
	req *MediaVideoCreateRequest,
	upstreamImageIDs []string,
	pinnedAccountID int64,
	idempotencyKey string,
) (any, error) {
	reqLog := logger.L().With(
		zap.Int64("user_id", apiKey.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.String("model", req.Model),
	)

	account, err := h.selectAccount(ctx, apiKey, req.Model, pinnedAccountID)
	if err != nil {
		return nil, err
	}

	// 账号槽位在建单期间短时占用；wrapReleaseOnDone 保证客户端断开或超时
	// 也会释放，不泄漏计数。
	if release := h.acquireAccountSlot(ctx, account); release != nil {
		defer release()
	}

	task := &MediaTaskRecord{
		ID:              NewMediaID(),
		UserID:          apiKey.UserID,
		APIKeyID:        apiKey.ID,
		AccountID:       account.ID,
		GroupID:         apiKey.GroupID,
		Platform:        h.tasks.Provider().Platform(),
		Model:           req.Model,
		DurationSeconds: req.DurationSeconds,
		Resolution:      req.Resolution,
		Ratio:           req.Ratio,
		CameraMovement:  req.CameraMovement,
		FileIDs:         req.FileIDs,
		Status:          MediaTaskStatusQueued,
		CreatedAt:       time.Now().Unix(),
	}

	// 冻结发生在建单之前：余额不足时上游不会收到任何请求。
	if billing := h.tasks.Billing(); billing != nil {
		unitPrice, resErr := billing.Reserve(ctx, task, apiKey, apiKey.User, account)
		if resErr != nil {
			return nil, resErr
		}
		task.UnitPrice = unitPrice
		// 登记兜底索引：任务过期时没人会来轮询，这条索引是唯一能让
		// 悬挂的冻结额被退回的线索。登记失败只告警不阻断——此时上游
		// 尚未建单，阻断会把一次本可成功的创建变成失败。
		if trackErr := h.tasks.TrackHold(ctx, task); trackErr != nil {
			reqLog.Error("media.track_pending_hold_failed",
				zap.String("task_id", task.ID), zap.Error(trackErr))
		}
	}

	job, createErr := h.tasks.Provider().Create(ctx, account, req, upstreamImageIDs, idempotencyKey)
	if createErr != nil {
		// 尚未建单，退回冻结额并把错误如实返回；同一 Key 重试是安全的。
		if billing := h.tasks.Billing(); billing != nil {
			if relErr := billing.Release(ctx, task, apiKey); relErr != nil {
				reqLog.Error("media.create_release_hold_failed",
					zap.String("task_id", task.ID), zap.Error(relErr))
			}
		}
		h.alertUpstreamBalanceExhausted(createErr, account, apiKey)
		return nil, createErr
	}

	// ==== 以下为「上游已建单」区间，任何失败都不得返回 error ====
	task.JobID = job.JobID
	task.UpstreamIdempotent = job.Idempotent
	if st := NormalizeMediaStatus(job.Status); st != "" {
		task.Status = st
	}

	if saveErr := h.tasks.Store().SaveTask(ctx, task, h.tasks.TaskTTL()); saveErr != nil {
		// 任务记录写失败意味着后续无法按 task_id 查询与结算，且冻结额会悬挂。
		// 告警必须带齐 task_id / job_id / hold 标识，供人工核对与释放。
		reqLog.Error("media.task_record_write_failed_after_upstream_create",
			zap.String("task_id", task.ID),
			zap.String("upstream_job_id", task.JobID),
			zap.String("hold_request_id", "media:"+task.ID),
			zap.Error(saveErr),
		)
	}
	if c != nil {
		_ = c // 保留 gin 上下文入参以便后续接入 ops 打点，不参与控制流
	}
	return task.PublicTask(false), nil
}

// acquireAccountSlot 获取账号并发槽位。
//
// ConcurrencyHelper.TryAcquireAccountSlot 不对 nil 并发服务做保护，会直接
// 空指针；本方法在服务缺失时降级为不占槽而不是崩溃。取不到槽位同样降级放行：
// 媒体任务是短连接建单，槽位只用于削峰，不作为准入门禁。
func (h *MediaTaskHandler) acquireAccountSlot(ctx context.Context, account *service.Account) func() {
	if h == nil || !handler.ConcurrencyReady(h.concurrencyHelper) || account == nil {
		return nil
	}
	release, acquired, err := h.concurrencyHelper.TryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
	if err != nil || !acquired {
		return nil
	}
	return release
}

// alertUpstreamBalanceExhausted 在上游返回 402 时打运营告警。
//
// 上游账号余额耗尽是运营事件而不是用户错误：用户侧只会看到出不了片，
// 没有这条告警运维无从知晓该补充上游额度。
func (h *MediaTaskHandler) alertUpstreamBalanceExhausted(err error, account *service.Account, apiKey *service.APIKey) {
	if err == nil || infraerrors.Reason(err) != infraerrors.Reason(ErrSeedanceInsufficientBalance) {
		return
	}
	logger.L().Error("ALERT media.upstream_account_balance_exhausted",
		zap.Int64("account_id", account.ID),
		zap.String("platform", account.Platform),
		zap.Int64("user_id", apiKey.UserID),
		zap.String("action", "top up the upstream account credits"),
	)
}

// selectAccount 选定上游账号。
//
// 带参考图时账号被锁定为图片所属账号且不允许更换——上游资源按账号隔离，
// 换账号必然 404，因此该账号不可用时直接失败而不是 failover。
func (h *MediaTaskHandler) selectAccount(ctx context.Context, apiKey *service.APIKey, model string, pinnedAccountID int64) (*service.Account, error) {
	if pinnedAccountID > 0 {
		account, err := h.gatewayService.GetMediaTaskAccount(ctx, pinnedAccountID)
		if err != nil || account == nil {
			return nil, ErrMediaAccountUnavailable
		}
		return account, nil
	}
	account, err := h.gatewayService.SelectAccountForModelWithExclusions(ctx, apiKey.GroupID, "", model, nil)
	if err != nil || account == nil {
		return nil, ErrMediaAccountUnavailable
	}
	return account, nil
}

// GetVideo 查询任务状态。状态与下载固定使用创建时绑定的账号。
func (h *MediaTaskHandler) GetVideo(c *gin.Context) {
	if !h.pollable() {
		mediaError(c, ErrMediaTaskUnavailable)
		return
	}
	apiKey, ok := h.mediaAuth(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	task, err := h.tasks.ResolveTask(ctx, c.Param("task_id"), apiKey.UserID, apiKey.ID)
	if err != nil {
		mediaError(c, err)
		return
	}

	downloadable, err := h.refreshTaskStatus(ctx, apiKey, task)
	if err != nil {
		mediaError(c, err)
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, task.PublicTask(downloadable))
}

// refreshTaskStatus 向上游取最新状态并落库，成功且就绪时结算、失败时退款。
// 就地更新 task（Status / Error / CompletedAt），返回结果是否可下载。
//
// 形状无关，供各入口共用。provider 未接入时返回
// (false, nil)——pollable 的含义是「任务不被搁死」，不是「仍能与上游通信」，
// 调用方据此返回记录中的最新已知状态。未知上游状态一律 fail-closed。
func (h *MediaTaskHandler) refreshTaskStatus(ctx context.Context, apiKey *service.APIKey, task *MediaTaskRecord) (bool, error) {
	if h.tasks.Provider() == nil {
		return false, nil
	}

	account, err := h.gatewayService.GetMediaTaskAccount(ctx, task.AccountID)
	if err != nil || account == nil {
		return false, ErrMediaAccountUnavailable
	}

	job, err := h.tasks.Provider().Status(ctx, account, task.JobID)
	if err != nil {
		return false, err
	}
	status := NormalizeMediaStatus(job.Status)
	if status == "" {
		// 未知状态 fail-closed：不猜测映射，避免把未完成任务当成终态结算。
		return false, infraerrors.New(http.StatusBadGateway, "upstream_error", "unsupported upstream task status")
	}
	task.Status = status
	task.Error = job.Error
	if IsTerminalMediaStatus(status) && task.CompletedAt == nil {
		now := time.Now().Unix()
		task.CompletedAt = &now
	}
	_ = h.tasks.Store().SaveTask(ctx, task, h.tasks.TaskTTL())

	// 成功态还需确认结果确实可下载：上游对未就绪返回 409，此时仍报成功
	// 但标记暂不可下载，且不进入结算。
	downloadable := false
	if status == MediaTaskStatusSucceeded {
		if ref, refErr := h.tasks.Provider().FetchDownloadRef(ctx, account, task.JobID); refErr == nil && ref != nil && !ref.NotReady {
			downloadable = true
			h.settle(ctx, task, apiKey, account)
		}
	}
	if status == MediaTaskStatusFailed {
		h.releaseHold(ctx, task, apiKey)
	}
	return downloadable, nil
}

// GetVideoContent 透传下载，支持 Range。签名地址每次重新获取，不持久化。
func (h *MediaTaskHandler) GetVideoContent(c *gin.Context) {
	if !h.pollable() {
		mediaError(c, ErrMediaTaskUnavailable)
		return
	}
	apiKey, ok := h.mediaAuth(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	task, err := h.tasks.ResolveTask(ctx, c.Param("task_id"), apiKey.UserID, apiKey.ID)
	if err != nil {
		mediaError(c, err)
		return
	}

	account, ref, err := h.fetchDownloadForTask(ctx, apiKey, task)
	if err != nil {
		mediaError(c, err)
		return
	}
	if ref == nil || ref.NotReady {
		mediaJSONError(c, http.StatusConflict, "not_ready", "media result is not ready yet")
		return
	}
	if streamErr := h.streamDownload(c, account, ref); streamErr != nil {
		mediaError(c, streamErr)
	}
}

// fetchDownloadForTask 取任务的短期下载引用，就绪时顺带结算一次。
//
// 形状无关，供 /v1/media/content 等下载入口共用。
// 结果未就绪时返回 ref.NotReady=true（不产生结算），由调用方决定 409；
// provider 缺失时 fail-closed。
func (h *MediaTaskHandler) fetchDownloadForTask(ctx context.Context, apiKey *service.APIKey, task *MediaTaskRecord) (*service.Account, *MediaDownloadRef, error) {
	// 下载必须与上游通信，provider 缺失时只能 fail-closed。
	if h.tasks.Provider() == nil {
		return nil, nil, ErrMediaTaskUnavailable
	}
	account, err := h.gatewayService.GetMediaTaskAccount(ctx, task.AccountID)
	if err != nil || account == nil {
		return nil, nil, ErrMediaAccountUnavailable
	}
	ref, err := h.tasks.Provider().FetchDownloadRef(ctx, account, task.JobID)
	if err != nil {
		return nil, nil, err
	}
	if ref == nil || ref.NotReady {
		return account, &MediaDownloadRef{NotReady: true}, nil
	}
	// 下载路径也可能是首个观察到「成功且可下载」的入口，与轮询竞争同一 claim。
	h.settle(ctx, task, apiKey, account)
	return account, ref, nil
}

// streamDownload 透传下载字节，支持 Range；成功时写响应头与状态码并拷贝正文，
// 建单前失败以 error 返回交由各入口按自己的信封渲染。
func (h *MediaTaskHandler) streamDownload(c *gin.Context, account *service.Account, ref *MediaDownloadRef) error {
	resp, err := h.tasks.Provider().Download(c.Request.Context(), account, ref, c.GetHeader("Range"))
	if err != nil {
		return err
	}
	if resp.Close != nil {
		defer func() { _ = resp.Close() }()
	}
	for k, vs := range resp.Header {
		for _, v := range vs {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.Header().Set("Cache-Control", "no-store")
	c.Status(resp.StatusCode)
	if resp.Body != nil {
		_, _ = io.Copy(c.Writer, resp.Body)
	}
	return nil
}

// UploadFile 上传参考图，换取 SUB2 侧 file id。
//
// 上传不计费也不做幂等：重复上传只多占一份短期存储，不值得引入第二个幂等 scope。
func (h *MediaTaskHandler) UploadFile(c *gin.Context) {
	if !h.enabled() {
		mediaError(c, ErrMediaTaskDisabled)
		return
	}
	apiKey, ok := h.mediaAuth(c)
	if !ok {
		return
	}
	var body struct {
		ImageB64 string `json:"image_b64"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.ImageB64) == "" {
		mediaJSONError(c, http.StatusBadRequest, "invalid_request_error", "image_b64 is required")
		return
	}

	rec, err := h.uploadReferenceFile(c.Request.Context(), apiKey, body.ImageB64)
	if err != nil {
		mediaError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, rec.PublicFile())
}

// uploadReferenceFile 是上传参考图的形状无关编排：选账号 → 占槽 → 上传上游
// → 落库（TTL 不超过上游有效期）。供各上传入口共用；失败以 infraerror 返回。
func (h *MediaTaskHandler) uploadReferenceFile(ctx context.Context, apiKey *service.APIKey, imageB64 string) (*MediaFileRecord, error) {
	account, err := h.gatewayService.SelectAccountForModelWithExclusions(ctx, apiKey.GroupID, "", "", nil)
	if err != nil || account == nil {
		return nil, ErrMediaAccountUnavailable
	}

	// 上传同样占用账号槽位，避免被当作免费图床刷量。
	if release := h.acquireAccountSlot(ctx, account); release != nil {
		defer release()
	}

	uploaded, err := h.tasks.Provider().UploadReferenceImage(ctx, account, imageB64)
	if err != nil {
		return nil, err
	}

	rec := &MediaFileRecord{
		ID:        NewMediaID(),
		UserID:    apiKey.UserID,
		APIKeyID:  apiKey.ID,
		AccountID: account.ID,
		Platform:  h.tasks.Provider().Platform(),
		ImageID:   uploaded.ImageID,
		Format:    uploaded.Format,
		Size:      uploaded.Size,
		ExpiresAt: uploaded.ExpiresAt,
		CreatedAt: time.Now().Unix(),
	}
	// 本地 TTL 不得超过上游有效期：上游过期后引用它必然失败。
	ttl := h.tasks.TaskTTL()
	if rec.ExpiresAt > 0 {
		if remain := time.Until(time.Unix(rec.ExpiresAt, 0)); remain > 0 && remain < ttl {
			ttl = remain
		}
	}
	if err := h.tasks.Store().SaveFile(ctx, rec, ttl); err != nil {
		return nil, ErrMediaTaskUnavailable.WithCause(err)
	}
	return rec, nil
}

// settle 在首次观察到「成功且可下载」时结算一次。claim 保证轮询与下载并发
// 时只有一方进入扣费；失败则释放 claim 以便下次重试。
func (h *MediaTaskHandler) settle(ctx context.Context, task *MediaTaskRecord, apiKey *service.APIKey, account *service.Account) {
	billing := h.tasks.Billing()
	if billing == nil {
		return
	}
	won, err := h.tasks.Store().ClaimSettlement(ctx, task.ID, h.tasks.ClaimTTL())
	if err != nil || !won {
		return
	}
	if err := billing.Capture(ctx, task, apiKey, apiKey.User, account); err == nil {
		// 冻结额已结清，清掉兜底索引避免 sweeper 重复处理。
		h.tasks.UntrackHold(ctx, task.ID)
	} else {
		if relErr := h.tasks.Store().ReleaseSettlement(ctx, task.ID); relErr != nil {
			logger.L().Error("media.settlement_claim_release_failed",
				zap.String("task_id", task.ID), zap.Error(relErr))
		}
		logger.L().Error("media.settlement_failed",
			zap.String("task_id", task.ID),
			zap.String("upstream_job_id", task.JobID),
			zap.Error(err))
	}
}

// releaseHold 在任务失败时退回冻结额。全额退回，不留手续费。
func (h *MediaTaskHandler) releaseHold(ctx context.Context, task *MediaTaskRecord, apiKey *service.APIKey) {
	billing := h.tasks.Billing()
	if billing == nil {
		return
	}
	// 复用结算 claim 做去重：失败任务只退一次。
	won, err := h.tasks.Store().ClaimSettlement(ctx, task.ID, h.tasks.ClaimTTL())
	if err != nil || !won {
		return
	}
	if err := billing.Release(ctx, task, apiKey); err == nil {
		h.tasks.UntrackHold(ctx, task.ID)
	} else {
		_ = h.tasks.Store().ReleaseSettlement(ctx, task.ID)
		logger.L().Error("media.release_hold_failed",
			zap.String("task_id", task.ID),
			zap.String("upstream_job_id", task.JobID),
			zap.Error(err))
	}
}

func mediaError(c *gin.Context, err error) {
	status := infraerrors.Code(err)
	code := infraerrors.Reason(err)
	message := infraerrors.Message(err)
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	if strings.TrimSpace(code) == "" {
		code = "MEDIA_TASK_ERROR"
	}
	mediaJSONError(c, status, code, message)
}

func mediaJSONError(c *gin.Context, status int, code, message string) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, gin.H{"error": gin.H{"type": code, "code": code, "message": message}})
}
