package users

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ajac-zero/latticelm/internal/authctx"
)

// API provides HTTP handlers for user management.
type API struct {
	store *Store
}

// NewAPI creates a new user API handler.
func NewAPI(store *Store) *API {
	return &API{store: store}
}

// RegisterRoutes registers user API routes.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/users/me", a.HandleMe)
	mux.HandleFunc("/api/users/", a.HandleUsers)
	mux.HandleFunc("/api/users", a.HandleListUsersOnly)
}

// UserResponse is the API response for user data.
type UserResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
	OIDCIss   string `json:"oidc_iss,omitempty"`
	OIDCSub   string `json:"oidc_sub,omitempty"`
}

// ListUsersResponse is the API response for listing users.
type ListUsersResponse struct {
	Users []*UserResponse `json:"users"`
	Total int             `json:"total"`
	Page  int             `json:"page"`
	Limit int             `json:"limit"`
}

// UpdateUserRequest is the request body for updating a user.
type UpdateUserRequest struct {
	Role   string `json:"role,omitempty"`
	Status string `json:"status,omitempty"`
}

// HandleMe returns the current authenticated user's information.
// This endpoint requires authentication and returns the user from the database.
func (a *API) HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user ID from request context
	// This is set by the SessionMiddleware or auth middleware
	userID := r.Context().Value(authctx.UserIDKey)
	if userID == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"error": "Not authenticated",
		})
		return
	}

	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"error": "Invalid user ID",
		})
		return
	}

	// Fetch user from database
	user, err := a.store.GetByID(r.Context(), userIDStr)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to fetch user",
		})
		return
	}

	// Return user data
	writeJSON(w, http.StatusOK, UserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		Role:      string(user.Role),
		Status:    string(user.Status),
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// HandleListUsersOnly handles GET /api/users (list without trailing slash).
func (a *API) HandleListUsersOnly(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.HandleListUsers(w, r)
}

// HandleUsers handles user management operations for specific users (get, update, delete).
// This handles /api/users/:id paths. All operations require admin privileges.
func (a *API) HandleUsers(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from path: /api/users/UUID
	path := strings.TrimPrefix(r.URL.Path, "/api/users/")
	userID := strings.TrimSpace(path)

	if userID == "" || userID == "me" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Single user operations
	switch r.Method {
	case http.MethodGet:
		a.HandleGetUser(w, r, userID)
	case http.MethodPatch:
		a.HandleUpdateUser(w, r, userID)
	case http.MethodDelete:
		a.HandleDeleteUser(w, r, userID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleListUsers lists all users with pagination and filtering (admin only).
func (a *API) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	roleStr := r.URL.Query().Get("role")
	statusStr := r.URL.Query().Get("status")
	search := r.URL.Query().Get("search")

	opts := ListOptions{
		Page:   page,
		Limit:  limit,
		Search: search,
	}

	if roleStr != "" {
		opts.Role = Role(roleStr)
	}
	if statusStr != "" {
		opts.Status = Status(statusStr)
	}

	result, err := a.store.ListWithOptions(r.Context(), opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to list users",
		})
		return
	}

	// Convert to response format
	users := make([]*UserResponse, len(result.Users))
	for i, u := range result.Users {
		users[i] = &UserResponse{
			ID:        u.ID,
			Email:     u.Email,
			Name:      u.Name,
			Role:      string(u.Role),
			Status:    string(u.Status),
			CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: u.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	writeJSON(w, http.StatusOK, ListUsersResponse{
		Users: users,
		Total: result.Total,
		Page:  opts.Page,
		Limit: opts.Limit,
	})
}

// HandleGetUser retrieves a single user by ID (admin only).
func (a *API) HandleGetUser(w http.ResponseWriter, r *http.Request, userID string) {
	if !a.requireAdmin(w, r) {
		return
	}

	user, err := a.store.GetByID(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": "User not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, UserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		Role:      string(user.Role),
		Status:    string(user.Status),
		OIDCIss:   user.OIDCIss,
		OIDCSub:   user.OIDCSub,
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// HandleUpdateUser updates a user's role or status (admin only).
func (a *API) HandleUpdateUser(w http.ResponseWriter, r *http.Request, userID string) {
	if !a.requireAdmin(w, r) {
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid request body",
		})
		return
	}

	// Get current user
	user, err := a.store.GetByID(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": "User not found",
		})
		return
	}

	// Validate and apply changes
	if req.Role != "" {
		newRole := Role(req.Role)
		if newRole != RoleAdmin && newRole != RoleUser {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error": "Invalid role. Must be 'admin' or 'user'",
			})
			return
		}

		// Prevent self-demotion
		currentUserID := a.getUserID(r)
		if currentUserID == userID && newRole != RoleAdmin {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error": "Cannot change your own role",
			})
			return
		}

		// Prevent demoting last admin
		if user.Role == RoleAdmin && newRole != RoleAdmin {
			if err := a.checkLastAdmin(r.Context(), userID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{
					"error": err.Error(),
				})
				return
			}
		}

		user.Role = newRole
	}

	if req.Status != "" {
		newStatus := Status(req.Status)
		if newStatus != StatusActive && newStatus != StatusSuspended && newStatus != StatusDeleted {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error": "Invalid status. Must be 'active', 'suspended', or 'deleted'",
			})
			return
		}
		user.Status = newStatus
	}

	// Update user
	if err := a.store.Update(r.Context(), user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to update user",
		})
		return
	}

	writeJSON(w, http.StatusOK, UserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		Role:      string(user.Role),
		Status:    string(user.Status),
		UpdatedAt: user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// HandleDeleteUser soft-deletes a user (admin only).
func (a *API) HandleDeleteUser(w http.ResponseWriter, r *http.Request, userID string) {
	if !a.requireAdmin(w, r) {
		return
	}

	// Prevent self-deletion
	currentUserID := a.getUserID(r)
	if currentUserID == userID {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "Cannot delete your own account",
		})
		return
	}

	user, err := a.store.GetByID(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": "User not found",
		})
		return
	}

	// Prevent deleting last admin
	if user.Role == RoleAdmin {
		if err := a.checkLastAdmin(r.Context(), userID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
	}

	// Soft delete by setting status to deleted
	user.Status = StatusDeleted
	if err := a.store.Update(r.Context(), user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to delete user",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "User deleted successfully",
		"id":      userID,
	})
}

