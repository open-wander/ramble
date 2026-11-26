//go:build security

package handlers

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
)

func decodeUserBody(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &m))
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	require.Equal(t, []string{"token_expires_at", "token_org", "token_scope", "username"}, keys)
	return m
}

func TestGetCurrentUserAPI_ReadToken(t *testing.T) {
	f := newAPIFixture(t)
	f.app.Get("/v1/user", RequireAPIToken(models.ScopeRead), GetCurrentUserAPI)
	u := mkAPIUser(t, "alice", true, "")
	plain, tok := mkAPIToken(t, u, models.ScopeRead)

	status, body := doReq(t, f.app, "GET", "/v1/user", "Bearer "+plain)
	require.Equal(t, 200, status)
	m := decodeUserBody(t, body)
	assert.Equal(t, u.Username, m["username"])
	assert.Equal(t, "read", m["token_scope"])
	assert.Nil(t, m["token_org"])
	exp, err := time.Parse(time.RFC3339, m["token_expires_at"].(string))
	require.NoError(t, err)
	assert.True(t, tok.ExpiresAt.Truncate(time.Second).Equal(exp.Truncate(time.Second)))

	assert.NotContains(t, body, plain)
	assert.NotContains(t, body, tok.Hash)
	assert.NotContains(t, body, u.Email)
}

func TestGetCurrentUserAPI_WriteToken(t *testing.T) {
	f := newAPIFixture(t)
	f.app.Get("/v1/user", RequireAPIToken(models.ScopeRead), GetCurrentUserAPI)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)

	status, body := doReq(t, f.app, "GET", "/v1/user", "Bearer "+plain)
	require.Equal(t, 200, status)
	assert.Equal(t, "write", decodeUserBody(t, body)["token_scope"])
}

func TestGetCurrentUserAPI_OrgToken(t *testing.T) {
	f := newAPIFixture(t)
	f.app.Get("/v1/user", RequireAPIToken(models.ScopeRead), GetCurrentUserAPI)
	u := mkAPIUser(t, "alice", true, "")
	org := models.Organization{Name: "acme-" + u.Username}
	other := models.Organization{Name: "other-" + u.Username}
	require.NoError(t, database.DB.Create(&org).Error)
	require.NoError(t, database.DB.Create(&other).Error)
	for _, o := range []models.Organization{org, other} {
		require.NoError(t, database.DB.Create(&models.Membership{UserID: u.ID, OrganizationID: o.ID, Role: "owner"}).Error)
	}
	plain, _, err := apitoken.Svc.Create(u.ID, "org", models.ScopeRead, &org.ID, 30)
	require.NoError(t, err)

	status, body := doReq(t, f.app, "GET", "/v1/user", "Bearer "+plain)
	require.Equal(t, 200, status)
	assert.Equal(t, org.Name, decodeUserBody(t, body)["token_org"])
}

func TestGetCurrentUserAPI_Unauthorized(t *testing.T) {
	f := newAPIFixture(t)
	f.app.Get("/v1/user", RequireAPIToken(models.ScopeRead), GetCurrentUserAPI)
	u := mkAPIUser(t, "alice", true, "")

	revoked, rtok := mkAPIToken(t, u, models.ScopeRead)
	_, err := apitoken.Svc.Revoke(u.ID, rtok.ID)
	require.NoError(t, err)
	expired, etok := mkAPIToken(t, u, models.ScopeRead)
	require.NoError(t, database.DB.Model(&models.APIToken{}).Where("id = ?", etok.ID).
		Update("expires_at", time.Now().Add(-time.Hour)).Error)

	for name, header := range map[string]string{
		"missing": "", "revoked": "Bearer " + revoked, "expired": "Bearer " + expired,
	} {
		t.Run(name, func(t *testing.T) {
			status, body := doReq(t, f.app, "GET", "/v1/user", header)
			assert.Equal(t, 401, status)
			assert.Equal(t, wantUnauthorizedBody, body)
		})
	}
}

// userApp mounts GetCurrentUserAPI behind a stand-in for RequireAPIToken that
// sets the given locals, so edge cases need no database.
func userApp(tok *models.APIToken, u *models.User) *fiber.App {
	app := fiber.New()
	app.Get("/v1/user", func(c *fiber.Ctx) error {
		if tok != nil {
			c.Locals(LocalAPIToken, tok)
		}
		if u != nil {
			c.Locals("User", *u)
		}
		return c.Next()
	}, GetCurrentUserAPI)
	return app
}

func getUser(t *testing.T, app *fiber.App) (int, string) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", "/v1/user", nil))
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

const internalErrorBody = `{"error":{"code":"internal_error","message":"internal error"}}`

// An org-restricted token must never be reported as unrestricted
// (token_org null) when its membership cannot be found.
func TestGetCurrentUserAPI_OrgTokenWithoutMembershipIs500(t *testing.T) {
	org := uint(42)
	tok := &models.APIToken{Scope: models.ScopeRead, OrganizationID: &org, ExpiresAt: time.Now().Add(time.Hour)}
	status, body := getUser(t, userApp(tok, &models.User{Username: "alice"}))
	assert.Equal(t, 500, status)
	assert.JSONEq(t, internalErrorBody, body)
}

func TestGetCurrentUserAPI_ExpiryIsUTC(t *testing.T) {
	exp := time.Date(2027, 3, 1, 12, 0, 0, 0, time.FixedZone("plus-two", 2*3600))
	tok := &models.APIToken{Scope: models.ScopeRead, ExpiresAt: exp}
	status, body := getUser(t, userApp(tok, &models.User{Username: "alice"}))
	require.Equal(t, 200, status)
	m := decodeUserBody(t, body)
	assert.Equal(t, "2027-03-01T10:00:00Z", m["token_expires_at"])
}

func TestGetCurrentUserAPI_WithoutTokenLocalsIs500(t *testing.T) {
	status, body := getUser(t, userApp(nil, nil))
	assert.Equal(t, 500, status)
	assert.JSONEq(t, internalErrorBody, body)
}
