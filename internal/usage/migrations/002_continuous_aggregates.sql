-- Continuous aggregates for efficient analytics queries
-- These only work with TimescaleDB; skip gracefully on plain PostgreSQL.

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN

        -- Hourly rollup
        CREATE MATERIALIZED VIEW IF NOT EXISTS token_usage_hourly
        WITH (timescaledb.continuous) AS
        SELECT
            time_bucket('1 hour', time) AS bucket,
            tenant_id,
            user_sub,
            provider,
            model,
            SUM(input_tokens)  AS input_tokens,
            SUM(output_tokens) AS output_tokens,
            SUM(total_tokens)  AS total_tokens,
            COUNT(*)           AS request_count
        FROM token_usage
        GROUP BY bucket, tenant_id, user_sub, provider, model
        WITH NO DATA;

        -- Daily rollup
        CREATE MATERIALIZED VIEW IF NOT EXISTS token_usage_daily
        WITH (timescaledb.continuous) AS
        SELECT
            time_bucket('1 day', time) AS bucket,
            tenant_id,
            user_sub,
            provider,
            model,
            SUM(input_tokens)  AS input_tokens,
            SUM(output_tokens) AS output_tokens,
            SUM(total_tokens)  AS total_tokens,
            COUNT(*)           AS request_count
        FROM token_usage
        GROUP BY bucket, tenant_id, user_sub, provider, model
        WITH NO DATA;

        -- Retention policy: drop raw data older than 90 days
        PERFORM add_retention_policy('token_usage', INTERVAL '90 days', if_not_exists => TRUE);

    END IF;
END
$$;
