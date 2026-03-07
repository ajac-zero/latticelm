package admin

import (
	"encoding/json"
	"net/http"
)

// APIResponse is the standard JSON response wrapper.
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

// APIError represents an error response.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

// writeSuccess writes a successful JSON response.
func writeSuccess(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    data,
	})
}

// writeError writes an error JSON response.
func writeError(w http.ResponseWriter, statusCode int, code, message string) {
	writeJSON(w, statusCode, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
		},
	})
}
