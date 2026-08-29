package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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
	download, ok := h.resolve(c)
	if !ok {
		return
	}
	// `download=1` is supported for clients that cannot construct the
	// same-origin direct URL themselves. The dedicated /file route is preferred
	// because it keeps metadata and binary responses unambiguous.
	if wantsCCSwitchRedirect(c) {
		h.redirect(c, download)
		return
	}

	payload := gin.H{
		"download_url": download.DownloadURL,
		"file_name":    download.FileName,
		"release_url":  download.ReleaseURL,
		"version":      download.Version,
		"direct_url":   ccSwitchDirectDownloadPath(c),
	}
	response.Success(c, payload)
}

// Download resolves and redirects to the selected release asset. GitHub's
// asset endpoint supplies the attachment filename and streams the bytes, so
// the application does not buffer large installers in memory.
func (h *CCSwitchDownloadHandler) Download(c *gin.Context) {
	download, ok := h.resolve(c)
	if !ok {
		return
	}
	h.redirect(c, download)
}

func (h *CCSwitchDownloadHandler) ListVersions(c *gin.Context) {
	limit := 0
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 100 {
			response.BadRequest(c, "limit must be an integer between 1 and 100")
			return
		}
		limit = parsed
	}

	versions, err := h.service.ListVersions(c.Request.Context(), limit)
	if err != nil {
		response.Error(c, http.StatusBadGateway, "Unable to list CC Switch versions")
		return
	}
	response.Success(c, versions)
}

func (h *CCSwitchDownloadHandler) resolve(c *gin.Context) (*service.CCSwitchDownload, bool) {
	osName := c.Query("os")
	arch := c.Query("arch")
	// Keep the compact /{os} links used by the user-facing page compatible
	// with the original endpoint. Callers that need ARM64 should pass arch
	// explicitly; x64 is the conservative default for a link without it.
	if osName == "" {
		osName = c.Param("os")
	}
	if arch == "" && c.Param("os") != "" {
		arch = "amd64"
	}
	download, err := h.service.ResolveVersion(
		c.Request.Context(),
		osName,
		arch,
		c.Query("version"),
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCCSwitchPlatform):
			response.BadRequest(c, "os and arch must be supported values")
		case errors.Is(err, service.ErrInvalidCCSwitchVersion):
			response.BadRequest(c, "version must be a valid CC Switch release version")
		case errors.Is(err, service.ErrCCSwitchVersionNotFound):
			response.Error(c, http.StatusNotFound, "The requested CC Switch version was not found")
		case errors.Is(err, service.ErrCCSwitchAssetNotFound):
			response.Error(c, http.StatusNotFound, "No compatible CC Switch download was found")
		default:
			response.Error(c, http.StatusBadGateway, "Unable to resolve the CC Switch download")
		}
		return nil, false
	}
	return download, true
}

func (h *CCSwitchDownloadHandler) redirect(c *gin.Context, download *service.CCSwitchDownload) {
	// The service validates this URL against the pinned CC Switch repository
	// before it reaches the handler. Disable intermediary caching because the
	// empty version (latest) is intentionally dynamic.
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Redirect(http.StatusFound, download.DownloadURL)
}

func wantsCCSwitchRedirect(c *gin.Context) bool {
	return c.Query("download") == "1" || c.Query("download") == "true" ||
		c.Query("redirect") == "1" || c.Query("redirect") == "true"
}

func ccSwitchDirectDownloadPath(c *gin.Context) string {
	query := url.Values{}
	osName := c.Query("os")
	if osName == "" {
		osName = c.Param("os")
	}
	if osName != "" {
		query.Set("os", osName)
	}
	arch := c.Query("arch")
	if arch == "" && c.Param("os") != "" {
		arch = "amd64"
	}
	if arch != "" {
		query.Set("arch", arch)
	}
	if version := c.Query("version"); version != "" {
		query.Set("version", version)
	}
	// Preserve any reverse-proxy path prefix visible to the application. This
	// keeps the metadata URL useful for clients mounted below a custom prefix;
	// the frontend still constructs its own URL from VITE_API_BASE_URL.
	const routeBase = "/api/v1/downloads/cc-switch"
	path := routeBase + "/file"
	if requestPath := c.Request.URL.Path; requestPath != "" {
		if index := strings.LastIndex(requestPath, routeBase); index >= 0 {
			path = requestPath[:index] + routeBase + "/file"
		}
	}
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return path
}
