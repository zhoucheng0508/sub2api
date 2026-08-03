package service

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

type TLSFingerprintRouterRepository interface {
	List(context.Context) ([]*model.TLSFingerprintRouter, error)
	GetByID(context.Context, int64) (*model.TLSFingerprintRouter, error)
	Create(context.Context, *model.TLSFingerprintRouter) (*model.TLSFingerprintRouter, error)
	Update(context.Context, *model.TLSFingerprintRouter) (*model.TLSFingerprintRouter, error)
	Delete(context.Context, int64) error
}

type TLSFingerprintRouterCache interface {
	Get(context.Context) ([]*model.TLSFingerprintRouter, bool)
	Set(context.Context, []*model.TLSFingerprintRouter) error
	Invalidate(context.Context) error
	NotifyUpdate(context.Context) error
	SubscribeUpdates(context.Context, func())
}

type TLSFingerprintRouterMatchResult struct {
	Matched                 bool
	RouterID                int64
	RouterName              string
	RuleName                string
	TLSFingerprintProfileID int64
	UpstreamUserAgent       string
	UpstreamOriginator      string
}

type cachedTLSFingerprintRouterRule struct {
	model.TLSFingerprintRouterRule
	pattern string
	regex   *regexp.Regexp
}

type cachedTLSFingerprintRouter struct {
	*model.TLSFingerprintRouter
	rules []cachedTLSFingerprintRouterRule
}

// CUSTOM(VOTE-AI-OPENAI-TLS): resolves inbound User-Agent to one coherent TLS/HTTP identity.
type TLSFingerprintRouterService struct {
	repo  TLSFingerprintRouterRepository
	cache TLSFingerprintRouterCache
	mu    sync.RWMutex
	local map[int64]*cachedTLSFingerprintRouter
}

func NewTLSFingerprintRouterService(repo TLSFingerprintRouterRepository, cache TLSFingerprintRouterCache) *TLSFingerprintRouterService {
	s := &TLSFingerprintRouterService{repo: repo, cache: cache, local: make(map[int64]*cachedTLSFingerprintRouter)}
	ctx := context.Background()
	if err := s.reload(ctx); err != nil {
		logger.LegacyPrintf("service.tls_fp_router", "initial load failed: %v", err)
	}
	if cache != nil {
		cache.SubscribeUpdates(ctx, func() { _ = s.refresh(context.Background()) })
	}
	return s
}

func (s *TLSFingerprintRouterService) List(ctx context.Context) ([]*model.TLSFingerprintRouter, error) {
	return s.repo.List(ctx)
}
func (s *TLSFingerprintRouterService) GetByID(ctx context.Context, id int64) (*model.TLSFingerprintRouter, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *TLSFingerprintRouterService) Create(ctx context.Context, router *model.TLSFingerprintRouter) (*model.TLSFingerprintRouter, error) {
	normalizeTLSFingerprintRouter(router)
	if err := router.Validate(); err != nil {
		return nil, err
	}
	created, err := s.repo.Create(ctx, router)
	if err == nil {
		s.invalidateAndReload()
	}
	return created, err
}

func (s *TLSFingerprintRouterService) Update(ctx context.Context, router *model.TLSFingerprintRouter) (*model.TLSFingerprintRouter, error) {
	normalizeTLSFingerprintRouter(router)
	if err := router.Validate(); err != nil {
		return nil, err
	}
	updated, err := s.repo.Update(ctx, router)
	if err == nil {
		s.invalidateAndReload()
	}
	return updated, err
}

func (s *TLSFingerprintRouterService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidateAndReload()
	return nil
}

func (s *TLSFingerprintRouterService) MatchUserAgent(routerID int64, userAgent string) TLSFingerprintRouterMatchResult {
	result := TLSFingerprintRouterMatchResult{RouterID: routerID}
	if s == nil || routerID <= 0 {
		return result
	}
	router := s.get(routerID)
	if router == nil || !router.Enabled {
		return result
	}
	result.RouterName = router.Name
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		return result
	}
	for _, rule := range router.rules {
		if rule.Enabled && tlsRouterRuleMatches(rule, ua) {
			result.Matched = true
			result.RuleName = rule.Name
			result.TLSFingerprintProfileID = rule.TLSFingerprintProfileID
			result.UpstreamUserAgent = rule.UpstreamUserAgent
			result.UpstreamOriginator = rule.UpstreamOriginator
			return result
		}
	}
	return result
}

