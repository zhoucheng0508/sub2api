package repository

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Unique renewable leases avoid the stale-release and fixed-counter TTL races.
// The shared hash tag keeps all keys used by a script in one Redis cluster slot.
const videoDownloadPrefix = "{media_video_download}:"

var videoDownloadAcquire = redis.NewScript(`
local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000)
for i=1,2 do redis.call('ZREMRANGEBYSCORE',KEYS[i],'-inf',now) end
redis.call('ZREMRANGEBYSCORE',KEYS[3],'-inf',now-60000)
if redis.call('ZCARD',KEYS[3])>=tonumber(ARGV[4]) then return 0 end
redis.call('ZADD',KEYS[3],now,ARGV[1]); redis.call('PEXPIRE',KEYS[3],61000)
if redis.call('ZCARD',KEYS[1])>=tonumber(ARGV[2]) or redis.call('ZCARD',KEYS[2])>=tonumber(ARGV[3]) then return 0 end
for i=1,2 do redis.call('ZADD',KEYS[i],now+60000,ARGV[1]); redis.call('PEXPIRE',KEYS[i],120000) end
return 1
`)

var videoDownloadRefresh = redis.NewScript(`
local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000)
for i=1,2 do
 local score=redis.call('ZSCORE',KEYS[i],ARGV[1])
 if not score or tonumber(score)<=now then return 0 end
end
for i=1,2 do redis.call('ZADD',KEYS[i],now+60000,ARGV[1]); redis.call('PEXPIRE',KEYS[i],120000) end
return 1
`)

var videoDownloadRelease = redis.NewScript(`
for i=1,2 do redis.call('ZREM',KEYS[i],ARGV[1]) end
return 1
`)

var videoDownloadBytes = redis.NewScript(`
local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000)
for i=1,2 do
 local score=redis.call('ZSCORE',KEYS[i],ARGV[1])
 if not score or tonumber(score)<=now then return -1 end
end
local function refill(key,rate,cap)
 local tokens=tonumber(redis.call('HGET',key,'tokens')) or cap
 local last=tonumber(redis.call('HGET',key,'time')) or now
 return math.min(cap,tokens+math.max(0,now-last)*rate/1000)
end
local n=tonumber(ARGV[2]); local gr=tonumber(ARGV[3]); local ur=tonumber(ARGV[4])
local g=refill(KEYS[3],gr,131072); local u=refill(KEYS[4],ur,32768)
local delay=math.max(0,(n-g)*1000/gr,(n-u)*1000/ur)
if delay<=0 then g=g-n; u=u-n end
redis.call('HSET',KEYS[3],'tokens',g,'time',now); redis.call('PEXPIRE',KEYS[3],120000)
redis.call('HSET',KEYS[4],'tokens',u,'time',now); redis.call('PEXPIRE',KEYS[4],120000)
return math.ceil(delay)
`)

type mediaVideoDownloadLimiter struct {
	rdb      *redis.Client
	settings *service.SettingService
}

func NewMediaVideoDownloadLimiter(rdb *redis.Client, settings *service.SettingService) service.MediaVideoDownloadLimiter {
	return &mediaVideoDownloadLimiter{rdb, settings}
}

func (l *mediaVideoDownloadLimiter) Acquire(ctx context.Context, userID int64, limits service.MediaVideoDownloadSettings) (service.MediaVideoDownloadPermit, error) {
	if l == nil || l.rdb == nil || userID <= 0 {
		return nil, service.ErrMediaVideoDownloadUnavailable
	}
	id := uuid.NewString()
	user := strconv.FormatInt(userID, 10)
	keys := []string{videoDownloadPrefix + "active", videoDownloadPrefix + "user:" + user + ":active", videoDownloadPrefix + "user:" + user + ":requests"}
	opCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	allowed, err := videoDownloadAcquire.Run(opCtx, l.rdb, keys, id, limits.GlobalConcurrency, limits.UserConcurrency, limits.UserRequestsPerMinute).Int()
	cancel()
	if err != nil {
		// The server may have acquired the lease even when its reply was lost.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = videoDownloadRelease.Run(cleanupCtx, l.rdb, keys[:2], id).Err()
		cleanupCancel()
		return nil, service.ErrMediaVideoDownloadUnavailable
	}
	if allowed != 1 {
		return nil, service.ErrMediaVideoDownloadLimited
	}
	leaseCtx, leaseCancel := context.WithCancel(ctx)
	p := &mediaVideoDownloadPermit{limiter: l, id: id, keys: keys[:2], byteKeys: []string{keys[0], keys[1], videoDownloadPrefix + "bytes", videoDownloadPrefix + "user:" + user + ":bytes"}, ctx: leaseCtx, cancel: leaseCancel, done: make(chan struct{})}
	go p.heartbeat()
	return p, nil
}

type mediaVideoDownloadPermit struct {
	limiter        *mediaVideoDownloadLimiter
	id             string
	keys, byteKeys []string
	ctx            context.Context
	cancel         context.CancelFunc
	done           chan struct{}
	once           sync.Once
}

func (p *mediaVideoDownloadPermit) Context() context.Context { return p.ctx }

func (p *mediaVideoDownloadPermit) heartbeat() {
	defer close(p.done)
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(p.ctx, 2*time.Second)
			ok, err := videoDownloadRefresh.Run(ctx, p.limiter.rdb, p.keys, p.id).Int()
			cancel()
			if err != nil || ok != 1 {
				p.cancel()
				return
			}
		}
	}
}

func (p *mediaVideoDownloadPermit) WaitN(ctx context.Context, n int) error {
	if n <= 0 || n > 32768 {
		return service.ErrMediaVideoDownloadUnavailable
	}
	for {
		if err := p.ctx.Err(); err != nil {
			return err
		}
		limits, err := p.limiter.settings.GetMediaVideoDownloadSettingsCached(ctx)
		if err != nil {
			return service.ErrMediaVideoDownloadUnavailable
		}
		opCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		delay, err := videoDownloadBytes.Run(opCtx, p.limiter.rdb, p.byteKeys, p.id, n, limits.GlobalKiBPerSecond*1024, limits.UserKiBPerSecond*1024).Int64()
		cancel()
		if err != nil || delay < 0 {
			return service.ErrMediaVideoDownloadUnavailable
		}
		if delay == 0 {
			return nil
		}
		if delay > 1000 {
			delay = 1000
		}
		timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-p.ctx.Done():
			timer.Stop()
			return p.ctx.Err()
		case <-timer.C:
		}
	}
}

func (p *mediaVideoDownloadPermit) Release() {
	p.once.Do(func() {
		p.cancel()
		<-p.done
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = videoDownloadRelease.Run(ctx, p.limiter.rdb, p.keys, p.id).Err()
	})
}
