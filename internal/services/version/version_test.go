package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetLatestVersion_Success(t *testing.T) {
	ResetCache()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Method = %s, want GET", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(githubRelease{
			TagName: "v1.2.3",
		})
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	SetHTTPClient(&http.Client{Timeout: 5 * time.Second})
	defer func() {
		SetBaseURL(DefaultBaseURL)
	}()

	version := GetLatestVersion()
	if version != "v1.2.3" {
		t.Errorf("GetLatestVersion() = %s, want v1.2.3", version)
	}
}

func TestGetLatestVersion_Cached(t *testing.T) {
	ResetCache()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(githubRelease{
			TagName: "v1.0.0",
		})
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	SetHTTPClient(&http.Client{Timeout: 5 * time.Second})
	defer func() {
		SetBaseURL(DefaultBaseURL)
	}()

	// First call - fetches from server
	v1 := GetLatestVersion()
	if v1 != "v1.0.0" {
		t.Errorf("First GetLatestVersion() = %s, want v1.0.0", v1)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 after first call", callCount)
	}

	// Second call - should use cache
	v2 := GetLatestVersion()
	if v2 != "v1.0.0" {
		t.Errorf("Second GetLatestVersion() = %s, want v1.0.0", v2)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (cache should be used)", callCount)
	}
}

func TestGetLatestVersion_NotFound(t *testing.T) {
	ResetCache()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	SetHTTPClient(&http.Client{Timeout: 5 * time.Second})
	defer func() {
		SetBaseURL(DefaultBaseURL)
	}()

	version := GetLatestVersion()
	if version != "" {
		t.Errorf("GetLatestVersion() = %s, want empty string for 404", version)
	}
}

func TestGetLatestVersion_ServerError(t *testing.T) {
	ResetCache()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	SetHTTPClient(&http.Client{Timeout: 5 * time.Second})
	defer func() {
		SetBaseURL(DefaultBaseURL)
	}()

	version := GetLatestVersion()
	if version != "" {
		t.Errorf("GetLatestVersion() = %s, want empty string for 500", version)
	}
}

func TestGetLatestVersion_InvalidJSON(t *testing.T) {
	ResetCache()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	SetHTTPClient(&http.Client{Timeout: 5 * time.Second})
	defer func() {
		SetBaseURL(DefaultBaseURL)
	}()

	version := GetLatestVersion()
	if version != "" {
		t.Errorf("GetLatestVersion() = %s, want empty string for invalid JSON", version)
	}
}

func TestGetLatestVersion_ConnectionError(t *testing.T) {
	ResetCache()

	// Use an invalid URL to simulate connection error
	SetBaseURL("http://localhost:99999")
	SetHTTPClient(&http.Client{Timeout: 100 * time.Millisecond})
	defer func() {
		SetBaseURL(DefaultBaseURL)
	}()

	version := GetLatestVersion()
	if version != "" {
		t.Errorf("GetLatestVersion() = %s, want empty string for connection error", version)
	}
}

func TestResetCache(t *testing.T) {
	// Set up a server that returns different versions
	version := "v1.0.0"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(githubRelease{
			TagName: version,
		})
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	SetHTTPClient(&http.Client{Timeout: 5 * time.Second})
	defer func() {
		SetBaseURL(DefaultBaseURL)
	}()

	ResetCache()

	// First call
	v1 := GetLatestVersion()
	if v1 != "v1.0.0" {
		t.Errorf("First GetLatestVersion() = %s, want v1.0.0", v1)
	}

	// Change the version
	version = "v2.0.0"

	// Should still return cached v1.0.0
	v2 := GetLatestVersion()
	if v2 != "v1.0.0" {
		t.Errorf("Cached GetLatestVersion() = %s, want v1.0.0", v2)
	}

	// Reset cache
	ResetCache()

	// Now should fetch new version
	v3 := GetLatestVersion()
	if v3 != "v2.0.0" {
		t.Errorf("After reset GetLatestVersion() = %s, want v2.0.0", v3)
	}
}

func TestGithubRelease_Unmarshal(t *testing.T) {
	jsonData := `{"tag_name": "v1.5.0"}`

	var release githubRelease
	if err := json.Unmarshal([]byte(jsonData), &release); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if release.TagName != "v1.5.0" {
		t.Errorf("TagName = %s, want v1.5.0", release.TagName)
	}
}

func TestGithubRelease_ExtraFields(t *testing.T) {
	// GitHub response has many more fields than we use
	jsonData := `{
		"tag_name": "v1.5.0",
		"name": "Release 1.5.0",
		"body": "Release notes here",
		"draft": false,
		"prerelease": false,
		"created_at": "2024-01-15T10:00:00Z"
	}`

	var release githubRelease
	if err := json.Unmarshal([]byte(jsonData), &release); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if release.TagName != "v1.5.0" {
		t.Errorf("TagName = %s, want v1.5.0", release.TagName)
	}
}
