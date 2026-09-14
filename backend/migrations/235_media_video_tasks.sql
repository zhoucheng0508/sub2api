CREATE TABLE IF NOT EXISTS media_video_tasks (
    task_id TEXT PRIMARY KEY,
    upstream_task_id TEXT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    group_id BIGINT,
    upstream_account_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL,
    model TEXT NOT NULL,
    prompt_hash TEXT NOT NULL,
    duration INTEGER NOT NULL,
    ratio TEXT NOT NULL,
    resolution TEXT NOT NULL,
    has_images BOOLEAN NOT NULL DEFAULT FALSE,
    idempotency_key_hash TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    progress DOUBLE PRECISION,
    downloadable BOOLEAN NOT NULL DEFAULT FALSE,
    error JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    last_polled_at TIMESTAMPTZ,
    next_poll_at TIMESTAMPTZ,
    billing_status TEXT NOT NULL DEFAULT 'not_billed',
    CONSTRAINT media_video_tasks_idem_unique UNIQUE(user_id, api_key_id, idempotency_key_hash)
);
CREATE INDEX IF NOT EXISTS idx_media_video_tasks_poll ON media_video_tasks(status, next_poll_at, expires_at);
CREATE INDEX IF NOT EXISTS idx_media_video_tasks_owner ON media_video_tasks(user_id, api_key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_media_video_tasks_expiry ON media_video_tasks(expires_at);
