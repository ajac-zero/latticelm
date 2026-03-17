CREATE TABLE IF NOT EXISTS models (
    name              TEXT PRIMARY KEY,
    provider          TEXT NOT NULL,
    provider_model_id TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMP NOT NULL DEFAULT NOW()
);