func (s *TLSFingerprintRouterService) get(id int64) *cachedTLSFingerprintRouter {
	s.mu.RLock()
	row := s.local[id]
	s.mu.RUnlock()
	if row != nil {
		return row
	}
	if err := s.refresh(context.Background()); err != nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.local[id]
}

func (s *TLSFingerprintRouterService) refresh(ctx context.Context) error {
	if s.cache != nil {
		if rows, ok := s.cache.Get(ctx); ok {
			s.setLocal(rows)
			return nil
		}
	}
	return s.reload(ctx)
}

func (s *TLSFingerprintRouterService) reload(ctx context.Context) error {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Set(ctx, rows)
	}
	s.setLocal(rows)
	return nil
}

func (s *TLSFingerprintRouterService) setLocal(rows []*model.TLSFingerprintRouter) {
	next := make(map[int64]*cachedTLSFingerprintRouter, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		cached := &cachedTLSFingerprintRouter{TLSFingerprintRouter: row}
		for _, rule := range row.Rules {
			rule.MatchType = model.NormalizeTLSRouterMatchType(rule.MatchType)
			pattern := strings.TrimSpace(rule.Pattern)
			entry := cachedTLSFingerprintRouterRule{TLSFingerprintRouterRule: rule, pattern: pattern}
			if rule.MatchType == model.TLSRouterMatchRegex {
				if !rule.CaseSensitive {
					pattern = "(?i)" + pattern
				}
				entry.regex, _ = regexp.Compile(pattern)
			} else if !rule.CaseSensitive {
				entry.pattern = strings.ToLower(pattern)
			}
			cached.rules = append(cached.rules, entry)
		}
		next[row.ID] = cached
	}
	s.mu.Lock()
	s.local = next
	s.mu.Unlock()
}

func (s *TLSFingerprintRouterService) invalidateAndReload() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if s.cache != nil {
		_ = s.cache.Invalidate(ctx)
	}
	if err := s.reload(ctx); err != nil {
		logger.LegacyPrintf("service.tls_fp_router", "reload failed: %v", err)
	}
	if s.cache != nil {
		_ = s.cache.NotifyUpdate(ctx)
	}
}

func normalizeTLSFingerprintRouter(router *model.TLSFingerprintRouter) {
	if router == nil {
		return
	}
	router.Name = strings.TrimSpace(router.Name)
	if router.Rules == nil {
		router.Rules = []model.TLSFingerprintRouterRule{}
	}
	for i := range router.Rules {
		rule := &router.Rules[i]
		rule.Name = strings.TrimSpace(rule.Name)
		rule.Pattern = strings.TrimSpace(rule.Pattern)
		rule.MatchType = model.NormalizeTLSRouterMatchType(rule.MatchType)
		rule.UpstreamUserAgent = strings.TrimSpace(rule.UpstreamUserAgent)
		rule.UpstreamOriginator = strings.TrimSpace(rule.UpstreamOriginator)
	}
}

func tlsRouterRuleMatches(rule cachedTLSFingerprintRouterRule, userAgent string) bool {
	value, pattern := userAgent, rule.pattern
	if !rule.CaseSensitive && rule.MatchType != model.TLSRouterMatchRegex {
		value = strings.ToLower(value)
	}
	switch rule.MatchType {
	case model.TLSRouterMatchPrefix:
		return strings.HasPrefix(value, pattern)
	case model.TLSRouterMatchExact:
		return value == pattern
	case model.TLSRouterMatchRegex:
		return rule.regex != nil && rule.regex.MatchString(userAgent)
	default:
		return strings.Contains(value, pattern)
	}
}
