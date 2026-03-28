package apikeys

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

type migration struct {
	Version     int
	Description string
	SQL         string
}

func loadMigrations() ([]migration, error) {
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

// Migrate applies pending api_keys migrations and returns the current version.
func Migrate(ctx context.Context, db *sql.DB) (int, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS api_keys_schema_migrations (
		version     INTEGER PRIMARY KEY,
		description TEXT NOT NULL,
		applied_at  TIMESTAMP NOT NULL
	)`); err != nil {
		return 0, fmt.Errorf("create api_keys_schema_migrations table: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return 0, fmt.Errorf("load migrations: %w", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT version FROM api_keys_schema_migrations`)
	if err != nil {
		return 0, fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]struct{})
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return 0, err
		}
		applied[v] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	currentVersion := 0
	for _, m := range migrations {
		if _, ok := applied[m.Version]; ok {
			if m.Version > currentVersion {
				currentVersion = m.Version
			}
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

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO api_keys_schema_migrations (version, description, applied_at) VALUES ($1, $2, $3)`,
			m.Version, m.Description, time.Now(),
		); err != nil {
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
