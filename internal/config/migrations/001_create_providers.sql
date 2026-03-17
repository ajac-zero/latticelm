CREATE TABLE IF NOT EXISTS providers (
    name       TEXT PRIMARY KEY,
    type       TEXT NOT NULL,
    config     TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
