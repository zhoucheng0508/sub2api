-- CUSTOM(VOTE-AI-RISK-LIFECYCLE): persist structured audit outcomes and
-- moderation-owned account disable state. The ownership row lets the admin
-- unban flow distinguish automatic moderation bans from manual disables.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS audit_status VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS audit_code VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS audit_retryable BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS side_effect_status VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS notification_status VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS side_effect_error TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS content_moderation_user_state (
    user_id                     BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    moderation_owned_disabled   BOOLEAN NOT NULL DEFAULT FALSE,
    disabled_log_id             BIGINT,
    disabled_at                 TIMESTAMPTZ,
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT content_moderation_user_state_disabled_fields CHECK (
        (moderation_owned_disabled = TRUE AND disabled_log_id IS NOT NULL AND disabled_at IS NOT NULL)
        OR
        (moderation_owned_disabled = FALSE AND disabled_log_id IS NULL AND disabled_at IS NULL)
    )
);

COMMENT ON TABLE content_moderation_user_state IS
    'Current moderation-owned user disable state; historical moderation logs remain authoritative history';
COMMENT ON COLUMN content_moderation_user_state.disabled_log_id IS
    'Originating moderation log ID without a foreign key so log retention cleanup cannot erase ownership state';

CREATE INDEX IF NOT EXISTS idx_content_moderation_user_state_owned_disabled
    ON content_moderation_user_state (updated_at DESC)
    WHERE moderation_owned_disabled = TRUE;

-- Direct status changes can come from user administration paths outside the
-- moderation service. Clear stale ownership whenever such a path re-enables
-- (or otherwise transitions away from disabled) a user.
CREATE OR REPLACE FUNCTION vote_ai_clear_content_moderation_ownership_on_user_status_change()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.status = 'disabled' AND NEW.status <> 'disabled' THEN
        UPDATE content_moderation_user_state
        SET moderation_owned_disabled = FALSE,
            disabled_log_id = NULL,
            disabled_at = NULL,
            updated_at = NOW()
        WHERE user_id = NEW.id
          AND moderation_owned_disabled = TRUE;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_vote_ai_content_moderation_user_status_ownership ON users;
CREATE TRIGGER trg_vote_ai_content_moderation_user_status_ownership
AFTER UPDATE OF status ON users
FOR EACH ROW
EXECUTE FUNCTION vote_ai_clear_content_moderation_ownership_on_user_status_change();
