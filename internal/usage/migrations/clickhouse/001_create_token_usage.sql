CREATE TABLE IF NOT EXISTS token_usage (
    time          DateTime64(3),
    tenant_id     String,
    user_sub      String,
    provider      String,
    model         String,
    input_tokens  Int32,
    output_tokens Int32,
    total_tokens  Int32,
    response_id   String,
    stream        Bool
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(time)
ORDER BY (tenant_id, user_sub, time)
TTL time + INTERVAL 90 DAY DELETE
