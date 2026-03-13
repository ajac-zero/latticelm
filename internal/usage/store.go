package usage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// UsageEvent represents a single token usage record.
type UsageEvent struct {
	Time         time.Time
	TenantID     string
	UserSub      string
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
	ResponseID   string
	Stream       bool
}

// Store handles buffered, async writes of token usage events.
type Store struct {
	db            *sql.DB
	logger        *slog.Logger
	buffer        chan UsageEvent
	done          chan struct{}
	wg            sync.WaitGroup
	flushInterval time.Duration
	batchSize     int
	analyticsMode AnalyticsMode
}

// NewStore creates a usage store with a background flush goroutine.
// Call Close() to flush remaining events and stop the goroutine.
func NewStore(db *sql.DB, logger *slog.Logger, bufferSize int, flushInterval time.Duration, analyticsMode AnalyticsMode) *Store {
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	s := &Store{
		db:            db,
		logger:        logger,
		buffer:        make(chan UsageEvent, bufferSize),
		done:          make(chan struct{}),
		flushInterval: flushInterval,
		batchSize:     100,
		analyticsMode: analyticsMode,
	}

	s.wg.Add(1)
	go s.flushLoop()

	return s
}

// Record enqueues a usage event for async persistence.
// If the buffer is full the event is dropped and a warning is logged.
func (s *Store) Record(evt UsageEvent) {
	if evt.Time.IsZero() {
		evt.Time = time.Now()
	}

	select {
	case s.buffer <- evt:
	default:
		s.logger.Warn("usage buffer full, dropping event",
			slog.String("response_id", evt.ResponseID),
		)
	}
}

// Close flushes remaining events and stops the background goroutine.
func (s *Store) Close() error {
	close(s.done)
	s.wg.Wait()
	return nil
}

func (s *Store) flushLoop() {
	defer s.wg.Done()

	batch := make([]UsageEvent, 0, s.batchSize)
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case evt, ok := <-s.buffer:
			if !ok {
				// Channel closed — flush remaining
				if len(batch) > 0 {
					s.insertBatch(batch)
				}
				return
			}
			batch = append(batch, evt)
			if len(batch) >= s.batchSize {
				s.insertBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				s.insertBatch(batch)
				batch = batch[:0]
			}

		case <-s.done:
			// Drain remaining buffered events
			for {
				select {
				case evt := <-s.buffer:
					batch = append(batch, evt)
				default:
					if len(batch) > 0 {
						s.insertBatch(batch)
					}
					return
				}
			}
		}
	}
}

func (s *Store) insertBatch(batch []UsageEvent) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.Error("usage batch insert: begin tx", slog.String("error", err.Error()))
		return
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO token_usage (time, tenant_id, user_sub, provider, model, input_tokens, output_tokens, total_tokens, response_id, stream)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`)
	if err != nil {
		s.logger.Error("usage batch insert: prepare", slog.String("error", err.Error()))
		_ = tx.Rollback()
		return
	}
	defer stmt.Close()

	for _, evt := range batch {
		totalTokens := evt.InputTokens + evt.OutputTokens
		_, err := stmt.ExecContext(ctx,
			evt.Time, evt.TenantID, evt.UserSub, evt.Provider, evt.Model,
			evt.InputTokens, evt.OutputTokens, totalTokens,
			evt.ResponseID, evt.Stream,
		)
		if err != nil {
			s.logger.Error("usage batch insert: exec",
				slog.String("response_id", evt.ResponseID),
				slog.String("error", err.Error()),
			)
		}
	}

	if err := tx.Commit(); err != nil {
		s.logger.Error("usage batch insert: commit", slog.String("error", err.Error()))
	}
}

// --- Read query types and methods ---

// SummaryRow holds an aggregated usage summary.
type SummaryRow struct {
	TenantID     string `json:"tenant_id,omitempty"`
	UserSub      string `json:"user_sub,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	RequestCount int64  `json:"request_count"`
}

// QueryFilter holds common filter parameters for usage queries.
type QueryFilter struct {
	TenantID string
	UserSub  string
	Model    string
	Provider string
	Start    time.Time
	End      time.Time
}

