package conversation

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrate_Fresh(t *testing.T) {
	db := setupPostgresDB(t)
	defer db.Close()

	ctx := context.Background()
	version, err := Migrate(ctx, db, "pgx")
	require.NoError(t, err)
	assert.Equal(t, expectedSchemaVersion, version)

	// schema_migrations table should exist with one row
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count))
	assert.Equal(t, 1, count)

	// conversations table should exist
	var tableName sql.NullString
	require.NoError(t, db.QueryRow(`SELECT to_regclass('public.conversations')`).Scan(&tableName))
	assert.True(t, tableName.Valid)
}

func TestMigrate_Idempotent(t *testing.T) {
	db := setupPostgresDB(t)
	defer db.Close()

	ctx := context.Background()

	v1, err := Migrate(ctx, db, "pgx")
	require.NoError(t, err)

	// Running again should be a no-op
	v2, err := Migrate(ctx, db, "pgx")
	require.NoError(t, err)
	assert.Equal(t, v1, v2)

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestCheckSchemaVersion_Match(t *testing.T) {
	db := setupPostgresDB(t)
	defer db.Close()

	ctx := context.Background()
	_, err := Migrate(ctx, db, "pgx")
	require.NoError(t, err)

	assert.NoError(t, CheckSchemaVersion(ctx, db))
}

func TestCheckSchemaVersion_Mismatch(t *testing.T) {
	db := setupPostgresDB(t)
	defer db.Close()

	ctx := context.Background()
	require.NoError(t, createSchemaMigrationsTable(db))

	// Insert a fake version that doesn't match
	_, err := db.Exec(`INSERT INTO schema_migrations (version, description, applied_at) VALUES ($1, $2, $3)`, 999, "fake", time.Now())
	require.NoError(t, err)

	assert.Error(t, CheckSchemaVersion(ctx, db))
}

func TestLoadMigrations(t *testing.T) {
	migrations, err := loadMigrations()
	require.NoError(t, err)
	assert.NotEmpty(t, migrations)

	// Verify ordering
	for i := 1; i < len(migrations); i++ {
		assert.Greater(t, migrations[i].Version, migrations[i-1].Version)
	}
}

func TestNewSQLStore_UsessMigrations(t *testing.T) {
	db := setupPostgresDB(t)
	defer db.Close()

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
	defer store.Close()

	// schema_migrations table must exist and be at the expected version
	var version int
	require.NoError(t, db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version))
	assert.Equal(t, expectedSchemaVersion, version)
}
