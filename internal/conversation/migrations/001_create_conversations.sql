CREATE TABLE IF NOT EXISTS conversations (
	id         TEXT PRIMARY KEY,
	model      TEXT NOT NULL,
	messages   TEXT NOT NULL,
	owner_iss  TEXT NOT NULL DEFAULT '',
	owner_sub  TEXT NOT NULL DEFAULT '',
	tenant_id  TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
