package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/sync/singleflight"
)

const (
	ccSwitchRepository   = "farion1231/cc-switch"
	ccSwitchReleaseURL   = "https://github.com/farion1231/cc-switch/releases/latest"
	ccSwitchCacheTTL     = 15 * time.Minute
	ccSwitchFailureTTL   = time.Minute
	ccSwitchFetchTimeout = 30 * time.Second
)

var (
	ErrInvalidCCSwitchPlatform = errors.New("invalid CC Switch platform")
	ErrCCSwitchAssetNotFound   = errors.New("compatible CC Switch asset not found")
)

type CCSwitchDownload struct {
	DownloadURL string `json:"download_url"`
	FileName    string `json:"file_name"`
	ReleaseURL  string `json:"release_url"`
}

type CCSwitchDownloadService struct {
	githubClient GitHubReleaseClient
	cacheTTL     time.Duration
	now          func() time.Time

	mu        sync.RWMutex
	fetch     singleflight.Group
	release   *GitHubRelease
	expiresAt time.Time
	cachedErr error
}

func NewCCSwitchDownloadService(githubClient GitHubReleaseClient) *CCSwitchDownloadService {
	return newCCSwitchDownloadService(githubClient, ccSwitchCacheTTL, time.Now)
}

func newCCSwitchDownloadService(githubClient GitHubReleaseClient, cacheTTL time.Duration, now func() time.Time) *CCSwitchDownloadService {
	return &CCSwitchDownloadService{githubClient: githubClient, cacheTTL: cacheTTL, now: now}
}

func (s *CCSwitchDownloadService) Resolve(ctx context.Context, osName, arch string) (*CCSwitchDownload, error) {
	if !isAllowedCCSwitchPlatform(osName, arch) {
		return nil, ErrInvalidCCSwitchPlatform
	}

	release, err := s.latestRelease(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch CC Switch release: %w", err)
	}

	asset := selectCCSwitchAsset(release.Assets, osName, arch)
	if asset == nil {
		return nil, ErrCCSwitchAssetNotFound
	}
	if err := validateCCSwitchDownloadURL(asset.BrowserDownloadURL); err != nil {
		return nil, fmt.Errorf("invalid CC Switch download URL: %w", err)
	}

	releaseURL := release.HTMLURL
	if err := validateCCSwitchReleaseURL(releaseURL); err != nil {
		releaseURL = ccSwitchReleaseURL
	}
	return &CCSwitchDownload{DownloadURL: asset.BrowserDownloadURL, FileName: asset.Name, ReleaseURL: releaseURL}, nil
}

func (s *CCSwitchDownloadService) latestRelease(ctx context.Context) (*GitHubRelease, error) {
	if release, err, ok := s.cachedRelease(); ok {
		return release, err
	}

	resultCh := s.fetch.DoChan("latest", func() (any, error) {
		if release, cachedErr, ok := s.cachedRelease(); ok {
			return release, cachedErr
		}

		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ccSwitchFetchTimeout)
		defer cancel()
		release, fetchErr := s.githubClient.FetchLatestRelease(fetchCtx, ccSwitchRepository)
		now := s.now()
		s.mu.Lock()
		defer s.mu.Unlock()
		if fetchErr != nil || release == nil {
			if fetchErr == nil {
				fetchErr = errors.New("GitHub returned an empty release")
			}
			s.expiresAt = now.Add(ccSwitchFailureTTL)
			if s.release != nil {
				return s.release, nil
			}
			s.cachedErr = fetchErr
			return nil, fetchErr
		}
		s.release = release
		s.expiresAt = now.Add(s.cacheTTL)
		s.cachedErr = nil
		return release, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		return result.Val.(*GitHubRelease), nil
	}
}

func (s *CCSwitchDownloadService) cachedRelease() (*GitHubRelease, error, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.now().Before(s.expiresAt) {
		return nil, nil, false
	}
	if s.release != nil {
		return s.release, nil, true
	}
	if s.cachedErr != nil {
		return nil, s.cachedErr, true
	}
	return nil, nil, false
}

func isAllowedCCSwitchPlatform(osName, arch string) bool {
	validOS := osName == "windows" || osName == "macos" || osName == "linux"
	validArch := arch == "amd64" || arch == "arm64"
	return validOS && validArch
}

func selectCCSwitchAsset(assets []GitHubAsset, osName, arch string) *GitHubAsset {
	bestScore := -1
	bestIndex := -1
	for i := range assets {
		score, ok := scoreCCSwitchAsset(assets[i].Name, osName, arch)
		if ok && score > bestScore {
			bestScore = score
			bestIndex = i
		}
	}
	if bestIndex < 0 {
		return nil
	}
	return &assets[bestIndex]
}

