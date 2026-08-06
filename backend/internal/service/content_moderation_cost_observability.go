package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	voteaimoderation "github.com/Wei-Shaw/sub2api/internal/custom/voteai/moderation"
)

type contentModerationPricingSnapshot struct {
	version              string
	uncachedInputUSDPerM float64
	cachedInputUSDPerM   float64
	outputUSDPerM        float64
}

type contentModerationCostSnapshot struct {
	estimatedUSD float64
	priced       int64
	unpriced     int64
	byVersionUSD map[string]float64
}

func newContentModerationPricingSnapshot(cfg *ContentModerationConfig) (contentModerationPricingSnapshot, bool) {
	if cfg == nil || !contentModerationPricingConfigured(cfg.AIChat) {
		return contentModerationPricingSnapshot{}, false
	}
	snapshot := contentModerationPricingSnapshot{
		version:              strings.TrimSpace(cfg.AIChat.PricingVersion),
		uncachedInputUSDPerM: *cfg.AIChat.UncachedInputUSDPerMillionTokens,
		cachedInputUSDPerM:   *cfg.AIChat.CachedInputUSDPerMillionTokens,
		outputUSDPerM:        *cfg.AIChat.OutputUSDPerMillionTokens,
	}
	for _, rate := range []float64{snapshot.uncachedInputUSDPerM, snapshot.cachedInputUSDPerM, snapshot.outputUSDPerM} {
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 || rate > maxContentModerationUSDPerMillionTokens {
			return contentModerationPricingSnapshot{}, false
		}
	}
	return snapshot, true
}

func estimateContentModerationCostUSD(usage *voteaimoderation.Usage, pricing contentModerationPricingSnapshot) (float64, bool) {
	if !contentModerationUsageComplete(usage) {
		return 0, false
	}
	cost := (float64(*usage.UncachedPromptTokens)*pricing.uncachedInputUSDPerM +
		float64(*usage.CachedPromptTokens)*pricing.cachedInputUSDPerM +
		float64(*usage.CompletionTokens)*pricing.outputUSDPerM) / 1_000_000
	if math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 {
		return 0, false
	}
	return cost, true
}

// recordContentModerationAuditCost prices a complete usage sample immediately.
// This preserves the rate version that was active for the call instead of
// recalculating historical tokens when an administrator changes pricing.
func (s *ContentModerationService) recordContentModerationAuditCost(usage *voteaimoderation.Usage, cfg *ContentModerationConfig) {
	if s == nil || !contentModerationUsageComplete(usage) {
		return
	}
	pricing, configured := newContentModerationPricingSnapshot(cfg)
	if !configured {
		s.auditCostUnpricedSamples.Add(1)
		return
	}
	cost, ok := estimateContentModerationCostUSD(usage, pricing)
	if !ok {
		s.auditCostUnpricedSamples.Add(1)
		return
	}
	s.auditCostMu.Lock()
	s.auditEstimatedCostUSD += cost
	if s.auditCostByVersionUSD == nil {
		s.auditCostByVersionUSD = make(map[string]float64)
	}
	s.auditCostByVersionUSD[pricing.version] += cost
	s.auditCostMu.Unlock()
	s.auditCostPricedSamples.Add(1)
}

func (s *ContentModerationService) contentModerationAuditCostSnapshot() contentModerationCostSnapshot {
	if s == nil {
		return contentModerationCostSnapshot{}
	}
	s.auditCostMu.RLock()
	snapshot := contentModerationCostSnapshot{
		estimatedUSD: s.auditEstimatedCostUSD,
		priced:       s.auditCostPricedSamples.Load(),
		unpriced:     s.auditCostUnpricedSamples.Load(),
		byVersionUSD: make(map[string]float64, len(s.auditCostByVersionUSD)),
	}
	for version, cost := range s.auditCostByVersionUSD {
		snapshot.byVersionUSD[version] = cost
	}
	s.auditCostMu.RUnlock()
	return snapshot
}

func contentModerationCostCoverage(priced, unpriced, usageUnknown int64) string {
	if priced+unpriced+usageUnknown == 0 {
		return ContentModerationCostCoverageNoSamples
	}
	if priced == 0 {
		return ContentModerationCostCoverageUnknown
	}
	if unpriced > 0 || usageUnknown > 0 {
		return ContentModerationCostCoveragePartial
	}
	return ContentModerationCostCoverageComplete
}

func (s *ContentModerationService) contentModerationMetricsStartedAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	started := s.metricsStartedUnixNano.Load()
	if started == 0 {
		now := time.Now().UTC().UnixNano()
		s.metricsStartedUnixNano.CompareAndSwap(0, now)
		started = s.metricsStartedUnixNano.Load()
	}
	return time.Unix(0, started).UTC()
}

func (s *ContentModerationService) contentModerationBusinessCostUSD(ctx context.Context, since time.Time) (*float64, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	reader, ok := s.repo.(ContentModerationBusinessCostReader)
	if !ok {
		return nil, nil
	}
	cost, err := reader.SumBusinessActualCostSince(ctx, since)
	if err != nil {
		return nil, err
	}
	if math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 {
		return nil, fmt.Errorf("invalid business actual cost: %v", cost)
	}
	return &cost, nil
}
