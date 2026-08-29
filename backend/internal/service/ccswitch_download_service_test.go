package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const ccSwitchReleaseFixture = `{
  "tag_name": "v9.8.7",
  "html_url": "https://github.com/farion1231/cc-switch/releases/tag/v9.8.7",
  "assets": [
    {"name":"CC-Switch-v9.8.7-Windows-x64.msi","browser_download_url":"https://github.com/farion1231/cc-switch/releases/download/v9.8.7/CC-Switch-v9.8.7-Windows-x64.msi"},
    {"name":"CC-Switch-v9.8.7-Windows-arm64.exe","browser_download_url":"https://github.com/farion1231/cc-switch/releases/download/v9.8.7/CC-Switch-v9.8.7-Windows-arm64.exe"},
    {"name":"CC-Switch-v9.8.7-macOS-x64.dmg","browser_download_url":"https://github.com/farion1231/cc-switch/releases/download/v9.8.7/CC-Switch-v9.8.7-macOS-x64.dmg"},
    {"name":"CC-Switch-v9.8.7-macOS-arm64.dmg","browser_download_url":"https://github.com/farion1231/cc-switch/releases/download/v9.8.7/CC-Switch-v9.8.7-macOS-arm64.dmg"},
    {"name":"CC-Switch-v9.8.7-Linux-x86_64.AppImage","browser_download_url":"https://github.com/farion1231/cc-switch/releases/download/v9.8.7/CC-Switch-v9.8.7-Linux-x86_64.AppImage"},
    {"name":"CC-Switch-v9.8.7-Linux-aarch64.AppImage","browser_download_url":"https://github.com/farion1231/cc-switch/releases/download/v9.8.7/CC-Switch-v9.8.7-Linux-aarch64.AppImage"},
    {"name":"checksums.sha256","browser_download_url":"https://github.com/farion1231/cc-switch/releases/download/v9.8.7/checksums.sha256"}
  ]
}`

type ccSwitchGitHubClient struct {
	release        *GitHubRelease
	err            error
	calls          atomic.Int32
	fetch          func(context.Context) (*GitHubRelease, error)
	versionRelease *GitHubRelease
	versionErr     error
	versionCalls   atomic.Int32
	recentReleases []*GitHubRelease
	recentErr      error
	recentCalls    atomic.Int32
}

func (c *ccSwitchGitHubClient) FetchLatestRelease(ctx context.Context, _ string) (*GitHubRelease, error) {
	c.calls.Add(1)
	if c.fetch != nil {
		return c.fetch(ctx)
	}
	return c.release, c.err
}
func (c *ccSwitchGitHubClient) FetchReleaseByTag(context.Context, string, string) (*GitHubRelease, error) {
	c.versionCalls.Add(1)
	if c.versionRelease != nil || c.versionErr != nil {
		return c.versionRelease, c.versionErr
	}
	return c.release, c.err
}

func (c *ccSwitchGitHubClient) FetchRecentReleases(context.Context, string, int) ([]*GitHubRelease, error) {
	c.recentCalls.Add(1)
	if c.recentReleases != nil || c.recentErr != nil {
		return c.recentReleases, c.recentErr
	}
	return nil, errors.New("unexpected call")
}
func (*ccSwitchGitHubClient) DownloadFile(context.Context, string, string, int64) error {
	return errors.New("unexpected call")
}
func (*ccSwitchGitHubClient) FetchChecksumFile(context.Context, string) ([]byte, error) {
	return nil, errors.New("unexpected call")
}

func fixtureCCSwitchRelease(t *testing.T) *GitHubRelease {
	t.Helper()
	var release GitHubRelease
	require.NoError(t, json.Unmarshal([]byte(ccSwitchReleaseFixture), &release))
	return &release
}

func TestCCSwitchDownloadServiceResolvePlatforms(t *testing.T) {
	tests := []struct {
		os, arch, fileName string
	}{
		{"windows", "amd64", "CC-Switch-v9.8.7-Windows-x64.msi"},
		{"windows", "arm64", "CC-Switch-v9.8.7-Windows-arm64.exe"},
		{"macos", "amd64", "CC-Switch-v9.8.7-macOS-x64.dmg"},
		{"macos", "arm64", "CC-Switch-v9.8.7-macOS-arm64.dmg"},
		{"linux", "amd64", "CC-Switch-v9.8.7-Linux-x86_64.AppImage"},
		{"linux", "arm64", "CC-Switch-v9.8.7-Linux-aarch64.AppImage"},
	}
	for _, tt := range tests {
		t.Run(tt.os+"_"+tt.arch, func(t *testing.T) {
			client := &ccSwitchGitHubClient{release: fixtureCCSwitchRelease(t)}
			got, err := NewCCSwitchDownloadService(client).Resolve(context.Background(), tt.os, tt.arch)
			require.NoError(t, err)
			require.Equal(t, tt.fileName, got.FileName)
			require.Contains(t, got.DownloadURL, "/farion1231/cc-switch/releases/download/v9.8.7/")
			require.Equal(t, "https://github.com/farion1231/cc-switch/releases/tag/v9.8.7", got.ReleaseURL)
		})
	}
}

