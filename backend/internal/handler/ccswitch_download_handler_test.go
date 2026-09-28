package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type ccSwitchHandlerGitHubClient struct {
	release        *service.GitHubRelease
	err            error
	versionRelease *service.GitHubRelease
	versionErr     error
	recentReleases []*service.GitHubRelease
	recentErr      error
}

func (c *ccSwitchHandlerGitHubClient) FetchLatestRelease(context.Context, string) (*service.GitHubRelease, error) {
	return c.release, c.err
}
func (c *ccSwitchHandlerGitHubClient) FetchReleaseByTag(context.Context, string, string) (*service.GitHubRelease, error) {
	if c.versionRelease != nil || c.versionErr != nil {
		return c.versionRelease, c.versionErr
	}
	return c.release, c.err
}

func (c *ccSwitchHandlerGitHubClient) FetchRecentReleases(context.Context, string, int) ([]*service.GitHubRelease, error) {
	if c.recentReleases != nil || c.recentErr != nil {
		return c.recentReleases, c.recentErr
	}
	return nil, errors.New("unexpected call")
}
func (*ccSwitchHandlerGitHubClient) DownloadFile(context.Context, string, string, int64) error {
	return errors.New("unexpected call")
}
func (*ccSwitchHandlerGitHubClient) FetchChecksumFile(context.Context, string) ([]byte, error) {
	return nil, errors.New("unexpected call")
}

func runCCSwitchDownloadHandler(t *testing.T, client service.GitHubReleaseClient, query string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewCCSwitchDownloadHandler(service.NewCCSwitchDownloadService(client))
	r := gin.New()
	r.GET("/api/v1/downloads/cc-switch", h.Resolve)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/cc-switch?"+query, nil)
	r.ServeHTTP(recorder, request)
	return recorder
}

func runCCSwitchDownloadRedirect(t *testing.T, client service.GitHubReleaseClient, path, query string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewCCSwitchDownloadHandler(service.NewCCSwitchDownloadService(client))
	r := gin.New()
	r.GET("/api/v1/downloads/cc-switch", h.Resolve)
	r.GET("/api/v1/downloads/cc-switch/file", h.Download)
	r.GET("/api/v1/downloads/cc-switch/:os", h.Download)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path+"?"+query, nil)
	r.ServeHTTP(recorder, request)
	return recorder
}

