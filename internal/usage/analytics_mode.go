package usage

import (
	"fmt"
	"strings"
)

// AnalyticsMode selects the usage analytics backend behavior.
type AnalyticsMode string

const (
	AnalyticsModePGX        AnalyticsMode = "pgx"
	AnalyticsModeTimescaleDB AnalyticsMode = "timescaledb"
	AnalyticsModeClickHouse AnalyticsMode = "clickhouse"
)

// ParseAnalyticsMode normalizes and validates analytics mode values.
// Empty mode defaults to "pgx".
func ParseAnalyticsMode(raw string) (AnalyticsMode, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch AnalyticsMode(normalized) {
	case "":
		return AnalyticsModePGX, nil
	case AnalyticsModePGX, AnalyticsModeTimescaleDB, AnalyticsModeClickHouse:
		return AnalyticsMode(normalized), nil
	default:
		return "", fmt.Errorf("invalid analytics mode %q (expected \"pgx\", \"timescaledb\", or \"clickhouse\")", raw)
	}
}
