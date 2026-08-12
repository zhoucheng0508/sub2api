package handler

import (
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CCSwitchDownloadHandler struct {
	service *service.CCSwitchDownloadService
}

func NewCCSwitchDownloadHandler(downloadService *service.CCSwitchDownloadService) *CCSwitchDownloadHandler {
	return &CCSwitchDownloadHandler{service: downloadService}
}

func (h *CCSwitchDownloadHandler) Resolve(c *gin.Context) {
	download, err := h.service.Resolve(c.Request.Context(), c.Query("os"), c.Query("arch"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCCSwitchPlatform):
			response.BadRequest(c, "os and arch must be supported values")
		case errors.Is(err, service.ErrCCSwitchAssetNotFound):
			response.Error(c, http.StatusNotFound, "No compatible CC Switch download was found")
		default:
			response.Error(c, http.StatusBadGateway, "Unable to resolve the CC Switch download")
		}
		return
	}
	response.Success(c, download)
}
