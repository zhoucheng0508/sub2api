-- Prepare durable bookkeeping for the explicit post-rollout cleanup command.
--
-- The old task table and groups.video_price_per_request remain intact here so
-- this migration is safe while old and new application versions overlap. Run
-- `go run ./cmd/finalize-legacy-media-video --execute
-- --confirm-all-instances-upgraded` only after every old instance has exited.

CREATE TABLE IF NOT EXISTS legacy_media_video_cleanup_refunds (
    task_id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    amount NUMERIC(20,8) NOT NULL CHECK (amount > 0),
    refunded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    balance_cache_invalidated_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_legacy_media_video_cleanup_pending_cache
    ON legacy_media_video_cleanup_refunds(user_id)
    WHERE balance_cache_invalidated_at IS NULL;

COMMENT ON TABLE legacy_media_video_cleanup_refunds IS
    'Audit and retry state for refunds made while retiring the legacy database-backed video implementation';
