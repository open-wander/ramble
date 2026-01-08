package update

import (
	"testing"

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
