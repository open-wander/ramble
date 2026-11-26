package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
	"rmbl/internal/services/logger"
	"rmbl/internal/testutil/logtest"
)

func uptr(v uint) *uint { return &v }

func TestTokenAllowsOrg(t *testing.T) {
	orgA, orgB := uint(1), uint(2)
	restricted := &models.APIToken{OrganizationID: &orgA}
	unrestricted := &models.APIToken{}
	tests := []struct {
		name   string
		tok    *models.APIToken
		target *uint
		want   bool
	}{
		{"restricted same org", restricted, &orgA, true},
		{"restricted other org", restricted, &orgB, false},
		{"restricted personal target", restricted, nil, false},
		{"unrestricted org", unrestricted, &orgA, true},
		{"unrestricted personal", unrestricted, nil, true},
		{"no token (session)", nil, &orgB, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tokenAllowsOrg(tt.tok, tt.target))
		})
	}
}

func TestAPIOrgForbidden_Responses(t *testing.T) {
	app := fiber.New()
	orgA, orgB := uint(1), uint(2)
	app.Get("/x", func(c *fiber.Ctx) error {
		c.Locals(LocalAPIToken, &models.APIToken{OrganizationID: &orgA})
		target := &orgB
		if c.Query("t") == "a" {
			target = &orgA
		}
		if !TokenAllowsOrg(c, target) {
			return APIOrgForbidden(c)
		}
		return c.SendString("ok")
	})
	resp, _ := app.Test(httptest.NewRequest("GET", "/x?t=b", nil))
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, 403, resp.StatusCode)
	assert.Contains(t, string(body), `"code":"forbidden"`)
	resp, _ = app.Test(httptest.NewRequest("GET", "/x?t=a", nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
		ok     bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer abc", "abc", true},
		{"BEARER abc", "abc", true},
		{"", "", false},
		{"Bearer", "", false},
		{"Bearer ", "", false},
		{"Bearer   ", "", false},
		{"Bearer a b", "", false},
		{"Basic abc", "", false},
		{"Token abc", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			app := fiber.New()
			var got string
			var ok bool
			app.Get("/", func(c *fiber.Ctx) error {
				got, ok = bearerToken(c)
				return nil
			})
			req := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			_, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRequireAPITokenInvalidScopePanics(t *testing.T) {
	assert.Panics(t, func() { RequireAPIToken("admin") })
	assert.Panics(t, func() { RequireAPIToken("") })
	assert.NotPanics(t, func() { RequireAPIToken(models.ScopeRead) })
}

// --- helpers shared with the security tests ---

type apiFixture struct {
	app *fiber.App
	buf *bytes.Buffer
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	apitoken.Init(database.DB)
	buf := &bytes.Buffer{}
	base := logger.NewJSON(buf)
	app := fiber.New()
	app.Use(requestid.New())
	app.Use(logger.RequestLogger(base))
	return &apiFixture{app: app, buf: buf}
}

func mkAPIUser(t *testing.T, name string, verified bool, provider string) models.User {
	t.Helper()
	sfx, err := apitoken.Generate()
	require.NoError(t, err)
	sfx = strings.ToLower(sfx[5:15])
	u := models.User{Username: name + "-" + sfx, Email: name + sfx + "@example.com", EmailVerified: verified, Provider: provider}
	require.NoError(t, database.DB.Create(&u).Error)
	return u
}

func mkAPIToken(t *testing.T, u models.User, scope string) (string, *models.APIToken) {
	t.Helper()
	plain, tok, err := apitoken.Svc.Create(u.ID, "t", scope, nil, 30)
	require.NoError(t, err)
	return plain, tok
}

func doReq(t *testing.T, app *fiber.App, method, path, auth string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestRequireAPIToken_SetsLocals(t *testing.T) {
	f := newAPIFixture(t)
	u := mkAPIUser(t, "alice", true, "")
	u.IsAdmin = true
	require.NoError(t, database.DB.Save(&u).Error)
	plain, tok := mkAPIToken(t, u, models.ScopeRead)

	f.app.Get("/v1/_t", RequireAPIToken(models.ScopeRead), func(c *fiber.Ctx) error {
		got, ok := c.Locals("User").(models.User)
		require.True(t, ok)
		assert.Equal(t, u.ID, got.ID)
		assert.False(t, got.IsAdmin, "tokens never grant admin")
		id, ok := c.Locals("UserID").(uint)
		require.True(t, ok)
		assert.Equal(t, u.ID, id)
		at, ok := APITokenFromCtx(c)
		require.True(t, ok)
		assert.Equal(t, tok.ID, at.ID)
		return c.SendString("ok")
	})
	status, _ := doReq(t, f.app, "GET", "/v1/_t", "Bearer "+plain)
	assert.Equal(t, 200, status)

	var row models.APIToken
	require.NoError(t, database.DB.First(&row, tok.ID).Error)
	require.NotNil(t, row.LastUsedAt)
}

func TestHTTPRequestLogCarriesTokenPrefix(t *testing.T) {
	f := newAPIFixture(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, tok := mkAPIToken(t, u, models.ScopeRead)
	f.app.Get("/v1/_t", RequireAPIToken(models.ScopeRead), func(c *fiber.Ctx) error { return c.SendString("ok") })
	f.app.Get("/open", func(c *fiber.Ctx) error { return c.SendString("ok") })

	doReq(t, f.app, "GET", "/v1/_t", "Bearer "+plain)
	raw := f.buf.String()
	assert.False(t, strings.Contains(raw, plain), "full token must not be logged")
	line := logtest.Find(t, f.buf, "http.request")
	require.NotNil(t, line)
	assert.Equal(t, tok.Prefix, line["token_prefix"])
	assert.Len(t, line["token_prefix"], 12)

	f.buf.Reset()
	doReq(t, f.app, "GET", "/open", "")
	line = logtest.Find(t, f.buf, "http.request")
	require.NotNil(t, line)
	assert.NotContains(t, line, "token_prefix")
}

func TestAuditLogTokenRequest(t *testing.T) {
	f := newAPIFixture(t)
	u := mkAPIUser(t, "bob", true, "")
	plain, tok := mkAPIToken(t, u, models.ScopeWrite)
	f.app.Post("/v1/_a", RequireAPIToken(models.ScopeWrite), func(c *fiber.Ctx) error {
		AuditLog(c, "test.token_action", "thing", 7, "thing", map[string]any{"note": "x"})
		return c.SendString("ok")
	})
	f.app.Post("/session-a", func(c *fiber.Ctx) error {
		AuditLog(c, "test.session_action", "thing", 8, "thing", nil)
		return c.SendString("ok")
	})
	status, _ := doReq(t, f.app, "POST", "/v1/_a", "Bearer "+plain)
	require.Equal(t, 200, status)

	var row models.AuditLog
	require.NoError(t, database.DB.Where("action = ? AND actor_id = ?", "test.token_action", u.ID).First(&row).Error)
	var d map[string]any
	require.NoError(t, json.Unmarshal([]byte(row.Details), &d))
	assert.Equal(t, "token", d["via"])
	assert.Equal(t, float64(tok.ID), d["token_id"], "token_id must not be redacted")
	assert.Equal(t, "x", d["note"])
	assert.NotContains(t, row.Details, plain)
	assert.NotContains(t, row.Details, apitoken.HashToken(plain))
	assert.NotContains(t, row.Details, strconv.Quote("[REDACTED]"))

	doReq(t, f.app, "POST", "/session-a", "")
	var srow models.AuditLog
	require.NoError(t, database.DB.Where("action = ?", "test.session_action").Order("id DESC").First(&srow).Error)
	assert.NotContains(t, srow.Details, "via")
}
