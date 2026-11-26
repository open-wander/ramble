package apiclient

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		token   string
		wantErr string
	}{
		{"https", "https://registry.example.com", "t", ""},
		{"no scheme means https", "registry.example.com/", "t", ""},
		{"http remote refused", "http://registry.example.com", "t", "refusing to send a token"},
		{"http localhost", "http://localhost:8080", "t", ""},
		{"http ipv4 loopback", "http://127.0.0.1:8080", "t", ""},
		{"http ipv6 loopback", "http://[::1]:8080", "t", ""},
		{"empty token", "https://registry.example.com", "", "no API token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.url, tt.token)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, c)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, c)
		})
	}
}

func TestNewSendsBearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"alice"}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, "tok")
	require.NoError(t, err)
	resp, err := c.GetCurrentUserWithResponse(t.Context())
	require.NoError(t, err)
	require.NotNil(t, resp.JSON200)
	assert.Equal(t, "alice", resp.JSON200.Username)
}

func TestLimitedDoerCapsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("a"), 3*maxResponseBody))
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := limitedDoer{srv.Client()}.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Len(t, b, maxResponseBody)
}
