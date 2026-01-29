package update

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"0.1.0", [3]int{0, 1, 0}},
		{"10.20.30", [3]int{10, 20, 30}},
		{"1.0.0-rc1", [3]int{1, 0, 0}},
		{"2.0.0-beta+build123", [3]int{2, 0, 0}},
		{"v1.2.3", [3]int{0, 0, 0}}, // v prefix not stripped here
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseVersion(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected bool
	}{
		{"major bump", "2.0.0", "1.0.0", true},
		{"minor bump", "1.2.0", "1.1.0", true},
		{"patch bump", "1.0.2", "1.0.1", true},
		{"same version", "1.0.0", "1.0.0", false},
		{"older major", "1.0.0", "2.0.0", false},
		{"older minor", "1.1.0", "1.2.0", false},
		{"older patch", "1.0.1", "1.0.2", false},
		{"complex newer", "1.10.0", "1.9.0", true},
		{"with prerelease", "1.1.0", "1.0.0-rc1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNewerVersion(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompareVersions(t *testing.T) {
	// Should return nil when versions are the same
	result := compareVersions("v1.0.0", "v1.0.0", "https://example.com")
	assert.Nil(t, result)

	// Should return nil when current is newer
	result = compareVersions("v2.0.0", "v1.0.0", "https://example.com")
	assert.Nil(t, result)

	// Should return info when update is available
	result = compareVersions("v1.0.0", "v2.0.0", "https://example.com/releases/v2.0.0")
	assert.NotNil(t, result)
	assert.Equal(t, "v1.0.0", result.CurrentVersion)
	assert.Equal(t, "v2.0.0", result.LatestVersion)
	assert.Equal(t, "https://example.com/releases/v2.0.0", result.ReleaseURL)
}

func TestGetDownloadURL(t *testing.T) {
	url := getDownloadURL("v1.0.0")
	assert.Contains(t, url, "github.com/open-wander/ramble/releases")
	assert.Contains(t, url, "ramble_")
}

func TestCheckForUpdateDevVersion(t *testing.T) {
	// Should return nil for dev builds
	result := CheckForUpdate("dev")
	assert.Nil(t, result)

	result = CheckForUpdate("")
	assert.Nil(t, result)
}

func TestFormatUpdateMessage(t *testing.T) {
	info := &ReleaseInfo{
		CurrentVersion: "v1.0.0",
		LatestVersion:  "v2.0.0",
		DownloadURL:    "https://example.com/download",
		ReleaseURL:     "https://example.com/releases/v2.0.0",
	}

	msg := info.FormatUpdateMessage()
	assert.Contains(t, msg, "v2.0.0")
	assert.Contains(t, msg, "v1.0.0")
	assert.Contains(t, msg, "https://example.com/releases/v2.0.0")
}

func TestCheckForUpdateAsync(t *testing.T) {
	// Test that the async check returns a channel
	ch := CheckForUpdateAsync("dev")
	assert.NotNil(t, ch)

	// Should receive nil for dev version
	result := <-ch
	assert.Nil(t, result)
}

func TestCheckForUpdateAsync_EmptyVersion(t *testing.T) {
	ch := CheckForUpdateAsync("")
	result := <-ch
	assert.Nil(t, result)
}

func TestGetDownloadURL_Darwin(t *testing.T) {
	// Test that URL contains expected parts
	url := getDownloadURL("v1.2.3")
	assert.Contains(t, url, "github.com/open-wander/ramble/releases")
	assert.Contains(t, url, "latest/download")
}

func TestGetDownloadURL_VersionFormat(t *testing.T) {
	url := getDownloadURL("v2.0.0")
	assert.Contains(t, url, "ramble_")
}

func TestReleaseInfo_Struct(t *testing.T) {
	info := ReleaseInfo{
		CurrentVersion: "v1.0.0",
		LatestVersion:  "v2.0.0",
		DownloadURL:    "https://example.com/download",
		ReleaseURL:     "https://example.com/releases",
	}

	assert.Equal(t, "v1.0.0", info.CurrentVersion)
	assert.Equal(t, "v2.0.0", info.LatestVersion)
	assert.Equal(t, "https://example.com/download", info.DownloadURL)
	assert.Equal(t, "https://example.com/releases", info.ReleaseURL)
}

func TestCacheData_Struct(t *testing.T) {
	now := time.Now()
	cache := cacheData{
		LastCheck:     now,
		LatestVersion: "v1.0.0",
		ReleaseURL:    "https://example.com",
	}

	assert.Equal(t, now, cache.LastCheck)
	assert.Equal(t, "v1.0.0", cache.LatestVersion)
	assert.Equal(t, "https://example.com", cache.ReleaseURL)
}

func TestGithubRelease_Struct(t *testing.T) {
	release := githubRelease{
		TagName: "v1.0.0",
		HTMLURL: "https://github.com/open-wander/ramble/releases/tag/v1.0.0",
	}

	assert.Equal(t, "v1.0.0", release.TagName)
	assert.Contains(t, release.HTMLURL, "github.com")
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "open-wander/ramble", GitHubRepo)
	assert.Equal(t, 24*time.Hour, CheckInterval)
}

func TestGetCachePath(t *testing.T) {
	path := getCachePath()
	assert.Contains(t, path, "ramble")
	assert.Contains(t, path, "update-check.json")
}

func TestLoadCache_NoFile(t *testing.T) {
	// If no cache file exists, should return nil
	cache := loadCache()
	// This is system-dependent, so we just verify it doesn't panic
	_ = cache
}

func TestSaveCache(t *testing.T) {
	// Just verify it doesn't panic
	cache := &cacheData{
		LastCheck:     time.Now(),
		LatestVersion: "v1.0.0",
		ReleaseURL:    "https://example.com",
	}
	saveCache(cache)
}

func TestParseVersion_EdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected [3]int
	}{
		{"", [3]int{0, 0, 0}},
		{"1", [3]int{1, 0, 0}},
		{"1.2", [3]int{1, 2, 0}},
		{"1.2.3.4", [3]int{1, 2, 3}}, // extra parts ignored
		{"abc", [3]int{0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseVersion(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNewerVersion_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected bool
	}{
		{"empty versions", "", "", false},
		{"a empty", "", "1.0.0", false},
		{"b empty", "1.0.0", "", true},
		{"both zeros", "0.0.0", "0.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNewerVersion(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompareVersions_WithVPrefix(t *testing.T) {
	// Both with v prefix
	result := compareVersions("v1.0.0", "v1.1.0", "https://example.com")
	assert.NotNil(t, result)
	assert.Equal(t, "v1.0.0", result.CurrentVersion)
	assert.Equal(t, "v1.1.0", result.LatestVersion)
}

func TestCompareVersions_MixedVPrefix(t *testing.T) {
	// Mixed prefixes should still work
	result := compareVersions("1.0.0", "v1.1.0", "https://example.com")
	assert.NotNil(t, result)

	result = compareVersions("v1.0.0", "1.1.0", "https://example.com")
	assert.NotNil(t, result)
}
