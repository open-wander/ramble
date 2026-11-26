package handlers

import (
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
)

func cleanupTokenTestData(t *testing.T) {
	database.DB.Exec("DELETE FROM api_tokens")
	database.DB.Exec("DELETE FROM audit_logs WHERE action LIKE 'token.%'")
	cleanupOrgTestData(t)
}

// tokenApp builds an app where the request is authenticated as user.
func tokenApp(t *testing.T, user models.User) *fiber.App {
	t.Helper()
	apitoken.Init(database.DB)
	app := setupTestApp()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		_ = sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})
	app.Get("/settings/tokens", RequireAuth, GetTokenSettings)
	app.Post("/settings/tokens", RequireAuth, RequireVerifiedEmail, PostCreateToken)
	app.Delete("/settings/tokens/:id<int>", RequireAuth, RequireVerifiedEmail, DeleteToken)
	return app
}

func doTokenReq(t *testing.T, app *fiber.App, method, path string, form url.Values) (int, string) {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func tokenCount(t *testing.T, uid uint) int64 {
	var n int64
	require.NoError(t, database.DB.Model(&models.APIToken{}).Where("user_id = ?", uid).Count(&n).Error)
	return n
}

var newTokenRe = regexp.MustCompile(`rmbl_[0-9A-Za-z]{40}`)

func TestTokens_UnauthenticatedRedirects(t *testing.T) {
	app := setupTestApp()
	app.Get("/settings/tokens", RequireAuth, GetTokenSettings)
	req := httptest.NewRequest("GET", "/settings/tokens", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

func TestTokens_CreateShowsPlaintextOnce(t *testing.T) {
	defer cleanupTokenTestData(t)
	alice := createTestUser(t, "alice")
	app := tokenApp(t, alice)

	status, body := doTokenReq(t, app, "POST", "/settings/tokens",
		url.Values{"name": {"ci"}, "scope": {"write"}, "expires_in_days": {"30"}})
	require.Equal(t, 200, status)
	plain := newTokenRe.FindString(body)
	require.NotEmpty(t, plain, "response must show the full token")
	assert.Contains(t, body, "Copy this token now. It will not be shown again.")

	status, body = doTokenReq(t, app, "GET", "/settings/tokens", nil)
	require.Equal(t, 200, status)
	assert.NotContains(t, body, plain)
	assert.NotContains(t, body, "Copy this token now")
	assert.Contains(t, body, plain[:12])
	assert.Contains(t, body, "ci")
	assert.Contains(t, body, "Active")
	assert.Contains(t, body, "Never")

	hash := apitoken.HashToken(plain)
	assert.NotContains(t, body, hash)
}

func TestTokens_NoStoreHeader(t *testing.T) {
	defer cleanupTokenTestData(t)
	alice := createTestUser(t, "alice")
	app := tokenApp(t, alice)
	req := httptest.NewRequest("POST", "/settings/tokens", strings.NewReader("name=ci&scope=read"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
}

func TestTokens_ValidationErrors(t *testing.T) {
	long := strings.Repeat("a", 65)
	cases := []struct {
		name string
		form url.Values
		want string
	}{
		{"empty name", url.Values{"name": {""}, "scope": {"read"}}, "1-64 characters"},
		{"long name", url.Values{"name": {long}, "scope": {"read"}}, "1-64 characters"},
		{"bad scope", url.Values{"name": {"ci"}, "scope": {"admin"}}, "read or write"},
		{"zero expiry", url.Values{"name": {"ci"}, "scope": {"read"}, "expires_in_days": {"0"}}, "between 1 and 365"},
		{"negative expiry", url.Values{"name": {"ci"}, "scope": {"read"}, "expires_in_days": {"-1"}}, "between 1 and 365"},
		{"too long expiry", url.Values{"name": {"ci"}, "scope": {"read"}, "expires_in_days": {"366"}}, "between 1 and 365"},
		{"non-numeric expiry", url.Values{"name": {"ci"}, "scope": {"read"}, "expires_in_days": {"abc"}}, "between 1 and 365"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer cleanupTokenTestData(t)
			alice := createTestUser(t, "alice")
			app := tokenApp(t, alice)
			status, body := doTokenReq(t, app, "POST", "/settings/tokens", tc.form)
			assert.Equal(t, 422, status)
			assert.Contains(t, body, tc.want)
			assert.NotContains(t, body, "Copy this token now")
			assert.EqualValues(t, 0, tokenCount(t, alice.ID))
		})
	}
}

func TestTokens_NonMemberOrgForbidden(t *testing.T) {
	defer cleanupTokenTestData(t)
	alice := createTestUser(t, "alice")
	org := models.Organization{Name: "testorgx"}
	require.NoError(t, database.DB.Create(&org).Error)
	app := tokenApp(t, alice)

	for _, v := range []string{fmt.Sprint(org.ID), "garbage"} {
		status, body := doTokenReq(t, app, "POST", "/settings/tokens",
			url.Values{"name": {"ci"}, "scope": {"read"}, "organization_id": {v}})
		assert.Equal(t, 403, status, v)
		assert.NotContains(t, body, "Copy this token now")
		assert.EqualValues(t, 0, tokenCount(t, alice.ID))
	}
}

func TestTokens_MemberOrgAllowedAndListed(t *testing.T) {
	defer cleanupTokenTestData(t)
	alice := createTestUser(t, "alice")
	org := models.Organization{Name: "testorgm"}
	require.NoError(t, database.DB.Create(&org).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: alice.ID, OrganizationID: org.ID, Role: "member"}).Error)
	app := tokenApp(t, alice)

	status, body := doTokenReq(t, app, "GET", "/settings/tokens", nil)
	require.Equal(t, 200, status)
	assert.Contains(t, body, "testorgm")

	status, _ = doTokenReq(t, app, "POST", "/settings/tokens",
		url.Values{"name": {"ci"}, "scope": {"read"}, "organization_id": {fmt.Sprint(org.ID)}})
	require.Equal(t, 200, status)
	assert.EqualValues(t, 1, tokenCount(t, alice.ID))
}

func TestTokens_LimitReached(t *testing.T) {
	defer cleanupTokenTestData(t)
	alice := createTestUser(t, "alice")
	app := tokenApp(t, alice)
	for i := 0; i < apitoken.MaxActiveTokens; i++ {
		_, _, err := apitoken.Svc.Create(alice.ID, fmt.Sprintf("t%d", i), "read", nil, 30)
		require.NoError(t, err)
	}
	status, body := doTokenReq(t, app, "POST", "/settings/tokens",
		url.Values{"name": {"ci"}, "scope": {"read"}})
	assert.Equal(t, 422, status)
	assert.Contains(t, body, "You have reached the limit of 25 active tokens")
	assert.EqualValues(t, apitoken.MaxActiveTokens, tokenCount(t, alice.ID))
}

func TestTokens_Revoke(t *testing.T) {
	defer cleanupTokenTestData(t)
	alice := createTestUser(t, "alice")
	bob := createTestUser(t, "bob")
	_, mine, err := apitoken.Svc.Create(alice.ID, "mine", "read", nil, 30)
	require.NoError(t, err)
	_, theirs, err := apitoken.Svc.Create(bob.ID, "theirs", "read", nil, 30)
	require.NoError(t, err)
	app := tokenApp(t, alice)

	status, body := doTokenReq(t, app, "DELETE", fmt.Sprintf("/settings/tokens/%d", theirs.ID), nil)
	assert.Equal(t, 404, status)
	assert.NotContains(t, body, "<tr")
	var other models.APIToken
	require.NoError(t, database.DB.First(&other, theirs.ID).Error)
	assert.Nil(t, other.RevokedAt)

	status, body = doTokenReq(t, app, "DELETE", fmt.Sprintf("/settings/tokens/%d", mine.ID), nil)
	assert.Equal(t, 200, status)
	assert.Contains(t, body, "Revoked")
	var own models.APIToken
	require.NoError(t, database.DB.First(&own, mine.ID).Error)
	assert.NotNil(t, own.RevokedAt)

	_, body = doTokenReq(t, app, "GET", "/settings/tokens", nil)
	assert.Contains(t, body, "Revoked")
	assert.NotContains(t, body, "hx-delete")
}

func TestTokens_AuditEvents(t *testing.T) {
	defer cleanupTokenTestData(t)
	alice := createTestUser(t, "alice")
	app := tokenApp(t, alice)

	_, body := doTokenReq(t, app, "POST", "/settings/tokens",
		url.Values{"name": {"ci"}, "scope": {"write"}, "expires_in_days": {"30"}})
	plain := newTokenRe.FindString(body)
	require.NotEmpty(t, plain)
	var tok models.APIToken
	require.NoError(t, database.DB.Where("user_id = ?", alice.ID).First(&tok).Error)
	status, _ := doTokenReq(t, app, "DELETE", fmt.Sprintf("/settings/tokens/%d", tok.ID), nil)
	require.Equal(t, 200, status)

	for _, action := range []string{"token.create", "token.revoke"} {
		var ev models.AuditLog
		require.NoError(t, database.DB.Where("action = ?", action).First(&ev).Error, action)
		assert.Equal(t, "token", ev.TargetType)
		assert.Equal(t, tok.ID, ev.TargetID)
		assert.Contains(t, ev.TargetName, "ci")
		assert.Contains(t, ev.TargetName, tok.Prefix)
		assert.NotContains(t, ev.TargetName, plain)
		assert.NotContains(t, ev.Details, plain)
	}

	// A repeated revoke succeeds but does not write a second audit event.
	status, _ = doTokenReq(t, app, "DELETE", fmt.Sprintf("/settings/tokens/%d", tok.ID), nil)
	require.Equal(t, 200, status)
	var revokes int64
	database.DB.Model(&models.AuditLog{}).Where("action = ? AND target_id = ?", "token.revoke", tok.ID).Count(&revokes)
	assert.EqualValues(t, 1, revokes)
}

func TestTokens_RevokeErrorIsNotSwapped(t *testing.T) {
	defer cleanupTokenTestData(t)
	alice := createTestUser(t, "alice")
	app := tokenApp(t, alice)
	resp, err := app.Test(httptest.NewRequest("DELETE", "/settings/tokens/999999", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "none", resp.Header.Get("HX-Reswap"))
}
