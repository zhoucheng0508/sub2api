package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// RegisterCommonRoutes 注册通用路由（健康检查、状态等）
//
// The optional limiter keeps this helper source-compatible with small route
// harnesses that only need health/setup routes, while production wiring can
// protect the public GitHub-backed download endpoints by client IP.
func RegisterCommonRoutes(r *gin.Engine, h *handler.Handlers, limiters ...*servermiddleware.PanelRateLimiter) {
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Claude Code 遥测日志（忽略，直接返回200）
	r.POST("/api/event_logging/batch", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Setup status endpoint (always returns needs_setup: false in normal mode)
	// This is used by the frontend to detect when the service has restarted after setup
	r.GET("/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"needs_setup": false,
				"step":        "completed",
			},
		})
	})

	downloads := r.Group("/api/v1/downloads/cc-switch")
	if len(limiters) > 0 && limiters[0] != nil {
		// Resolving latest/versioned releases calls the GitHub API on a cache
		// miss. Reuse the configured public-IP panel budget to keep anonymous
		// callers from exhausting the upstream API quota.
		downloads.Use(limiters[0].PublicTrustedIP())
	}
	downloads.GET("", h.CCSwitchDownload.Resolve)
	downloads.GET("/versions", h.CCSwitchDownload.ListVersions)
	// Binary installer endpoint. It resolves the same OS/architecture/version
	// parameters and redirects directly to the validated release asset.
	downloads.GET("/file", h.CCSwitchDownload.Download)
	// Compact aliases retained for links embedded by earlier Sub2API pages.
	// They default to x64; ARM64 callers can add ?arch=arm64. The handler
	// validates the parameter against the supported platform set.
	downloads.GET("/:os", h.CCSwitchDownload.Download)
}
