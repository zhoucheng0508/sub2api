package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type MediaVideoHandler struct {
	videos *service.MediaVideoService
	openAI *OpenAIGatewayHandler
}

func NewMediaVideoHandler(v *service.MediaVideoService) *MediaVideoHandler {
	return &MediaVideoHandler{videos: v}
}

func ProvideMediaVideoHandler(v *service.MediaVideoService, openAI *OpenAIGatewayHandler) *MediaVideoHandler {
	h := NewMediaVideoHandler(v)
	h.openAI = openAI
	return h
}

// Billing reads authoritative wallet amounts; it never submits a provider job.
func (h *MediaVideoHandler) Billing(c *gin.Context) {
	api, subject, ok := h.auth(c)
	if !ok {
		return
	}
	available, frozen, err := h.videos.Balance(c.Request.Context(), subject.UserID)
	if err != nil {
		h.err(c, 503, "balance is temporarily unavailable")
		return
	}
	c.Header("Cache-Control", "no-store")
	price := service.ResolveMediaVideoPrice(api.Group)
	c.JSON(200, gin.H{"currency": "internal", "balance": available, "frozen_balance": frozen, "video_price_per_request": price, "can_create": available >= price})
}
func (h *MediaVideoHandler) Create(c *gin.Context) {
	api, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || api == nil {
		h.err(c, 401, "invalid api key")
		return
	}
	sub, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		h.err(c, 401, "invalid user context")
		return
	}
	var req service.MediaVideoCreateRequest
	if c.ShouldBindJSON(&req) != nil {
		h.err(c, 400, "invalid JSON body")
		return
	}
	if !service.GroupAllowsImageGeneration(api.Group) {
		h.err(c, 403, "Video generation is not enabled for this group")
		return
	}
	readOnly := c.GetHeader("X-Media-Video-Replay-Only") == "true"
	old, replayStatus, replayErr := h.videos.Replay(c.Request.Context(), sub.UserID, api.ID, c.GetHeader("Idempotency-Key"), req)
	if replayErr != nil {
		if readOnly || replayStatus != 404 {
			h.err(c, replayStatus, replayErr.Error())
			return
		}
	} else if readOnly || old.Status != "creating" || old.UpstreamTaskID != "" || old.SubmissionState == "submitting" || old.SubmissionState == "uncertain" {
		c.JSON(200, taskJSON(old))
		return
	}
	// Only a new order or a prepared submission needs current spending access.
	if !middleware.CheckMediaVideoAdmission(c) {
		return
	}
	if h.openAI == nil {
		h.err(c, 503, "Video request audit is unavailable")
		return
	}
	moderationBody, _ := json.Marshal(map[string]string{"model": req.Model, "prompt": req.Prompt})
	decision := h.openAI.checkSecurityAudit(c, requestLogger(c, "handler.media_video.security_audit"), api, sub, service.ContentModerationProtocolOpenAIImages, req.Model, moderationBody)
	if decision != nil && !decision.AllowNextStage {
		h.openAI.openAISecurityAuditError(c, decision)
		return
	}
	beforeSubmit := func(account *service.Account) (func(), error) {
		decision := h.openAI.checkSecurityAuditForAccount(c, requestLogger(c, "handler.media_video.account_audit"), api, sub, account, service.ContentModerationProtocolOpenAIImages, req.Model, moderationBody)
		if decision != nil && !decision.AllowNextStage {
			h.openAI.openAISecurityAuditError(c, decision)
			return nil, errors.New("video request audit rejected")
		}
		if h.openAI.concurrencyHelper == nil {
			return nil, errors.New("video concurrency control is unavailable")
		}
		userRelease, acquired, err := h.openAI.concurrencyHelper.TryAcquireUserSlotForAPIKey(c.Request.Context(), sub.UserID, sub.Concurrency, api.ID)
		if err != nil {
			return nil, err
		}
		if !acquired {
			return nil, errors.New("user concurrency limit reached")
		}
		accountRelease, acquired, err := h.openAI.concurrencyHelper.TryAcquireAccountSlot(c.Request.Context(), account.ID, account.Concurrency)
		if err != nil || !acquired {
			userRelease()
			if err != nil {
				return nil, err
			}
			return nil, errors.New("account concurrency limit reached")
		}
		return func() { accountRelease(); userRelease() }, nil
	}
	t, status, e := h.videos.Create(c.Request.Context(), sub.UserID, api.ID, api.Group, c.GetHeader("Idempotency-Key"), req, beforeSubmit)
	if e != nil {
		if c.Writer.Written() {
			return
		}
		if errors.Is(e, service.ErrLaogouInvalidCursor) {
			h.err(c, 400, e.Error())
			return
		}
		h.err(c, status, e.Error())
		return
	}
	c.JSON(status, taskJSON(t))
}
func (h *MediaVideoHandler) List(c *gin.Context) {
	api, sub, ok := h.auth(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := c.DefaultQuery("status", "queued,running")
	cursor := c.Query("cursor")
	ts, hasMore, nextCursor, e := h.videos.List(c.Request.Context(), sub.UserID, api.ID, limit, status, cursor)
	if e != nil {
		h.err(c, 500, e.Error())
		return
	}
	data := make([]any, 0, len(ts))
	for _, t := range ts {
		data = append(data, taskJSON(t))
	}
	c.JSON(200, gin.H{"object": "list", "data": data, "has_more": hasMore, "next_cursor": nextCursor})
}
func (h *MediaVideoHandler) Get(c *gin.Context) {
	api, sub, ok := h.auth(c)
	if !ok {
		return
	}
	t, e := h.videos.Get(c.Request.Context(), c.Param("task_id"), sub.UserID, api.ID)
	if e != nil {
		h.err(c, 404, "video task not found")
		return
	}
	c.JSON(200, taskJSON(t))
}
func (h *MediaVideoHandler) Content(c *gin.Context) {
	api, sub, ok := h.auth(c)
	if !ok {
		return
	}
	t, e := h.videos.Get(c.Request.Context(), c.Param("task_id"), sub.UserID, api.ID)
	if e != nil || t == nil || !t.Downloadable {
		h.err(c, 404, "video content not available")
		return
	}
	status, e := h.videos.Content(c.Request.Context(), t, c.Writer, c.Request)
	if e != nil {
		if c.Writer.Written() {
			// Normal return would terminate chunked/H2 responses successfully,
			// even when only part of a video was delivered. Let net/http abort
			// the connection or stream after the service has released resources.
			panic(http.ErrAbortHandler)
		}
		// A stream can fail before its first byte (for example if the shared
		// limiter becomes unavailable). Do not send JSON with video length/range
		// headers or accidentally report a successful upstream 200/206 status.
		for _, name := range []string{"Content-Length", "Content-Range", "Content-Disposition", "Accept-Ranges"} {
			c.Writer.Header().Del(name)
		}
		if errors.Is(e, service.ErrMediaVideoDownloadUnavailable) {
			status = 503
			c.Header("Retry-After", "5")
		} else if status < 400 {
			status = 502
		}
		c.Writer.Header().Del("Content-Type")
		h.err(c, status, e.Error())
	}
}
func (h *MediaVideoHandler) Models(c *gin.Context) {
	api, _, ok := h.auth(c)
	if !ok {
		return
	}
	groupID := api.GroupID
	b, e := h.videos.Models(c.Request.Context(), groupID)
	if e != nil {
		h.err(c, 503, e.Error())
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Data(200, "application/json", b)
}
func (h *MediaVideoHandler) auth(c *gin.Context) (*service.APIKey, middleware.AuthSubject, bool) {
	api, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || api == nil {
		h.err(c, 401, "invalid api key")
		return nil, middleware.AuthSubject{}, false
	}
	sub, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		h.err(c, 401, "invalid user context")
		return nil, middleware.AuthSubject{}, false
	}
	return api, sub, true
}
func (h *MediaVideoHandler) err(c *gin.Context, status int, msg string) {
	if status <= 0 {
		status = 500
	}
	typ := "invalid_request_error"
	if status == 401 {
		typ = "authentication_error"
	} else if status == 404 {
		typ = "not_found_error"
	} else if status == 409 {
		typ = "conflict_error"
	} else if status >= 500 {
		typ = "upstream_error"
	}
	c.JSON(status, gin.H{"error": gin.H{"type": typ, "message": msg, "code": "VIDEO_ERROR"}})
}
func taskJSON(t *service.MediaVideoTask) gin.H {
	// The original request must never be replayed while its provider outcome
	// is unknown; this flag makes the required operator action visible.
	v := gin.H{"task_id": t.TaskID, "object": "media.video_task", "status": t.Status, "model": t.Model, "duration_seconds": t.Duration, "ratio": t.Ratio, "resolution": t.Resolution, "has_images": t.HasImages, "downloadable": t.Downloadable, "price": t.PriceSnapshot, "currency": t.Currency, "billing_status": t.BillingStatus, "created_at": unix(t.CreatedAt), "updated_at": unix(t.UpdatedAt), "expires_at": unix(t.ExpiresAt)}
	v["reconciliation_required"] = t.SubmissionState == "uncertain"
	if t.Progress != nil {
		v["progress"] = *t.Progress
	} else {
		v["progress"] = nil
	}
	if t.CompletedAt != nil {
		v["completed_at"] = unix(t.CompletedAt)
	}
	if len(t.Error) > 0 {
		var e any
		if json.Unmarshal(t.Error, &e) == nil {
			v["error"] = e
		} else {
			v["error"] = gin.H{"type": "video_generation_error", "message": string(t.Error)}
		}
	}
	return v
}
func unix(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Unix()
}
