package usage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ajac-zero/latticelm/internal/auth"
)

// mockBackend implements Backend for handler tests.
type mockBackend struct {
	summaryRows []SummaryRow
	topRows     []TopRow
	trendRows   []TrendRow
	summaryErr  error
	topErr      error
	trendsErr   error
	recorded    []UsageEvent
}

func (m *mockBackend) Record(evt UsageEvent) { m.recorded = append(m.recorded, evt) }
func (m *mockBackend) Close() error          { return nil }
func (m *mockBackend) QuerySummary(_ context.Context, _ QueryFilter) ([]SummaryRow, error) {
	return m.summaryRows, m.summaryErr
}
func (m *mockBackend) QueryTop(_ context.Context, _ QueryFilter, _ string, _ int) ([]TopRow, error) {
	return m.topRows, m.topErr
}
func (m *mockBackend) QueryTrends(_ context.Context, _ QueryFilter, _ string, _ string) ([]TrendRow, error) {
	return m.trendRows, m.trendsErr
}

// mockUserResolver implements UserResolver.
type mockUserResolver struct {
	names map[string]string
}

func (m *mockUserResolver) ResolveUserSub(_ context.Context, sub string) (string, string, error) {
	if name, ok := m.names[sub]; ok {
		return name, "", nil
	}
	return "", "", nil
}

func newTestAPI(b *mockBackend) *API {
	return NewAPI(b)
}

func TestHandleSummary_OK(t *testing.T) {
	backend := &mockBackend{
		summaryRows: []SummaryRow{
			{Provider: "openai", Model: "gpt-4", TotalTokens: 100, RequestCount: 1},
		},
	}
	api := newTestAPI(backend)

	req := httptest.NewRequest(http.MethodGet, "/v1/usage/summary", nil)
	rr := httptest.NewRecorder()

	api.handleSummary(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp SummaryResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, int64(100), resp.Data[0].TotalTokens)
}

