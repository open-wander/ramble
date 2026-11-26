//go:build security

package handlers

import (
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
)

const wantUnauthorizedBody = `{"error":{"code":"unauthorized","message":"invalid or expired token"}}`

func TestRequireAPIToken_Rejections_Identical401(t *testing.T) {
	f := newAPIFixture(t)
	u := mkAPIUser(t, "alice", true, "")

	revoked, rtok := mkAPIToken(t, u, models.ScopeRead)
	_, err := apitoken.Svc.Revoke(u.ID, rtok.ID)
	require.NoError(t, err)

	expired, etok := mkAPIToken(t, u, models.ScopeRead)
	require.NoError(t, database.DB.Model(&models.APIToken{}).Where("id = ?", etok.ID).
		Update("expires_at", time.Now().Add(-time.Hour)).Error)

	valid, _ := mkAPIToken(t, u, models.ScopeRead)
	unknown, err := apitoken.Generate()
	require.NoError(t, err)

	reached := false
	f.app.Get("/v1/_t", RequireAPIToken(models.ScopeRead), func(c *fiber.Ctx) error {
		reached = true
		return c.SendString("ok")
	})

	tests := []struct{ name, header string }{
		{"missing", ""},
		{"basic", "Basic x"},
		{"bearer without token", "Bearer"},
		{"bearer spaces", "Bearer    "},
		{"token scheme", "Token " + valid},
		{"wrong prefix", "Bearer abc_" + valid[5:]},
		{"token with trailing junk", "Bearer " + valid + " extra"},
		{"unknown", "Bearer " + unknown},
		{"revoked", "Bearer " + revoked},
		{"expired", "Bearer " + expired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached = false
			status, body := doReq(t, f.app, "GET", "/v1/_t", tt.header)
			assert.Equal(t, 401, status)
			assert.Equal(t, wantUnauthorizedBody, body)
			assert.False(t, reached)
		})
	}

	// Sanity: the same fixture accepts the valid token and a lowercase scheme.
	status, _ := doReq(t, f.app, "GET", "/v1/_t", "Bearer "+valid)
	assert.Equal(t, 200, status)
	status, _ = doReq(t, f.app, "GET", "/v1/_t", "bearer "+valid)
	assert.Equal(t, 200, status)
}

func TestRequireAPIToken_Scope(t *testing.T) {
	f := newAPIFixture(t)
	u := mkAPIUser(t, "alice", true, "")
	readTok, _ := mkAPIToken(t, u, models.ScopeRead)
	writeTok, _ := mkAPIToken(t, u, models.ScopeWrite)
	f.app.Get("/v1/_r", RequireAPIToken(models.ScopeRead), func(c *fiber.Ctx) error { return c.SendString("ok") })
	f.app.Post("/v1/_w", RequireAPIToken(models.ScopeWrite), func(c *fiber.Ctx) error { return c.SendString("ok") })

	status, body := doReq(t, f.app, "POST", "/v1/_w", "Bearer "+readTok)
	assert.Equal(t, 403, status)
	assert.Contains(t, body, `"code":"insufficient_scope"`)

	status, _ = doReq(t, f.app, "POST", "/v1/_w", "Bearer "+writeTok)
	assert.Equal(t, 200, status)
	status, _ = doReq(t, f.app, "GET", "/v1/_r", "Bearer "+writeTok)
	assert.Equal(t, 200, status)
	status, _ = doReq(t, f.app, "GET", "/v1/_r", "Bearer "+readTok)
	assert.Equal(t, 200, status)
}

func TestRequireAPIToken_EmailVerification(t *testing.T) {
	f := newAPIFixture(t)
	local := mkAPIUser(t, "alice", false, "")
	oauth := mkAPIUser(t, "bob", false, "github")
	localWrite, _ := mkAPIToken(t, local, models.ScopeWrite)
	localRead, _ := mkAPIToken(t, local, models.ScopeRead)
	oauthWrite, _ := mkAPIToken(t, oauth, models.ScopeWrite)
	f.app.Get("/v1/_r", RequireAPIToken(models.ScopeRead), func(c *fiber.Ctx) error { return c.SendString("ok") })
	f.app.Post("/v1/_w", RequireAPIToken(models.ScopeWrite), func(c *fiber.Ctx) error { return c.SendString("ok") })

	status, body := doReq(t, f.app, "POST", "/v1/_w", "Bearer "+localWrite)
	assert.Equal(t, 403, status)
	assert.Contains(t, body, `"code":"email_unverified"`)

	status, _ = doReq(t, f.app, "GET", "/v1/_r", "Bearer "+localWrite)
	assert.Equal(t, 200, status)
	status, _ = doReq(t, f.app, "GET", "/v1/_r", "Bearer "+localRead)
	assert.Equal(t, 200, status)

	status, _ = doReq(t, f.app, "POST", "/v1/_w", "Bearer "+oauthWrite)
	assert.Equal(t, 200, status)

	// Scope is checked before email: a read token of an unverified user on a
	// write route reports the scope problem.
	status, body = doReq(t, f.app, "POST", "/v1/_w", "Bearer "+localRead)
	assert.Equal(t, 403, status)
	assert.Contains(t, body, `"code":"insufficient_scope"`)
}

