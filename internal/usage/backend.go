package usage

import "context"

// Backend is the interface satisfied by all usage analytics backends.
type Backend interface {
	Record(evt UsageEvent)
	QuerySummary(ctx context.Context, f QueryFilter) ([]SummaryRow, error)
	QueryTop(ctx context.Context, f QueryFilter, dimension string, limit int) ([]TopRow, error)
	QueryTrends(ctx context.Context, f QueryFilter, granularity string) ([]TrendRow, error)
	Close() error
}
