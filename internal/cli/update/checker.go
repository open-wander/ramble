package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// GitHubRepo is the repository to check for updates
	GitHubRepo = "open-wander/ramble"
	// CheckInterval is how often to check for updates
	CheckInterval = 24 * time.Hour
)

// ReleaseInfo contains information about an available update
type ReleaseInfo struct {
	CurrentVersion string
	LatestVersion  string
	DownloadURL    string
	ReleaseURL     string
}

// githubRelease represents the GitHub API response
type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// cacheData stores the cached update check result
type cacheData struct {
	LastCheck     time.Time `json:"last_check"`
	LatestVersion string    `json:"latest_version"`
	ReleaseURL    string    `json:"release_url"`
}

// CheckForUpdate checks if a newer version is available
// Returns nil if current version is latest or if check fails
func CheckForUpdate(currentVersion string) *ReleaseInfo {
	// Skip check for dev builds
	if currentVersion == "dev" || currentVersion == "" {
		return nil
	}

	// Check cache first
	if cached := loadCache(); cached != nil {
		if time.Since(cached.LastCheck) < CheckInterval {
			return compareVersions(currentVersion, cached.LatestVersion, cached.ReleaseURL)
		}
	}

	// Fetch latest release from GitHub
	latest, err := fetchLatestRelease()
	if err != nil {
		return nil
	}

	// Save to cache
	saveCache(&cacheData{
		LastCheck:     time.Now(),
		LatestVersion: latest.TagName,
		ReleaseURL:    latest.HTMLURL,
	})

	return compareVersions(currentVersion, latest.TagName, latest.HTMLURL)
}

// CheckForUpdateAsync checks for updates in the background
// Results are written to the returned channel
func CheckForUpdateAsync(currentVersion string) <-chan *ReleaseInfo {
	ch := make(chan *ReleaseInfo, 1)
	go func() {
		ch <- CheckForUpdate(currentVersion)
		close(ch)
	}()
	return ch
}

func fetchLatestRelease() (*githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", GitHubRepo)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func compareVersions(current, latest, releaseURL string) *ReleaseInfo {
	// Normalize versions (remove 'v' prefix)
	currentNorm := strings.TrimPrefix(current, "v")
	latestNorm := strings.TrimPrefix(latest, "v")

	if !isNewerVersion(latestNorm, currentNorm) {
		return nil
	}

	return &ReleaseInfo{
		CurrentVersion: current,
		LatestVersion:  latest,
		DownloadURL:    getDownloadURL(latest),
		ReleaseURL:     releaseURL,
	}
}

// isNewerVersion returns true if version a is newer than version b
// Uses simple semver comparison (major.minor.patch)
func isNewerVersion(a, b string) bool {
	aParts := parseVersion(a)
	bParts := parseVersion(b)

	for i := 0; i < 3; i++ {
		if aParts[i] > bParts[i] {
			return true
		}
		if aParts[i] < bParts[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) [3]int {
	var parts [3]int
	// Remove any pre-release suffix (e.g., -rc1, -beta)
	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}
	fmt.Sscanf(v, "%d.%d.%d", &parts[0], &parts[1], &parts[2])
	return parts
}

func getDownloadURL(version string) string {
	var arch string
	switch runtime.GOARCH {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "arm64"
	default:
		arch = runtime.GOARCH
	}

	var os string
	var ext string
	switch runtime.GOOS {
	case "darwin":
		os = "Darwin"
		ext = "tar.gz"
	case "linux":
		os = "Linux"
		ext = "tar.gz"
	case "windows":
		os = "Windows"
		ext = "zip"
	default:
		os = runtime.GOOS
		ext = "tar.gz"
	}

	return fmt.Sprintf(
		"https://github.com/%s/releases/latest/download/ramble_%s_%s.%s",
		GitHubRepo, os, arch, ext,
	)
}

func getCachePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	return filepath.Join(cacheDir, "ramble", "update-check.json")
}

func loadCache() *cacheData {
	data, err := os.ReadFile(getCachePath())
	if err != nil {
		return nil
	}

	var cache cacheData
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}

	return &cache
}

func saveCache(cache *cacheData) {
	path := getCachePath()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return
	}

	os.WriteFile(path, data, 0644)
}

// FormatUpdateMessage returns a formatted message about the available update
func (r *ReleaseInfo) FormatUpdateMessage() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\nA new version is available: %s (current: %s)\n", r.LatestVersion, r.CurrentVersion))
	sb.WriteString(fmt.Sprintf("Release notes: %s\n", r.ReleaseURL))
	sb.WriteString("\nTo update:\n")

	switch runtime.GOOS {
	case "darwin", "linux":
		sb.WriteString(fmt.Sprintf("  curl -L %s | tar xz\n", r.DownloadURL))
		sb.WriteString("  sudo mv ramble /usr/local/bin/\n")
	case "windows":
		sb.WriteString(fmt.Sprintf("  Download: %s\n", r.DownloadURL))
	}

	return sb.String()
}
