package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	voteairiskstate "github.com/Wei-Shaw/sub2api/internal/custom/voteai/riskstate"
)

const (
	contentModerationSessionRiskCategory = "ai_session_risk"
	contentModerationCurrentRiskCategory = "ai_current_risk"
	contentModerationActorRiskCategory   = "ai_actor_bonus"
)

type contentModerationTierResult struct {
	Tier            string
	CurrentScore    float64
	CumulativeScore float64
	ActorBonus      float64
	State           voteairiskstate.State
}

func (cfg *ContentModerationConfig) aiSessionRiskConfig() voteairiskstate.Config {
	if cfg == nil {
		return voteairiskstate.DefaultConfig()
	}
	return voteairiskstate.NormalizeConfig(voteairiskstate.Config{
		ObserveThreshold:  cfg.AIChat.ObserveThreshold,
		BlockThreshold:    cfg.AIChat.ConfidenceThreshold,
		TTL:               time.Duration(cfg.AIChat.SessionRiskTTLMinutes) * time.Minute,
		HalfLife:          time.Duration(cfg.AIChat.SessionRiskHalfLifeMinutes) * time.Minute,
		BlockCooldown:     time.Duration(cfg.AIChat.SessionRiskBlockCooldownMinutes) * time.Minute,
		ModerateIncrement: 0.10,
		ElevatedIncrement: 0.20,
	})
}

func contentModerationRiskIdentity(input ContentModerationCheckInput) (sessionKey, actorKey, sessionHash string) {
	sessionID := strings.TrimSpace(input.SessionID)
	if input.UserID <= 0 || input.APIKeyID <= 0 || sessionID == "" {
		return "", "", ""
	}
	sessionHash = opaqueModerationRiskHash("session-id", sessionID)
	sessionKey = opaqueModerationRiskHash("session", fmt.Sprintf("%d\x00%d\x00%s", input.UserID, input.APIKeyID, sessionID))
	actorKey = opaqueModerationRiskHash("actor", fmt.Sprintf("%d\x00%d", input.UserID, input.APIKeyID))
	return sessionKey, actorKey, sessionHash
}

func opaqueModerationRiskHash(kind, value string) string {
	sum := sha256.Sum256([]byte("vote-ai-risk-v1\x00" + kind + "\x00" + value))
	return hex.EncodeToString(sum[:])
}

func moderationResultCategories(result *moderationAPIResult) []string {
	if result == nil {
		return nil
	}
	categories := make([]string, 0, len(result.CategoryScores))
	for category, score := range result.CategoryScores {
		if category == "ai_risk" || strings.HasPrefix(category, "ai_") || score <= 0 {
			continue
		}
		categories = append(categories, category)
	}
	sort.Strings(categories)
	return categories
}

func (s *ContentModerationService) getBlockedSessionRisk(ctx context.Context, input ContentModerationCheckInput, cfg *ContentModerationConfig) (voteairiskstate.State, bool) {
	state, found := s.getSessionRisk(ctx, input, cfg)
	return state, found && voteairiskstate.IsBlocked(state, time.Now())
}

func (s *ContentModerationService) getSessionRisk(ctx context.Context, input ContentModerationCheckInput, cfg *ContentModerationConfig) (voteairiskstate.State, bool) {
	if s == nil || cfg == nil || !cfg.AIChat.RiskLevelsEnabled || !cfg.AIChat.SessionRiskEnabled {
		return voteairiskstate.State{}, false
	}
	store, ok := s.hashCache.(ContentModerationSessionRiskStore)
	if !ok {
		slog.Warn("content_moderation.session_risk_store_unavailable")
		return voteairiskstate.State{}, false
	}
	sessionKey, _, _ := contentModerationRiskIdentity(input)
	if sessionKey == "" {
		return voteairiskstate.State{}, false
	}
	state, found, err := store.GetContentModerationSessionRisk(ctx, sessionKey)
	if err != nil {
		slog.Warn("content_moderation.session_risk_get_failed", "error", err)
		return voteairiskstate.State{}, false
	}
	return state, found
}

func (s *ContentModerationService) applyAIChatRiskState(ctx context.Context, input ContentModerationCheckInput, cfg *ContentModerationConfig, result *moderationAPIResult) contentModerationTierResult {
	riskCfg := cfg.aiSessionRiskConfig()
	currentScore := 0.0
	if result != nil {
		currentScore = result.CategoryScores["ai_risk"]
	}
	out := contentModerationTierResult{
		Tier:            voteairiskstate.TierForScore(currentScore, riskCfg.ObserveThreshold, riskCfg.BlockThreshold),
		CurrentScore:    currentScore,
		CumulativeScore: currentScore,
	}
	if s == nil || cfg == nil || !cfg.AIChat.SessionRiskEnabled {
		return out
	}
	store, ok := s.hashCache.(ContentModerationSessionRiskStore)
	if !ok {
		slog.Warn("content_moderation.session_risk_store_unavailable")
		return out
	}
	sessionKey, actorKey, sessionHash := contentModerationRiskIdentity(input)
	if sessionKey == "" {
		return out
	}
	event := voteairiskstate.Event{
		Score:       currentScore,
		Categories:  moderationResultCategories(result),
		Signals:     append([]string(nil), result.Signals...),
		RequestID:   input.RequestID,
		SessionHash: sessionHash,
		At:          time.Now().UTC(),
	}
	state, err := store.UpdateContentModerationSessionRisk(ctx, sessionKey, event, riskCfg)
	if err != nil {
		slog.Warn("content_moderation.session_risk_update_failed", "error", err)
		return out
	}
	out.State = state
	out.CumulativeScore = math.Max(currentScore, state.Score)
	if cfg.AIChat.ActorRiskEnabled && actorKey != "" {
		actorCfg := riskCfg
		actorCfg.TTL = 24 * time.Hour
		actorCfg.BlockThreshold = 1
		actorCfg.BlockCooldown = 0
		actorCfg.ModerateIncrement = 0.025
		actorCfg.ElevatedIncrement = 0.05
		actorState, actorErr := store.UpdateContentModerationSessionRisk(ctx, actorKey, event, actorCfg)
		if actorErr != nil {
			slog.Warn("content_moderation.actor_risk_update_failed", "error", actorErr)
		} else {
			out.ActorBonus = voteairiskstate.ActorBonus(actorState)
		}
	}
	out.CumulativeScore = math.Min(1, out.CumulativeScore+out.ActorBonus)
	out.Tier = voteairiskstate.TierForScore(out.CumulativeScore, riskCfg.ObserveThreshold, riskCfg.BlockThreshold)
	return out
}
