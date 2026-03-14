// Package authctx provides shared context keys for authentication.
// This package exists to break import cycles between auth and users.
package authctx

// ContextKey is the type used for authentication-related context keys.
type ContextKey string

const (
	// UserIDKey is the context key for the authenticated user's ID.
	UserIDKey ContextKey = "user_id"
	// IsAdminKey is the context key for the authenticated user's admin flag.
	IsAdminKey ContextKey = "is_admin"
	// OwnerIssKey is the context key for the authenticated user's OIDC issuer.
	OwnerIssKey ContextKey = "owner_iss"
	// OwnerSubKey is the context key for the authenticated user's OIDC subject.
	OwnerSubKey ContextKey = "owner_sub"
	// TenantIDKey is the context key for the authenticated user's tenant ID.
	TenantIDKey ContextKey = "tenant_id"
)