func TestCCSwitchDownloadHandlerResponses(t *testing.T) {
	release := &service.GitHubRelease{
		HTMLURL: "https://github.com/farion1231/cc-switch/releases/tag/v3.19.1",
		Assets: []service.GitHubAsset{{
			Name:               "CC-Switch-v3.19.1-Windows.msi",
			BrowserDownloadURL: "https://github.com/farion1231/cc-switch/releases/download/v3.19.1/CC-Switch-v3.19.1-Windows.msi",
		}},
	}

	success := runCCSwitchDownloadHandler(t, &ccSwitchHandlerGitHubClient{release: release}, "os=windows&arch=amd64")
	require.Equal(t, http.StatusOK, success.Code)
	require.Contains(t, success.Body.String(), `"download_url":"https://github.com/farion1231/cc-switch/releases/download/`)
	require.Contains(t, success.Body.String(), `"direct_url":"/api/v1/downloads/cc-switch/file?arch=amd64\u0026os=windows"`)

	redirect := runCCSwitchDownloadRedirect(t, &ccSwitchHandlerGitHubClient{release: release}, "/api/v1/downloads/cc-switch/file", "os=windows&arch=amd64")
	require.Equal(t, http.StatusFound, redirect.Code)
	require.Equal(t, release.Assets[0].BrowserDownloadURL, redirect.Header().Get("Location"))
	require.Equal(t, "no-store", redirect.Header().Get("Cache-Control"))

	// The old resolver route also accepts download=1 for clients that only know
	// the metadata endpoint.
	legacyRedirect := runCCSwitchDownloadRedirect(t, &ccSwitchHandlerGitHubClient{release: release}, "/api/v1/downloads/cc-switch", "os=windows&arch=amd64&download=1")
	require.Equal(t, http.StatusFound, legacyRedirect.Code)
	require.Equal(t, release.Assets[0].BrowserDownloadURL, legacyRedirect.Header().Get("Location"))

	compactRedirect := runCCSwitchDownloadRedirect(t, &ccSwitchHandlerGitHubClient{release: release}, "/api/v1/downloads/cc-switch/windows", "")
	require.Equal(t, http.StatusFound, compactRedirect.Code)
	require.Equal(t, release.Assets[0].BrowserDownloadURL, compactRedirect.Header().Get("Location"))

	badRequest := runCCSwitchDownloadHandler(t, &ccSwitchHandlerGitHubClient{release: release}, "os=darwin&arch=amd64")
	require.Equal(t, http.StatusBadRequest, badRequest.Code)
	badVersion := runCCSwitchDownloadHandler(t, &ccSwitchHandlerGitHubClient{release: release}, "os=windows&arch=amd64&version=../v3.19.1")
	require.Equal(t, http.StatusBadRequest, badVersion.Code)

	notFound := runCCSwitchDownloadHandler(t, &ccSwitchHandlerGitHubClient{release: &service.GitHubRelease{}}, "os=linux&arch=arm64")
	require.Equal(t, http.StatusNotFound, notFound.Code)

	badGateway := runCCSwitchDownloadHandler(t, &ccSwitchHandlerGitHubClient{err: errors.New("rate limited")}, "os=windows&arch=amd64")
	require.Equal(t, http.StatusBadGateway, badGateway.Code)
}

func TestCCSwitchDownloadHandlerListsVersions(t *testing.T) {
	client := &ccSwitchHandlerGitHubClient{recentReleases: []*service.GitHubRelease{
		{TagName: "v3.19.1", Name: "3.19.1", HTMLURL: "https://github.com/farion1231/cc-switch/releases/tag/v3.19.1"},
		{TagName: "v3.19.0", Name: "3.19.0", HTMLURL: "https://github.com/farion1231/cc-switch/releases/tag/v3.19.0"},
	}}
	gin.SetMode(gin.TestMode)
	h := NewCCSwitchDownloadHandler(service.NewCCSwitchDownloadService(client))
	r := gin.New()
	r.GET("/api/v1/downloads/cc-switch/versions", h.ListVersions)
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/downloads/cc-switch/versions?limit=1", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"latest_version":"3.19.1"`)
	require.Contains(t, recorder.Body.String(), `"version":"3.19.1"`)
	require.NotContains(t, recorder.Body.String(), `"version":"3.19.0"`)

	badLimit := httptest.NewRecorder()
	r.ServeHTTP(badLimit, httptest.NewRequest(http.MethodGet, "/api/v1/downloads/cc-switch/versions?limit=101", nil))
	require.Equal(t, http.StatusBadRequest, badLimit.Code)
}

func TestCCSwitchDownloadHandlerPreservesRequestPrefixInDirectURL(t *testing.T) {
	release := &service.GitHubRelease{
		Assets: []service.GitHubAsset{{
			Name:               "CC-Switch-v3.19.1-Windows.msi",
			BrowserDownloadURL: "https://github.com/farion1231/cc-switch/releases/download/v3.19.1/CC-Switch-v3.19.1-Windows.msi",
		}},
	}
	h := NewCCSwitchDownloadHandler(service.NewCCSwitchDownloadService(&ccSwitchHandlerGitHubClient{release: release}))
	r := gin.New()
	r.GET("/custom/api/v1/downloads/cc-switch", h.Resolve)
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/custom/api/v1/downloads/cc-switch?os=windows&arch=amd64", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"direct_url":"/custom/api/v1/downloads/cc-switch/file?arch=amd64\u0026os=windows"`)
}
