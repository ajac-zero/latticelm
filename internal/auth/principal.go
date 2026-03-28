package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
)

// Principal represents the authenticated identity extracted from JWT claims.
type Principal struct {
	Issuer   string
	Subject  string
	TenantID string
	Roles    []string
}

type principalKey struct{}

// ContextWithPrincipal stores a Principal in the context.
func ContextWithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFromContext retrieves the Principal from the context.
// Returns nil when authentication is disabled or no token was presented.
func PrincipalFromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}

// PrincipalFromClaims builds a Principal from validated JWT claims.
//
// Optional extraRoleClaims specify additional claim names to inspect for roles.
// This is useful when the admin configuration uses a non-standard claim name
// (e.g., "permissions") so those values are captured in the Principal's Roles
// and downstream checks like HasAdminRole work uniformly.
func PrincipalFromClaims(claims jwt.MapClaims, extraRoleClaims ...string) *Principal {
	p := &Principal{}

	if iss, ok := claims["iss"].(string); ok {
		p.Issuer = iss
	}
	if sub, ok := claims["sub"].(string); ok {
		p.Subject = sub
	}

	// Tenant ID from common OIDC claims (org_id, tenant_id, tid).
	for _, key := range []string{"org_id", "tenant_id", "tid"} {
		if val, ok := claims[key].(string); ok && val != "" {
			p.TenantID = val
			break
		}
	}

	// Extract roles from common claims.
	for _, key := range []string{"role", "roles", "groups"} {
		if raw, exists := claims[key]; exists {
			p.Roles = extractStringSlice(raw)
			if len(p.Roles) > 0 {
				break
			}
		}
	}

	// Also extract from caller-specified claim names so custom admin claims
	// (e.g., "permissions") propagate into the Principal.
	for _, key := range extraRoleClaims {
		if raw, exists := claims[key]; exists {
			extra := extractStringSlice(raw)
			p.Roles = append(p.Roles, extra...)
		}
	}

	return p
}

// HasAdminRole returns true when the principal holds an admin role according
// to the given configuration.
func (p *Principal) HasAdminRole(cfg AdminConfig) bool {
	if p == nil || !cfg.Enabled {
		return false
	}

	allowedValues := cfg.AllowedValues
	if len(allowedValues) == 0 {
		allowedValues = []string{"admin"}
	}

	allowed := make(map[string]struct{}, len(allowedValues))
	for _, v := range allowedValues {
		allowed[v] = struct{}{}
	}

	for _, r := range p.Roles {
		if _, ok := allowed[r]; ok {
			return true
		}
	}
	return false
}

// OwnsConversation checks whether the principal owns a conversation given
// the stored owner fields. When tenantID is non-empty on either side, both
// issuer+subject AND tenant must match.
func (p *Principal) OwnsConversation(ownerIss, ownerSub, tenantID string) bool {
	if p == nil {
		return false
	}

	if p.Issuer != ownerIss || p.Subject != ownerSub {
		return false
	}

	// If the conversation is scoped to a tenant, ensure the caller is in the same tenant.
	if tenantID != "" && p.TenantID != tenantID {
		return false
	}

	return true
}

func extractStringSlice(raw interface{}) []string {
	switch v := raw.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
