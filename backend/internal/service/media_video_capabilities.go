package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	mediaVideoMaxReferenceImages   = 30
	mediaVideoMaxImageBytes        = 12 << 20
	mediaVideoMaxImagesTotalBytes  = 70 << 20
	mediaVideoCapabilityTTL        = 5 * time.Minute
	mediaVideoCapabilityMaxEntries = 64
)

type MediaVideoModelCapability struct {
	ID                 string   `json:"id"`
	Durations          []int    `json:"durations"`
	MaxReferenceImages int      `json:"max_reference_images"`
	Ratios             []string `json:"ratios,omitempty"`
	Resolutions        []string `json:"resolutions,omitempty"`
}

// Models and creation share this normalized capability contract. Local upload
// safety ceilings are also reflected in the public response, not just validation.
type MediaVideoCapabilities struct {
	Platform            string                      `json:"platform"`
	Models              []MediaVideoModelCapability `json:"models"`
	Ratios              []string                    `json:"ratios"`
	Resolutions         []string                    `json:"resolutions"`
	CameraMovement      []string                    `json:"camera_movement,omitempty"`
	MaxImageBytes       int                         `json:"max_image_bytes"`
	MaxImagesTotalBytes int                         `json:"max_images_total_bytes"`
}

type mediaVideoCapabilityEntry struct {
	value   *MediaVideoCapabilities
	body    json.RawMessage
	expires time.Time
}

type mediaVideoCapabilityCache struct {
	mu      sync.Mutex
	entries map[string]mediaVideoCapabilityEntry
	flight  singleflight.Group
}

var errMediaVideoCapabilitiesUnavailable = errors.New("video capabilities are temporarily unavailable")

func parseMediaVideoCapabilities(body []byte) (*MediaVideoCapabilities, error) {
	var value MediaVideoCapabilities
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, errMediaVideoCapabilitiesUnavailable
	}
	if len(value.Models) == 0 || len(value.Ratios) == 0 || len(value.Resolutions) == 0 || value.MaxImageBytes <= 0 || value.MaxImagesTotalBytes <= 0 {
		return nil, errMediaVideoCapabilitiesUnavailable
	}
	value.MaxImagesTotalBytes = min(value.MaxImagesTotalBytes, mediaVideoMaxImagesTotalBytes)
	value.MaxImageBytes = min(value.MaxImageBytes, mediaVideoMaxImageBytes, value.MaxImagesTotalBytes)
	seen := make(map[string]bool)
	validOptions := func(options []string) bool {
		for _, option := range options {
			if option == "" || strings.TrimSpace(option) != option {
				return false
			}
		}
		return true
	}
	if !validOptions(value.Ratios) || !validOptions(value.Resolutions) {
		return nil, errMediaVideoCapabilitiesUnavailable
	}
	for i := range value.Models {
		model := &value.Models[i]
		if model.ID == "" || strings.TrimSpace(model.ID) != model.ID || seen[model.ID] || len(model.Durations) == 0 || model.MaxReferenceImages < 0 {
			return nil, errMediaVideoCapabilitiesUnavailable
		}
		seen[model.ID] = true
		for _, duration := range model.Durations {
			if duration <= 0 {
				return nil, errMediaVideoCapabilitiesUnavailable
			}
		}
		if !validOptions(model.Ratios) || !validOptions(model.Resolutions) {
			return nil, errMediaVideoCapabilitiesUnavailable
		}
		model.MaxReferenceImages = min(model.MaxReferenceImages, mediaVideoMaxReferenceImages)
	}
	return &value, nil
}

// ValidateSeedanceVideoRequest uses an explicit capability snapshot. It never
// assumes that a particular model name or duration is permanently supported.
func ValidateSeedanceVideoRequest(req MediaVideoCreateRequest, capabilities *MediaVideoCapabilities) error {
	sizes, err := validateMediaVideoRequestBasics(req)
	if err != nil {
		return err
	}
	return validateMediaVideoCapabilities(req, sizes, capabilities)
}

func validateMediaVideoRequestBasics(req MediaVideoCreateRequest) ([]int, error) {
	if strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Prompt) == "" {
		return nil, errors.New("model and prompt are required")
	}
	if req.Duration <= 0 || req.Ratio == "" || req.Resolution == "" {
		return nil, errors.New("duration, ratio and resolution are required")
	}
	if len(req.Images) > mediaVideoMaxReferenceImages {
		return nil, errors.New("too many reference images")
	}
	sizes := make([]int, 0, len(req.Images))
	total := 0
	for _, image := range req.Images {
		parts := strings.SplitN(image, ",", 2)
		if len(parts) != 2 || !strings.HasPrefix(strings.ToLower(parts[0]), "data:image/") || !strings.HasSuffix(strings.ToLower(parts[0]), ";base64") {
			return nil, errors.New("images must be Base64 Data URLs")
		}
		decoded, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, errors.New("invalid Base64 image")
		}
		if len(decoded) == 0 || len(decoded) > mediaVideoMaxImageBytes {
			return nil, errors.New("reference image must contain 1 byte to 12 MiB")
		}
		total += len(decoded)
		if total > mediaVideoMaxImagesTotalBytes {
			return nil, errors.New("reference images exceed 70 MiB total")
		}
		sizes = append(sizes, len(decoded))
	}
	return sizes, nil
}