func TestRequireAPIToken_OrgRestrictedTokenThroughRoute(t *testing.T) {
	f := newAPIFixture(t)
	u := mkAPIUser(t, "alice", true, "")
	orgA := models.Organization{Name: "org-a-" + u.Username}
	orgB := models.Organization{Name: "org-b-" + u.Username}
	require.NoError(t, database.DB.Create(&orgA).Error)
	require.NoError(t, database.DB.Create(&orgB).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: u.ID, OrganizationID: orgA.ID, Role: "owner"}).Error)
	plain, _, err := apitoken.Svc.Create(u.ID, "org", models.ScopeWrite, &orgA.ID, 30)
	require.NoError(t, err)

	f.app.Post("/v1/_o/:which", RequireAPIToken(models.ScopeWrite), func(c *fiber.Ctx) error {
		var target *uint
		switch c.Params("which") {
		case "a":
			target = &orgA.ID
		case "b":
			target = &orgB.ID
		}
		if !TokenAllowsOrg(c, target) {
			return APIOrgForbidden(c)
		}
		return c.SendString("ok")
	})
	for path, want := range map[string]int{"/v1/_o/a": 200, "/v1/_o/b": 403, "/v1/_o/personal": 403} {
		status, body := doReq(t, f.app, "POST", path, "Bearer "+plain)
		assert.Equal(t, want, status, path)
		if want == 403 {
			assert.Contains(t, body, `"code":"forbidden"`)
		}
	}
}

func TestRequireAPIToken_OrgRestrictedTokenNarrowsMemberships(t *testing.T) {
	f := newAPIFixture(t)
	u := mkAPIUser(t, "alice", true, "")
	orgA := models.Organization{Name: "mem-a-" + u.Username}
	orgB := models.Organization{Name: "mem-b-" + u.Username}
	require.NoError(t, database.DB.Create(&orgA).Error)
	require.NoError(t, database.DB.Create(&orgB).Error)
	for _, o := range []models.Organization{orgA, orgB} {
		require.NoError(t, database.DB.Create(&models.Membership{UserID: u.ID, OrganizationID: o.ID, Role: "owner"}).Error)
	}
	restricted, _, err := apitoken.Svc.Create(u.ID, "a-only", models.ScopeRead, &orgA.ID, 30)
	require.NoError(t, err)
	unrestricted, _, err := apitoken.Svc.Create(u.ID, "all", models.ScopeRead, nil, 30)
	require.NoError(t, err)

	f.app.Get("/v1/_mem", RequireAPIToken(models.ScopeRead), func(c *fiber.Ctx) error {
		user := c.Locals("User").(models.User)
		ids := make([]string, 0, len(user.Memberships))
		for _, m := range user.Memberships {
			ids = append(ids, strconv.FormatUint(uint64(m.OrganizationID), 10))
		}
		sort.Strings(ids)
		return c.SendString(strings.Join(ids, ","))
	})

	a := strconv.FormatUint(uint64(orgA.ID), 10)
	b := strconv.FormatUint(uint64(orgB.ID), 10)
	both := []string{a, b}
	sort.Strings(both)

	status, body := doReq(t, f.app, "GET", "/v1/_mem", "Bearer "+restricted)
	assert.Equal(t, 200, status)
	assert.Equal(t, a, body, "org-restricted token must only carry its own org's membership")

	status, body = doReq(t, f.app, "GET", "/v1/_mem", "Bearer "+unrestricted)
	assert.Equal(t, 200, status)
	assert.Equal(t, strings.Join(both, ","), body)
}