// TopRow holds a single entry in the top-consumers result.
type TopRow struct {
	Key          string `json:"key"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	RequestCount int64  `json:"request_count"`
}

// TrendRow holds a single time-bucket in the trends result.
type TrendRow struct {
	Bucket       time.Time `json:"bucket"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	TotalTokens  int64     `json:"total_tokens"`
	RequestCount int64     `json:"request_count"`
}

// hasTimescaleDB checks whether the TimescaleDB extension is available.
func (s *Store) hasTimescaleDB(ctx context.Context) bool {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb')`,
	).Scan(&exists)
	return err == nil && exists
}

// hasUsageRollups checks whether continuous aggregate rollups exist.
func (s *Store) hasUsageRollups(ctx context.Context) bool {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT to_regclass('token_usage_hourly') IS NOT NULL AND to_regclass('token_usage_daily') IS NOT NULL`,
	).Scan(&exists)
	return err == nil && exists
}

// useRollups determines whether to use continuous aggregates for analytics.
func (s *Store) useRollups(ctx context.Context) bool {
	if s.analyticsMode != AnalyticsModeLicensed {
		return false
	}
	return s.hasUsageRollups(ctx)
}

// appendFilters builds WHERE clauses and parameter list from a QueryFilter.
func appendFilters(f QueryFilter, paramIdx int) ([]string, []interface{}) {
	var clauses []string
	var args []interface{}

	if f.TenantID != "" {
		clauses = append(clauses, fmt.Sprintf("tenant_id = $%d", paramIdx))
		args = append(args, f.TenantID)
		paramIdx++
	}
	if f.UserSub != "" {
		clauses = append(clauses, fmt.Sprintf("user_sub = $%d", paramIdx))
		args = append(args, f.UserSub)
		paramIdx++
	}
	if f.Model != "" {
		clauses = append(clauses, fmt.Sprintf("model = $%d", paramIdx))
		args = append(args, f.Model)
		paramIdx++
	}
	if f.Provider != "" {
		clauses = append(clauses, fmt.Sprintf("provider = $%d", paramIdx))
		args = append(args, f.Provider)
		paramIdx++
	}
	if !f.Start.IsZero() {
		clauses = append(clauses, fmt.Sprintf("time >= $%d", paramIdx))
		args = append(args, f.Start)
		paramIdx++
	}
	if !f.End.IsZero() {
		clauses = append(clauses, fmt.Sprintf("time < $%d", paramIdx))
		args = append(args, f.End)
	}

	return clauses, args
}

// QuerySummary returns aggregated token usage grouped by the available dimensions.
func (s *Store) QuerySummary(ctx context.Context, f QueryFilter) ([]SummaryRow, error) {
	table := "token_usage"
	timeCol := "time"
	countExpr := "COUNT(*)"
	if s.useRollups(ctx) {
		// Use hourly aggregate for ranges ≤ 7 days, daily for longer.
		if !f.Start.IsZero() && !f.End.IsZero() && f.End.Sub(f.Start) <= 7*24*time.Hour {
			table = "token_usage_hourly"
		} else {
			table = "token_usage_daily"
		}
		timeCol = "bucket"
		countExpr = "SUM(request_count)"
	}

	clauses, args := appendFilters(f, 1)
	// Rename "time" references to the actual column when querying aggregates.
	for i, c := range clauses {
		clauses[i] = strings.Replace(c, "time ", timeCol+" ", 1)
	}

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	q := fmt.Sprintf(`SELECT tenant_id, user_sub, provider, model,
		SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), %s
		FROM %s %s
		GROUP BY tenant_id, user_sub, provider, model
		ORDER BY SUM(total_tokens) DESC`, countExpr, table, where)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query summary: %w", err)
	}
	defer rows.Close()

	var results []SummaryRow
	for rows.Next() {
		var r SummaryRow
		if err := rows.Scan(&r.TenantID, &r.UserSub, &r.Provider, &r.Model,
			&r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.RequestCount); err != nil {
			return nil, fmt.Errorf("scan summary row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// QueryTop returns the top consumers by total tokens, grouped by the given dimension.
// dimension must be one of "user_sub", "model", "provider", "tenant_id".
func (s *Store) QueryTop(ctx context.Context, f QueryFilter, dimension string, limit int) ([]TopRow, error) {
	switch dimension {
	case "user_sub", "model", "provider", "tenant_id":
	default:
		return nil, fmt.Errorf("invalid dimension %q", dimension)
	}

	if limit <= 0 {
		limit = 10
	}

	table := "token_usage"
	timeCol := "time"
	countExpr := "COUNT(*)"
	if s.useRollups(ctx) {
		if !f.Start.IsZero() && !f.End.IsZero() && f.End.Sub(f.Start) <= 7*24*time.Hour {
			table = "token_usage_hourly"
		} else {
			table = "token_usage_daily"
		}
		timeCol = "bucket"
		countExpr = "SUM(request_count)"
	}

	clauses, args := appendFilters(f, 1)
	for i, c := range clauses {
		clauses[i] = strings.Replace(c, "time ", timeCol+" ", 1)
	}

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	args = append(args, limit)
	limitParam := fmt.Sprintf("$%d", len(args))

	// #nosec G201 -- table/column names are fixed allowlist values; filters remain parameterized.
	q := fmt.Sprintf(`SELECT %s, SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), %s
		FROM %s %s
		GROUP BY %s
		ORDER BY SUM(total_tokens) DESC
		LIMIT %s`, dimension, countExpr, table, where, dimension, limitParam)

	// #nosec G701 -- query string is built from allowlisted identifiers and parameterized filters.
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query top: %w", err)
	}
	defer rows.Close()

	var results []TopRow
	for rows.Next() {
		var r TopRow
		if err := rows.Scan(&r.Key, &r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.RequestCount); err != nil {
			return nil, fmt.Errorf("scan top row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// QueryTrends returns time-bucketed token usage for charting.
// granularity must be "hourly" or "daily".
func (s *Store) QueryTrends(ctx context.Context, f QueryFilter, granularity string) ([]TrendRow, error) {
	if f.Start.IsZero() || f.End.IsZero() {
		return nil, fmt.Errorf("start and end times are required for trends")
	}

	table := "token_usage"
	timeCol := "time"
	countExpr := "COUNT(*)"
	useRollups := s.useRollups(ctx)
	useTSDB := s.hasTimescaleDB(ctx)

	if useRollups {
		switch granularity {
		case "hourly":
			table = "token_usage_hourly"
		default:
			table = "token_usage_daily"
			granularity = "daily"
		}
		timeCol = "bucket"
		countExpr = "SUM(request_count)"
	}

	clauses, args := appendFilters(f, 1)
	for i, c := range clauses {
		clauses[i] = strings.Replace(c, "time ", timeCol+" ", 1)
	}

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	var bucketExpr string
	if useRollups {
		// Querying the continuous aggregate — bucket column is already the bucket.
		bucketExpr = timeCol
	} else {
		if useTSDB {
			// OSS TimescaleDB: use time_bucket on the raw hypertable.
			interval := "1 day"
			if granularity == "hourly" {
				interval = "1 hour"
			}
			bucketExpr = fmt.Sprintf("time_bucket('%s', %s)", interval, timeCol)
		} else {
			// Plain PostgreSQL fallback: use date_trunc on raw table.
			trunc := "day"
			if granularity == "hourly" {
				trunc = "hour"
			}
			bucketExpr = fmt.Sprintf("date_trunc('%s', %s)", trunc, timeCol)
		}
	}

	// #nosec G201 -- bucket/table selection is limited to allowlisted values; filters remain parameterized.
	q := fmt.Sprintf(`SELECT %s AS bucket, SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), %s
		FROM %s %s
		GROUP BY bucket
		ORDER BY bucket ASC`, bucketExpr, countExpr, table, where)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query trends: %w", err)
	}
	defer rows.Close()

	var results []TrendRow
	for rows.Next() {
		var r TrendRow
		if err := rows.Scan(&r.Bucket, &r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.RequestCount); err != nil {
			return nil, fmt.Errorf("scan trend row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
