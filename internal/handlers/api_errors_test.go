package handlers

import (
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAPIPath(t *testing.T) {
	tests := map[string]bool{
		"/v1":            true,
		"/v1/":           true,
		"/v1/packs":      true,
		"/V1/Packs":      true,
		"/v10":           false,
		"/v1x/packs":     false,
		"/":              false,
		"/search":        false,
		"/user/v1/packs": false,
	}
	for p, want := range tests {
		assert.Equal(t, want, IsAPIPath(p), p)
	}
}

func TestAPIErrorHandler(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: APIErrorHandler})
	app.Get("/v1/_boom", func(c *fiber.Ctx) error { return errors.New("secret detail") })
	app.Get("/v1/_forbidden", func(c *fiber.Ctx) error { return fiber.ErrForbidden })
	app.Get("/v1/_teapot", func(c *fiber.Ctx) error { return fiber.NewError(418, "secret teapot") })
	app.Get("/v1/_bad", func(c *fiber.Ctx) error { return fiber.NewError(400, "secret bad") })
	app.Get("/v1/_missing", func(c *fiber.Ctx) error { return fiber.NewError(404, "secret missing") })
	app.Get("/v1/_method", func(c *fiber.Ctx) error { return fiber.NewError(405, "secret method") })
	app.Get("/v1/_slow", func(c *fiber.Ctx) error { return fiber.NewError(429, "secret slow") })
	app.Get("/other/_boom", func(c *fiber.Ctx) error { return errors.New("secret detail") })

	tests := []struct {
		path    string
		status  int
		code    string
		message string
	}{
		{"/v1/_boom", 500, "internal_error", "internal error"},
		{"/v1/_forbidden", 403, "forbidden", "forbidden"},
		{"/v1/_teapot", 500, "internal_error", "internal error"},
		{"/v1/_bad", 400, "bad_request", "bad request"},
		{"/v1/_missing", 404, "not_found", "not found"},
		{"/v1/_method", 405, "method_not_allowed", "method not allowed"},
		{"/v1/_slow", 429, "rate_limited", "too many requests"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("GET", tt.path, nil))
			require.NoError(t, err)
			body, _ := io.ReadAll(resp.Body)
			assert.NotContains(t, string(body), "secret")
			assert.Equal(t, tt.status, resp.StatusCode)
			assert.JSONEq(t, `{"error":{"code":"`+tt.code+`","message":"`+tt.message+`"}}`, string(body))
		})
	}

	t.Run("non-v1 keeps the default handler", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest("GET", "/other/_boom", nil))
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, 500, resp.StatusCode)
		assert.Equal(t, "secret detail", string(body))
		assert.Contains(t, resp.Header.Get("Content-Type"), "text/plain")
	})
}

// TestAPIRoutes_NotFound hits every /v1 route with an input that matches
// nothing and asserts the exact status and code.
func TestAPIRoutes_NotFound(t *testing.T) {
	defer cleanupTestData(t)
	createTestUser(t, "errshapeuser")

	tests := []struct {
		name    string
		route   string
		handler fiber.Handler
		url     string
		message string
	}{
		{"namespaced list packs", "/:username/v1/packs", ListPacksAPI, "/nobody-here/v1/packs", "namespace not found"},
		{"namespaced get pack ns", "/:username/v1/packs/:packname", GetPackAPI, "/nobody-here/v1/packs/x", "namespace not found"},
		{"namespaced get pack", "/:username/v1/packs/:packname", GetPackAPI, "/errshapeuser/v1/packs/missing", "pack not found"},
		{"namespaced get job ns", "/:username/v1/jobs/:jobname", GetJobAPI, "/nobody-here/v1/jobs/x", "namespace not found"},
		{"namespaced get job", "/:username/v1/jobs/:jobname", GetJobAPI, "/errshapeuser/v1/jobs/missing", "job not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupTestApp()
			app.Get(tt.route, tt.handler)
			resp, err := app.Test(httptest.NewRequest("GET", tt.url, nil))
			require.NoError(t, err)
			assertAPIError(t, resp, 404, "not_found", tt.message)
		})
	}
}

// TestAPIRoutes_DatabaseError checks every /v1 handler answers 500
// internal_error when its first query fails. The table is the one the first
// query hits, so the failure is the one being tested.
func TestAPIRoutes_DatabaseError(t *testing.T) {
	defer cleanupTestData(t)
	createTestUser(t, "errdbuser")
	tests := []struct {
		name    string
		route   string
		handler fiber.Handler
		url     string
		table   string
	}{
		{"list packs ns lookup", "/:username/v1/packs", ListPacksAPI, "/someone/v1/packs", "users"},
		{"get pack ns lookup", "/:username/v1/packs/:packname", GetPackAPI, "/someone/v1/packs/x", "users"},
		{"get job ns lookup", "/:username/v1/jobs/:jobname", GetJobAPI, "/someone/v1/jobs/x", "users"},
		{"list packs org lookup", "/:username/v1/packs", ListPacksAPI, "/nobody-here/v1/packs", "organizations"},
		{"get pack org lookup", "/:username/v1/packs/:packname", GetPackAPI, "/nobody-here/v1/packs/x", "organizations"},
		{"get job org lookup", "/:username/v1/jobs/:jobname", GetJobAPI, "/nobody-here/v1/jobs/x", "organizations"},
		{"list packs query", "/:username/v1/packs", ListPacksAPI, "/errdbuser/v1/packs", "nomad_resources"},
		{"get pack query", "/:username/v1/packs/:packname", GetPackAPI, "/errdbuser/v1/packs/x", "nomad_resources"},
		{"get job query", "/:username/v1/jobs/:jobname", GetJobAPI, "/errdbuser/v1/jobs/x", "nomad_resources"},
		{"all packs", "/v1/packs", ListAllPacksAPI, "/v1/packs", "nomad_resources"},
		{"all jobs", "/v1/jobs", ListAllJobsAPI, "/v1/jobs", "nomad_resources"},
		{"search packs", "/v1/packs/search", SearchPacksAPI, "/v1/packs/search?q=a", "nomad_resources"},
		{"search jobs", "/v1/jobs/search", SearchJobsAPI, "/v1/jobs/search?q=a", "nomad_resources"},
		{"recent", "/v1/recent", ListRecentAPI, "/v1/recent", "nomad_resources"},
		{"registries", "/v1/registries", ListUserRegistriesAPI, "/v1/registries", "nomad_resources"},
		{"search registries", "/v1/registries/search", SearchRegistriesAPI, "/v1/registries/search?q=a", "nomad_resources"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failingDB(t, tt.table, "query")
			app := setupTestApp()
			app.Get(tt.route, tt.handler)
			resp, err := app.Test(httptest.NewRequest("GET", tt.url, nil))
			require.NoError(t, err)
			assertAPIError(t, resp, 500, "internal_error", "internal error")
		})
	}
}
