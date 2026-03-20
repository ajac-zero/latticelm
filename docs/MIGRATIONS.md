# SQL Schema Migrations

The SQL conversation store uses a minimal built-in migration system based on
embedded SQL files. There is no external tooling dependency.

## How It Works

1. At startup, `NewSQLStore` calls `Migrate`, which:
   - Creates a `schema_migrations` tracking table if it does not exist.
   - Reads all `*.sql` files from `internal/conversation/migrations/`, ordered
     by their numeric prefix.
   - Runs any migration that has not yet been recorded in `schema_migrations`,
     wrapping each in a transaction so a failure leaves the schema unchanged.
2. After all migrations are applied, `CheckSchemaVersion` confirms the database
   is at the expected version. If it is not, the server refuses to start.

## Migration Files

Files live in `internal/conversation/migrations/` and are named:

```
{version}_{description}.sql
```

Examples: `001_create_conversations.sql`, `002_add_index_on_tenant_id.sql`.

The version must be a positive integer. Files are applied in ascending version
order.

## Adding a New Migration

1. Create a new file in `internal/conversation/migrations/` with the next
   sequential version number, e.g. `002_add_index_on_tenant_id.sql`.
2. Write valid DDL for PostgreSQL. Prefer `IF NOT EXISTS` / `IF EXISTS` guards.
3. Increment `expectedSchemaVersion` in `internal/conversation/migrate.go` to
   match the new highest version number.
4. Run `go test ./internal/conversation/...` to verify the migration applies
   cleanly.

## Applying Migrations in Production

Migrations run automatically when the gateway starts. No manual steps are
required under normal circumstances.

### Rollback

The migration system is append-only — it does not support automatic rollback.
To undo a change:

1. Write a new migration that reverses the schema change (e.g. `DROP COLUMN`,
   `DROP INDEX`).
2. Decrement `expectedSchemaVersion` only if the rolled-back version is the
   version the binary now expects.

### schema_migrations Table

| Column      | Type      | Description                        |
|-------------|-----------|------------------------------------|
| version     | INTEGER   | Migration version number (PK)      |
| description | TEXT      | Human-readable description         |
| applied_at  | TIMESTAMP | When the migration was applied     |

You can inspect applied migrations with:

```sql
SELECT * FROM schema_migrations ORDER BY version;
```
