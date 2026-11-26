//go:build security

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
)

func TestOrgRoutes_Middleware(t *testing.T) {
	apitoken.Init(database.DB)
	_, readTok, _ := mkUserTok(t, "alice", models.ScopeRead, false)
	_, writeTok, _ := mkUserTok(t, "bob", models.ScopeWrite, false)
	unverified, uTok, _ := mkUserTok(t, "carol", models.ScopeWrite, false)
	require.NoError(t, database.DB.Model(&models.User{}).Where("id = ?", unverified.ID).Update("email_verified", false).Error)
	app := stackApp(t, true)

	const name = "route-mw-org"
	tests := []struct {
		name, method, path, token, body string
		status                          int
		code                            string
	}{
		{"post no token", "POST", "/v1/orgs", "", `{}`, 401, "unauthorized"},
		{"patch no token", "PATCH", "/v1/orgs/" + name, "", `{}`, 401, "unauthorized"},
		{"get no token", "GET", "/v1/orgs/" + name, "", "", 401, "unauthorized"},
		{"post read token", "POST", "/v1/orgs", readTok, `{"name":"` + name + `"}`, 403, "insufficient_scope"},
		{"patch read token", "PATCH", "/v1/orgs/" + name, readTok, `{}`, 403, "insufficient_scope"},
		{"post unverified", "POST", "/v1/orgs", uTok, `{"name":"` + name + `"}`, 403, "email_unverified"},
		{"get unknown org", "GET", "/v1/orgs/" + name, readTok, "", 404, "not_found"},
		{"post invalid name", "POST", "/v1/orgs", writeTok, `{"name":"-x"}`, 422, "invalid_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := sendBody(t, app, tt.method, tt.path, tt.token, tt.body)
			require.Equal(t, tt.status, status, body)
			assert.Contains(t, body, `"code":"`+tt.code+`"`)
		})
	}
	var n int64
	require.NoError(t, database.DB.Model(&models.Organization{}).Where("name = ?", name).Count(&n).Error)
	assert.Zero(t, n)

	status, body := sendBody(t, app, "POST", "/v1/orgs", writeTok, `{"name":"`+name+`"}`)
	require.Equal(t, 201, status, body)
	status, body = sendBody(t, app, "GET", "/v1/orgs/"+name, readTok, "")
	assert.Equal(t, 200, status, body)
}

func TestOrgRoutes_SharedWriteLimiter(t *testing.T) {
	apitoken.Init(database.DB)
	_, plain, _ := mkUserTok(t, "dave", models.ScopeWrite, false)
	app := stackApp(t, true)

	const limit = 120
	for i := range limit {
		if i%2 == 0 {
			status, body := sendBody(t, app, "POST", "/v1/orgs", plain, `{`)
			require.Equal(t, 400, status, body)
		} else {
			status, body := sendBody(t, app, "PATCH", "/v1/orgs/route-limiter-none", plain, `{}`)
			require.Equal(t, 404, status, body)
		}
	}
	status, body := sendBody(t, app, "POST", "/v1/resources", plain, `{`)
	assert.Equal(t, 429, status, body)
}
