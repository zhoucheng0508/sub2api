package service

import (
	"context"
	"testing"
	"time"

	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
	"github.com/stretchr/testify/require"
)

type contentModerationBusinessCostTestRepo struct {
	*contentModerationTestRepo
	cost  float64
	err   error
	since time.Time
}

func (r *contentModerationBusinessCostTestRepo) SumBusinessActualCostSince(_ context.Context, since time.Time) (float64, error) {
	r.since = since
	return r.cost, r.err
}

func TestContentModerationAuditCostUsesRateSnapshotAtCallTime(t *testing.T) {
	svc := &ContentModerationService{}
	cfg := defaultContentModerationConfig()
	cfg.AIChat.PricingVersion = "deepseek-2026-08-a"
	cfg.AIChat.UncachedInputUSDPerMillionTokens = costFloat64Ptr(1)
	cfg.AIChat.CachedInputUSDPerMillionTokens = costFloat64Ptr(0.1)
	cfg.AIChat.OutputUSDPerMillionTokens = costFloat64Ptr(2)
	usage := completeCostUsage(1000, 800, 200, 100)

	svc.recordContentModerationAuditUsage(&moderationAPIResult{Stage: voteaimoderation.StageFast, Usage: usage}, 100, cfg)

	cfg.AIChat.PricingVersion = "deepseek-2026-08-b"
	cfg.AIChat.UncachedInputUSDPerMillionTokens = costFloat64Ptr(2)
	cfg.AIChat.CachedInputUSDPerMillionTokens = costFloat64Ptr(0.2)
	cfg.AIChat.OutputUSDPerMillionTokens = costFloat64Ptr(4)
	svc.recordContentModerationAuditUsage(&moderationAPIResult{Stage: voteaimoderation.StageFast, Usage: usage}, 100, cfg)

	snapshot := svc.contentModerationAuditCostSnapshot()
	require.Equal(t, int64(2), snapshot.priced)
	require.Zero(t, snapshot.unpriced)
	require.InDelta(t, 0.00144, snapshot.estimatedUSD, 0.000000001)
	require.InDelta(t, 0.00048, snapshot.byVersionUSD["deepseek-2026-08-a"], 0.000000001)
	require.InDelta(t, 0.00096, snapshot.byVersionUSD["deepseek-2026-08-b"], 0.000000001)
}

func TestContentModerationAuditCostSeparatesUnpricedAndUnknownUsage(t *testing.T) {
	svc := &ContentModerationService{}
	cfg := defaultContentModerationConfig()

	svc.recordContentModerationAuditUsage(&moderationAPIResult{
		Stage: voteaimoderation.StageFast,
		Usage: completeCostUsage(1000, 800, 200, 100),
	}, 100, cfg)
	svc.recordContentModerationAuditUsage(&moderationAPIResult{
		Stage: voteaimoderation.StageFull,
		Usage: &voteaimoderation.Usage{PromptTokens: costIntPtr(100)},
	}, 100, cfg)

	snapshot := svc.contentModerationAuditCostSnapshot()
	require.Zero(t, snapshot.priced)
	require.Equal(t, int64(1), snapshot.unpriced)
	require.Equal(t, int64(1), svc.auditUsageUnknown.Load())
	require.Equal(t, ContentModerationCostCoverageUnknown, contentModerationCostCoverage(snapshot.priced, snapshot.unpriced, svc.auditUsageUnknown.Load()))
}

func TestContentModerationGetStatusReportsCostCoverageAndBusinessRatio(t *testing.T) {
	started := time.Date(2026, 8, 5, 1, 2, 3, 0, time.UTC)
	repo := &contentModerationBusinessCostTestRepo{
		contentModerationTestRepo: &contentModerationTestRepo{},
		cost:                      2,
	}
	svc := &ContentModerationService{
		settingRepo: &contentModerationTestSettingRepo{values: map[string]string{}},
		repo:        repo,
	}
	svc.metricsStartedUnixNano.Store(started.UnixNano())
	cfg := defaultContentModerationConfig()
	cfg.AIChat.PricingVersion = "deepseek-2026-08"
	cfg.AIChat.UncachedInputUSDPerMillionTokens = costFloat64Ptr(1)
	cfg.AIChat.CachedInputUSDPerMillionTokens = costFloat64Ptr(0.1)
	cfg.AIChat.OutputUSDPerMillionTokens = costFloat64Ptr(2)
	svc.recordContentModerationAuditUsage(&moderationAPIResult{
		Stage: voteaimoderation.StageFast,
		Usage: completeCostUsage(1000, 800, 200, 100),
	}, 100, cfg)

	status, err := svc.GetStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, started, status.MetricsStartedAt)
	require.Equal(t, started, repo.since)
	require.Equal(t, ContentModerationCostCoverageComplete, status.AuditCostCoverage)
	require.True(t, status.AuditCostComplete)
	require.False(t, status.AuditCostPartial)
	require.NotNil(t, status.AuditEstimatedCostUSD)
	require.InDelta(t, 0.00048, *status.AuditEstimatedCostUSD, 0.000000001)
	require.Equal(t, int64(1), status.AuditCostPricedSamples)
	require.Zero(t, status.AuditCostUnpricedSamples)
	require.NotNil(t, status.BusinessActualCostUSD)
	require.InDelta(t, 2, *status.BusinessActualCostUSD, 0.000000001)
	require.NotNil(t, status.AuditCostPerBusinessUSD)
	require.InDelta(t, 0.00024, *status.AuditCostPerBusinessUSD, 0.000000001)
}

func TestValidateContentModerationPricingConfigRequiresCompleteVersionedRates(t *testing.T) {
	cfg := defaultContentModerationConfig()
	require.NoError(t, validateContentModerationPricingConfig(cfg.AIChat))
	require.False(t, contentModerationPricingConfigured(cfg.AIChat))

	cfg.AIChat.PricingVersion = "deepseek-2026-08"
	cfg.AIChat.UncachedInputUSDPerMillionTokens = costFloat64Ptr(1)
	require.Error(t, validateContentModerationPricingConfig(cfg.AIChat))

	cfg.AIChat.CachedInputUSDPerMillionTokens = costFloat64Ptr(0.1)
	cfg.AIChat.OutputUSDPerMillionTokens = costFloat64Ptr(2)
	require.NoError(t, validateContentModerationPricingConfig(cfg.AIChat))
	require.True(t, contentModerationPricingConfigured(cfg.AIChat))

	cfg.AIChat.OutputUSDPerMillionTokens = costFloat64Ptr(-1)
	require.Error(t, validateContentModerationPricingConfig(cfg.AIChat))
}

func completeCostUsage(prompt, cached, uncached, completion int) *voteaimoderation.Usage {
	total := prompt + completion
	return &voteaimoderation.Usage{
		PromptTokens:         costIntPtr(prompt),
		CachedPromptTokens:   costIntPtr(cached),
		UncachedPromptTokens: costIntPtr(uncached),
		CompletionTokens:     costIntPtr(completion),
		TotalTokens:          costIntPtr(total),
	}
}

func costIntPtr(value int) *int {
	return &value
}

func costFloat64Ptr(value float64) *float64 {
	return &value
}
