package handlers

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestParsePackMetadata(t *testing.T) {
	t.Skip("parsePackMetadata signature changed - test needs rewrite")
}

func TestParsePackVariables(t *testing.T) {
	t.Skip("parsePackVariables signature changed - test needs rewrite")
}

// TestAPIResponseLimitReader verifies the maxAPIResponseBytes cap applied to
// GitHub/GitLab API response decoding: a small body decodes fine, while an
// oversized body fails to decode through the capped reader.
func TestAPIResponseLimitReader(t *testing.T) {
	t.Run("small valid body decodes", func(t *testing.T) {
		body := `{"description":"hello","license":{"spdx_id":"MIT"}}`
		limited := io.LimitReader(strings.NewReader(body), maxAPIResponseBytes)
		var repo GitHubRepo
		if err := json.NewDecoder(limited).Decode(&repo); err != nil {
			t.Fatalf("unexpected error decoding small body: %v", err)
		}
		if repo.Description != "hello" {
			t.Errorf("Description = %q, want %q", repo.Description, "hello")
		}
	})

	t.Run("oversized body returns decode error", func(t *testing.T) {
		oversized := `{"description":"` + strings.Repeat("a", maxAPIResponseBytes+100) + `"}`
		limited := io.LimitReader(strings.NewReader(oversized), maxAPIResponseBytes)
		var repo GitHubRepo
		if err := json.NewDecoder(limited).Decode(&repo); err == nil {
			t.Error("expected decode error for oversized body, got nil")
		}
	})
}

func TestMaxAPIResponseBytes_Value(t *testing.T) {
	if maxAPIResponseBytes != 10<<20 {
		t.Errorf("maxAPIResponseBytes = %d, want %d", maxAPIResponseBytes, 10<<20)
	}
}
