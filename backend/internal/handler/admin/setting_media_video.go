package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetMediaVideoDownloadSettings(c *gin.Context) {
	settings, err := h.settingService.GetMediaVideoDownloadSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *SettingHandler) UpdateMediaVideoDownloadSettings(c *gin.Context) {
	var settings service.MediaVideoDownloadSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		response.BadRequest(c, "视频下载设置格式不正确")
		return
	}
	if err := h.settingService.SetMediaVideoDownloadSettings(c.Request.Context(), settings); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}
