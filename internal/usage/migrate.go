package usage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed migrations/clickhouse/*.sql
var clickhouseMigrationsFS embed.FS

const (
	// expectedSchemaVersionOSS is the schema version for OSS analytics (no continuous aggregates).
	expectedSchemaVersionOSS = 1
	// expectedSchemaVersionLicensed is the schema version for licensed analytics.
	expectedSchemaVersionLicensed = 2
)

// migration represents a single schema migration.
type migration struct {
	Version     int
	Description string
	SQL         string
}

// loadMigrations reads and parses all *.sql files from the embedded migrations
// directory. Files must be named {version}_{description}.sql where version is
// a positive integer (e.g. 001_create_token_usage.sql).
func loadMigrations(mode AnalyticsMode) ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".sql")
		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("migration filename %q must be {version}_{description}.sql", entry.Name())
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("migration filename %q: version %q is not an integer", entry.Name(), parts[0])
		}
		if mode == AnalyticsModePGX && version == expectedSchemaVersionLicensed {
			// Skip licensed-only migrations when running in OSS mode.
			continue
		}

		data, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}

		migrations = append(migrations, migration{
			Version:     version,
			Description: strings.ReplaceAll(parts[1], "_", " "),
			SQL:         string(data),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// createSchemaMigrationsTable creates the usage_schema_migrations tracking table
// if it does not already exist. We use a separate table from users/conversations
// to avoid conflicts and allow independent versioning.
func createSchemaMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS usage_schema_migrations (
		version     INTEGER PRIMARY KEY,
		description TEXT NOT NULL,
		applied_at  TIMESTAMP NOT NULL
	)`)
	return err
}

// appliedVersions returns a set of migration versions already recorded in
// usage_schema_migrations.
func appliedVersions(ctx context.Context, db *sql.DB) (map[int]struct{}, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM usage_schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]struct{})
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = struct{}{}
	}
	return applied, rows.Err()
}

// recordMigration inserts a row into usage_schema_migrations inside the given
// transaction.
func recordMigration(ctx context.Context, tx *sql.Tx, driver string, m migration) error {
	var q string
	if driver == "pgx" || driver == "postgres" {
		q = `INSERT INTO usage_schema_migrations (version, description, applied_at) VALUES ($1, $2, $3)`
	} else {
		q = `INSERT INTO usage_schema_migrations (version, description, applied_at) VALUES (?, ?, ?)`
	}
	_, err := tx.ExecContext(ctx, q, m.Version, m.Description, time.Now())
	return err
}

// Migrate applies all pending migrations in order and returns the current
// schema version. It is safe to call on an already-migrated database.
func Migrate(ctx context.Context, db *sql.DB, driver string, mode AnalyticsMode) (int, error) {
	if err := createSchemaMigrationsTable(db); err != nil {
		return 0, fmt.Errorf("create usage_schema_migrations table: %w", err)
	}

	migrations, err := loadMigrations(mode)
	if err != nil {
		return 0, fmt.Errorf("load migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("query applied migrations: %w", err)
	}

	currentVersion := 0
	for v := range applied {
		if v > currentVersion {
			currentVersion = v
		}
	}
	for _, m := range migrations {
		if _, ok := applied[m.Version]; ok {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return currentVersion, fmt.Errorf("begin transaction for migration %d: %w", m.Version, err)
		}

		if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
			_ = tx.Rollback()
			return currentVersion, fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Description, err)
		}

		if err := recordMigration(ctx, tx, driver, m); err != nil {
			_ = tx.Rollback()
			return currentVersion, fmt.Errorf("record migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return currentVersion, fmt.Errorf("commit migration %d: %w", m.Version, err)
		}

		currentVersion = m.Version
	}

	return currentVersion, nil
}

// CheckSchemaVersion verifies that the current schema version matches the
// expected version and returns an error if it does not.
func CheckSchemaVersion(ctx context.Context, db *sql.DB, mode AnalyticsMode) error {
	var version int
	err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM usage_schema_migrations`).Scan(&version)
	if err != nil {
		return fmt.Errorf("query schema version: %w", err)
	}

	expected := expectedSchemaVersionOSS
	if mode == AnalyticsModeTimescaleDB {
		expected = expectedSchemaVersionLicensed
	}
	if version < expected {
		return fmt.Errorf("schema version mismatch: database is at version %d, expected %d; run migrations before starting the server", version, expected)
	}
	return nil
}

// --- ClickHouse migrations ---

// loadClickHouseMigrations reads all SQL files from the embedded clickhouse
// migrations directory, splitting multi-statement files on "---" separators.
func loadClickHouseMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(clickhouseMigrationsFS, "migrations/clickhouse")
	if err != nil {
		return nil, fmt.Errorf("read clickhouse migrations dir: %w", err)
	}

	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".sql")
		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("migration filename %q must be {version}_{description}.sql", entry.Name())
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("migration filename %q: version %q is not an integer", entry.Name(), parts[0])
		}

		data, err := clickhouseMigrationsFS.ReadFile("migrations/clickhouse/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read clickhouse migration %q: %w", entry.Name(), err)
		}

		migrations = append(migrations, migration{
			Version:     version,
			Description: strings.ReplaceAll(parts[1], "_", " "),
			SQL:         string(data),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// MigrateClickHouse applies all pending ClickHouse schema migrations and returns
// the current schema version. It is safe to call on an already-migrated database.
// Each migration file may contain multiple statements separated by "---" on its own line.
func MigrateClickHouse(ctx context.Context, db *sql.DB) (int, error) {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS usage_schema_migrations (
		version     Int32,
		description String,
		applied_at  DateTime
	) ENGINE = MergeTree() ORDER BY version`)
	if err != nil {
		return 0, fmt.Errorf("create clickhouse usage_schema_migrations table: %w", err)
	}

	migrations, err := loadClickHouseMigrations()
	if err != nil {
		return 0, fmt.Errorf("load clickhouse migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("query applied clickhouse migrations: %w", err)
	}

	currentVersion := 0
	for v := range applied {
		if v > currentVersion {
			currentVersion = v
		}
	}

	for _, m := range migrations {
		if _, ok := applied[m.Version]; ok {
			continue
		}

		// ClickHouse DDL does not support multi-statement transactions; execute
		// each statement individually. Statements are separated by "---".
		stmts := strings.Split(m.SQL, "---")
		for _, stmt := range stmts {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return currentVersion, fmt.Errorf("apply clickhouse migration %d (%s): %w", m.Version, m.Description, err)
			}
		}

		_, err = db.ExecContext(ctx,
			`INSERT INTO usage_schema_migrations (version, description, applied_at) VALUES (?, ?, ?)`,
			m.Version, m.Description, time.Now(),
		)
		if err != nil {
			return currentVersion, fmt.Errorf("record clickhouse migration %d: %w", m.Version, err)
		}

		currentVersion = m.Version
	}

	return currentVersion, nil
}
