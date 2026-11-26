//go:build security

package server

import (
	"io"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/handlers"
	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
	"rmbl/internal/services/logger"
)

func TestMain(m *testing.M) {
	_, cleanup := database.SetupTestDB()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func mkUserTok(t *testing.T, name, scope string, admin bool) (models.User, string, *models.APIToken) {
	t.Helper()
	apitoken.Init(database.DB)
	handlers.InitSession()
	sfx, err := apitoken.Generate()
	require.NoError(t, err)
	sfx = strings.ToLower(sfx[5:15])
	u := models.User{Username: name + "-" + sfx, Email: name + sfx + "@example.com", EmailVerified: true, IsAdmin: admin}
	require.NoError(t, database.DB.Create(&u).Error)
	plain, tok, err := apitoken.Svc.Create(u.ID, "t", scope, nil, 30)
	require.NoError(t, err)
	return u, plain, tok
}

// stackApp builds the real middleware stack from Run() plus test routes.
func stackApp(t *testing.T, withRoutes bool) *fiber.App {
	t.Helper()
	app := fiber.New(fiberConfig(nil))
	useRequestLogging(app, logger.Log)
	useSecurityMiddleware(app, false)
	useSessionMiddleware(app, "test")
	if withRoutes {
		registerRoutes(app)
	}
	return app
}

func send(t *testing.T, app *fiber.App, method, path string, hdr map[string]string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestCSRF_BearerExemptionThroughStack(t *testing.T) {
	_, plain, _ := mkUserTok(t, "alice", models.ScopeWrite, false)
	app := stackApp(t, false)
	ok := func(c *fiber.Ctx) error { return c.SendString("ok") }
	app.Post("/v1/_test", handlers.RequireAPIToken(models.ScopeWrite), ok)
	app.Post("/other", handlers.RequireAPIToken(models.ScopeWrite), ok)
	bearer := "Bearer " + plain

	// (a) Bearer, no cookies: no CSRF token needed.
	status, _ := send(t, app, "POST", "/v1/_test", map[string]string{"Authorization": bearer})
	assert.Equal(t, 200, status)

	// (b) Same request with a session cookie: cookie-authenticated, CSRF applies.
	status, _ = send(t, app, "POST", "/v1/_test", map[string]string{
		"Authorization": bearer, "Cookie": handlers.SessionCookieName + "=anything"})
	assert.Equal(t, 403, status)

	// (c) Non-/v1 path keeps CSRF.
	status, _ = send(t, app, "POST", "/other", map[string]string{"Authorization": bearer})
	assert.Equal(t, 403, status)

	// No bearer header on /v1: CSRF applies.
	status, _ = send(t, app, "POST", "/v1/_test", nil)
	assert.Equal(t, 403, status)
}

func TestAdminRoutesIgnoreBearerToken(t *testing.T) {
	_, plain, _ := mkUserTok(t, "alice", models.ScopeWrite, true)
	app := stackApp(t, true)

	anonStatus, _ := send(t, app, "GET", "/admin/", nil)
	req := httptest.NewRequest("GET", "/admin/", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	reqAnon := httptest.NewRequest("GET", "/admin/", nil)
	respAnon, err := app.Test(reqAnon, -1)
	require.NoError(t, err)

	assert.Equal(t, 302, anonStatus)
	assert.Equal(t, respAnon.StatusCode, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
	assert.Equal(t, respAnon.Header.Get("Location"), resp.Header.Get("Location"))
}

func TestAPIWriteLimiterThroughStack(t *testing.T) {
	_, plainA, _ := mkUserTok(t, "alice", models.ScopeWrite, false)
	_, plainB, _ := mkUserTok(t, "bob", models.ScopeWrite, false)
	app := stackApp(t, false)
	limiter := handlers.NewAPIWriteLimiter()
	h := func(c *fiber.Ctx) error { return c.SendString("ok") }
	app.Post("/v1/_x", handlers.RequireAPIToken(models.ScopeWrite), limiter, h)
	app.Get("/v1/_x", handlers.RequireAPIToken(models.ScopeRead), limiter, h)
	a := map[string]string{"Authorization": "Bearer " + plainA}
	b := map[string]string{"Authorization": "Bearer " + plainB}

	// Reads are not counted.
	for range 130 {
		status, _ := send(t, app, "GET", "/v1/_x", a)
		require.Equal(t, 200, status)
	}
	for i := range 120 {
		status, _ := send(t, app, "POST", "/v1/_x", a)
		require.Equal(t, 200, status, "request "+strconv.Itoa(i+1))
	}
	status, body := send(t, app, "POST", "/v1/_x", a)
	assert.Equal(t, 429, status)
	assert.Equal(t, `{"error":{"code":"rate_limited","message":"rate limit exceeded"}}`, body)

	status, _ = send(t, app, "POST", "/v1/_x", b)
	assert.Equal(t, 200, status, "second token is unaffected")
}
