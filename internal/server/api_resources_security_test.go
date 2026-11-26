//go:build security

package server

import (
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
	"rmbl/internal/services/resource"
)

func postCreate(t *testing.T, app *fiber.App, token, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/resources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func countByName(t *testing.T, name string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, database.DB.Model(&models.NomadResource{}).Where("name = ?", name).Count(&n).Error)
	return n
}

func TestCreateResourceRoute_Middleware(t *testing.T) {
	u, readTok, _ := mkUserTok(t, "alice", models.ScopeRead, false)
	_, writeTok, _ := mkUserTok(t, "bob", models.ScopeWrite, false)
	unverified, uTok, _ := mkUserTok(t, "carol", models.ScopeWrite, false)
	require.NoError(t, database.DB.Model(&models.User{}).Where("id = ?", unverified.ID).Update("email_verified", false).Error)
	app := stackApp(t, true)

	const name = "route-mw-resource"
	body := `{"name":"` + name + `","type":"pack","namespace":"` + u.Username + `","repository_url":"https://github.com/x/y","version":"v1.0.0","license":"MIT"}`

	tests := []struct {
		name, token string
		status      int
		code        string
	}{
		{"read token", readTok, 403, "insufficient_scope"},
		{"no token", "", 401, "unauthorized"},
		{"unknown token", "rmbl_" + strings.Repeat("a", 40), 401, "unauthorized"},
		{"unverified local account", uTok, 403, "email_unverified"},
		// bob's token with alice's namespace: valid token, foreign namespace.
		{"valid write token foreign namespace", writeTok, 404, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, resp := postCreate(t, app, tt.token, body)
			assert.Equal(t, tt.status, status, resp)
			assert.Contains(t, resp, `"code":"`+tt.code+`"`)
		})
	}
	assert.EqualValues(t, 0, countByName(t, name))
}

// TestCreateResourceRoute_SharedWriteLimiter proves the route sits behind the
// shared write limiter: every request counts, including rejected ones.
func TestCreateResourceRoute_SharedWriteLimiter(t *testing.T) {
	apitoken.Init(database.DB)
	_, plain, _ := mkUserTok(t, "alice", models.ScopeWrite, false)
	app := stackApp(t, true)

	const limit = 120
	for i := range limit {
		status, body := postCreate(t, app, plain, `{`)
		require.Equal(t, 400, status, "request %d: %s", i+1, body)
	}
	status, body := postCreate(t, app, plain, `{`)
	assert.Equal(t, 429, status)
	assert.Contains(t, body, `"code":"rate_limited"`)

	// A different token has its own budget.
	_, other, _ := mkUserTok(t, "bob", models.ScopeWrite, false)
	status, _ = postCreate(t, app, other, `{`)
	assert.Equal(t, 400, status)
}

func sendBody(t *testing.T, app *fiber.App, method, path, token, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestWriteResourceRoutes_Middleware(t *testing.T) {
	apitoken.Init(database.DB)
	resource.Init(database.DB)
	u, readTok, _ := mkUserTok(t, "alice", models.ScopeRead, false)
	_, writeTok, _ := mkUserTok(t, "bob", models.ScopeWrite, false)
	app := stackApp(t, true)
	r := models.NomadResource{Name: "route-write-res", Type: models.ResourceType("pack"), UserID: u.ID, Description: "keep"}
	require.NoError(t, database.DB.Create(&r).Error)
	path := "/v1/resources/" + strconv.FormatUint(uint64(r.ID), 10)

	tests := []struct {
		name, method, token string
		status              int
		code                string
	}{
		{"patch read token", "PATCH", readTok, 403, "insufficient_scope"},
		{"patch no token", "PATCH", "", 401, "unauthorized"},
		{"patch foreign user", "PATCH", writeTok, 403, "forbidden"},
		{"delete read token", "DELETE", readTok, 403, "insufficient_scope"},
		{"delete no token", "DELETE", "", 401, "unauthorized"},
		{"delete foreign user", "DELETE", writeTok, 403, "forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, resp := sendBody(t, app, tt.method, path, tt.token, `{"description":"changed"}`)
			assert.Equal(t, tt.status, status, resp)
			assert.Contains(t, resp, `"code":"`+tt.code+`"`)
		})
	}
	var got models.NomadResource
	require.NoError(t, database.DB.First(&got, r.ID).Error)
	assert.Equal(t, "keep", got.Description)
}

