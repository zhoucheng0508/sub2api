-- CUSTOM(VOTE-AI-AUDIT-OBSERVABILITY): redacted per-request diagnostics for
-- incremental audits. Full conversations and credentials must never be stored.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS audit_details JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN content_moderation_logs.audit_details IS
    'Redacted audit target, stage, cache, usage, prefix, local-rule and hash-promotion diagnostics';
