package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	contentModerationEmailDedupeKeyPrefix   = "content_moderation:email_dedupe:v1:"
	contentModerationEmailDedupeIndexPrefix = "content_moderation:email_dedupe_index:v1:"
	contentModerationEmailDedupeIndexTTL    = 35 * time.Minute
	contentModerationEmailMaxRiskRank       = 3
)

var (
	contentModerationEmailDedupeReserveScript = redis.NewScript(`
local token = ARGV[1]
local scope_count = tonumber(ARGV[2])
local rank_count = tonumber(ARGV[3])
local requested_rank = tonumber(ARGV[4])
local index_ttl = tonumber(ARGV[5])

for scope = 0, scope_count - 1 do
  local first_key = 2 + (scope * rank_count)
  for rank = requested_rank, rank_count - 1 do
    if redis.call('EXISTS', KEYS[first_key + rank]) == 1 then
      return {0, scope + 1}
    end
  end
end

for scope = 0, scope_count - 1 do
  local first_key = 2 + (scope * rank_count)
  local ttl = tonumber(ARGV[6 + scope])
  for rank = 0, requested_rank do
    local marker = KEYS[first_key + rank]
    local acquired = redis.call('SET', marker, token, 'EX', ttl, 'NX')
    if acquired then
      redis.call('SADD', KEYS[1], marker)
    end
  end
end

local current_index_ttl = redis.call('TTL', KEYS[1])
if current_index_ttl < index_ttl then
  redis.call('EXPIRE', KEYS[1], index_ttl)
end
return {1, 0}
`)
	contentModerationEmailDedupeReleaseScript = redis.NewScript(`
local token = ARGV[1]
local deleted = 0
for i = 2, #KEYS do
  local value = redis.call('GET', KEYS[i])
  if value == token then
    redis.call('DEL', KEYS[i])
    redis.call('SREM', KEYS[1], KEYS[i])
    deleted = deleted + 1
  elseif not value then
    redis.call('SREM', KEYS[1], KEYS[i])
  end
end
if redis.call('SCARD', KEYS[1]) == 0 then
  redis.call('DEL', KEYS[1])
end
return deleted
`)
	contentModerationEmailDedupeClearScript = redis.NewScript(`
local members = redis.call('SMEMBERS', KEYS[1])
for _, key in ipairs(members) do
  redis.call('DEL', key)
end
redis.call('DEL', KEYS[1])
return #members
`)
	contentModerationUserStateClearScript = redis.NewScript(`
local risk_members = redis.call('SMEMBERS', KEYS[1])
local email_members = redis.call('SMEMBERS', KEYS[2])
for _, key in ipairs(risk_members) do
  redis.call('DEL', key)
end
for _, key in ipairs(email_members) do
  redis.call('DEL', key)
end
redis.call('DEL', KEYS[1], KEYS[2])
redis.call('INCR', KEYS[3])
return #risk_members + #email_members
`)
)

var _ service.ContentModerationEmailDedupeStore = (*contentModerationHashCache)(nil)

func (c *contentModerationHashCache) TryReserveContentModerationEmail(ctx context.Context, userID int64, scopes []service.ContentModerationEmailDedupeScope, riskRank int) (service.ContentModerationEmailDedupeReserveResult, error) {
	empty := service.ContentModerationEmailDedupeReserveResult{ConflictScopeIndex: -1}
	if c == nil || c.rdb == nil {
		return empty, errors.New("content moderation email dedupe Redis client is unavailable")
	}
	if userID <= 0 || len(scopes) == 0 {
		return empty, errors.New("content moderation email dedupe identity is incomplete")
	}
	if riskRank < 0 || riskRank > contentModerationEmailMaxRiskRank {
		return empty, fmt.Errorf("content moderation email dedupe risk rank %d is invalid", riskRank)
	}
	normalizedScopes := make([]service.ContentModerationEmailDedupeScope, len(scopes))
	seenScopes := make(map[string]struct{}, len(scopes))
	maxTTL := time.Duration(0)
	for i := range scopes {
		scopeHash := strings.TrimSpace(scopes[i].Hash)
		if scopeHash == "" {
			return empty, errors.New("content moderation email dedupe scope hash is empty")
		}
		if scopes[i].TTL <= 0 {
			return empty, errors.New("content moderation email dedupe TTL must be positive")
		}
		if _, exists := seenScopes[scopeHash]; exists {
			return empty, errors.New("content moderation email dedupe scope hashes must be unique")
		}
		seenScopes[scopeHash] = struct{}{}
		normalizedScopes[i] = service.ContentModerationEmailDedupeScope{Hash: scopeHash, TTL: scopes[i].TTL}
		maxTTL = max(maxTTL, scopes[i].TTL)
	}
	leaseToken, err := newContentModerationEmailDedupeLeaseToken()
	if err != nil {
		return empty, err
	}

	indexKey := contentModerationEmailDedupeIndexKey(userID)
	keys := make([]string, 0, 1+len(normalizedScopes)*(contentModerationEmailMaxRiskRank+1))
	keys = append(keys, indexKey)
	for _, scope := range normalizedScopes {
		for rank := 0; rank <= contentModerationEmailMaxRiskRank; rank++ {
			keys = append(keys, contentModerationEmailDedupeMarkerKey(userID, scope.Hash, rank))
		}
	}
	indexTTLSeconds := max(durationSecondsCeiling(maxTTL), int64(contentModerationEmailDedupeIndexTTL/time.Second))
	args := make([]any, 0, 5+len(normalizedScopes))
	args = append(args, leaseToken, len(normalizedScopes), contentModerationEmailMaxRiskRank+1, riskRank, indexTTLSeconds)
	for _, scope := range normalizedScopes {
		args = append(args, durationSecondsCeiling(scope.TTL))
	}
	values, err := contentModerationEmailDedupeReserveScript.Run(ctx, c.rdb, keys, args...).Slice()
	if err != nil {
		return empty, err
	}
	if len(values) != 2 {
		return empty, fmt.Errorf("content moderation email dedupe reserve returned %d values", len(values))
	}
	acquired, err := redisScriptInt64(values[0])
	if err != nil {
		return empty, fmt.Errorf("decode content moderation email dedupe reserve result: %w", err)
	}
	conflictScope, err := redisScriptInt64(values[1])
	if err != nil {
		return empty, fmt.Errorf("decode content moderation email dedupe conflict scope: %w", err)
	}
	if acquired != 1 {
		empty.ConflictScopeIndex = int(conflictScope) - 1
		return empty, nil
	}
	lease := &service.ContentModerationEmailDedupeLease{
		UserID:   userID,
		Token:    leaseToken,
		Scopes:   normalizedScopes,
		RiskRank: riskRank,
	}
	return service.ContentModerationEmailDedupeReserveResult{
		Acquired:           true,
		ConflictScopeIndex: -1,
		Lease:              lease,
	}, nil
}