// TestWriteResourceRoutes_SharedWriteLimiter proves POST, PATCH and DELETE
// count against one per-token limiter.
func TestWriteResourceRoutes_SharedWriteLimiter(t *testing.T) {
	apitoken.Init(database.DB)
	_, plain, _ := mkUserTok(t, "alice", models.ScopeWrite, false)
	app := stackApp(t, true)

	const limit = 120
	for i := range limit {
		var status int
		var body string
		switch i % 3 {
		case 0:
			status, body = sendBody(t, app, "POST", "/v1/resources", plain, `{`)
			require.Equal(t, 400, status, body)
		case 1:
			status, body = sendBody(t, app, "PATCH", "/v1/resources/999999", plain, `{}`)
			require.Equal(t, 404, status, body)
		default:
			status, body = sendBody(t, app, "DELETE", "/v1/resources/999999", plain, `{}`)
			require.Equal(t, 404, status, body)
		}
	}
	status, body := sendBody(t, app, "PATCH", "/v1/resources/999999", plain, `{}`)
	assert.Equal(t, 429, status, body)
	assert.Contains(t, body, `"code":"rate_limited"`)
}

func TestVersionAndWebhookRoutes_Middleware(t *testing.T) {
	apitoken.Init(database.DB)
	resource.Init(database.DB)
	u, readTok, _ := mkUserTok(t, "alice", models.ScopeRead, false)
	_, writeTok, _ := mkUserTok(t, "bob", models.ScopeWrite, false)
	app := stackApp(t, true)
	r := models.NomadResource{Name: "route-version-res", Type: models.ResourceType("pack"), UserID: u.ID, WebhookSecret: "keep-secret"}
	require.NoError(t, database.DB.Create(&r).Error)
	require.NoError(t, database.DB.Create(&models.ResourceVersion{ResourceID: r.ID, Version: "v1.0.0", FetchStatus: models.FetchStatusFailed}).Error)
	base := "/v1/resources/" + strconv.FormatUint(uint64(r.ID), 10)

	tests := []struct {
		name, method, path, token, body string
		status                          int
		code                            string
	}{
		{"list no token", "GET", base + "/versions", "", "", 401, "unauthorized"},
		{"create no token", "POST", base + "/versions", "", `{"version":"v2"}`, 401, "unauthorized"},
		{"create read token", "POST", base + "/versions", readTok, `{"version":"v2"}`, 403, "insufficient_scope"},
		{"create foreign user", "POST", base + "/versions", writeTok, `{"version":"v2"}`, 403, "forbidden"},
		{"retry no token", "POST", base + "/versions/v1.0.0/retry", "", "", 401, "unauthorized"},
		{"retry read token", "POST", base + "/versions/v1.0.0/retry", readTok, "", 403, "insufficient_scope"},
		{"retry foreign user", "POST", base + "/versions/v1.0.0/retry", writeTok, "", 403, "forbidden"},
		{"secret no token", "POST", base + "/webhook-secret", "", "", 401, "unauthorized"},
		{"secret read token", "POST", base + "/webhook-secret", readTok, "", 403, "insufficient_scope"},
		{"secret foreign user", "POST", base + "/webhook-secret", writeTok, "", 403, "forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, resp := sendBody(t, app, tt.method, tt.path, tt.token, tt.body)
			assert.Equal(t, tt.status, status, resp)
			assert.Contains(t, resp, `"code":"`+tt.code+`"`)
		})
	}
	var got models.NomadResource
	require.NoError(t, database.DB.First(&got, r.ID).Error)
	assert.Equal(t, "keep-secret", got.WebhookSecret)
	var n int64
	require.NoError(t, database.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", r.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n)
	var v models.ResourceVersion
	require.NoError(t, database.DB.Where("resource_id = ?", r.ID).First(&v).Error)
	assert.Equal(t, models.FetchStatusFailed, v.FetchStatus)
}
