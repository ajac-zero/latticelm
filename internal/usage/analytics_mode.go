package usage

import (
	"fmt"
	"strings"
)

// AnalyticsMode selects the usage analytics backend behavior.
type AnalyticsMode string

const (
	AnalyticsModeOSS      AnalyticsMode = "oss"
	AnalyticsModeLicensed AnalyticsMode = "licensed"
)

// ParseAnalyticsMode normalizes and validates analytics mode values.
// Empty mode defaults to "oss".
func ParseAnalyticsMode(raw string) (AnalyticsMode, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch AnalyticsMode(normalized) {
	case "":
		return AnalyticsModeOSS, nil
	case AnalyticsModeOSS, AnalyticsModeLicensed:
		return AnalyticsMode(normalized), nil
	default:
		return "", fmt.Errorf("invalid analytics mode %q (expected \"oss\" or \"licensed\")", raw)
	}
}
