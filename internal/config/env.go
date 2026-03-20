package config

import (
	"fmt"
	"strconv"
	"strings"
	"os"
)

// applyEnvOverrides reads well-known environment variables and overrides the
// corresponding fields in cfg. Env vars always take precedence over values
// loaded from the config file or GATEWAY_CONFIG. Unset vars are ignored.
//
// Naming convention: field path in SCREAMING_SNAKE_CASE.
// Lists (slices) are comma-separated strings.
func applyEnvOverrides(cfg *Config) error {
	var err error

	// Server
	setStr(&cfg.Server.Address, "SERVER_ADDRESS")
	if err = setInt64(&cfg.Server.MaxRequestBodySize, "SERVER_MAX_REQUEST_BODY_SIZE"); err != nil {
		return err
	}

	// Logging
	setStr(&cfg.Logging.Format, "LOG_FORMAT")
	setStr(&cfg.Logging.Level, "LOG_LEVEL")

	// Auth
	if err = setBool(&cfg.Auth.Enabled, "AUTH_ENABLED"); err != nil {
		return err
	}
	setStr(&cfg.Auth.Issuer, "AUTH_ISSUER")
	setStr(&cfg.Auth.DiscoveryURL, "AUTH_DISCOVERY_URL")
	setStrSlice(&cfg.Auth.Audiences, "AUTH_AUDIENCE")
	setStr(&cfg.Auth.ClientID, "AUTH_CLIENT_ID")
	setStr(&cfg.Auth.ClientSecret, "AUTH_CLIENT_SECRET")
	setStr(&cfg.Auth.RedirectURI, "AUTH_REDIRECT_URI")
	setStr(&cfg.Auth.AdminEmail, "AUTH_ADMIN_EMAIL")

	// UI
	if err = setBool(&cfg.UI.Enabled, "UI_ENABLED"); err != nil {
		return err
	}
	setStr(&cfg.UI.Claim, "UI_CLAIM")
	setStrSlice(&cfg.UI.AllowedValues, "UI_ALLOWED_VALUES")
	setStrSlice(&cfg.UI.IPAllowlist, "UI_IP_ALLOWLIST")

	// Rate limiting
	if err = setBool(&cfg.RateLimit.Enabled, "RATE_LIMIT_ENABLED"); err != nil {
		return err
	}
	setStr(&cfg.RateLimit.RedisURL, "RATE_LIMIT_REDIS_URL")
	setStrSlice(&cfg.RateLimit.TrustedProxyCIDRs, "RATE_LIMIT_TRUSTED_PROXY_CIDRS")
	if err = setFloat64(&cfg.RateLimit.RequestsPerSecond, "RATE_LIMIT_REQUESTS_PER_SECOND"); err != nil {
		return err
	}
	if err = setInt(&cfg.RateLimit.Burst, "RATE_LIMIT_BURST"); err != nil {
		return err
	}
	if err = setInt(&cfg.RateLimit.MaxPromptTokens, "RATE_LIMIT_MAX_PROMPT_TOKENS"); err != nil {
		return err
	}
	if err = setInt(&cfg.RateLimit.MaxOutputTokens, "RATE_LIMIT_MAX_OUTPUT_TOKENS"); err != nil {
		return err
	}
	if err = setInt(&cfg.RateLimit.MaxConcurrentRequests, "RATE_LIMIT_MAX_CONCURRENT_REQUESTS"); err != nil {
		return err
	}
	if err = setInt64(&cfg.RateLimit.DailyTokenQuota, "RATE_LIMIT_DAILY_TOKEN_QUOTA"); err != nil {
		return err
	}

	// Observability
	if err = setBool(&cfg.Observability.Enabled, "OBSERVABILITY_ENABLED"); err != nil {
		return err
	}
	if err = setBool(&cfg.Observability.Metrics.Enabled, "METRICS_ENABLED"); err != nil {
		return err
	}
	setStr(&cfg.Observability.Metrics.Path, "METRICS_PATH")
	if err = setBool(&cfg.Observability.Tracing.Enabled, "TRACING_ENABLED"); err != nil {
		return err
	}
	setStr(&cfg.Observability.Tracing.ServiceName, "TRACING_SERVICE_NAME")
	setStr(&cfg.Observability.Tracing.Sampler.Type, "TRACING_SAMPLER_TYPE")
	if err = setFloat64(&cfg.Observability.Tracing.Sampler.Rate, "TRACING_SAMPLER_RATE"); err != nil {
		return err
	}
	setStr(&cfg.Observability.Tracing.Exporter.Type, "TRACING_EXPORTER_TYPE")
	setStr(&cfg.Observability.Tracing.Exporter.Endpoint, "TRACING_EXPORTER_ENDPOINT")
	if err = setBool(&cfg.Observability.Tracing.Exporter.Insecure, "TRACING_EXPORTER_INSECURE"); err != nil {
		return err
	}

	// Usage
	if err = setBool(&cfg.Usage.Enabled, "USAGE_ENABLED"); err != nil {
		return err
	}
	setStr(&cfg.Usage.AnalyticsMode, "USAGE_ANALYTICS_MODE")
	setStr(&cfg.Usage.DSN, "USAGE_DSN")
	if err = setInt(&cfg.Usage.BufferSize, "USAGE_BUFFER_SIZE"); err != nil {
		return err
	}
	setStr(&cfg.Usage.FlushInterval, "USAGE_FLUSH_INTERVAL")

	// Conversations
	if err = setBoolPtr(&cfg.Conversations.Enabled, "CONVERSATIONS_ENABLED"); err != nil {
		return err
	}
	if err = setBool(&cfg.Conversations.StoreByDefault, "CONVERSATIONS_STORE_BY_DEFAULT"); err != nil {
		return err
	}
	setStr(&cfg.Conversations.Store, "CONVERSATIONS_STORE")
	setStr(&cfg.Conversations.TTL, "CONVERSATIONS_TTL")
	setStr(&cfg.Conversations.MaxTTL, "CONVERSATIONS_MAX_TTL")
	setStr(&cfg.Conversations.DSN, "CONVERSATIONS_DSN")
	setStr(&cfg.Conversations.Driver, "CONVERSATIONS_DRIVER")
	if err = setInt(&cfg.Conversations.MaxOpenConns, "CONVERSATIONS_MAX_OPEN_CONNS"); err != nil {
		return err
	}
	if err = setInt(&cfg.Conversations.MaxIdleConns, "CONVERSATIONS_MAX_IDLE_CONNS"); err != nil {
		return err
	}
	setStr(&cfg.Conversations.ConnMaxLifetime, "CONVERSATIONS_CONN_MAX_LIFETIME")
	setStr(&cfg.Conversations.ConnMaxIdleTime, "CONVERSATIONS_CONN_MAX_IDLE_TIME")

	return nil
}

func setStr(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func setStrSlice(dst *[]string, key string) {
	v := os.Getenv(key)
	if v == "" {
		return
	}
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	*dst = result
}

func setBool(dst *bool, key string) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("env %s: invalid bool %q", key, v)
	}
	*dst = b
	return nil
}

func setBoolPtr(dst **bool, key string) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("env %s: invalid bool %q", key, v)
	}
	*dst = &b
	return nil
}

func setInt(dst *int, key string) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("env %s: invalid integer %q", key, v)
	}
	*dst = n
	return nil
}

func setInt64(dst *int64, key string) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fmt.Errorf("env %s: invalid integer %q", key, v)
	}
	*dst = n
	return nil
}

func setFloat64(dst *float64, key string) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fmt.Errorf("env %s: invalid float %q", key, v)
	}
	*dst = f
	return nil
}