func TestCCSwitchDownloadServiceRejectsInvalidParameters(t *testing.T) {
	service := NewCCSwitchDownloadService(&ccSwitchGitHubClient{release: fixtureCCSwitchRelease(t)})
	for _, tt := range []struct{ os, arch string }{
		{"darwin", "amd64"}, {"windows", "x64"}, {"", "amd64"}, {"linux", ""},
	} {
		_, err := service.Resolve(context.Background(), tt.os, tt.arch)
		require.ErrorIs(t, err, ErrInvalidCCSwitchPlatform)
	}
}

func TestCCSwitchDownloadServiceResolvesExplicitVersion(t *testing.T) {
	latest := fixtureCCSwitchRelease(t)
	version := fixtureCCSwitchRelease(t)
	version.TagName = "v3.19.1"
	version.HTMLURL = "https://github.com/farion1231/cc-switch/releases/tag/v3.19.1"
	for i := range version.Assets {
		version.Assets[i].Name = strings.Replace(version.Assets[i].Name, "v9.8.7", "v3.19.1", 1)
		version.Assets[i].BrowserDownloadURL = strings.Replace(version.Assets[i].BrowserDownloadURL, "v9.8.7", "v3.19.1", 1)
	}
	client := &ccSwitchGitHubClient{release: latest, versionRelease: version}
	downloadService := NewCCSwitchDownloadService(client)

	got, err := downloadService.Resolve(context.Background(), "windows", "amd64", "3.19.1")
	require.NoError(t, err)
	require.Equal(t, "v3.19.1", got.Version)
	require.Equal(t, "CC-Switch-v3.19.1-Windows-x64.msi", got.FileName)
	require.Equal(t, int32(1), client.versionCalls.Load())
	require.Equal(t, int32(0), client.calls.Load())

	// The explicit release is cached by tag, so changing the fake response does
	// not silently switch the binary during a short-lived download flow.
	client.versionRelease = latest
	got, err = downloadService.ResolveVersion(context.Background(), "windows", "amd64", "v3.19.1")
	require.NoError(t, err)
	require.Equal(t, "v3.19.1", got.Version)
	require.Equal(t, int32(1), client.versionCalls.Load())

	// Empty and `latest` retain the original latest-release behavior.
	got, err = downloadService.ResolveVersion(context.Background(), "windows", "amd64", "latest")
	require.NoError(t, err)
	require.Equal(t, "v9.8.7", got.Version)
	require.Equal(t, int32(1), client.calls.Load())
}

func TestCCSwitchDownloadServiceRejectsUnsafeOrUnknownVersion(t *testing.T) {
	client := &ccSwitchGitHubClient{release: fixtureCCSwitchRelease(t)}
	downloadService := NewCCSwitchDownloadService(client)
	for _, version := range []string{"../v3.19.1", "v3/19/1", "v3.19.1?x=1", "v3.19.1%2f", "-3.19.1", "v"} {
		_, err := downloadService.ResolveVersion(context.Background(), "windows", "amd64", version)
		require.ErrorIs(t, err, ErrInvalidCCSwitchVersion, version)
	}

	client.versionErr = ErrGitHubReleaseNotFound
	client.versionRelease = nil
	_, err := downloadService.ResolveVersion(context.Background(), "windows", "amd64", "3.19.1")
	require.ErrorIs(t, err, ErrCCSwitchVersionNotFound)
}

