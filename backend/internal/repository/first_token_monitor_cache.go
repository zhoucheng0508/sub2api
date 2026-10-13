package repository

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

var firstTokenMonitorScript = redis.NewScript(`
local key = KEYS[1]
local samples = KEYS[3]
local signature = ARGV[2]
local generation = tonumber(ARGV[7])
local stored_generation = tonumber(redis.call('HGET',key,'generation') or '-1')
if generation < stored_generation then return {0,0,''} end
if generation == stored_generation and redis.call('HGET',key,'signature') ~= signature then return {0,0,''} end
if redis.call('HGET', key, 'signature') ~= signature then
  redis.call('DEL', key, samples)
  redis.call('HSET', key, 'signature', signature, 'generation', generation)
end
if ARGV[1] ~= '1' then
  redis.call('DEL',samples)
  redis.call('HDEL',key,'until','reason','trigger_count')
  return {0,0,''}
end
local clock = redis.call('TIME')
local now = tonumber(clock[1])
local now_ms = now*1000 + math.floor(tonumber(clock[2])/1000)
if ARGV[3] ~= '' and not redis.call('SET',KEYS[2],1,'NX','EX',86400) then return {0,0,''} end
local until_at = tonumber(redis.call('HGET', key, 'until') or '0')
if until_at > now then
  return {tonumber(redis.call('HGET',key,'trigger_count') or '0'),until_at,redis.call('HGET',key,'reason') or ''}
end
if until_at > 0 then redis.call('HDEL',key,'until','reason','trigger_count'); redis.call('DEL', samples) end
if ARGV[3] == '' then return {0,0,''} end
local ms = tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE',samples,'-inf',now_ms-600000)
if ms <= tonumber(ARGV[4])*1000 then return {0,0,''} end
redis.call('ZADD',samples,now_ms,KEYS[2])
redis.call('EXPIRE',samples,600)
local count = redis.call('ZCARD',samples)
if count < tonumber(ARGV[5]) then return {count,0,''} end
until_at = now + tonumber(ARGV[6])*60
local reason = 'First token monitor: ' .. count .. ' requests within 10min > ' .. ARGV[4] .. 's; last first token ' .. ms .. 'ms; cooldown ' .. ARGV[6] .. 'min'
redis.call('DEL',samples)
redis.call('HSET',key,'trigger_count',count,'until',until_at,'reason',reason)
return {count,until_at,reason}
`)

func (c *tempUnschedCache) ObserveFirstToken(ctx context.Context, accountID int64, requestID string, ms *int, cfg service.FirstTokenMonitorConfig) (int64, int64, string, error) {
	enabled := "0"
	if cfg.Enabled {
		enabled = "1"
	}
	measurement := ""
	if ms != nil {
		measurement = fmt.Sprint(*ms)
	}
	signature := fmt.Sprintf("rolling10:%d:%d:%d:%s:%d", cfg.ThresholdSeconds, cfg.AnomalyCount, cfg.CooldownMinutes, cfg.Revision, cfg.Generation)
	key := fmt.Sprintf("first_token_monitor:{%d}", accountID)
	// Hash tag co-locates keys if Redis Cluster is introduced later.
	values, err := firstTokenMonitorScript.Run(ctx, c.rdb, []string{key, key + ":request:" + requestID, key + ":samples"}, enabled, signature, measurement, cfg.ThresholdSeconds, cfg.AnomalyCount, cfg.CooldownMinutes, cfg.Generation).Slice()
	if err != nil {
		return 0, 0, "", err
	}
	if len(values) != 3 {
		return 0, 0, "", fmt.Errorf("first token monitor: unexpected result length %d", len(values))
	}
	count, countOK := values[0].(int64)
	until, untilOK := values[1].(int64)
	reason, reasonOK := values[2].(string)
	if !countOK || !untilOK || !reasonOK {
		return 0, 0, "", fmt.Errorf("first token monitor: unexpected result types %T/%T/%T", values[0], values[1], values[2])
	}
	return count, until, reason, nil
}
