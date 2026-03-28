package apikeys

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthenticator_SkipsNonAPIKey(t *testing.T) {
	a := &Authenticator{Logger: slog.Default()}

	tests := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"regular jwt", "Bearer eyJhbGciOiJSUzI1NiJ9.test.sig"},
		{"basic auth", "Basic dXNlcjpwYXNz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			principal, err := a.Authenticate(req)
			assert.Nil(t, principal)
			assert.NoError(t, err)
		})
	}
}
