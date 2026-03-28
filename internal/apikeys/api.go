package apikeys

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/users"
)

// API exposes REST endpoints for API key management.
type API struct {
	store     *Store
	userStore *users.Store
	logger    *slog.Logger
	maxKeys   int // 0 = unlimited
}

// NewAPI creates API key management handlers.
func NewAPI(store *Store, userStore *users.Store, logger *slog.Logger, maxKeys int) *API {
	return &API{store: store, userStore: userStore, logger: logger, maxKeys: maxKeys}
}

// resolveAppUserID translates the authenticated Principal into the canonical
// application user ID (users.id). This is necessary because Principal.Subject
// is the OIDC sub for JWT-authenticated requests but the app UUID for
// session-authenticated requests.
func (a *API) resolveAppUserID(ctx context.Context, p *auth.Principal) (string, error) {
	// Session auth sets Subject to the app user ID.
	if u, err := a.userStore.GetByID(ctx, p.Subject); err == nil {
		return u.ID, nil
	}
	// JWT auth sets Subject to the OIDC sub.
	if u, err := a.userStore.GetByOIDC(ctx, p.Issuer, p.Subject); err == nil {
		return u.ID, nil
	}
	return "", fmt.Errorf("user not found for principal iss=%s sub=%s", p.Issuer, p.Subject)
}

// RegisterRoutes wires the API key endpoints onto the mux.
// These are mounted on the authenticated API mux (Bearer JWT / API key auth).
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/api-keys", a.handleAPIKeys)
	mux.HandleFunc("/v1/api-keys/", a.handleAPIKeyByID)
}

// RegisterAdminRoutes mirrors routes on the admin mux (session auth).
func (a *API) RegisterAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/api-keys", a.handleAPIKeys)
	mux.HandleFunc("/api/v1/api-keys/", a.handleAPIKeyByID)
}

type createKeyRequest struct {
	Name      string `json:"name"`
	ExpiresIn string `json:"expires_in"` // Go duration, e.g. "720h"
}

type keyResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Key        string     `json:"key,omitempty"` // only populated on creation
	KeyPrefix  string     `json:"key_prefix"`
	Status     string     `json:"status"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func (a *API) handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.listKeys(w, r)
	case http.MethodPost:
		a.createKey(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handleAPIKeyByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodDelete:
		a.deleteKey(w, r)
	case http.MethodPatch:
		a.revokeKey(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) createKey(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeErr(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := a.resolveAppUserID(r.Context(), principal)
	if err != nil {
		a.logger.Warn("cannot resolve user for API key creation", slog.String("error", err.Error()))
		writeErr(w, "user not found", http.StatusForbidden)
		return
	}

	var req createKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		writeErr(w, "name is required", http.StatusBadRequest)
		return
	}

	// Enforce per-user key limit.
	if a.maxKeys > 0 {
		existing, err := a.store.ListByUser(r.Context(), userID)
		if err != nil {
			a.logger.Error("failed to list API keys for limit check", slog.String("error", err.Error()))
			writeErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		active := 0
		for _, k := range existing {
			if k.Status == StatusActive {
				active++
			}
		}
		if active >= a.maxKeys {
			writeErr(w, fmt.Sprintf("maximum of %d active API keys reached", a.maxKeys), http.StatusConflict)
			return
		}
	}

	plaintext, hash, err := GenerateKey()
	if err != nil {
		a.logger.Error("failed to generate API key", slog.String("error", err.Error()))
		writeErr(w, "internal error", http.StatusInternalServerError)
		return
	}

	k := &APIKey{
		Name:      req.Name,
		KeyHash:   hash,
		KeyPrefix: KeyDisplayPrefix(plaintext),
		UserID:    userID,
	}

	if req.ExpiresIn != "" {
		d, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			writeErr(w, "invalid expires_in duration", http.StatusBadRequest)
			return
		}
		exp := time.Now().Add(d)
		k.ExpiresAt = &exp
	}

	if err := a.store.Create(r.Context(), k); err != nil {
		a.logger.Error("failed to create API key", slog.String("error", err.Error()))
		writeErr(w, "internal error", http.StatusInternalServerError)
		return
	}

	a.logger.Info("API key created",
		slog.String("key_id", k.ID),
		slog.String("user_id", k.UserID),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(keyResponse{
		ID:        k.ID,
		Name:      k.Name,
		Key:       plaintext, // only returned once
		KeyPrefix: k.KeyPrefix,
		Status:    string(k.Status),
		ExpiresAt: k.ExpiresAt,
		CreatedAt: k.CreatedAt,
	})
}

func (a *API) listKeys(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeErr(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := a.resolveAppUserID(r.Context(), principal)
	if err != nil {
		writeErr(w, "user not found", http.StatusForbidden)
		return
	}

	keys, err := a.store.ListByUser(r.Context(), userID)
	if err != nil {
		a.logger.Error("failed to list API keys", slog.String("error", err.Error()))
		writeErr(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := make([]keyResponse, 0, len(keys))
	for _, k := range keys {
		resp = append(resp, keyResponse{
			ID:         k.ID,
			Name:       k.Name,
			KeyPrefix:  k.KeyPrefix,
			Status:     string(k.Status),
			ExpiresAt:  k.ExpiresAt,
			CreatedAt:  k.CreatedAt,
			LastUsedAt: k.LastUsedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"keys": resp})
}

func (a *API) revokeKey(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeErr(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := a.resolveAppUserID(r.Context(), principal)
	if err != nil {
		writeErr(w, "user not found", http.StatusForbidden)
		return
	}

	id := extractKeyID(r.URL.Path)
	if id == "" {
		writeErr(w, "key id is required", http.StatusBadRequest)
		return
	}

	if err := a.store.Revoke(r.Context(), id, userID); err != nil {
		a.logger.Error("failed to revoke API key",
			slog.String("key_id", id),
			slog.String("error", err.Error()),
		)
		writeErr(w, "key not found", http.StatusNotFound)
		return
	}

	a.logger.Info("API key revoked",
		slog.String("key_id", id),
		slog.String("user_id", userID),
	)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})
}

func (a *API) deleteKey(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeErr(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := a.resolveAppUserID(r.Context(), principal)
	if err != nil {
		writeErr(w, "user not found", http.StatusForbidden)
		return
	}

	id := extractKeyID(r.URL.Path)
	if id == "" {
		writeErr(w, "key id is required", http.StatusBadRequest)
		return
	}

	if err := a.store.Delete(r.Context(), id, userID); err != nil {
		a.logger.Error("failed to delete API key",
			slog.String("key_id", id),
			slog.String("error", err.Error()),
		)
		writeErr(w, "key not found", http.StatusNotFound)
		return
	}

	a.logger.Info("API key deleted",
		slog.String("key_id", id),
		slog.String("user_id", userID),
	)
	w.WriteHeader(http.StatusNoContent)
}

func extractKeyID(path string) string {
	const suffix = "/api-keys/"
	idx := strings.LastIndex(path, suffix)
	if idx < 0 {
		return ""
	}
	return path[idx+len(suffix):]
}

func writeErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"message": msg},
	})
}
