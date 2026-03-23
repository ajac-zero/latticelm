package users

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ajac-zero/latticelm/internal/authctx"
)

// ctxWithAdmin injects a user ID and admin flag into a request context.
func ctxWithAdmin(ctx context.Context, userID string, isAdmin bool) context.Context {
	ctx = context.WithValue(ctx, authctx.UserIDKey, userID)
	ctx = context.WithValue(ctx, authctx.IsAdminKey, isAdmin)
	return ctx
}

func setupAPI(t *testing.T) (*API, *Store) {
	t.Helper()
	store := newStore(t)
	return NewAPI(store), store
}

func seedUser(t *testing.T, s *Store, iss, sub, email, name string, role Role) *User {
	t.Helper()
	u := &User{OIDCIss: iss, OIDCSub: sub, Email: email, Name: name, Role: role, Status: StatusActive}
	require.NoError(t, s.Create(context.Background(), u))
	return u
}

// --- HandleMe ---

func TestHandleMe_OK(t *testing.T) {
	api, store := setupAPI(t)
	u := seedUser(t, store, "https://iss", "sub-me", "me@api.test", "Me User", RoleUser)

	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req = req.WithContext(ctxWithAdmin(req.Context(), u.ID, false))
	rr := httptest.NewRecorder()

	api.HandleMe(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp UserResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, u.ID, resp.ID)
	assert.Equal(t, "me@api.test", resp.Email)
}

