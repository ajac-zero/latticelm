package conversation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrate_Fresh(t *testing.T) {
	db := setupSQLiteDB(t)
	defer db.Close()

	ctx := context.Background()
	version, err := Migrate(ctx, db, "sqlite3")
	require.NoError(t, err)
	assert.Equal(t, expectedSchemaVersion, version)

	// schema_migrations table should exist with one row
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count))
	assert.Equal(t, 1, count)

	// conversations table should exist
	var tableName string
	require.NoError(t, db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='conversations'`).Scan(&tableName))
	assert.Equal(t, "conversations", tableName)
}

func TestMigrate_Idempotent(t *testing.T) {
	db := setupSQLiteDB(t)
	defer db.Close()

	ctx := context.Background()

	v1, err := Migrate(ctx, db, "sqlite3")
	require.NoError(t, err)

	// Running again should be a no-op
	v2, err := Migrate(ctx, db, "sqlite3")
	require.NoError(t, err)
	assert.Equal(t, v1, v2)

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestCheckSchemaVersion_Match(t *testing.T) {
	db := setupSQLiteDB(t)
	defer db.Close()

	ctx := context.Background()
	_, err := Migrate(ctx, db, "sqlite3")
	require.NoError(t, err)

	assert.NoError(t, CheckSchemaVersion(ctx, db))
}

func TestCheckSchemaVersion_Mismatch(t *testing.T) {
	db := setupSQLiteDB(t)
	defer db.Close()

	ctx := context.Background()
	require.NoError(t, createSchemaMigrationsTable(db))

	// Insert a fake version that doesn't match
	_, err := db.Exec(`INSERT INTO schema_migrations (version, description, applied_at) VALUES (999, 'fake', ?)`, time.Now())
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
	db := setupSQLiteDB(t)
	defer db.Close()

	store, err := NewSQLStore(db, "sqlite3", time.Hour)
	require.NoError(t, err)
	defer store.Close()

	// schema_migrations table must exist and be at the expected version
	var version int
	require.NoError(t, db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version))
	assert.Equal(t, expectedSchemaVersion, version)
}
