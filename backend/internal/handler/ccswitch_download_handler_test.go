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
	release *service.GitHubRelease
	err     error
}

func (c *ccSwitchHandlerGitHubClient) FetchLatestRelease(context.Context, string) (*service.GitHubRelease, error) {
	return c.release, c.err
}
func (*ccSwitchHandlerGitHubClient) FetchRecentReleases(context.Context, string, int) ([]*service.GitHubRelease, error) {
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

	badRequest := runCCSwitchDownloadHandler(t, &ccSwitchHandlerGitHubClient{release: release}, "os=darwin&arch=amd64")
	require.Equal(t, http.StatusBadRequest, badRequest.Code)

	notFound := runCCSwitchDownloadHandler(t, &ccSwitchHandlerGitHubClient{release: &service.GitHubRelease{}}, "os=linux&arch=arm64")
	require.Equal(t, http.StatusNotFound, notFound.Code)

	badGateway := runCCSwitchDownloadHandler(t, &ccSwitchHandlerGitHubClient{err: errors.New("rate limited")}, "os=windows&arch=amd64")
	require.Equal(t, http.StatusBadGateway, badGateway.Code)
}
