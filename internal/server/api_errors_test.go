package server

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func errorShapeApp(t *testing.T) *fiber.App {
	t.Helper()
	app := fiber.New(fiberConfig(nil))
	registerRoutes(app)
	return app
}

func TestUnmatchedV1PathsAreJSONNotFound(t *testing.T) {
	app := errorShapeApp(t)
	want := `{"error":{"code":"not_found","message":"not found"}}`
	for _, path := range []string{"/v1/does-not-exist", "/v1/packs/x/y", "/v1/", "/V1/nope"} {
		t.Run(path, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("GET", path, nil))
			require.NoError(t, err)
			body, _ := io.ReadAll(resp.Body)
			assert.Equal(t, 404, resp.StatusCode)
			assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
			assert.Equal(t, want, string(body))
		})
	}
}

// Paths outside /v1 that no route matches keep Fiber's default plain-text 404.
func TestNonV1UnmatchedKeepsDefault(t *testing.T) {
	app := errorShapeApp(t)
	resp, err := app.Test(httptest.NewRequest("GET", "/a/b/c/d", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "Cannot GET /a/b/c/d", string(body))
	assert.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain"))
}

func TestErrorHandlerWiredIntoConfig(t *testing.T) {
	app := fiber.New(fiberConfig(nil))
	app.Get("/v1/_boom", func(c *fiber.Ctx) error { return errors.New("secret detail") })
	app.Get("/_boom", func(c *fiber.Ctx) error { return errors.New("secret detail") })

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/_boom", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, 500, resp.StatusCode)
	assert.Equal(t, `{"error":{"code":"internal_error","message":"internal error"}}`, string(body))

	resp, err = app.Test(httptest.NewRequest("GET", "/_boom", nil))
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	assert.Equal(t, 500, resp.StatusCode)
	assert.Equal(t, "secret detail", string(body))
}

func TestSearchLimiterShape(t *testing.T) {
	app := fiber.New(fiberConfig(nil))
	lim := newSearchLimiter()
	ok := func(c *fiber.Ctx) error { return c.SendString("ok") }
	app.Get("/v1/_search", lim, ok)
	app.Get("/search", lim, ok)

	// One limiter instance counts both routes by IP: 60 requests pass.
	for range 30 {
		for _, p := range []string{"/v1/_search", "/search"} {
			resp, err := app.Test(httptest.NewRequest("GET", p, nil))
			require.NoError(t, err)
			require.Equal(t, 200, resp.StatusCode)
		}
	}

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/_search", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, 429, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	assert.Equal(t, `{"error":{"code":"rate_limited","message":"too many requests"}}`, string(body))

	resp, err = app.Test(httptest.NewRequest("GET", "/search", nil))
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	assert.Equal(t, 429, resp.StatusCode)
	assert.Equal(t, "Too many search requests. Please try again later.", string(body))
}