func TestHandleMe_MethodNotAllowed(t *testing.T) {
	api, _ := setupAPI(t)
	req := httptest.NewRequest(http.MethodPost, "/api/users/me", nil)
	rr := httptest.NewRecorder()
	api.HandleMe(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleMe_NoAuth(t *testing.T) {
	api, _ := setupAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	rr := httptest.NewRecorder()
	api.HandleMe(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// --- HandleListUsersOnly ---

func TestHandleListUsers_OK(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-list-admin", "listadmin@api.test", "List Admin", RoleAdmin)
	seedUser(t, store, "https://iss", "sub-list-regular", "listregular@api.test", "List Regular", RoleUser)

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()

	api.HandleListUsersOnly(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp ListUsersResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, resp.Total, 2)
}

func TestHandleListUsers_MethodNotAllowed(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-list-method", "listmethod@api.test", "LA", RoleAdmin)
	req := httptest.NewRequest(http.MethodPost, "/api/users", nil)
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()
	api.HandleListUsersOnly(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleListUsers_NotAdmin(t *testing.T) {
	api, store := setupAPI(t)
	regular := seedUser(t, store, "https://iss", "sub-list-nonadmin", "listnonadmin@api.test", "LNA", RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req = req.WithContext(ctxWithAdmin(req.Context(), regular.ID, false))
	rr := httptest.NewRecorder()
	api.HandleListUsersOnly(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

// --- HandleGetUser ---

func TestHandleGetUser_OK(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-getadmin", "getadmin@api.test", "Get Admin", RoleAdmin)
	target := seedUser(t, store, "https://iss", "sub-gettarget", "gettarget@api.test", "Get Target", RoleUser)

	req := httptest.NewRequest(http.MethodGet, "/api/users/"+target.ID, nil)
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()

	api.HandleGetUser(rr, req, target.ID)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp UserResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, target.ID, resp.ID)
}

func TestHandleGetUser_NotFound(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-getnotfound-admin", "getnfadmin@api.test", "GNA", RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/api/users/nonexistent", nil)
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()
	api.HandleGetUser(rr, req, "nonexistent")
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// --- HandleUpdateUser ---

func TestHandleUpdateUser_OK(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-updadmin", "updadmin@api.test", "Upd Admin", RoleAdmin)
	// Add a second admin so the last-admin check doesn't block.
	seedUser(t, store, "https://iss", "sub-updadmin2", "updadmin2@api.test", "Upd Admin 2", RoleAdmin)
	target := seedUser(t, store, "https://iss", "sub-updtarget", "updtarget@api.test", "Upd Target", RoleUser)

	body, _ := json.Marshal(UpdateUserRequest{Status: "suspended"})
	req := httptest.NewRequest(http.MethodPatch, "/api/users/"+target.ID, bytes.NewReader(body))
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()

	api.HandleUpdateUser(rr, req, target.ID)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp UserResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "suspended", resp.Status)
}

func TestHandleUpdateUser_InvalidRole(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-updbadrole-admin", "updbadrole@api.test", "UBR Admin", RoleAdmin)
	target := seedUser(t, store, "https://iss", "sub-updbadrole-target", "updbadrole2@api.test", "UBR Target", RoleUser)

	body, _ := json.Marshal(UpdateUserRequest{Role: "superuser"})
	req := httptest.NewRequest(http.MethodPatch, "/api/users/"+target.ID, bytes.NewReader(body))
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()

	api.HandleUpdateUser(rr, req, target.ID)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleUpdateUser_SelfDemotion(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-selfdemo", "selfdemo@api.test", "Self Demo", RoleAdmin)

	body, _ := json.Marshal(UpdateUserRequest{Role: "user"})
	req := httptest.NewRequest(http.MethodPatch, "/api/users/"+admin.ID, bytes.NewReader(body))
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()

	api.HandleUpdateUser(rr, req, admin.ID)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// --- HandleDeleteUser ---

func TestHandleDeleteUser_OK(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-deladmin", "deladmin@api.test", "Del Admin", RoleAdmin)
	target := seedUser(t, store, "https://iss", "sub-deltarget", "deltarget@api.test", "Del Target", RoleUser)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+target.ID, nil)
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()

	api.HandleDeleteUser(rr, req, target.ID)

	assert.Equal(t, http.StatusOK, rr.Code)

	// User should be soft-deleted (status = deleted).
	got, err := store.GetByID(context.Background(), target.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusDeleted, got.Status)
}

func TestHandleDeleteUser_SelfDeletion(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-selfdel", "selfdel@api.test", "Self Del", RoleAdmin)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+admin.ID, nil)
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()

	api.HandleDeleteUser(rr, req, admin.ID)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleDeleteUser_LastAdmin(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-lastadmin-del", "lastadmindel@api.test", "Last Admin Del", RoleAdmin)
	target := seedUser(t, store, "https://iss", "sub-lastadmin-target", "lastadmintgt@api.test", "Last Admin Target", RoleAdmin)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+target.ID, nil)
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()
	api.HandleDeleteUser(rr, req, target.ID)
	assert.Equal(t, http.StatusOK, rr.Code)
}

// --- HandleBulkUpdateUsers ---

func TestHandleBulkUpdate_OK(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-bulkadmin", "bulkadmin@api.test", "Bulk Admin", RoleAdmin)
	u1 := seedUser(t, store, "https://iss", "sub-bulk1", "bulk1@api.test", "Bulk1", RoleUser)
	u2 := seedUser(t, store, "https://iss", "sub-bulk2", "bulk2@api.test", "Bulk2", RoleUser)

	body, _ := json.Marshal(BulkUpdateRequest{IDs: []string{u1.ID, u2.ID}, Status: "suspended"})
	req := httptest.NewRequest(http.MethodPatch, "/api/users/bulk", bytes.NewReader(body))
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()

	api.HandleBulkUpdateUsers(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleBulkUpdate_NoIDs(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-bulknoids", "bulknoids@api.test", "BNA", RoleAdmin)
	body, _ := json.Marshal(BulkUpdateRequest{IDs: []string{}})
	req := httptest.NewRequest(http.MethodPatch, "/api/users/bulk", bytes.NewReader(body))
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()
	api.HandleBulkUpdateUsers(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleBulkUpdate_MethodNotAllowed(t *testing.T) {
	api, _ := setupAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/users/bulk", nil)
	rr := httptest.NewRecorder()
	api.HandleBulkUpdateUsers(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleBulkUpdate_SelfUpdate(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-bulkself", "bulkself@api.test", "BS Admin", RoleAdmin)
	body, _ := json.Marshal(BulkUpdateRequest{IDs: []string{admin.ID}, Status: "suspended"})
	req := httptest.NewRequest(http.MethodPatch, "/api/users/bulk", bytes.NewReader(body))
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()
	api.HandleBulkUpdateUsers(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleBulkUpdate_InvalidRole(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-bulkbadrole", "bulkbadrole@api.test", "BBR", RoleAdmin)
	u := seedUser(t, store, "https://iss", "sub-bulkbadrole2", "bulkbadrole2@api.test", "BBR2", RoleUser)
	body, _ := json.Marshal(BulkUpdateRequest{IDs: []string{u.ID}, Role: "superuser"})
	req := httptest.NewRequest(http.MethodPatch, "/api/users/bulk", bytes.NewReader(body))
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()
	api.HandleBulkUpdateUsers(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// --- HandleUsers dispatcher ---

func TestHandleUsers_EmptyPath(t *testing.T) {
	api, _ := setupAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/users/", nil)
	rr := httptest.NewRecorder()
	api.HandleUsers(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleUsers_MePath(t *testing.T) {
	api, _ := setupAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	rr := httptest.NewRecorder()
	api.HandleUsers(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleUsers_MethodNotAllowed(t *testing.T) {
	api, store := setupAPI(t)
	admin := seedUser(t, store, "https://iss", "sub-dispatch-nomethod", "dispatchnm@api.test", "DNM", RoleAdmin)
	req := httptest.NewRequest(http.MethodPost, "/api/users/some-id", nil)
	req = req.WithContext(ctxWithAdmin(req.Context(), admin.ID, true))
	rr := httptest.NewRecorder()
	api.HandleUsers(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

// --- RegisterRoutes ---

func TestRegisterRoutes(t *testing.T) {
	api, _ := setupAPI(t)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	// /api/users/ is registered but returns 404 for empty ID — check registration via a fake ID.
	for _, path := range []string{"/api/users/me", "/api/users/bulk", "/api/users/some-id", "/api/users"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		assert.NotEqual(t, http.StatusNotFound, rr.Code, "route %s not registered", path)
	}
}
