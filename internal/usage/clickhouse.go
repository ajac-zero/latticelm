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

// ClickHouseStore handles buffered, async writes of token usage events to ClickHouse.
type ClickHouseStore struct {
	db            *sql.DB
	logger        *slog.Logger
	buffer        chan UsageEvent
	done          chan struct{}
	wg            sync.WaitGroup
	flushInterval time.Duration
	batchSize     int
}

// NewClickHouseStore creates a ClickHouse usage store with a background flush goroutine.
// Call Close() to flush remaining events and stop the goroutine.
func NewClickHouseStore(db *sql.DB, logger *slog.Logger, bufferSize int, flushInterval time.Duration) *ClickHouseStore {
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	s := &ClickHouseStore{
		db:            db,
		logger:        logger,
		buffer:        make(chan UsageEvent, bufferSize),
		done:          make(chan struct{}),
		flushInterval: flushInterval,
		batchSize:     100,
	}

	s.wg.Add(1)
	go s.flushLoop()

	return s
}

// Record enqueues a usage event for async persistence.
func (s *ClickHouseStore) Record(evt UsageEvent) {
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
func (s *ClickHouseStore) Close() error {
	close(s.done)
	s.wg.Wait()
	return nil
}

func (s *ClickHouseStore) flushLoop() {
	defer s.wg.Done()

	batch := make([]UsageEvent, 0, s.batchSize)
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case evt, ok := <-s.buffer:
			if !ok {
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

// insertBatch sends a batch of events to ClickHouse via the native batch protocol.
// In clickhouse-go/v2's database/sql interface, Begin()/Prepare()/Commit() maps
// to a ClickHouse batch, which is the recommended high-throughput insert path.
func (s *ClickHouseStore) insertBatch(batch []UsageEvent) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	scope, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.Error("clickhouse batch insert: begin", slog.String("error", err.Error()))
		return
	}

	stmt, err := scope.PrepareContext(ctx,
		`INSERT INTO token_usage (time, tenant_id, user_sub, provider, model, input_tokens, output_tokens, total_tokens, response_id, stream)`)
	if err != nil {
		s.logger.Error("clickhouse batch insert: prepare", slog.String("error", err.Error()))
		_ = scope.Rollback()
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
			s.logger.Error("clickhouse batch insert: exec",
				slog.String("response_id", evt.ResponseID),
				slog.String("error", err.Error()),
			)
		}
	}

	if err := scope.Commit(); err != nil {
		s.logger.Error("clickhouse batch insert: commit", slog.String("error", err.Error()))
	}
}

// appendFiltersCH builds ClickHouse WHERE clauses using ? placeholders.
func appendFiltersCH(f QueryFilter, timeCol string) ([]string, []interface{}) {
	var clauses []string
	var args []interface{}

	if f.TenantID != "" {
		clauses = append(clauses, "tenant_id = ?")
		args = append(args, f.TenantID)
	}
	if f.UserSub != "" {
		clauses = append(clauses, "user_sub = ?")
		args = append(args, f.UserSub)
	}
	if f.Model != "" {
		clauses = append(clauses, "model = ?")
		args = append(args, f.Model)
	}
	if f.Provider != "" {
		clauses = append(clauses, "provider = ?")
		args = append(args, f.Provider)
	}
	if !f.Start.IsZero() {
		clauses = append(clauses, timeCol+" >= ?")
		args = append(args, f.Start)
	}
	if !f.End.IsZero() {
		clauses = append(clauses, timeCol+" < ?")
		args = append(args, f.End)
	}

	return clauses, args
}

// QuerySummary returns aggregated token usage grouped by all dimensions.
// Queries the raw token_usage table; ClickHouse's columnar engine handles
// this efficiently without needing pre-aggregated rollups.
func (s *ClickHouseStore) QuerySummary(ctx context.Context, f QueryFilter) ([]SummaryRow, error) {
	clauses, args := appendFiltersCH(f, "time")
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	q := fmt.Sprintf(`SELECT tenant_id, user_sub, provider, model,
		SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), count()
		FROM token_usage %s
		GROUP BY tenant_id, user_sub, provider, model
		ORDER BY SUM(total_tokens) DESC`, where)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse query summary: %w", err)
	}
	defer rows.Close()

	var results []SummaryRow
	for rows.Next() {
		var r SummaryRow
		if err := rows.Scan(&r.TenantID, &r.UserSub, &r.Provider, &r.Model,
			&r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.RequestCount); err != nil {
			return nil, fmt.Errorf("clickhouse scan summary row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// QueryTop returns the top consumers by total tokens for a given dimension.
// dimension must be one of "user_sub", "model", "provider", "tenant_id".
func (s *ClickHouseStore) QueryTop(ctx context.Context, f QueryFilter, dimension string, limit int) ([]TopRow, error) {
	switch dimension {
	case "user_sub", "model", "provider", "tenant_id":
	default:
		return nil, fmt.Errorf("invalid dimension %q", dimension)
	}

	if limit <= 0 {
		limit = 10
	}

	clauses, args := appendFiltersCH(f, "time")
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	args = append(args, limit)

	// #nosec G201 -- dimension is validated against an allowlist above.
	q := fmt.Sprintf(`SELECT %s, SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), count()
		FROM token_usage %s
		GROUP BY %s
		ORDER BY SUM(total_tokens) DESC
		LIMIT ?`, dimension, where, dimension)
	// #nosec G701
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse query top: %w", err)
	}
	defer rows.Close()

	var results []TopRow
	for rows.Next() {
		var r TopRow
		if err := rows.Scan(&r.Key, &r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.RequestCount); err != nil {
			return nil, fmt.Errorf("clickhouse scan top row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// QueryTrends returns time-bucketed token usage from the materialized rollup tables.
// granularity must be "hourly" or "daily".
func (s *ClickHouseStore) QueryTrends(ctx context.Context, f QueryFilter, granularity string) ([]TrendRow, error) {
	if f.Start.IsZero() || f.End.IsZero() {
		return nil, fmt.Errorf("start and end times are required for trends")
	}

	table := "token_usage_daily"
	if granularity == "hourly" {
		table = "token_usage_hourly"
	}

	clauses, args := appendFiltersCH(f, "bucket")
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	// SummingMergeTree requires SUM() because background merges may be pending.
	// #nosec G201 -- table is selected from a fixed allowlist above.
	q := fmt.Sprintf(`SELECT bucket, SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), SUM(request_count)
		FROM %s %s
		GROUP BY bucket
		ORDER BY bucket ASC`, table, where)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse query trends: %w", err)
	}
	defer rows.Close()

	var results []TrendRow
	for rows.Next() {
		var r TrendRow
		if err := rows.Scan(&r.Bucket, &r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.RequestCount); err != nil {
			return nil, fmt.Errorf("clickhouse scan trend row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
