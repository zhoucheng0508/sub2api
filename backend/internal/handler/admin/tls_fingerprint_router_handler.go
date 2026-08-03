package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type TLSFingerprintRouterHandler struct {
	service *service.TLSFingerprintRouterService
}

func NewTLSFingerprintRouterHandler(service *service.TLSFingerprintRouterService) *TLSFingerprintRouterHandler {
	return &TLSFingerprintRouterHandler{service: service}
}

type tlsFingerprintRouterRequest struct {
	Name        string                           `json:"name" binding:"required"`
	Description *string                          `json:"description"`
	Enabled     *bool                            `json:"enabled"`
	Rules       []model.TLSFingerprintRouterRule `json:"rules"`
}

func (h *TLSFingerprintRouterHandler) List(c *gin.Context) {
	rows, err := h.service.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rows)
}

func (h *TLSFingerprintRouterHandler) GetByID(c *gin.Context) {
	id, ok := parseTLSRouterID(c)
	if !ok {
		return
	}
	row, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if row == nil {
		response.NotFound(c, "Router not found")
		return
	}
	response.Success(c, row)
}

func (h *TLSFingerprintRouterHandler) Create(c *gin.Context) {
	var req tlsFingerprintRouterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row, err := h.service.Create(c.Request.Context(), &model.TLSFingerprintRouter{
		Name: req.Name, Description: req.Description, Enabled: enabled, Rules: req.Rules,
	})
	if err != nil {
		handleTLSRouterError(c, err)
		return
	}
	response.Success(c, row)
}

func (h *TLSFingerprintRouterHandler) Update(c *gin.Context) {
	id, ok := parseTLSRouterID(c)
	if !ok {
		return
	}
	var req tlsFingerprintRouterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	existing, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if existing == nil {
		response.NotFound(c, "Router not found")
		return
	}
	existing.Name, existing.Description, existing.Rules = req.Name, req.Description, req.Rules
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	row, err := h.service.Update(c.Request.Context(), existing)
	if err != nil {
		handleTLSRouterError(c, err)
		return
	}
	response.Success(c, row)
}

func (h *TLSFingerprintRouterHandler) Delete(c *gin.Context) {
	id, ok := parseTLSRouterID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Router deleted successfully"})
}

func parseTLSRouterID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid router ID")
		return 0, false
	}
	return id, true
}

func handleTLSRouterError(c *gin.Context, err error) {
	if _, ok := err.(*model.ValidationError); ok {
		response.BadRequest(c, err.Error())
		return
	}
	response.ErrorFrom(c, err)
}
