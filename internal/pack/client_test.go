package pack

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedURL string
	}{
		{
			name:        "with https",
			input:       "https://example.com",
			expectedURL: "https://example.com",
		},
		{
			name:        "with http",
			input:       "http://example.com",
			expectedURL: "http://example.com",
		},
		{
			name:        "without scheme",
			input:       "example.com",
			expectedURL: "https://example.com",
		},
		{
			name:        "with trailing slash",
			input:       "https://example.com/",
			expectedURL: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.input)
			assert.Equal(t, tt.expectedURL, client.BaseURL)
		})
	}
}

func TestListAllPacks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/packs", r.URL.Path)
		json.NewEncoder(w).Encode(map[string][]PackSummary{
			"packs": {
				{Name: "pack1", Description: "First pack"},
				{Name: "pack2", Description: "Second pack"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	packs, err := client.ListAllPacks()

	require.NoError(t, err)
	assert.Len(t, packs, 2)
	assert.Equal(t, "pack1", packs[0].Name)
	assert.Equal(t, "pack2", packs[1].Name)
}

func TestListPacks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/myuser", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		json.NewEncoder(w).Encode(map[string][]PackSummary{
			"packs": {
				{Name: "mypack", Description: "My pack"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	packs, err := client.ListPacks("myuser")

	require.NoError(t, err)
	assert.Len(t, packs, 1)
	assert.Equal(t, "mypack", packs[0].Name)
}

func TestGetPack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/myuser/mypack", r.URL.Path)
		json.NewEncoder(w).Encode(PackDetail{
			Name:        "mypack",
			Description: "My pack description",
			Versions: []PackVersion{
				{Version: "v1.0.0", URL: "https://example.com/v1.0.0.tar.gz"},
				{Version: "v0.9.0", URL: "https://example.com/v0.9.0.tar.gz"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	detail, err := client.GetPack("myuser", "mypack")

	require.NoError(t, err)
	assert.Equal(t, "mypack", detail.Name)
	assert.Len(t, detail.Versions, 2)
	assert.Equal(t, "v1.0.0", detail.Versions[0].Version)
}

func TestGetPackNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetPack("myuser", "nonexistent")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pack not found")
}

func TestSearchPacks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/packs/search", r.URL.Path)
		assert.Equal(t, "mysql", r.URL.Query().Get("q"))
		json.NewEncoder(w).Encode(map[string][]PackSummary{
			"packs": {
				{Name: "mysql", Description: "MySQL database"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	packs, err := client.SearchPacks("mysql")

	require.NoError(t, err)
	assert.Len(t, packs, 1)
	assert.Equal(t, "mysql", packs[0].Name)
}

func TestListAllJobs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/jobs", r.URL.Path)
		json.NewEncoder(w).Encode(map[string][]JobSummary{
			"jobs": {
				{Name: "job1", Description: "First job"},
				{Name: "job2", Description: "Second job"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	jobs, err := client.ListAllJobs()

	require.NoError(t, err)
	assert.Len(t, jobs, 2)
	assert.Equal(t, "job1", jobs[0].Name)
}

func TestSearchJobs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/jobs/search", r.URL.Path)
		assert.Equal(t, "postgres", r.URL.Query().Get("q"))
		json.NewEncoder(w).Encode(map[string][]JobSummary{
			"jobs": {
				{Name: "postgres", Description: "PostgreSQL database"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	jobs, err := client.SearchJobs("postgres")

	require.NoError(t, err)
	assert.Len(t, jobs, 1)
	assert.Equal(t, "postgres", jobs[0].Name)
}

func TestGetRawContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/pack/raw" {
			w.Write([]byte("job \"example\" {}"))
		} else if r.URL.Path == "/user/pack/v/v1.0.0/raw" {
			w.Write([]byte("job \"example\" { version = \"1.0.0\" }"))
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)

	// Without version
	content, err := client.GetRawContent("user", "pack", "")
	require.NoError(t, err)
	assert.Equal(t, "job \"example\" {}", content)

	// With version
	content, err = client.GetRawContent("user", "pack", "v1.0.0")
	require.NoError(t, err)
	assert.Contains(t, content, "version")
}

func TestListRegistries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/registries", r.URL.Path)
		json.NewEncoder(w).Encode(map[string][]string{
			"registries": {"user1", "user2", "org1"},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	registries, err := client.ListRegistries()

	require.NoError(t, err)
	assert.Len(t, registries, 3)
	assert.Contains(t, registries, "user1")
}
