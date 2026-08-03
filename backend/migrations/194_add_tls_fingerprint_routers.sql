-- CUSTOM(VOTE-AI-OPENAI-TLS): route inbound User-Agent to a TLS profile and paired identity headers.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS tls_fingerprint_routers (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    rules       JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tls_fingerprint_routers_enabled
    ON tls_fingerprint_routers (enabled);

COMMENT ON TABLE tls_fingerprint_routers IS
    'Ordered User-Agent rules selecting TLS fingerprints and paired OpenAI identity headers';
