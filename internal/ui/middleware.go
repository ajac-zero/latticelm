package ui

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// SecurityHeadersMiddleware adds browser security headers to every admin response.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; font-src 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// IPAllowlistMiddleware rejects requests whose source IP is not in the allowlist.
// If allowlist is empty, all source IPs are permitted.
func IPAllowlistMiddleware(allowlist []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(allowlist) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			parsed := net.ParseIP(ip)
			if parsed == nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			for _, cidr := range allowlist {
				if cidr.Contains(parsed) {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, "forbidden", http.StatusForbidden)
		})
	}
}

// ParseCIDRs parses a list of CIDR strings into IPNet values.
func ParseCIDRs(cidrs []string) ([]*net.IPNet, error) {
	result := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}
		result = append(result, network)
	}
	return result, nil
}

// clientIP extracts the real client IP from the request using RemoteAddr only.
// Proxy forwarding headers are intentionally ignored for the admin allowlist to
// prevent header spoofing by untrusted intermediaries.
func clientIP(r *http.Request) string {
	// Strip port from RemoteAddr (format: "host:port" or "[::1]:port")
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		return strings.TrimSpace(h)
	}
	return strings.TrimSpace(host)
}
