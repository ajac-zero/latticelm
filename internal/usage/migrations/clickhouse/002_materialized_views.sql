CREATE TABLE IF NOT EXISTS token_usage_hourly (
    bucket        DateTime,
    tenant_id     String,
    user_sub      String,
    provider      String,
    model         String,
    input_tokens  Int64,
    output_tokens Int64,
    total_tokens  Int64,
    request_count Int64
) ENGINE = SummingMergeTree()
ORDER BY (tenant_id, user_sub, provider, model, bucket)
---
CREATE MATERIALIZED VIEW IF NOT EXISTS token_usage_hourly_mv
TO token_usage_hourly
AS SELECT
    toStartOfHour(time) AS bucket,
    tenant_id,
    user_sub,
    provider,
    model,
    SUM(input_tokens)  AS input_tokens,
    SUM(output_tokens) AS output_tokens,
    SUM(total_tokens)  AS total_tokens,
    count()            AS request_count
FROM token_usage
GROUP BY bucket, tenant_id, user_sub, provider, model
---
CREATE TABLE IF NOT EXISTS token_usage_daily (
    bucket        DateTime,
    tenant_id     String,
    user_sub      String,
    provider      String,
    model         String,
    input_tokens  Int64,
    output_tokens Int64,
    total_tokens  Int64,
    request_count Int64
) ENGINE = SummingMergeTree()
ORDER BY (tenant_id, user_sub, provider, model, bucket)
---
CREATE MATERIALIZED VIEW IF NOT EXISTS token_usage_daily_mv
TO token_usage_daily
AS SELECT
    toStartOfDay(time) AS bucket,
    tenant_id,
    user_sub,
    provider,
    model,
    SUM(input_tokens)  AS input_tokens,
    SUM(output_tokens) AS output_tokens,
    SUM(total_tokens)  AS total_tokens,
    count()            AS request_count
FROM token_usage
GROUP BY bucket, tenant_id, user_sub, provider, model
