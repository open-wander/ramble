//go:build security

package server

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
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