// requireAdmin checks if the current user is an admin.
func (a *API) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	// Check if user is admin via session
	// The session middleware should have set is_admin in the context
	isAdmin := r.Context().Value(authctx.IsAdminKey)
	if isAdmin != nil {
		if admin, ok := isAdmin.(bool); ok && admin {
			return true
		}
	}

	// Fallback: check user role from database
	userID := a.getUserID(r)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"error": "Not authenticated",
		})
		return false
	}

	user, err := a.store.GetByID(r.Context(), userID)
	if err != nil || !user.IsAdmin() {
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"error": "Admin access required",
		})
		return false
	}

	return true
}

// getUserID extracts the user ID from the request context.
func (a *API) getUserID(r *http.Request) string {
	userID := r.Context().Value(authctx.UserIDKey)
	if userID == nil {
		return ""
	}
	if id, ok := userID.(string); ok {
		return id
	}
	return ""
}

// checkLastAdmin ensures we're not demoting/deleting the last admin.
func (a *API) checkLastAdmin(ctx context.Context, excludeUserID string) error {
	// Count active admins
	users, err := a.store.List(ctx, StatusActive)
	if err != nil {
		return fmt.Errorf("failed to check admin count")
	}

	adminCount := 0
	for _, u := range users {
		if u.Role == RoleAdmin && u.ID != excludeUserID {
			adminCount++
		}
	}

	if adminCount == 0 {
		return fmt.Errorf("cannot demote or delete the last admin")
	}

	return nil
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
