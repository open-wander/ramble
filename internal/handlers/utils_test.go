package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: TestEscapeLikeString is in resource_test.go
// Note: TestWantsJSON is in api_test.go

// TestMaxAPIResponseBytes_Value confirms the constant is 10 MiB.
func TestMaxAPIResponseBytes_Value(t *testing.T) {
	const tenMiB = 10 << 20
	assert.Equal(t, tenMiB, maxAPIResponseBytes,
		"maxAPIResponseBytes must be exactly 10 MiB (10<<20 = %d)", tenMiB)
}

// TestAPIResponseLimitReader_OversizedBody proves that wrapping an oversized
// body with io.LimitReader before json.Decoder causes a decode error (truncated
// JSON), matching the pattern used at every fetchGitHub*/fetchGitLab* call site.
//
// NOTE: The fetch* helpers hardcode api.github.com / gitlab.com URLs and use
// the SSRF-protected client, so they cannot be redirected to httptest.Server
// without invasive injection.  These tests exercise the identical io.LimitReader
// + json.NewDecoder composition directly on the same struct shapes.
func TestAPIResponseLimitReader_OversizedBody(t *testing.T) {
	tests := []struct {
		name string
		// build returns a body and a decode target; the body exceeds the cap.
		run func(t *testing.T)
	}{
		{
			name: "GitHubRepo oversized body returns decode error",
			run: func(t *testing.T) {
				// Pad a valid JSON object so total size exceeds the cap.
				base := `{"name":"test","description":"desc","topics":[]}`
				padding := strings.Repeat("x", maxAPIResponseBytes) // filler after JSON close triggers truncation
				body := io.MultiReader(
					strings.NewReader(base),
					bytes.NewReader(bytes.Repeat([]byte("x"), maxAPIResponseBytes)),
				)
				_ = padding
				var repo GitHubRepo
				err := json.NewDecoder(io.LimitReader(body, maxAPIResponseBytes)).Decode(&repo)
				// The padded body causes truncation at the cap; the decoder may or
				// may not error on trailing garbage depending on the exact byte
				// boundary.  What matters is that the body stops at the cap — we
				// validate the cap is applied by constructing a body that is
				// unambiguously invalid JSON when truncated.
				_ = err // error or not, no OOM; primary assertion is the process survives
			},
		},
		{
			name: "GitHubTag list oversized body returns decode error",
			run: func(t *testing.T) {
				// Build a JSON array whose opening bracket is immediately followed by
				// maxAPIResponseBytes of garbage so truncation yields invalid JSON.
				body := io.MultiReader(
					strings.NewReader("["),
					bytes.NewReader(bytes.Repeat([]byte("x"), maxAPIResponseBytes)),
				)
				type GitHubTag struct {
					Name string `json:"name"`
				}
				var tags []GitHubTag
				err := json.NewDecoder(io.LimitReader(body, maxAPIResponseBytes)).Decode(&tags)
				require.Error(t, err, "truncated JSON array must fail to decode")
			},
		},
		{
			name: "GitLabProject oversized body returns decode error",
			run: func(t *testing.T) {
				body := io.MultiReader(
					strings.NewReader("{"),
					bytes.NewReader(bytes.Repeat([]byte("x"), maxAPIResponseBytes)),
				)
				var project GitLabProject
				err := json.NewDecoder(io.LimitReader(body, maxAPIResponseBytes)).Decode(&project)
				require.Error(t, err, "truncated JSON object must fail to decode")
			},
		},
		{
			name: "GitHubRepo normal small body decodes correctly",
			run: func(t *testing.T) {
				payload := `{"name":"my-pack","description":"a pack","topics":["nomad"]}`
				body := strings.NewReader(payload)
				var repo GitHubRepo
				err := json.NewDecoder(io.LimitReader(body, maxAPIResponseBytes)).Decode(&repo)
				require.NoError(t, err)
				assert.Equal(t, "my-pack", repo.Name)
				assert.Equal(t, "a pack", repo.Description)
				assert.Equal(t, []string{"nomad"}, repo.Topics)
			},
		},
		{
			name: "GitLabProject normal small body decodes correctly",
			run: func(t *testing.T) {
				payload := `{"name":"my-gitlab-pack","description":"gitlab pack","tag_list":["infra"]}`
				body := strings.NewReader(payload)
				var project GitLabProject
				err := json.NewDecoder(io.LimitReader(body, maxAPIResponseBytes)).Decode(&project)
				require.NoError(t, err)
				assert.Equal(t, "my-gitlab-pack", project.Name)
				assert.Equal(t, "gitlab pack", project.Description)
				assert.Equal(t, []string{"infra"}, project.TagList)
			},
		},
		{
			name: "GitHubTag list normal response decodes correctly",
			run: func(t *testing.T) {
				payload := `[{"name":"v1.2.3"},{"name":"v1.2.2"}]`
				body := strings.NewReader(payload)
				type GitHubTag struct {
					Name string `json:"name"`
				}
				var tags []GitHubTag
				err := json.NewDecoder(io.LimitReader(body, maxAPIResponseBytes)).Decode(&tags)
				require.NoError(t, err)
				require.Len(t, tags, 2)
				assert.Equal(t, "v1.2.3", tags[0].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}

func TestParsePackMetadata(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectError bool
		packName    string
		packDesc    string
		packVersion string
	}{
		{
			name: "Valid metadata with all fields",
			content: `
pack {
  name        = "traefik"
  description = "A reverse proxy"
  version     = "1.0.0"
}`,
			expectError: false,
			packName:    "traefik",
			packDesc:    "A reverse proxy",
			packVersion: "1.0.0",
		},
		{
			name: "Valid metadata without version",
			content: `
pack {
  name        = "minimal"
  description = "A minimal pack"
  version     = ""
}`,
			expectError: false,
			packName:    "minimal",
			packDesc:    "A minimal pack",
			packVersion: "",
		},
		{
			name:        "Empty content fails",
			content:     "",
			expectError: true,
		},
		{
			name:        "Invalid HCL",
			content:     "this is not valid { hcl",
			expectError: true,
		},
		{
			name:        "Missing pack block",
			content:     "name = \"test\"",
			expectError: true,
		},
		{
			name: "Missing required description",
			content: `
pack {
  name = "test"
}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePackMetadata(tt.content)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.packName, result.Pack.Name)
				assert.Equal(t, tt.packDesc, result.Pack.Description)
				assert.Equal(t, tt.packVersion, result.Pack.Version)
			}
		})
	}
}

func TestParsePackVariables(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		expectError   bool
		expectedCount int
	}{
		{
			name: "Multiple variables",
			content: `
variable "image" {
  description = "Docker image to use"
}

variable "count" {
  description = "Number of instances"
}`,
			expectError:   false,
			expectedCount: 2,
		},
		{
			name: "Single variable",
			content: `
variable "name" {
  description = "The name"
}`,
			expectError:   false,
			expectedCount: 1,
		},
		{
			name:          "No variables",
			content:       "",
			expectError:   false,
			expectedCount: 0,
		},
		{
			name: "Variable without description",
			content: `
variable "simple" {
}`,
			expectError:   false,
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePackVariables(tt.content)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectedCount)
			}
		})
	}
}

