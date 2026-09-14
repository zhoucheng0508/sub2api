package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const settingKeyMediaVideoDownload = "media_video_download_protection"

// Download limits are independent from chat/image generation limits. Rates are
// aggregate KiB/s, shared by all requests and keys belonging to the same user.
type MediaVideoDownloadSettings struct {
	GlobalConcurrency     int `json:"global_concurrency"`
	UserConcurrency       int `json:"user_concurrency"`
	GlobalKiBPerSecond    int `json:"global_kib_per_second"`
	UserKiBPerSecond      int `json:"user_kib_per_second"`
	UserRequestsPerMinute int `json:"user_requests_per_minute"`
	WriteTimeoutSeconds   int `json:"write_timeout_seconds"`
	MaxDurationSeconds    int `json:"max_duration_seconds"`
}

func DefaultMediaVideoDownloadSettings() MediaVideoDownloadSettings {
	return MediaVideoDownloadSettings{
		GlobalConcurrency: 4, UserConcurrency: 2,
		GlobalKiBPerSecond: 1536, UserKiBPerSecond: 512,
		UserRequestsPerMinute: 60, WriteTimeoutSeconds: 30, MaxDurationSeconds: 1800,
	}
}

func (v MediaVideoDownloadSettings) Validate() error {
	if v.GlobalConcurrency < 1 || v.GlobalConcurrency > 128 || v.UserConcurrency < 1 || v.UserConcurrency > 16 || v.UserConcurrency > v.GlobalConcurrency {
		return infraerrors.New(400, "VIDEO_DOWNLOAD_SETTINGS_INVALID", "全站并发需为 1–128，每用户并发需为 1–16，且不能超过全站并发")
	}
	if v.GlobalKiBPerSecond < 64 || v.GlobalKiBPerSecond > 1048576 || v.UserKiBPerSecond < 64 || v.UserKiBPerSecond > v.GlobalKiBPerSecond {
		return infraerrors.New(400, "VIDEO_DOWNLOAD_SETTINGS_INVALID", "限速需为 64–1048576 KiB/s，每用户限速不能超过全站限速")
	}
	if v.UserRequestsPerMinute < 1 || v.UserRequestsPerMinute > 600 || v.WriteTimeoutSeconds < 5 || v.WriteTimeoutSeconds > 120 || v.MaxDurationSeconds < 60 || v.MaxDurationSeconds > 7200 {
		return infraerrors.New(400, "VIDEO_DOWNLOAD_SETTINGS_INVALID", "请求频率需为 1–600 次/分钟，写入超时需为 5–120 秒，下载总时限需为 60–7200 秒")
	}
	return nil
}

type cachedMediaVideoDownloadSettings struct {
	value   MediaVideoDownloadSettings
	expires time.Time
	err     error
}

func (s *SettingService) GetMediaVideoDownloadSettings(ctx context.Context) (MediaVideoDownloadSettings, error) {
	value := DefaultMediaVideoDownloadSettings()
	if s == nil || s.settingRepo == nil {
		return value, errors.New("video settings repository is unavailable")
	}
	raw, err := s.settingRepo.GetValue(ctx, settingKeyMediaVideoDownload)
	if errors.Is(err, ErrSettingNotFound) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return value, err
	}
	return value, value.Validate()
}

func (s *SettingService) GetMediaVideoDownloadSettingsCached(ctx context.Context) (MediaVideoDownloadSettings, error) {
	if s == nil {
		return MediaVideoDownloadSettings{}, errors.New("video settings unavailable")
	}
	s.mediaVideoDownloadMu.Lock()
	defer s.mediaVideoDownloadMu.Unlock()
	if c := s.mediaVideoDownloadCache; c != nil && time.Now().Before(c.expires) {
		return c.value, c.err
	}
	// One disconnected client must not poison the shared settings refresh.
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	value, err := s.GetMediaVideoDownloadSettings(dbCtx)
	// Briefly cache failures too, so a database outage cannot serialize a new
	// two-second query for every waiting download. Errors still fail closed.
	ttl := 2 * time.Second
	if err != nil {
		ttl = time.Second
	}
	s.mediaVideoDownloadCache = &cachedMediaVideoDownloadSettings{value: value, expires: time.Now().Add(ttl), err: err}
	return value, err
}

func (s *SettingService) SetMediaVideoDownloadSettings(ctx context.Context, value MediaVideoDownloadSettings) error {
	if err := value.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	s.mediaVideoDownloadMu.Lock()
	defer s.mediaVideoDownloadMu.Unlock()
	dbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.settingRepo.Set(dbCtx, settingKeyMediaVideoDownload, string(data)); err != nil {
		return err
	}
	s.mediaVideoDownloadCache = &cachedMediaVideoDownloadSettings{value: value, expires: time.Now().Add(2 * time.Second)}
	return nil
}
