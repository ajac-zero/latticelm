package usage

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/ajac-zero/latticelm/internal/auth"
)

// API provides HTTP handlers for usage analytics.
type API struct {
	store Backend
}

// NewAPI creates a new usage API handler.
func NewAPI(store Backend) *API {
	return &API{store: store}
}

// RegisterRoutes registers usage API routes on the provided mux.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/usage/summary", a.handleSummary)
	mux.HandleFunc("/v1/usage/top", a.handleTop)
	mux.HandleFunc("/v1/usage/trends", a.handleTrends)
}

// parseFilter extracts common query filter parameters from the request.
// Non-admin callers are automatically scoped to their own identity.
func parseFilter(r *http.Request) QueryFilter {
	q := r.URL.Query()
	f := QueryFilter{
		TenantID: q.Get("tenant_id"),
		UserSub:  q.Get("user_sub"),
		Model:    q.Get("model"),
		Provider: q.Get("provider"),
	}

	if v := q.Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Start = t
		}
	}
	if v := q.Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.End = t
		}
	}

	// Scope non-admin callers to their own identity.
	principal := auth.PrincipalFromContext(r.Context())
	if principal != nil {
		if f.TenantID == "" && principal.TenantID != "" {
			f.TenantID = principal.TenantID
		}
		if f.UserSub == "" {
			f.UserSub = principal.Subject
		}
	}

	return f
}

// SummaryResponse is the JSON envelope for the summary endpoint.
type SummaryResponse struct {
	Data  []SummaryRow `json:"data"`
	Start string       `json:"start"`
	End   string       `json:"end"`
}

func (a *API) handleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	f := parseFilter(r)

	// Default to last 24 hours when no time range specified.
	if f.Start.IsZero() && f.End.IsZero() {
		f.End = time.Now().UTC()
		f.Start = f.End.Add(-24 * time.Hour)
	}

	rows, err := a.store.QuerySummary(r.Context(), f)
	if err != nil {
		writeUsageJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query summary"})
		return
	}
	if rows == nil {
		rows = []SummaryRow{}
	}

	writeUsageJSON(w, http.StatusOK, SummaryResponse{
		Data:  rows,
		Start: f.Start.Format(time.RFC3339),
		End:   f.End.Format(time.RFC3339),
	})
}

// TopResponse is the JSON envelope for the top-consumers endpoint.
type TopResponse struct {
	Dimension string   `json:"dimension"`
	Data      []TopRow `json:"data"`
	Start     string   `json:"start"`
	End       string   `json:"end"`
}

func (a *API) handleTop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	f := parseFilter(r)

	if f.Start.IsZero() && f.End.IsZero() {
		f.End = time.Now().UTC()
		f.Start = f.End.Add(-24 * time.Hour)
	}

	dimension := r.URL.Query().Get("dimension")
	if dimension == "" {
		dimension = "model"
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 10
	}

	rows, err := a.store.QueryTop(r.Context(), f, dimension, limit)
	if err != nil {
		writeUsageJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []TopRow{}
	}

	writeUsageJSON(w, http.StatusOK, TopResponse{
		Dimension: dimension,
		Data:      rows,
		Start:     f.Start.Format(time.RFC3339),
		End:       f.End.Format(time.RFC3339),
	})
}

// TrendsResponse is the JSON envelope for the trends endpoint.
type TrendsResponse struct {
	Granularity string     `json:"granularity"`
	Data        []TrendRow `json:"data"`
	Start       string     `json:"start"`
	End         string     `json:"end"`
}

func (a *API) handleTrends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	f := parseFilter(r)

	if f.Start.IsZero() && f.End.IsZero() {
		f.End = time.Now().UTC()
		f.Start = f.End.Add(-30 * 24 * time.Hour)
	}

	granularity := r.URL.Query().Get("granularity")
	if granularity != "hourly" {
		granularity = "daily"
	}

	rows, err := a.store.QueryTrends(r.Context(), f, granularity)
	if err != nil {
		writeUsageJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query trends"})
		return
	}
	if rows == nil {
		rows = []TrendRow{}
	}

	writeUsageJSON(w, http.StatusOK, TrendsResponse{
		Granularity: granularity,
		Data:        rows,
		Start:       f.Start.Format(time.RFC3339),
		End:         f.End.Format(time.RFC3339),
	})
}

func writeUsageJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
