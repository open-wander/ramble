//go:build security

package handlers

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

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