func validateMediaVideoCapabilities(req MediaVideoCreateRequest, imageSizes []int, capabilities *MediaVideoCapabilities) error {
	if capabilities == nil {
		return errMediaVideoCapabilitiesUnavailable
	}
	for _, model := range capabilities.Models {
		if model.ID != req.Model {
			continue
		}
		if !slices.Contains(model.Durations, req.Duration) {
			return fmt.Errorf("duration is not supported for model %s", req.Model)
		}
		if len(imageSizes) > model.MaxReferenceImages {
			return fmt.Errorf("model %s supports at most %d reference images", req.Model, model.MaxReferenceImages)
		}
		ratios, resolutions := model.Ratios, model.Resolutions
		if len(ratios) == 0 {
			ratios = capabilities.Ratios
		}
		if len(resolutions) == 0 {
			resolutions = capabilities.Resolutions
		}
		if !slices.Contains(ratios, req.Ratio) {
			return errors.New("ratio is not supported for this model")
		}
		if !slices.Contains(resolutions, req.Resolution) {
			return errors.New("resolution is not supported for this model")
		}
		total := 0
		for _, size := range imageSizes {
			if size > capabilities.MaxImageBytes {
				return errors.New("reference image exceeds the current upstream limit")
			}
			total += size
		}
		if total > capabilities.MaxImagesTotalBytes {
			return errors.New("reference images exceed the current upstream total limit")
		}
		return nil
	}
	return errors.New("model is not supported by the current video capabilities")
}

func (s *MediaVideoService) accountVideoCapabilities(ctx context.Context, account Account) (mediaVideoCapabilityEntry, error) {
	proxy := ""
	if account.Proxy != nil {
		proxy = account.Proxy.URL()
	}
	key := hashString(strconv.FormatInt(account.ID, 10) + "\x00" + account.GetCredential("api_key") + "\x00" + proxy)
	cache := &s.capabilities
	lookup := func() (mediaVideoCapabilityEntry, bool) {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		entry, ok := cache.entries[key]
		return entry, ok && time.Now().Before(entry.expires)
	}
	if entry, ok := lookup(); ok {
		return entry, nil
	}
	result := cache.flight.DoChan(key, func() (any, error) {
		if entry, ok := lookup(); ok {
			return entry, nil
		}
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if s.ctx != nil {
			stop := context.AfterFunc(s.ctx, cancel)
			defer stop()
		}
		resp, status, err := s.callResponse(fetchCtx, account, http.MethodGet, "/v1/media/models", nil, "")
		if err != nil {
			return nil, errMediaVideoCapabilitiesUnavailable
		}
		defer func() { _ = resp.Body.Close() }()
		if status < 200 || status >= 300 {
			return nil, errMediaVideoCapabilitiesUnavailable
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
		if err != nil || len(body) > 2<<20 {
			return nil, errMediaVideoCapabilitiesUnavailable
		}
		value, err := parseMediaVideoCapabilities(body)
		if err != nil {
			return nil, err
		}
		normalized, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		entry := mediaVideoCapabilityEntry{value, normalized, time.Now().Add(mediaVideoCapabilityTTL)}
		cache.mu.Lock()
		defer cache.mu.Unlock()
		if cache.entries == nil {
			cache.entries = make(map[string]mediaVideoCapabilityEntry)
		}
		if len(cache.entries) >= mediaVideoCapabilityMaxEntries {
			oldestKey := ""
			var oldestTime time.Time
			for candidate, old := range cache.entries {
				if oldestKey == "" || old.expires.Before(oldestTime) {
					oldestKey, oldestTime = candidate, old.expires
				}
			}
			delete(cache.entries, oldestKey)
		}
		cache.entries[key] = entry
		return entry, nil
	})
	select {
	case <-ctx.Done():
		return mediaVideoCapabilityEntry{}, ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return mediaVideoCapabilityEntry{}, value.Err
		}
		entry, ok := value.Val.(mediaVideoCapabilityEntry)
		if !ok {
			return mediaVideoCapabilityEntry{}, errMediaVideoCapabilitiesUnavailable
		}
		return entry, nil
	}
}

func (s *MediaVideoService) selectVideoAccount(ctx context.Context, accounts []Account, req MediaVideoCreateRequest, imageSizes []int) (Account, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var validationErr error
	lookupFailed := false
	for _, index := range rand.Perm(len(accounts)) {
		if ctx.Err() != nil {
			break
		}
		account := accounts[index]
		entry, err := s.accountVideoCapabilities(ctx, account)
		if err != nil {
			lookupFailed = true
			continue
		}
		if err := validateMediaVideoCapabilities(req, imageSizes, entry.value); err != nil {
			validationErr = err
			continue
		}
		return account, 0, nil
	}
	if validationErr != nil && !lookupFailed {
		return Account{}, http.StatusBadRequest, validationErr
	}
	return Account{}, http.StatusServiceUnavailable, errMediaVideoCapabilitiesUnavailable
}

func (s *MediaVideoService) Models(ctx context.Context, groupID *int64) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var accounts []Account
	var err error
	if groupID != nil {
		accounts, err = s.accounts.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformLaogou)
	} else {
		accounts, err = s.accounts.ListSchedulableByPlatform(ctx, PlatformLaogou)
	}
	if err != nil || len(accounts) == 0 {
		return nil, errors.New("no available Laogou account")
	}
	// Publish one coherent upstream snapshot, not a union that could advertise
	// duration/ratio combinations no individual supplier account supports.
	for _, account := range accounts {
		if ctx.Err() != nil {
			break
		}
		entry, err := s.accountVideoCapabilities(ctx, account)
		if err == nil {
			return append(json.RawMessage(nil), entry.body...), nil
		}
	}
	return nil, errMediaVideoCapabilitiesUnavailable
}