func TestCCSwitchDownloadServiceListsRecentVersions(t *testing.T) {
	newRelease := func(tag, name, publishedAt string, draft, prerelease bool) *GitHubRelease {
		return &GitHubRelease{
			TagName:     tag,
			Name:        name,
			PublishedAt: publishedAt,
			HTMLURL:     "https://github.com/farion1231/cc-switch/releases/tag/" + tag,
			Draft:       draft,
			Prerelease:  prerelease,
		}
	}
	client := &ccSwitchGitHubClient{recentReleases: []*GitHubRelease{
		newRelease("v4.0.0", "Four", "2026-08-01T00:00:00Z", false, false),
		newRelease("v4.0.0", "Duplicate", "2026-08-01T00:00:00Z", false, false),
		newRelease("v3.20.0-rc1", "RC", "2026-07-31T00:00:00Z", false, true),
		newRelease("v3.19.1", "Stable", "2026-07-30T00:00:00Z", true, false),
		newRelease("not-a-version", "Bad", "2026-07-29T00:00:00Z", false, false),
	}}
	downloadService := NewCCSwitchDownloadService(client)

	list, err := downloadService.ListVersions(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, "4.0.0", list.LatestVersion)
	require.Len(t, list.Versions, 2)
	require.Equal(t, "4.0.0", list.Versions[0].Version)
	require.Equal(t, "v3.20.0-rc1", list.Versions[1].TagName)
	require.True(t, list.Versions[1].Prerelease)
	require.Equal(t, int32(1), client.recentCalls.Load())

	trimmed, err := downloadService.ListVersions(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, trimmed.Versions, 1)
	require.Equal(t, int32(1), client.recentCalls.Load())
}

func TestCCSwitchDownloadServiceListsVersionFailureAndUsesStaleData(t *testing.T) {
	now := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	client := &ccSwitchGitHubClient{recentReleases: []*GitHubRelease{{TagName: "v1.0.0", HTMLURL: "https://github.com/farion1231/cc-switch/releases/tag/v1.0.0"}}}
	downloadService := newCCSwitchDownloadService(client, 15*time.Minute, func() time.Time { return now })

	list, err := downloadService.ListVersions(context.Background(), 20)
	require.NoError(t, err)
	require.Len(t, list.Versions, 1)

	now = now.Add(16 * time.Minute)
	client.recentReleases = nil
	client.recentErr = errors.New("rate limited")
	list, err = downloadService.ListVersions(context.Background(), 20)
	require.NoError(t, err)
	require.Len(t, list.Versions, 1)
	require.Equal(t, "1.0.0", list.Versions[0].Version)
}

func TestCCSwitchDownloadServiceVersionFailureUsesStaleDataImmediately(t *testing.T) {
	now := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	release := fixtureCCSwitchRelease(t)
	release.TagName = "v9.8.7"
	client := &ccSwitchGitHubClient{versionRelease: release}
	downloadService := newCCSwitchDownloadService(client, 15*time.Minute, func() time.Time { return now })

	first, err := downloadService.Resolve(context.Background(), "windows", "amd64", "9.8.7")
	require.NoError(t, err)
	require.Equal(t, "CC-Switch-v9.8.7-Windows-x64.msi", first.FileName)
	require.Equal(t, int32(1), client.versionCalls.Load())

	// A transient GitHub failure after expiry should serve the last known
	// release on the same request, matching latest/list stale-cache behavior.
	now = now.Add(16 * time.Minute)
	client.versionRelease = nil
	client.versionErr = errors.New("rate limited")
	stale, err := downloadService.Resolve(context.Background(), "windows", "amd64", "v9.8.7")
	require.NoError(t, err)
	require.Equal(t, first.FileName, stale.FileName)
	require.Equal(t, int32(2), client.versionCalls.Load())

	// The short failure TTL prevents another upstream call while GitHub is
	// unavailable, but still returns the same stale asset.
	staleAgain, err := downloadService.Resolve(context.Background(), "windows", "amd64", "v9.8.7")
	require.NoError(t, err)
	require.Equal(t, first.FileName, staleAgain.FileName)
	require.Equal(t, int32(2), client.versionCalls.Load())
}

