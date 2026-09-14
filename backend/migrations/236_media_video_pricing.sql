ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS video_price_per_request DECIMAL(20,8);

COMMENT ON COLUMN groups.video_price_per_request IS
    '固定视频订单单价（内部余额单位/条）；NULL 使用系统默认值';

ALTER TABLE media_video_tasks
    ADD COLUMN IF NOT EXISTS price_snapshot DECIMAL(20,8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS hold_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS actual_amount DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS currency VARCHAR(16) NOT NULL DEFAULT 'internal',
    ADD COLUMN IF NOT EXISTS held_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS settled_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS released_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS settlement_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS next_settlement_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_billing_error TEXT;

CREATE INDEX IF NOT EXISTS idx_media_video_tasks_billing_recovery
    ON media_video_tasks(billing_status, next_settlement_at, expires_at);
