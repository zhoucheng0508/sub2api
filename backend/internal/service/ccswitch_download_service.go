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
	ccSwitchRepository         = "farion1231/cc-switch"
	ccSwitchReleaseURL         = "https://github.com/farion1231/cc-switch/releases/latest"
	ccSwitchCacheTTL           = 15 * time.Minute
	ccSwitchFailureTTL         = time.Minute
	ccSwitchFetchTimeout       = 30 * time.Second
	ccSwitchVersionCacheLimit  = 32
	ccSwitchVersionListLimit   = 100
	ccSwitchVersionListDefault = 20
)

var (
	ErrInvalidCCSwitchPlatform = errors.New("invalid CC Switch platform")
	ErrInvalidCCSwitchVersion  = errors.New("invalid CC Switch version")
	ErrCCSwitchAssetNotFound   = errors.New("compatible CC Switch asset not found")
	ErrCCSwitchVersionNotFound = errors.New("CC Switch version not found")
	ErrGitHubReleaseNotFound   = errors.New("GitHub release not found")
)

type CCSwitchDownload struct {
	DownloadURL string `json:"download_url"`
	FileName    string `json:"file_name"`
	ReleaseURL  string `json:"release_url"`
	Version     string `json:"version"`
}

// CCSwitchReleaseVersion is the small, public representation used by the
// version picker. Assets are intentionally omitted; the resolver selects the
// platform asset after the caller chooses a version.
type CCSwitchReleaseVersion struct {
	Version     string `json:"version"`
	TagName     string `json:"tag_name"`
	Name        string `json:"name,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
	ReleaseURL  string `json:"release_url"`
	Prerelease  bool   `json:"prerelease,omitempty"`
}

type CCSwitchVersionList struct {
	Versions      []CCSwitchReleaseVersion `json:"versions"`
	LatestVersion string                   `json:"latest_version,omitempty"`
}

type cachedCCSwitchRelease struct {
	release   *GitHubRelease
	err       error
	expiresAt time.Time
}

type githubReleaseByTagClient interface {
	FetchReleaseByTag(ctx context.Context, repo, tag string) (*GitHubRelease, error)
}

type CCSwitchDownloadService struct {
	githubClient GitHubReleaseClient
	cacheTTL     time.Duration
	now          func() time.Time

	mu         sync.RWMutex
	fetch      singleflight.Group
	release    *GitHubRelease
	expiresAt  time.Time
	cachedErr  error
	versions   map[string]cachedCCSwitchRelease
	list       *CCSwitchVersionList
	listErr    error
	listExpiry time.Time
}

func NewCCSwitchDownloadService(githubClient GitHubReleaseClient) *CCSwitchDownloadService {
	return newCCSwitchDownloadService(githubClient, ccSwitchCacheTTL, time.Now)
}

func newCCSwitchDownloadService(githubClient GitHubReleaseClient, cacheTTL time.Duration, now func() time.Time) *CCSwitchDownloadService {
	return &CCSwitchDownloadService{
		githubClient: githubClient,
		cacheTTL:     cacheTTL,
		now:          now,
		versions:     make(map[string]cachedCCSwitchRelease),
	}
}

// Resolve keeps the original latest-release behavior when version is omitted.
// The variadic parameter preserves existing callers while allowing an exact release tag.
func (s *CCSwitchDownloadService) Resolve(ctx context.Context, osName, arch string, version ...string) (*CCSwitchDownload, error) {
	requestedVersion := ""
	if len(version) > 1 {
		return nil, ErrInvalidCCSwitchVersion
	}
	if len(version) == 1 {
		requestedVersion = version[0]
	}
	return s.ResolveVersion(ctx, osName, arch, requestedVersion)
}

func (s *CCSwitchDownloadService) ResolveVersion(ctx context.Context, osName, arch, version string) (*CCSwitchDownload, error) {
	if !isAllowedCCSwitchPlatform(osName, arch) {
		return nil, ErrInvalidCCSwitchPlatform
	}

	normalizedVersion, err := normalizeCCSwitchVersion(version)
	if err != nil {
		return nil, err
	}

	release, err := s.releaseForVersion(ctx, normalizedVersion)
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
	if normalizedVersion != "" && !ccSwitchDownloadURLMatchesVersion(asset.BrowserDownloadURL, normalizedVersion, release.TagName) {
		return nil, errors.New("CC Switch asset does not belong to the requested release")
	}

	releaseURL := release.HTMLURL
	if err := validateCCSwitchReleaseURL(releaseURL); err != nil {
		releaseURL = ccSwitchReleaseFallbackURL(normalizedVersion)
	}
	return &CCSwitchDownload{
		DownloadURL: asset.BrowserDownloadURL,
		FileName:    asset.Name,
		ReleaseURL:  releaseURL,
		Version:     release.TagName,
	}, nil
}

func (s *CCSwitchDownloadService) releaseForVersion(ctx context.Context, version string) (*GitHubRelease, error) {
	if version == "" {
		return s.latestRelease(ctx)
	}
	return s.versionRelease(ctx, version)
}

// ListVersions returns recent release tags suitable for a user-facing version
// picker. GitHub already orders releases newest-first; draft releases and
// malformed/duplicate tags are removed before applying the requested limit.
func (s *CCSwitchDownloadService) ListVersions(ctx context.Context, limit int) (*CCSwitchVersionList, error) {
	if limit <= 0 {
		limit = ccSwitchVersionListDefault
	}
	if limit > ccSwitchVersionListLimit {
		limit = ccSwitchVersionListLimit
	}

	if list, err, ok := s.cachedVersionList(); ok {
		return trimCCSwitchVersionList(list, limit), err
	}

	resultCh := s.fetch.DoChan("versions", func() (any, error) {
		if list, cachedErr, ok := s.cachedVersionList(); ok {
			return list, cachedErr
		}

		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ccSwitchFetchTimeout)
		defer cancel()
		releases, fetchErr := s.githubClient.FetchRecentReleases(fetchCtx, ccSwitchRepository, ccSwitchVersionListLimit)
		list := buildCCSwitchVersionList(releases)
		now := s.now()
		s.mu.Lock()
		defer s.mu.Unlock()
		if fetchErr != nil {
			s.listExpiry = now.Add(ccSwitchFailureTTL)
			if s.list != nil {
				return s.list, nil
			}
			s.listErr = fetchErr
			return nil, fetchErr
		}
		if list == nil {
			list = &CCSwitchVersionList{Versions: []CCSwitchReleaseVersion{}}
		}
		s.list = list
		s.listErr = nil
		s.listExpiry = now.Add(s.cacheTTL)
		return list, nil
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		list, ok := result.Val.(*CCSwitchVersionList)
		if !ok || list == nil {
			return nil, errors.New("cc switch version list fetch returned an invalid result")
		}
		return trimCCSwitchVersionList(list, limit), nil
	}
}

func (s *CCSwitchDownloadService) cachedVersionList() (*CCSwitchVersionList, error, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.now().Before(s.listExpiry) {
		return nil, nil, false
	}
	if s.list != nil {
		return s.list, nil, true
	}
	if s.listErr != nil {
		return nil, s.listErr, true
	}
	return nil, nil, false
}

func buildCCSwitchVersionList(releases []*GitHubRelease) *CCSwitchVersionList {
	list := &CCSwitchVersionList{Versions: make([]CCSwitchReleaseVersion, 0, len(releases))}
	seen := make(map[string]struct{}, len(releases))
	for _, release := range releases {
		if release == nil || release.Draft {
			continue
		}
		version, err := normalizeCCSwitchVersion(release.TagName)
		if err != nil || version == "" {
			continue
		}
		if _, exists := seen[version]; exists {
			continue
		}
		seen[version] = struct{}{}
		releaseURL := release.HTMLURL
		if err := validateCCSwitchReleaseURL(releaseURL); err != nil {
			releaseURL = ccSwitchReleaseFallbackURL(version)
		}
		entry := CCSwitchReleaseVersion{
			Version:     strings.TrimPrefix(version, "v"),
			TagName:     version,
			Name:        release.Name,
			PublishedAt: release.PublishedAt,
			ReleaseURL:  releaseURL,
			Prerelease:  release.Prerelease,
		}
		list.Versions = append(list.Versions, entry)
		if list.LatestVersion == "" && !release.Prerelease {
			list.LatestVersion = entry.Version
		}
	}
	if list.LatestVersion == "" && len(list.Versions) > 0 {
		list.LatestVersion = list.Versions[0].Version
	}
	return list
}

func trimCCSwitchVersionList(list *CCSwitchVersionList, limit int) *CCSwitchVersionList {
	if list == nil {
		return nil
	}
	result := &CCSwitchVersionList{LatestVersion: list.LatestVersion}
	if limit > len(list.Versions) {
		limit = len(list.Versions)
	}
	result.Versions = append([]CCSwitchReleaseVersion(nil), list.Versions[:limit]...)
	return result
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
		release, ok := result.Val.(*GitHubRelease)
		if !ok || release == nil {
			return nil, errors.New("cc switch release fetch returned an invalid result")
		}
		return release, nil
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

func (s *CCSwitchDownloadService) versionRelease(ctx context.Context, version string) (*GitHubRelease, error) {
	if release, err, ok := s.cachedVersionRelease(version); ok {
		return release, err
	}

	resultCh := s.fetch.DoChan("version:"+version, func() (any, error) {
		if release, cachedErr, ok := s.cachedVersionRelease(version); ok {
			return release, cachedErr
		}

		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ccSwitchFetchTimeout)
		defer cancel()
		release, fetchErr := s.fetchVersionRelease(fetchCtx, version)
		if fetchErr == nil && !releaseMatchesCCSwitchVersion(release, version) {
			fetchErr = ErrCCSwitchVersionNotFound
			release = nil
		}
		release, fetchErr = s.storeVersionRelease(version, release, fetchErr)
		return release, fetchErr
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		release, ok := result.Val.(*GitHubRelease)
		if !ok || release == nil {
			return nil, errors.New("cc switch version fetch returned an invalid result")
		}
		return release, nil
	}
}

func (s *CCSwitchDownloadService) fetchVersionRelease(ctx context.Context, version string) (*GitHubRelease, error) {
	if client, ok := s.githubClient.(githubReleaseByTagClient); ok {
		release, err := client.FetchReleaseByTag(ctx, ccSwitchRepository, version)
		if errors.Is(err, ErrGitHubReleaseNotFound) {
			return nil, ErrCCSwitchVersionNotFound
		}
		return release, err
	}

	// Compatibility path for alternate GitHubReleaseClient implementations.
	releases, err := s.githubClient.FetchRecentReleases(ctx, ccSwitchRepository, 100)
	if err != nil {
		return nil, err
	}
	for _, release := range releases {
		if releaseMatchesCCSwitchVersion(release, version) {
			return release, nil
		}
	}
	return nil, ErrCCSwitchVersionNotFound
}

func (s *CCSwitchDownloadService) cachedVersionRelease(version string) (*GitHubRelease, error, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.versions[version]
	if !ok || !s.now().Before(entry.expiresAt) {
		return nil, nil, false
	}
	return entry.release, entry.err, true
}

func (s *CCSwitchDownloadService) storeVersionRelease(version string, release *GitHubRelease, fetchErr error) (*GitHubRelease, error) {
	now := s.now()
	ttl := s.cacheTTL
	if fetchErr != nil {
		ttl = ccSwitchFailureTTL
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.versions == nil {
		s.versions = make(map[string]cachedCCSwitchRelease)
	}
	if stale, ok := s.versions[version]; fetchErr != nil && ok && stale.release != nil {
		release = stale.release
		fetchErr = nil
		ttl = ccSwitchFailureTTL
	}
	if _, exists := s.versions[version]; !exists && len(s.versions) >= ccSwitchVersionCacheLimit {
		for cachedVersion, entry := range s.versions {
			if !now.Before(entry.expiresAt) {
				delete(s.versions, cachedVersion)
			}
		}
		if len(s.versions) >= ccSwitchVersionCacheLimit {
			for cachedVersion := range s.versions {
				delete(s.versions, cachedVersion)
				break
			}
		}
	}
	s.versions[version] = cachedCCSwitchRelease{release: release, err: fetchErr, expiresAt: now.Add(ttl)}
	return release, fetchErr
}

func normalizeCCSwitchVersion(rawVersion string) (string, error) {
	version := strings.TrimSpace(rawVersion)
	if version == "" || strings.EqualFold(version, "latest") {
		return "", nil
	}
	if len(version) > 64 {
		return "", ErrInvalidCCSwitchVersion
	}
	if version[0] == 'v' || version[0] == 'V' {
		version = version[1:]
	}
	if version == "" || version[0] < '0' || version[0] > '9' {
		return "", ErrInvalidCCSwitchVersion
	}
	for i := 0; i < len(version); i++ {
		character := version[i]
		if (character >= '0' && character <= '9') ||
			(character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			character == '.' || character == '-' || character == '+' || character == '_' {
			continue
		}
		return "", ErrInvalidCCSwitchVersion
	}
	return "v" + version, nil
}

func releaseMatchesCCSwitchVersion(release *GitHubRelease, version string) bool {
	if release == nil || release.Draft {
		return false
	}
	releaseVersion, err := normalizeCCSwitchVersion(release.TagName)
	return err == nil && releaseVersion == version
}

func ccSwitchReleaseFallbackURL(version string) string {
	if version == "" {
		return ccSwitchReleaseURL
	}
	return "https://github.com/farion1231/cc-switch/releases/tag/" + url.PathEscape(version)
}

func ccSwitchDownloadURLMatchesVersion(rawURL, requestedVersion, releaseTag string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	const prefix = "/farion1231/cc-switch/releases/download/"
	path := parsed.EscapedPath()
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(segments) != 2 {
		return false
	}
	tag, err := url.PathUnescape(segments[0])
	if err != nil {
		return false
	}
	normalizedTag, err := normalizeCCSwitchVersion(tag)
	if err != nil {
		return false
	}
	if requestedVersion != "" {
		return normalizedTag == requestedVersion
	}
	normalizedReleaseTag, err := normalizeCCSwitchVersion(releaseTag)
	return err == nil && normalizedTag == normalizedReleaseTag
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
	if parsed.RawQuery != "" || parsed.Fragment != "" || !strings.HasPrefix(parsed.EscapedPath(), "/farion1231/cc-switch/releases/") {
		return errors.New("invalid release URL path")
	}
	return nil
}