func scoreCCSwitchAsset(name, osName, arch string) (int, bool) {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "checksum") || strings.Contains(lower, "sha256") || strings.Contains(lower, "signature") ||
		strings.HasSuffix(lower, ".sig") || strings.HasSuffix(lower, ".asc") || strings.Contains(lower, "source") {
		return 0, false
	}

	tokens := assetTokens(lower)
	if !matchesOS(tokens, lower, osName) || containsOtherOS(tokens, osName) {
		return 0, false
	}
	extensionScore, ok := installerExtensionScore(lower, osName)
	if !ok {
		return 0, false
	}

	archScore, ok := architectureScore(tokens, osName, arch)
	if !ok {
		return 0, false
	}
	return extensionScore + archScore, true
}

func assetTokens(name string) map[string]bool {
	parts := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	tokens := make(map[string]bool, len(parts))
	for _, part := range parts {
		tokens[part] = true
	}
	return tokens
}

func matchesOS(tokens map[string]bool, name, osName string) bool {
	switch osName {
	case "windows":
		return tokens["windows"] || tokens["win"]
	case "macos":
		return tokens["macos"] || tokens["darwin"] || tokens["mac"]
	case "linux":
		return tokens["linux"] || strings.HasSuffix(name, ".appimage")
	default:
		return false
	}
}

func containsOtherOS(tokens map[string]bool, osName string) bool {
	if osName != "windows" && (tokens["windows"] || tokens["win"]) {
		return true
	}
	if osName != "macos" && (tokens["macos"] || tokens["darwin"] || tokens["mac"]) {
		return true
	}
	return osName != "linux" && tokens["linux"]
}

func architectureScore(tokens map[string]bool, osName, arch string) (int, bool) {
	hasAMD64 := tokens["amd64"] || tokens["x86_64"] || tokens["x64"] || tokens["x86"] && tokens["64"]
	hasARM64 := tokens["arm64"] || tokens["aarch64"] || tokens["arm"] && tokens["64"]
	if arch == "amd64" {
		if hasARM64 {
			return 0, false
		}
		if hasAMD64 {
			return 20, true
		}
	} else {
		if hasAMD64 {
			return 0, false
		}
		if hasARM64 {
			return 20, true
		}
		// CC Switch publishes a single universal macOS DMG without an architecture token.
		if osName == "macos" {
			return 1, true
		}
		return 0, false
	}
	return 1, true // Architecture-neutral assets are only safe as the amd64 default.
}

func installerExtensionScore(name, osName string) (int, bool) {
	extensions := map[string][]string{
		"windows": {".msi", ".exe", ".zip"},
		"macos":   {".dmg", ".pkg", ".zip"},
		"linux":   {".appimage", ".deb", ".rpm", ".tar.gz"},
	}
	for i, extension := range extensions[osName] {
		if strings.HasSuffix(name, extension) {
			return 10 - i, true
		}
	}
	return 0, false
}

func validateCCSwitchDownloadURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" || parsed.Hostname() != "github.com" || parsed.Port() != "" || parsed.User != nil {
		return errors.New("URL must be an HTTPS github.com URL without user info or port")
	}
	const prefix = "/farion1231/cc-switch/releases/download/"
	escapedPath := parsed.EscapedPath()
	if parsed.RawQuery != "" || parsed.Fragment != "" || !strings.HasPrefix(escapedPath, prefix) {
		return errors.New("URL is outside the CC Switch release download path")
	}
	remainder := strings.TrimPrefix(escapedPath, prefix)
	segments := strings.Split(remainder, "/")
	lowerPath := strings.ToLower(escapedPath)
	if len(segments) != 2 || segments[0] == "" || segments[1] == "" || segments[0] == "." || segments[0] == ".." ||
		segments[1] == "." || segments[1] == ".." || strings.Contains(lowerPath, "%2f") || strings.Contains(lowerPath, "%5c") {
		return errors.New("URL must contain exactly one release tag and asset name")
	}
	return nil
}

func validateCCSwitchReleaseURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" || parsed.Hostname() != "github.com" || parsed.Port() != "" || parsed.User != nil {
		return errors.New("invalid release URL host")
	}
	if !strings.HasPrefix(parsed.EscapedPath(), "/farion1231/cc-switch/releases/") {
		return errors.New("invalid release URL path")
	}
	return nil
}
