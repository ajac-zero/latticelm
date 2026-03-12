package usage

import (
	"context"
	"database/sql"
	"log/slog"
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
}

// NewStore creates a usage store with a background flush goroutine.
// Call Close() to flush remaining events and stop the goroutine.
func NewStore(db *sql.DB, logger *slog.Logger, bufferSize int, flushInterval time.Duration) *Store {
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
