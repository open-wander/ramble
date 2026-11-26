package pack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenURLAllowed(t *testing.T) {
	tests := []struct {
		url string
		ok  bool
	}{
		{"https://ramble.openwander.org", true},
		{"http://localhost:8080", true},
		{"http://127.0.0.1:3000", true},
		{"http://127.1.2.3", true},
		{"http://[::1]:3000", true},
		{"http://example.com", false},
		{"http://10.0.0.1", false},
		{"http://192.168.1.10:8080", false},
		{"http://[2001:db8::1]", false},
		{"http://localhost.evil.com", false},
		{"http://127.0.0.1.nip.io", false},
		{"ftp://localhost", false},
		{"https://", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := TokenURLAllowed(tt.url)
			if tt.ok {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestCurrentUser(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr int // status of APIError, 0 for none
		wantMsg string
	}{
		{"ok", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`{"username":"alice","token_scope":"read","token_org":null}`))
		}, 0, ""},
		{"401", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"invalid token"}}`))
		}, 401, "invalid token"},
		{"redirect not followed", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/elsewhere", http.StatusMovedPermanently)
		}, 301, ""},
		{"500", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }, 500, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			u, err := NewClient(srv.URL).CurrentUser(context.Background(), "tok")
			if tt.wantErr == 0 {
				require.NoError(t, err)
				assert.Equal(t, "alice", u.Username)
				return
			}
			apiErr, ok := err.(*APIError)
			require.True(t, ok, "got %v", err)
			assert.Equal(t, tt.wantErr, apiErr.StatusCode)
			assert.Equal(t, tt.wantMsg, apiErr.Message)
		})
	}
}

func TestCurrentUserRefusesPlainHTTP(t *testing.T) {
	// The refusal must come before any connection attempt, not from a failed
	// request: the error names the refusal.
	_, err := NewClient("http://registry.example.invalid").CurrentUser(context.Background(), "s3cr3t")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to send a token")
	assert.NotContains(t, err.Error(), "s3cr3t")
}
