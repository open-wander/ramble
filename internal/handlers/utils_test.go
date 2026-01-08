package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Note: TestEscapeLikeString is in resource_test.go
// Note: TestWantsJSON is in api_test.go

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

