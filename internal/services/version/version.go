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

var (
	cachedVersion string
	cacheTime     time.Time
	cacheMutex    sync.RWMutex
)

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
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/" + githubRepo + "/releases/latest")
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
