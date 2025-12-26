package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDevelopmentPacks(t *testing.T) {
	app := setupTestApp()
	app.Get("/api/dev/packs", GetDevelopmentPacks)

	req := httptest.NewRequest("GET", "/api/dev/packs", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var packs []DevPack
	err = json.NewDecoder(resp.Body).Decode(&packs)
	assert.Nil(t, err)

	// We expect at least one pack "example-pack" which is in the examples directory
	found := false
	for _, p := range packs {
		if p.Name == "example-pack" {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected to find 'example-pack' in development packs")
}