func TestCCSwitchDownloadServiceRejectsMissingOrUnsafeAsset(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"http", "http://github.com/farion1231/cc-switch/releases/download/v1/app-Windows-x64.msi"},
		{"wrong host", "https://example.com/farion1231/cc-switch/releases/download/v1/app-Windows-x64.msi"},
		{"subdomain", "https://github.com.evil.test/farion1231/cc-switch/releases/download/v1/app-Windows-x64.msi"},
		{"userinfo", "https://user@github.com/farion1231/cc-switch/releases/download/v1/app-Windows-x64.msi"},
		{"port", "https://github.com:443/farion1231/cc-switch/releases/download/v1/app-Windows-x64.msi"},
		{"wrong repo", "https://github.com/other/cc-switch/releases/download/v1/app-Windows-x64.msi"},
		{"lookalike path", "https://github.com/farion1231/cc-switch-malicious/releases/download/v1/app-Windows-x64.msi"},
		{"encoded path", "https://github.com/farion1231/cc-switch/releases/download%2Fv1/app-Windows-x64.msi"},
		{"path traversal", "https://github.com/farion1231/cc-switch/releases/download/v1/../app-Windows-x64.msi"},
		{"extra segment", "https://github.com/farion1231/cc-switch/releases/download/v1/nested/app-Windows-x64.msi"},
		{"query", "https://github.com/farion1231/cc-switch/releases/download/v1/app-Windows-x64.msi?next=https://evil.test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release := fixtureCCSwitchRelease(t)
			release.Assets = []GitHubAsset{{Name: "app-Windows-x64.msi", BrowserDownloadURL: tt.url}}
			_, err := NewCCSwitchDownloadService(&ccSwitchGitHubClient{release: release}).Resolve(context.Background(), "windows", "amd64")
			require.Error(t, err)
		})
	}

	release := fixtureCCSwitchRelease(t)
	release.Assets = []GitHubAsset{{Name: "checksums-Windows-x64.msi", BrowserDownloadURL: "https://github.com/farion1231/cc-switch/releases/download/v1/checksums-Windows-x64.msi"}}
	_, err := NewCCSwitchDownloadService(&ccSwitchGitHubClient{release: release}).Resolve(context.Background(), "windows", "amd64")
	require.ErrorIs(t, err, ErrCCSwitchAssetNotFound)
}

func TestCCSwitchDownloadServiceRejectsAssetFromAnotherRelease(t *testing.T) {
	release := fixtureCCSwitchRelease(t)
	release.TagName = "v3.19.1"
	release.Assets = []GitHubAsset{{
		Name:               "CC-Switch-v3.19.1-Windows-x64.msi",
		BrowserDownloadURL: "https://github.com/farion1231/cc-switch/releases/download/v9.8.7/CC-Switch-v3.19.1-Windows-x64.msi",
	}}
	_, err := NewCCSwitchDownloadService(&ccSwitchGitHubClient{versionRelease: release}).Resolve(context.Background(), "windows", "amd64", "3.19.1")
	require.Error(t, err)
}

func TestCCSwitchDownloadServiceCachesAndExpiresRelease(t *testing.T) {
	now := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	client := &ccSwitchGitHubClient{release: fixtureCCSwitchRelease(t)}
	service := newCCSwitchDownloadService(client, 15*time.Minute, func() time.Time { return now })

	_, err := service.Resolve(context.Background(), "windows", "amd64")
	require.NoError(t, err)
	_, err = service.Resolve(context.Background(), "linux", "arm64")
	require.NoError(t, err)
	require.Equal(t, int32(1), client.calls.Load())

	now = now.Add(16 * time.Minute)
	_, err = service.Resolve(context.Background(), "macos", "amd64")
	require.NoError(t, err)
	require.Equal(t, int32(2), client.calls.Load())
}

func TestCCSwitchDownloadServiceCachesFailuresAndUsesStaleRelease(t *testing.T) {
	now := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	upstreamErr := errors.New("rate limited")
	client := &ccSwitchGitHubClient{err: upstreamErr}
	service := newCCSwitchDownloadService(client, 15*time.Minute, func() time.Time { return now })

	_, err := service.Resolve(context.Background(), "windows", "amd64")
	require.ErrorIs(t, err, upstreamErr)
	_, err = service.Resolve(context.Background(), "windows", "amd64")
	require.ErrorIs(t, err, upstreamErr)
	require.Equal(t, int32(1), client.calls.Load())

	now = now.Add(2 * time.Minute)
	client.err = nil
	client.release = fixtureCCSwitchRelease(t)
	_, err = service.Resolve(context.Background(), "windows", "amd64")
	require.NoError(t, err)
	require.Equal(t, int32(2), client.calls.Load())

	now = now.Add(16 * time.Minute)
	client.err = upstreamErr
	_, err = service.Resolve(context.Background(), "linux", "arm64")
	require.NoError(t, err)
	_, err = service.Resolve(context.Background(), "macos", "amd64")
	require.NoError(t, err)
	require.Equal(t, int32(3), client.calls.Load())
}

