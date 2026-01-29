package version

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const (
	githubRepo    = "open-wander/ramble"
	cacheDuration = 1 * time.Hour
)

// DefaultBaseURL is the default GitHub API base URL
const DefaultBaseURL = "https://api.github.com"

var (
	cachedVersion string
	cacheTime     time.Time
	cacheMutex    sync.RWMutex
	// httpClient is the HTTP client used for API requests (can be overridden in tests)
	httpClient = &http.Client{Timeout: 5 * time.Second}
	// baseURL is the base URL for API requests (can be overridden in tests)
	baseURL = DefaultBaseURL
)

// SetHTTPClient sets the HTTP client for testing
func SetHTTPClient(client *http.Client) {
	httpClient = client
}

// SetBaseURL sets the base URL for testing
func SetBaseURL(url string) {
	baseURL = url
}

// ResetCache clears the version cache (for testing)
func ResetCache() {
	cacheMutex.Lock()
	cachedVersion = ""
	cacheTime = time.Time{}
	cacheMutex.Unlock()
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// GetLatestVersion returns the latest release version from GitHub
// Results are cached for 1 hour
func GetLatestVersion() string {
	cacheMutex.RLock()
	if cachedVersion != "" && time.Since(cacheTime) < cacheDuration {
		v := cachedVersion
		cacheMutex.RUnlock()
		return v
	}
	cacheMutex.RUnlock()

	// Fetch from GitHub
	version := fetchLatestVersion()
	if version != "" {
		cacheMutex.Lock()
		cachedVersion = version
		cacheTime = time.Now()
		cacheMutex.Unlock()
	}

	return version
}

func fetchLatestVersion() string {
	resp, err := httpClient.Get(baseURL + "/repos/" + githubRepo + "/releases/latest")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return ""
	}

	return release.TagName
}
