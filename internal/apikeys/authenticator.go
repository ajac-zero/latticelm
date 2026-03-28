package apikeys

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/ajac-zero/latticelm/internal/auth"
)

// Authenticator validates API keys presented as Bearer tokens and returns
// a Principal built from the key owner's identity.
type Authenticator struct {
	Store  *Store
	Logger *slog.Logger
}

func (a *Authenticator) Authenticate(r *http.Request) (*auth.Principal, error) {
	token := bearerToken(r)
	if token == "" || !strings.HasPrefix(token, "sk-") {
		return nil, nil // not an API key credential
	}

	hash := HashKey(token)
	ko, err := a.Store.Authenticate(r.Context(), hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid API key")
		}
		return nil, fmt.Errorf("API key lookup failed: %w", err)
	}

	if ko.Key.Status != StatusActive {
		return nil, fmt.Errorf("API key revoked")
	}
	if ko.Key.IsExpired() {
		return nil, fmt.Errorf("API key expired")
	}
	if ko.UserStatus != "active" {
		return nil, fmt.Errorf("user account inactive")
	}

	// Async touch last_used_at without blocking the request.
	// Use context.WithoutCancel to detach from request lifecycle while preserving values.
	go a.Store.TouchLastUsed(context.WithoutCancel(r.Context()), ko.Key.ID)

	return &auth.Principal{
		Issuer:  ko.OIDCIss,
		Subject: ko.OIDCSub,
		Roles:   []string{ko.UserRole},
	}, nil
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return parts[1]
}
