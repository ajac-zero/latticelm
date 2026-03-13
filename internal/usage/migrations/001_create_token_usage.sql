-- Token usage tracking table for OLAP analytics
CREATE TABLE IF NOT EXISTS token_usage (
    time           TIMESTAMPTZ      NOT NULL,
    tenant_id      TEXT             NOT NULL DEFAULT '',
    user_sub       TEXT             NOT NULL DEFAULT '',
    provider       TEXT             NOT NULL,
    model          TEXT             NOT NULL,
    input_tokens   INTEGER          NOT NULL,
    output_tokens  INTEGER          NOT NULL,
    total_tokens   INTEGER          NOT NULL,
    response_id    TEXT             NOT NULL,
    stream         BOOLEAN          NOT NULL DEFAULT false
);

-- Convert to TimescaleDB hypertable if extension is available
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        PERFORM create_hypertable('token_usage', 'time', if_not_exists => TRUE);
    END IF;
END
$$;

-- Indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_token_usage_tenant_time ON token_usage (tenant_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_token_usage_user_time ON token_usage (user_sub, time DESC);
CREATE INDEX IF NOT EXISTS idx_token_usage_model_time ON token_usage (model, time DESC);
