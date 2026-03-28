package ratelimit

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/ajac-zero/latticelm/internal/auth"
)

// Identity represents the rate limiting identity extracted from a request.
type Identity struct {
	Tenant  string
	Subject string
	IP      string
}

// Key returns the rate limiting key for this identity.
func (id Identity) Key() string {
	if id.Subject != "" {
		if id.Tenant != "" && id.Tenant != id.Subject {
			return id.Tenant + ":" + id.Subject
		}
		return id.Subject
	}
	return id.IP
}

// TenantKey returns the tenant-level key (for quotas shared across a tenant).
func (id Identity) TenantKey() string {
	if id.Tenant != "" {
		return id.Tenant
	}
	if id.Subject != "" {
		return id.Subject
	}
	return id.IP
}

// extractIdentity extracts the rate limiting identity from the request.
// It checks the authenticated Principal first, falling back to client IP.
func extractIdentity(r *http.Request, trustedCIDRs []*net.IPNet) Identity {
	id := Identity{
		IP: getClientIP(r, trustedCIDRs),
	}

	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		return id
	}

	id.Subject = principal.Subject
	id.Tenant = principal.TenantID

	if id.Tenant == "" {
		id.Tenant = id.Subject
	}

	return id
}

// getClientIP extracts the client IP, only trusting forwarded headers from known proxy CIDRs.
func getClientIP(r *http.Request, trustedCIDRs []*net.IPNet) string {
	remoteIP := extractIP(r.RemoteAddr)

	if len(trustedCIDRs) == 0 {
		return remoteIP
	}

	parsedRemote := net.ParseIP(remoteIP)
	if parsedRemote == nil {
		return remoteIP
	}

	trusted := false
	for _, cidr := range trustedCIDRs {
		if cidr.Contains(parsedRemote) {
			trusted = true
			break
		}
	}

	if !trusted {
		return remoteIP
	}

	// Only trust forwarded headers from known proxy CIDRs
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	return remoteIP
}

// extractIP strips the port from an address string.
func extractIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// usageRecorderKey is the context key for the usage recorder function.
type usageRecorderKey struct{}

// UsageRecorder is a function that records token usage for quota tracking.
type UsageRecorder func(inputTokens, outputTokens int)

// WithUsageRecorder adds a usage recorder to the context.
func WithUsageRecorder(ctx context.Context, recorder UsageRecorder) context.Context {
	return context.WithValue(ctx, usageRecorderKey{}, recorder)
}

// RecordUsageFromContext calls the usage recorder stored in the context, if any.
func RecordUsageFromContext(ctx context.Context, inputTokens, outputTokens int) {
	if recorder, ok := ctx.Value(usageRecorderKey{}).(UsageRecorder); ok && recorder != nil {
		recorder(inputTokens, outputTokens)
	}
}
