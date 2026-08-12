package repository

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	tlsFPRouterCacheKey  = "tls_fingerprint_routers"
	tlsFPRouterPubSubKey = "tls_fingerprint_routers_updated"
	tlsFPRouterCacheTTL  = 24 * time.Hour
)

type tlsFingerprintRouterCache struct {
	rdb   *redis.Client
	mu    sync.RWMutex
	local []*model.TLSFingerprintRouter
}

func NewTLSFingerprintRouterCache(rdb *redis.Client) service.TLSFingerprintRouterCache {
	return &tlsFingerprintRouterCache{rdb: rdb}
}

func (c *tlsFingerprintRouterCache) Get(ctx context.Context) ([]*model.TLSFingerprintRouter, bool) {
	c.mu.RLock()
	if c.local != nil {
		rows := c.local
		c.mu.RUnlock()
		return rows, true
	}
	c.mu.RUnlock()
	data, err := c.rdb.Get(ctx, tlsFPRouterCacheKey).Bytes()
	if err != nil {
		if err != redis.Nil {
			slog.Warn("tls_fp_router_cache_get_failed", "error", err)
		}
		return nil, false
	}
	var rows []*model.TLSFingerprintRouter
	if err := json.Unmarshal(data, &rows); err != nil {
		slog.Warn("tls_fp_router_cache_unmarshal_failed", "error", err)
		return nil, false
	}
	c.mu.Lock()
	c.local = rows
	c.mu.Unlock()
	return rows, true
}

func (c *tlsFingerprintRouterCache) Set(ctx context.Context, rows []*model.TLSFingerprintRouter) error {
	data, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	if err := c.rdb.Set(ctx, tlsFPRouterCacheKey, data, tlsFPRouterCacheTTL).Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.local = rows
	c.mu.Unlock()
	return nil
}

func (c *tlsFingerprintRouterCache) Invalidate(ctx context.Context) error {
	c.mu.Lock()
	c.local = nil
	c.mu.Unlock()
	return c.rdb.Del(ctx, tlsFPRouterCacheKey).Err()
}

func (c *tlsFingerprintRouterCache) NotifyUpdate(ctx context.Context) error {
	return c.rdb.Publish(ctx, tlsFPRouterPubSubKey, "refresh").Err()
}

func (c *tlsFingerprintRouterCache) SubscribeUpdates(ctx context.Context, handler func()) {
	go func() {
		sub := c.rdb.Subscribe(ctx, tlsFPRouterPubSubKey)
		defer func() { _ = sub.Close() }()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-sub.Channel():
				if msg == nil {
					return
				}
				c.mu.Lock()
				c.local = nil
				c.mu.Unlock()
				handler()
			}
		}
	}()
}