func (c *contentModerationHashCache) ReleaseContentModerationEmail(ctx context.Context, lease service.ContentModerationEmailDedupeLease) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, errors.New("content moderation email dedupe Redis client is unavailable")
	}
	lease.Token = strings.TrimSpace(lease.Token)
	if lease.UserID <= 0 || lease.Token == "" || len(lease.Scopes) == 0 {
		return 0, errors.New("content moderation email dedupe lease is incomplete")
	}
	if lease.RiskRank < 0 || lease.RiskRank > contentModerationEmailMaxRiskRank {
		return 0, fmt.Errorf("content moderation email dedupe lease risk rank %d is invalid", lease.RiskRank)
	}
	keys := make([]string, 0, 1+len(lease.Scopes)*(lease.RiskRank+1))
	keys = append(keys, contentModerationEmailDedupeIndexKey(lease.UserID))
	for _, scope := range lease.Scopes {
		scopeHash := strings.TrimSpace(scope.Hash)
		if scopeHash == "" {
			return 0, errors.New("content moderation email dedupe lease scope hash is empty")
		}
		for rank := 0; rank <= lease.RiskRank; rank++ {
			keys = append(keys, contentModerationEmailDedupeMarkerKey(lease.UserID, scopeHash, rank))
		}
	}
	return contentModerationEmailDedupeReleaseScript.Run(ctx, c.rdb, keys, lease.Token).Int64()
}

func (c *contentModerationHashCache) ClearContentModerationEmailDedupe(ctx context.Context, userID int64) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, errors.New("content moderation email dedupe Redis client is unavailable")
	}
	if userID <= 0 {
		return 0, nil
	}
	return contentModerationEmailDedupeClearScript.Run(ctx, c.rdb, []string{contentModerationEmailDedupeIndexKey(userID)}).Int64()
}

// ClearContentModerationUserState removes only short-lived state associated
// with one user. Historical logs and the global flagged-input hash set are not
// part of either per-user index and remain untouched.
func (c *contentModerationHashCache) ClearContentModerationUserState(ctx context.Context, userID int64) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, errors.New("content moderation user state Redis client is unavailable")
	}
	if userID <= 0 {
		return 0, nil
	}
	keys := []string{
		contentModerationSessionRiskIndexKey(userID),
		contentModerationEmailDedupeIndexKey(userID),
		contentModerationUserEpochKey(userID),
	}
	return contentModerationUserStateClearScript.Run(ctx, c.rdb, keys).Int64()
}

func contentModerationEmailDedupeIndexKey(userID int64) string {
	return fmt.Sprintf("%s{%d}", contentModerationEmailDedupeIndexPrefix, userID)
}

func contentModerationEmailDedupeMarkerKey(userID int64, scopeHash string, riskRank int) string {
	return fmt.Sprintf("%s{%d}:%s:%d", contentModerationEmailDedupeKeyPrefix, userID, scopeHash, riskRank)
}

func newContentModerationEmailDedupeLeaseToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate content moderation email dedupe lease token: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func durationSecondsCeiling(ttl time.Duration) int64 {
	return max(int64(1), int64((ttl+time.Second-1)/time.Second))
}

func redisScriptInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case string:
		var decoded int64
		if _, err := fmt.Sscan(typed, &decoded); err != nil {
			return 0, err
		}
		return decoded, nil
	case []byte:
		return redisScriptInt64(string(typed))
	default:
		return 0, fmt.Errorf("unexpected Redis integer type %T", value)
	}
}