func TestCCSwitchDownloadServiceCoalescesConcurrentMisses(t *testing.T) {
	client := &ccSwitchGitHubClient{release: fixtureCCSwitchRelease(t)}
	service := NewCCSwitchDownloadService(client)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Resolve(context.Background(), "windows", "amd64")
			require.NoError(t, err)
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), client.calls.Load())
}

func TestCCSwitchDownloadServiceRejectsNilReleaseAndNeutralArmAsset(t *testing.T) {
	_, err := NewCCSwitchDownloadService(&ccSwitchGitHubClient{}).Resolve(context.Background(), "windows", "amd64")
	require.Error(t, err)

	release := fixtureCCSwitchRelease(t)
	release.Assets = []GitHubAsset{{
		Name:               "CC-Switch-v9.8.7-Windows.msi",
		BrowserDownloadURL: "https://github.com/farion1231/cc-switch/releases/download/v9.8.7/CC-Switch-v9.8.7-Windows.msi",
	}}
	_, err = NewCCSwitchDownloadService(&ccSwitchGitHubClient{release: release}).Resolve(context.Background(), "windows", "arm64")
	require.ErrorIs(t, err, ErrCCSwitchAssetNotFound)
}

func TestCCSwitchDownloadServiceMatchesCurrentReleaseNaming(t *testing.T) {
	release := fixtureCCSwitchRelease(t)
	release.Assets = []GitHubAsset{
		{Name: "CC-Switch-v3.19.1-Windows.msi", BrowserDownloadURL: "https://github.com/farion1231/cc-switch/releases/download/v3.19.1/CC-Switch-v3.19.1-Windows.msi"},
		{Name: "CC-Switch-v3.19.1-Windows-arm64.msi", BrowserDownloadURL: "https://github.com/farion1231/cc-switch/releases/download/v3.19.1/CC-Switch-v3.19.1-Windows-arm64.msi"},
		{Name: "CC-Switch-v3.19.1-macOS.dmg", BrowserDownloadURL: "https://github.com/farion1231/cc-switch/releases/download/v3.19.1/CC-Switch-v3.19.1-macOS.dmg"},
		{Name: "CC-Switch-v3.19.1-Linux-x86_64.AppImage", BrowserDownloadURL: "https://github.com/farion1231/cc-switch/releases/download/v3.19.1/CC-Switch-v3.19.1-Linux-x86_64.AppImage"},
		{Name: "CC-Switch-v3.19.1-Linux-arm64.AppImage", BrowserDownloadURL: "https://github.com/farion1231/cc-switch/releases/download/v3.19.1/CC-Switch-v3.19.1-Linux-arm64.AppImage"},
	}
	service := NewCCSwitchDownloadService(&ccSwitchGitHubClient{release: release})

	tests := []struct {
		os, arch, fileName string
	}{
		{"windows", "amd64", "CC-Switch-v3.19.1-Windows.msi"},
		{"windows", "arm64", "CC-Switch-v3.19.1-Windows-arm64.msi"},
		{"macos", "amd64", "CC-Switch-v3.19.1-macOS.dmg"},
		{"macos", "arm64", "CC-Switch-v3.19.1-macOS.dmg"},
		{"linux", "amd64", "CC-Switch-v3.19.1-Linux-x86_64.AppImage"},
		{"linux", "arm64", "CC-Switch-v3.19.1-Linux-arm64.AppImage"},
	}
	for _, tt := range tests {
		download, err := service.Resolve(context.Background(), tt.os, tt.arch)
		require.NoError(t, err)
		require.Equal(t, tt.fileName, download.FileName)
	}
}

func TestCCSwitchDownloadServiceCallerCancellationDoesNotPoisonSharedFetch(t *testing.T) {
	release := fixtureCCSwitchRelease(t)
	fetchStarted := make(chan struct{})
	allowFetch := make(chan struct{})
	client := &ccSwitchGitHubClient{fetch: func(ctx context.Context) (*GitHubRelease, error) {
		close(fetchStarted)
		select {
		case <-allowFetch:
			return release, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	service := NewCCSwitchDownloadService(client)
	callerCtx, cancelCaller := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := service.Resolve(callerCtx, "windows", "amd64")
		firstDone <- err
	}()
	<-fetchStarted
	cancelCaller()
	require.ErrorIs(t, <-firstDone, context.Canceled)

	secondDone := make(chan error, 1)
	go func() {
		_, err := service.Resolve(context.Background(), "windows", "amd64")
		secondDone <- err
	}()
	close(allowFetch)
	require.NoError(t, <-secondDone)
	require.Equal(t, int32(1), client.calls.Load())
}
