package auth

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrincipalFromClaims(t *testing.T) {
	tests := []struct {
		name     string
		claims   jwt.MapClaims
		expected *Principal
	}{
		{
			name: "basic claims",
			claims: jwt.MapClaims{
				"iss": "https://auth.example.com",
				"sub": "user-123",
			},
			expected: &Principal{
				Issuer:  "https://auth.example.com",
				Subject: "user-123",
			},
		},
		{
			name: "with org_id tenant",
			claims: jwt.MapClaims{
				"iss":    "https://auth.example.com",
				"sub":    "user-123",
				"org_id": "tenant-abc",
			},
			expected: &Principal{
				Issuer:   "https://auth.example.com",
				Subject:  "user-123",
				TenantID: "tenant-abc",
			},
		},
		{
			name: "with tenant_id",
			claims: jwt.MapClaims{
				"iss":       "https://auth.example.com",
				"sub":       "user-123",
				"tenant_id": "tenant-xyz",
			},
			expected: &Principal{
				Issuer:   "https://auth.example.com",
				Subject:  "user-123",
				TenantID: "tenant-xyz",
			},
		},
		{
			name: "with tid (Azure-style)",
			claims: jwt.MapClaims{
				"iss": "https://auth.example.com",
				"sub": "user-123",
				"tid": "azure-tenant",
			},
			expected: &Principal{
				Issuer:   "https://auth.example.com",
				Subject:  "user-123",
				TenantID: "azure-tenant",
			},
		},
		{
			name: "roles as string",
			claims: jwt.MapClaims{
				"iss":  "https://auth.example.com",
				"sub":  "user-123",
				"role": "admin",
			},
			expected: &Principal{
				Issuer:  "https://auth.example.com",
				Subject: "user-123",
				Roles:   []string{"admin"},
			},
		},
		{
			name: "roles as interface slice",
			claims: jwt.MapClaims{
				"iss":   "https://auth.example.com",
				"sub":   "user-123",
				"roles": []interface{}{"admin", "user"},
			},
			expected: &Principal{
				Issuer:  "https://auth.example.com",
				Subject: "user-123",
				Roles:   []string{"admin", "user"},
			},
		},
		{
			name: "groups claim",
			claims: jwt.MapClaims{
				"iss":    "https://auth.example.com",
				"sub":    "user-123",
				"groups": []interface{}{"platform-admin"},
			},
			expected: &Principal{
				Issuer:  "https://auth.example.com",
				Subject: "user-123",
				Roles:   []string{"platform-admin"},
			},
		},
		{
			name:   "empty claims",
			claims: jwt.MapClaims{},
			expected: &Principal{
				Issuer:  "",
				Subject: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PrincipalFromClaims(tt.claims)
			require.NotNil(t, p)
			assert.Equal(t, tt.expected.Issuer, p.Issuer)
			assert.Equal(t, tt.expected.Subject, p.Subject)
			assert.Equal(t, tt.expected.TenantID, p.TenantID)
			if tt.expected.Roles != nil {
				assert.Equal(t, tt.expected.Roles, p.Roles)
			}
		})
	}
}

func TestPrincipalContext(t *testing.T) {
	p := &Principal{Issuer: "iss", Subject: "sub"}
	ctx := ContextWithPrincipal(context.Background(), p)

	got := PrincipalFromContext(ctx)
	require.NotNil(t, got)
	assert.Equal(t, "iss", got.Issuer)
	assert.Equal(t, "sub", got.Subject)

	// Missing principal returns nil
	assert.Nil(t, PrincipalFromContext(context.Background()))
}

func TestPrincipal_OwnsConversation(t *testing.T) {
	tests := []struct {
		name      string
		principal *Principal
		ownerIss  string
		ownerSub  string
		tenantID  string
		expected  bool
	}{
		{
			name:      "same user same issuer",
			principal: &Principal{Issuer: "iss", Subject: "user-1"},
			ownerIss:  "iss",
			ownerSub:  "user-1",
			expected:  true,
		},
		{
			name:      "different subject",
			principal: &Principal{Issuer: "iss", Subject: "user-2"},
			ownerIss:  "iss",
			ownerSub:  "user-1",
			expected:  false,
		},
		{
			name:      "different issuer",
			principal: &Principal{Issuer: "iss-other", Subject: "user-1"},
			ownerIss:  "iss",
			ownerSub:  "user-1",
			expected:  false,
		},
		{
			name:      "same user same tenant",
			principal: &Principal{Issuer: "iss", Subject: "user-1", TenantID: "t1"},
			ownerIss:  "iss",
			ownerSub:  "user-1",
			tenantID:  "t1",
			expected:  true,
		},
		{
			name:      "same user different tenant",
			principal: &Principal{Issuer: "iss", Subject: "user-1", TenantID: "t2"},
			ownerIss:  "iss",
			ownerSub:  "user-1",
			tenantID:  "t1",
			expected:  false,
		},
		{
			name:      "owner has no tenant - caller has tenant",
			principal: &Principal{Issuer: "iss", Subject: "user-1", TenantID: "t1"},
			ownerIss:  "iss",
			ownerSub:  "user-1",
			tenantID:  "",
			expected:  true,
		},
		{
			name:      "nil principal",
			principal: nil,
			ownerIss:  "iss",
			ownerSub:  "user-1",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.principal.OwnsConversation(tt.ownerIss, tt.ownerSub, tt.tenantID))
		})
	}
}

func TestPrincipal_HasAdminRole(t *testing.T) {
	tests := []struct {
		name      string
		principal *Principal
		cfg       AdminConfig
		expected  bool
	}{
		{
			name:      "admin disabled",
			principal: &Principal{Roles: []string{"admin"}},
			cfg:       AdminConfig{Enabled: false},
			expected:  false,
		},
		{
			name:      "nil principal",
			principal: nil,
			cfg:       AdminConfig{Enabled: true},
			expected:  false,
		},
		{
			name:      "default admin role",
			principal: &Principal{Roles: []string{"admin"}},
			cfg:       AdminConfig{Enabled: true},
			expected:  true,
		},
		{
			name:      "no admin role",
			principal: &Principal{Roles: []string{"user"}},
			cfg:       AdminConfig{Enabled: true},
			expected:  false,
		},
		{
			name:      "custom allowed values",
			principal: &Principal{Roles: []string{"platform-admin"}},
			cfg:       AdminConfig{Enabled: true, AllowedValues: []string{"platform-admin"}},
			expected:  true,
		},
		{
			name:      "custom allowed values - no match",
			principal: &Principal{Roles: []string{"user"}},
			cfg:       AdminConfig{Enabled: true, AllowedValues: []string{"platform-admin"}},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.principal.HasAdminRole(tt.cfg))
		})
	}
}