func TestHandleSummary_MethodNotAllowed(t *testing.T) {
	api := newTestAPI(&mockBackend{})
	req := httptest.NewRequest(http.MethodPost, "/v1/usage/summary", nil)
	rr := httptest.NewRecorder()
	api.handleSummary(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleSummary_StoreError(t *testing.T) {
	backend := &mockBackend{summaryErr: assert.AnError}
	api := newTestAPI(backend)
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/summary", nil)
	rr := httptest.NewRecorder()
	api.handleSummary(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleSummary_EmptyRows(t *testing.T) {
	api := newTestAPI(&mockBackend{summaryRows: nil})
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/summary", nil)
	rr := httptest.NewRecorder()
	api.handleSummary(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp SummaryResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Data)
}

func TestHandleSummary_TimeRange(t *testing.T) {
	api := newTestAPI(&mockBackend{})
	start := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
	end := time.Now().Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/summary?start="+start+"&end="+end, nil)
	rr := httptest.NewRecorder()
	api.handleSummary(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp SummaryResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, start, resp.Start)
}

func TestHandleTop_OK(t *testing.T) {
	backend := &mockBackend{
		topRows: []TopRow{
			{Key: "gpt-4", TotalTokens: 500, RequestCount: 5},
		},
	}
	api := newTestAPI(backend)
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/top?dimension=model&limit=5", nil)
	rr := httptest.NewRecorder()
	api.handleTop(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp TopResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "model", resp.Dimension)
	assert.Len(t, resp.Data, 1)
}

func TestHandleTop_MethodNotAllowed(t *testing.T) {
	api := newTestAPI(&mockBackend{})
	req := httptest.NewRequest(http.MethodDelete, "/v1/usage/top", nil)
	rr := httptest.NewRecorder()
	api.handleTop(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleTop_DefaultDimensionAndLimit(t *testing.T) {
	backend := &mockBackend{topRows: []TopRow{{Key: "openai"}}}
	api := newTestAPI(backend)
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/top", nil)
	rr := httptest.NewRecorder()
	api.handleTop(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp TopResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "model", resp.Dimension)
}

func TestHandleTop_InvalidDimension(t *testing.T) {
	backend := &mockBackend{topErr: assert.AnError}
	api := newTestAPI(backend)
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/top?dimension=invalid", nil)
	rr := httptest.NewRecorder()
	api.handleTop(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleTop_UserSubResolution(t *testing.T) {
	backend := &mockBackend{
		topRows: []TopRow{
			{Key: "sub-123", TotalTokens: 100},
		},
	}
	resolver := &mockUserResolver{names: map[string]string{"sub-123": "Alice"}}
	api := NewAPI(backend, WithUserResolver(resolver))

	req := httptest.NewRequest(http.MethodGet, "/v1/usage/top?dimension=user_sub", nil)
	rr := httptest.NewRecorder()
	api.handleTop(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp TopResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "Alice", resp.Data[0].Key)
}

func TestHandleTrends_OK(t *testing.T) {
	bucket := time.Now().Truncate(24 * time.Hour)
	backend := &mockBackend{
		trendRows: []TrendRow{
			{Bucket: bucket, TotalTokens: 300, RequestCount: 3},
		},
	}
	api := newTestAPI(backend)
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/trends?granularity=daily", nil)
	rr := httptest.NewRecorder()
	api.handleTrends(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp TrendsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "daily", resp.Granularity)
	assert.Len(t, resp.Data, 1)
}

func TestHandleTrends_MethodNotAllowed(t *testing.T) {
	api := newTestAPI(&mockBackend{})
	req := httptest.NewRequest(http.MethodPut, "/v1/usage/trends", nil)
	rr := httptest.NewRecorder()
	api.handleTrends(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleTrends_StoreError(t *testing.T) {
	backend := &mockBackend{trendsErr: assert.AnError}
	api := newTestAPI(backend)
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/trends", nil)
	rr := httptest.NewRecorder()
	api.handleTrends(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleTrends_HourlyGranularity(t *testing.T) {
	api := newTestAPI(&mockBackend{})
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/trends?granularity=hourly", nil)
	rr := httptest.NewRecorder()
	api.handleTrends(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp TrendsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "hourly", resp.Granularity)
}

func TestHandleTrends_UserSubResolution(t *testing.T) {
	bucket := time.Now().Truncate(24 * time.Hour)
	backend := &mockBackend{
		trendRows: []TrendRow{
			{Bucket: bucket, Key: "sub-456", TotalTokens: 50},
		},
	}
	resolver := &mockUserResolver{names: map[string]string{"sub-456": "Bob"}}
	api := NewAPI(backend, WithUserResolver(resolver))

	req := httptest.NewRequest(http.MethodGet, "/v1/usage/trends?dimension=user_sub", nil)
	rr := httptest.NewRecorder()
	api.handleTrends(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp TrendsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "Bob", resp.Data[0].Key)
}

func TestParseFilter_AuthScoping(t *testing.T) {
	principal := &auth.Principal{Subject: "user-abc", TenantID: "tenant-xyz"}
	ctx := auth.ContextWithPrincipal(context.Background(), principal)
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/summary", nil)
	req = req.WithContext(ctx)

	f := parseFilter(req)
	assert.Equal(t, "user-abc", f.UserSub)
	assert.Equal(t, "tenant-xyz", f.TenantID)
}

func TestParseFilter_ExplicitParamsNotOverriddenByAuth(t *testing.T) {
	principal := &auth.Principal{Subject: "user-abc", TenantID: "tenant-xyz"}
	ctx := auth.ContextWithPrincipal(context.Background(), principal)
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/summary?tenant_id=other-tenant", nil)
	req = req.WithContext(ctx)

	f := parseFilter(req)
	// Explicit tenant_id is not overridden by principal.
	assert.Equal(t, "other-tenant", f.TenantID)
	// user_sub is still scoped to principal when not specified.
	assert.Equal(t, "user-abc", f.UserSub)
}

func TestParseFilter_NoAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/summary?model=gpt-4", nil)
	f := parseFilter(req)
	assert.Equal(t, "gpt-4", f.Model)
	assert.Empty(t, f.UserSub)
	assert.Empty(t, f.TenantID)
}

func TestRegisterRoutes(t *testing.T) {
	api := newTestAPI(&mockBackend{})
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	api.RegisterAdminRoutes(mux)

	for _, path := range []string{
		"/v1/usage/summary",
		"/v1/usage/top",
		"/v1/usage/trends",
		"/api/v1/usage/summary",
		"/api/v1/usage/top",
		"/api/v1/usage/trends",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		assert.NotEqual(t, http.StatusNotFound, rr.Code, "route %s not registered", path)
	}
}

func TestResolveUserSubs_NoResolver(t *testing.T) {
	api := newTestAPI(&mockBackend{})
	result := api.resolveUserSubs(context.Background(), []string{"sub-1", "sub-2"})
	assert.Empty(t, result)
}

func TestResolveUserSubs_WithResolver(t *testing.T) {
	resolver := &mockUserResolver{names: map[string]string{"sub-1": "Alice"}}
	api := NewAPI(&mockBackend{}, WithUserResolver(resolver))
	result := api.resolveUserSubs(context.Background(), []string{"sub-1", "sub-2"})
	assert.Equal(t, "Alice", result["sub-1"])
	assert.NotContains(t, result, "sub-2")
}
