package config

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var configMigrationsFS embed.FS

// Store provides DB-backed storage for providers and models.
// Provider configs are encrypted per-row with AES-256-GCM; the provider type
// is stored in plaintext for queryability. Models have no secrets and are
// stored as plain rows.
//
// Required env vars:
//
//	DATABASE_URL   - PostgreSQL DSN
//	ENCRYPTION_KEY - base64-encoded 32-byte key (generate: openssl rand -base64 32)
type Store struct {
	db  *sql.DB
	key []byte
}

// NewStore creates a Store. encryptionKeyB64 must be the output of
// `openssl rand -base64 32` (decodes to exactly 32 bytes for AES-256-GCM).
func NewStore(db *sql.DB, encryptionKeyB64 string) (*Store, error) {
	key, err := base64.StdEncoding.DecodeString(encryptionKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode ENCRYPTION_KEY: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must decode to 32 bytes (got %d); generate with: openssl rand -base64 32", len(key))
	}
	return &Store{db: db, key: key}, nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Migrate applies all pending config schema migrations. Safe to call on an
// already-migrated database.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS config_schema_migrations (
		version     INTEGER PRIMARY KEY,
		description TEXT NOT NULL,
		applied_at  TIMESTAMP NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create config_schema_migrations: %w", err)
	}

	migrations, err := loadConfigMigrations()
	if err != nil {
		return fmt.Errorf("load config migrations: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `SELECT version FROM config_schema_migrations`)
	if err != nil {
		return fmt.Errorf("query applied config migrations: %w", err)
	}
	defer rows.Close()
	applied := make(map[int]struct{})
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, m := range migrations {
		if _, ok := applied[m.version]; ok {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin config migration %d: %w", m.version, err)
		}
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply config migration %d (%s): %w", m.version, m.description, err)
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO config_schema_migrations (version, description, applied_at) VALUES ($1, $2, $3)`,
			m.version, m.description, time.Now(),
		)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record config migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit config migration %d: %w", m.version, err)
		}
	}
	return nil
}

// IsSeeded reports whether any providers have been stored.
func (s *Store) IsSeeded(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM providers)`).Scan(&exists)
	return exists, err
}

// SeedIfEmpty inserts providers and models only when the store does not yet
// contain any providers. The existence check and seed happen in one
// transaction so concurrent startups cannot double-seed.
func (s *Store) SeedIfEmpty(ctx context.Context, providers map[string]ProviderEntry, models []ModelEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `LOCK TABLE providers IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return fmt.Errorf("lock providers for seed: %w", err)
	}

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM providers)`).Scan(&exists); err != nil {
		return fmt.Errorf("check providers for seed: %w", err)
	}
	if exists {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit empty seed transaction: %w", err)
		}
		committed = true
		return nil
	}

	providerNames := make([]string, 0, len(providers))
	for name := range providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)
	for _, name := range providerNames {
		entry := providers[name]
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal provider %q seed config: %w", name, err)
		}
		encConfig, err := s.encryptBlob(data)
		if err != nil {
			return fmt.Errorf("encrypt provider %q seed config: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO providers (name, type, config, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (name) DO UPDATE SET type = EXCLUDED.type, config = EXCLUDED.config, updated_at = NOW()
		`, name, entry.Type, encConfig); err != nil {
			return fmt.Errorf("seed provider %q: %w", name, err)
		}
	}

	for _, m := range models {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO models (name, provider, provider_model_id, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (name) DO UPDATE SET provider = EXCLUDED.provider, provider_model_id = EXCLUDED.provider_model_id, updated_at = NOW()
		`, m.Name, m.Provider, m.ProviderModelID); err != nil {
			return fmt.Errorf("seed model %q: %w", m.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed transaction: %w", err)
	}
	committed = true
	return nil
}

// Seed inserts providers and models from a seed config. Existing rows are
// overwritten via upsert.
func (s *Store) Seed(ctx context.Context, providers map[string]ProviderEntry, models []ModelEntry) error {
	for name, entry := range providers {
		if err := s.UpsertProvider(ctx, name, entry); err != nil {
			return fmt.Errorf("seed provider %q: %w", name, err)
		}
	}
	for _, m := range models {
		if err := s.UpsertModel(ctx, m); err != nil {
			return fmt.Errorf("seed model %q: %w", m.Name, err)
		}
	}
	return nil
}

// ListProviders returns all providers, decrypting each config blob.
func (s *Store) ListProviders(ctx context.Context) (map[string]ProviderEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, type, config FROM providers`)
	if err != nil {
		return nil, fmt.Errorf("query providers: %w", err)
	}
	defer rows.Close()

	providers := make(map[string]ProviderEntry)
	for rows.Next() {
		var name, typ, encConfig string
		if err := rows.Scan(&name, &typ, &encConfig); err != nil {
			return nil, err
		}
		data, err := s.decryptBlob(encConfig)
		if err != nil {
			return nil, fmt.Errorf("decrypt provider %q config: %w", name, err)
		}
		var entry ProviderEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("unmarshal provider %q config: %w", name, err)
		}
		entry.Type = typ
		providers[name] = entry
	}
	return providers, rows.Err()
}

// ListModels returns all models.
func (s *Store) ListModels(ctx context.Context) ([]ModelEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, provider, provider_model_id FROM models`)
	if err != nil {
		return nil, fmt.Errorf("query models: %w", err)
	}
	defer rows.Close()

	var models []ModelEntry
	for rows.Next() {
		var m ModelEntry
		if err := rows.Scan(&m.Name, &m.Provider, &m.ProviderModelID); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

// UpsertProvider encrypts and stores a provider config, inserting or updating.
func (s *Store) UpsertProvider(ctx context.Context, name string, entry ProviderEntry) error {
	// #nosec G117 - Data is encrypted via encryptBlob() before storage, so the marshaled API key is never exposed.
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal provider %q config: %w", name, err)
	}
	encConfig, err := s.encryptBlob(data)
	if err != nil {
		return fmt.Errorf("encrypt provider %q config: %w", name, err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO providers (name, type, config, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (name) DO UPDATE SET type = EXCLUDED.type, config = EXCLUDED.config, updated_at = NOW()
	`, name, entry.Type, encConfig)
	return err
}

// UpsertModel stores a model entry, inserting or updating.
func (s *Store) UpsertModel(ctx context.Context, m ModelEntry) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO models (name, provider, provider_model_id, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (name) DO UPDATE SET provider = EXCLUDED.provider, provider_model_id = EXCLUDED.provider_model_id, updated_at = NOW()
	`, m.Name, m.Provider, m.ProviderModelID)
	return err
}

// DeleteProvider removes a provider by name.
func (s *Store) DeleteProvider(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM providers WHERE name = $1`, name)
	return err
}

// DeleteModel removes a model by name.
func (s *Store) DeleteModel(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM models WHERE name = $1`, name)
	return err
}

func (s *Store) encryptBlob(data []byte) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (s *Store) decryptBlob(encoded string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

type configMigration struct {
	version     int
	description string
	sql         string
}

func loadConfigMigrations() ([]configMigration, error) {
	entries, err := fs.ReadDir(configMigrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read config migrations dir: %w", err)
	}

	var migrations []configMigration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".sql")
		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("config migration %q must be {version}_{description}.sql", entry.Name())
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("config migration %q: version %q is not an integer", entry.Name(), parts[0])
		}
		data, err := configMigrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read config migration %q: %w", entry.Name(), err)
		}
		migrations = append(migrations, configMigration{
			version:     version,
			description: strings.ReplaceAll(parts[1], "_", " "),
			sql:         string(data),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})
	return migrations, nil
}
