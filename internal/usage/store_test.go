package usage

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"database/sql"
)

func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	pgCtr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	testcontainers.CleanupContainer(t, pgCtr)
	require.NoError(t, err)

	connStr, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.PingContext(ctx))

	_, err = Migrate(ctx, db)
	require.NoError(t, err)

	return db
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestMigrate(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)

	// Running a second time should be idempotent.
	version, err := Migrate(ctx, db)
	require.NoError(t, err)
	assert.Equal(t, expectedSchemaVersion, version)
}

func TestCheckSchemaVersion(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)

	err := CheckSchemaVersion(ctx, db)
	require.NoError(t, err)
}

func TestStore_RecordAndQuery(t *testing.T) {
	db := setupDB(t)
	logger := newTestLogger()

	store := NewStore(db, logger, 100, 100*time.Millisecond)

	now := time.Now().UTC().Truncate(time.Second)
	store.Record(UsageEvent{
		Time:         now,
		TenantID:     "tenant-a",
		UserSub:      "user-1",
		Provider:     "openai",
		Model:        "gpt-4",
		InputTokens:  100,
		OutputTokens: 50,
		ResponseID:   "resp-1",
		Stream:       false,
	})
	store.Record(UsageEvent{
		Time:         now,
		TenantID:     "tenant-a",
		UserSub:      "user-2",
		Provider:     "anthropic",
		Model:        "claude-3",
		InputTokens:  200,
		OutputTokens: 80,
		ResponseID:   "resp-2",
		Stream:       true,
	})

	// Force flush.
	require.NoError(t, store.Close())

	ctx := context.Background()

	t.Run("QuerySummary no filter", func(t *testing.T) {
		rows, err := store.QuerySummary(ctx, QueryFilter{})
		require.NoError(t, err)
		assert.Len(t, rows, 2)
		total := int64(0)
		for _, r := range rows {
			total += r.TotalTokens
		}
		assert.Equal(t, int64(430), total)
	})

	t.Run("QuerySummary filter by tenant", func(t *testing.T) {
		rows, err := store.QuerySummary(ctx, QueryFilter{TenantID: "tenant-a"})
		require.NoError(t, err)
		assert.Len(t, rows, 2)
	})

	t.Run("QuerySummary filter by model", func(t *testing.T) {
		rows, err := store.QuerySummary(ctx, QueryFilter{Model: "gpt-4"})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, int64(150), rows[0].TotalTokens)
	})

	t.Run("QueryTop by model", func(t *testing.T) {
		rows, err := store.QueryTop(ctx, QueryFilter{}, "model", 5)
		require.NoError(t, err)
		assert.Len(t, rows, 2)
		// claude-3 has more total tokens (280) so should be first.
		assert.Equal(t, "claude-3", rows[0].Key)
		assert.Equal(t, int64(280), rows[0].TotalTokens)
	})

	t.Run("QueryTop by provider", func(t *testing.T) {
		rows, err := store.QueryTop(ctx, QueryFilter{}, "provider", 5)
		require.NoError(t, err)
		assert.Len(t, rows, 2)
	})

	t.Run("QueryTop invalid dimension", func(t *testing.T) {
		_, err := store.QueryTop(ctx, QueryFilter{}, "invalid", 5)
		assert.Error(t, err)
	})

	t.Run("QueryTop default limit", func(t *testing.T) {
		rows, err := store.QueryTop(ctx, QueryFilter{}, "model", 0)
		require.NoError(t, err)
		assert.Len(t, rows, 2)
	})

	t.Run("QueryTrends daily", func(t *testing.T) {
		f := QueryFilter{
			Start: now.Add(-time.Hour),
			End:   now.Add(time.Hour),
		}
		rows, err := store.QueryTrends(ctx, f, "daily", "")
		require.NoError(t, err)
		assert.NotEmpty(t, rows)
		total := int64(0)
		for _, r := range rows {
			total += r.TotalTokens
		}
		assert.Equal(t, int64(430), total)
	})

	t.Run("QueryTrends hourly with dimension", func(t *testing.T) {
		f := QueryFilter{
			Start: now.Add(-time.Hour),
			End:   now.Add(time.Hour),
		}
		rows, err := store.QueryTrends(ctx, f, "hourly", "model")
		require.NoError(t, err)
		assert.Len(t, rows, 2)
	})

	t.Run("QueryTrends requires time range", func(t *testing.T) {
		_, err := store.QueryTrends(ctx, QueryFilter{}, "daily", "")
		assert.Error(t, err)
	})

	t.Run("QueryTrends invalid dimension", func(t *testing.T) {
		f := QueryFilter{Start: now.Add(-time.Hour), End: now.Add(time.Hour)}
		_, err := store.QueryTrends(ctx, f, "daily", "invalid")
		assert.Error(t, err)
	})
}

func TestStore_RecordZeroTime(t *testing.T) {
	db := setupDB(t)
	logger := newTestLogger()

	store := NewStore(db, logger, 100, 50*time.Millisecond)

	// Time zero should be replaced with time.Now() in Record.
	store.Record(UsageEvent{
		TenantID:     "tenant-x",
		UserSub:      "user-x",
		Provider:     "openai",
		Model:        "gpt-4",
		InputTokens:  10,
		OutputTokens: 5,
		ResponseID:   "resp-zero-time",
	})

	require.NoError(t, store.Close())

	rows, err := store.QuerySummary(context.Background(), QueryFilter{TenantID: "tenant-x"})
	require.NoError(t, err)
	assert.Len(t, rows, 1)
}

func TestStore_BufferFull(t *testing.T) {
	db := setupDB(t)
	logger := newTestLogger()

	// Buffer of 1 — second event should be dropped without panic.
	store := NewStore(db, logger, 1, time.Minute)

	store.Record(UsageEvent{ResponseID: "r1", Provider: "openai", Model: "gpt-4", InputTokens: 1})
	store.Record(UsageEvent{ResponseID: "r2", Provider: "openai", Model: "gpt-4", InputTokens: 2})

	require.NoError(t, store.Close())
}

func TestStore_DefaultParams(t *testing.T) {
	db := setupDB(t)
	logger := newTestLogger()

	// Zero values should apply defaults without panic.
	store := NewStore(db, logger, 0, 0)
	require.NoError(t, store.Close())
}

func TestAppendFilters(t *testing.T) {
	tests := []struct {
		name          string
		filter        QueryFilter
		expectedCount int
	}{
		{
			name:          "empty filter",
			filter:        QueryFilter{},
			expectedCount: 0,
		},
		{
			name:          "all fields set",
			filter:        QueryFilter{TenantID: "t", UserSub: "u", Model: "m", Provider: "p", Start: time.Now(), End: time.Now()},
			expectedCount: 6,
		},
		{
			name:          "only time range",
			filter:        QueryFilter{Start: time.Now(), End: time.Now()},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clauses, args := appendFilters(tt.filter, 1)
			assert.Len(t, clauses, tt.expectedCount)
			assert.Len(t, args, tt.expectedCount)
		})
	}
}
