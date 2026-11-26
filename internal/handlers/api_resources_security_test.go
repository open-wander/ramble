//go:build security

package handlers

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
)

func TestCreateResourceAPI_OrgRestrictedToken(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	orgA := mkOrgWithMember(t, u, "member")
	orgB := mkOrgWithMember(t, u, "member")
	plain, _, err := apitoken.Svc.Create(u.ID, "t", models.ScopeWrite, &orgA.ID, 30)
	require.NoError(t, err)

	tests := []struct {
		name       string
		ns         string
		wantStatus int
	}{
		{"other org", orgB.Name, 403},
		{"personal namespace", u.Username, 403},
		{"own org", orgA.Name, 201},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := uniqueName(t, "restr")
			status, body := postResource(t, app, plain, validBody(t, tt.ns, name, nil))
			require.Equal(t, tt.wantStatus, status, body)
			if tt.wantStatus == 403 {
				assert.Equal(t, "forbidden", apiErrCode(t, body))
				assert.EqualValues(t, 0, countResources(t, name))
			} else {
				assert.EqualValues(t, 1, countResources(t, name))
			}
		})
	}
}

func TestCreateResourceAPI_TokenRestrictionBeforeValidation(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	orgA := mkOrgWithMember(t, u, "member")
	plain, _, err := apitoken.Svc.Create(u.ID, "t", models.ScopeWrite, &orgA.ID, 30)
	require.NoError(t, err)
	// Invalid name in the personal namespace: the restriction answers first.
	status, body := postResource(t, app, plain, validBody(t, u.Username, "-x", nil))
	assert.Equal(t, 403, status, body)
	assert.Equal(t, "forbidden", apiErrCode(t, body))
}

func TestCreateResourceAPI_NonMemberOfOrgForbidden(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	other := mkAPIUser(t, "bob", true, "")
	org := mkOrgWithMember(t, other, "owner")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	name := uniqueName(t, "nm")
	status, body := postResource(t, app, plain, validBody(t, org.Name, name, nil))
	assert.Equal(t, 403, status, body)
	assert.Equal(t, "forbidden", apiErrCode(t, body))
	assert.EqualValues(t, 0, countResources(t, name))
}

func TestCreateResourceAPI_AuditAndLogsNeverHoldSecrets(t *testing.T) {
	app, buf := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, tok := mkAPIToken(t, u, models.ScopeWrite)
	name := uniqueName(t, "aud")

	status, body := postResource(t, app, plain, validBody(t, u.Username, name, nil))
	require.Equal(t, 201, status, body)
	var resp CreateResourceResponse
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	require.NotEmpty(t, resp.WebhookSecret)

	var row models.AuditLog
	require.NoError(t, database.DB.Where("action = ? AND target_id = ?", "resource.create", resp.ID).First(&row).Error)
	assert.Equal(t, u.ID, row.ActorID)
	var d map[string]any
	require.NoError(t, json.Unmarshal([]byte(row.Details), &d))
	assert.Equal(t, "token", d["via"])
	assert.Equal(t, float64(tok.ID), d["token_id"])
	for _, secret := range []string{resp.WebhookSecret, plain, apitoken.HashToken(plain)} {
		assert.NotContains(t, row.Details, secret)
		assert.NotContains(t, buf.String(), secret)
	}
}

func TestCreateResourceAPI_ErrorBodiesNeverEchoInput(t *testing.T) {
	app, buf := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	const marker = "zz-secret-marker-zz"
	status, body := postResource(t, app, plain, `{"name":"`+marker+`","unknown":"`+marker+`"}`)
	assert.Equal(t, 400, status)
	assert.NotContains(t, body, marker)
	assert.NotContains(t, buf.String(), marker)
}

func TestGetResourceAPI_OrgRestrictedToken(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	orgA := mkOrgWithMember(t, u, "member")
	orgB := mkOrgWithMember(t, u, "member")
	wr, _ := mkAPIToken(t, u, models.ScopeWrite)
	restricted, _, err := apitoken.Svc.Create(u.ID, "t", models.ScopeRead, &orgA.ID, 30)
	require.NoError(t, err)

	type res struct {
		ns   string
		name string
		id   uint
	}
	mk := func(ns string) res {
		name := uniqueName(t, "rr")
		status, body := postResource(t, app, wr, validBody(t, ns, name, nil))
		require.Equal(t, 201, status, body)
		var r ResourceResponse
		require.NoError(t, json.Unmarshal([]byte(body), &r))
		return res{ns, name, r.ID}
	}
	own, other, personal := mk(orgA.Name), mk(orgB.Name), mk(u.Username)

	tests := []struct {
		name       string
		r          res
		wantStatus int
	}{
		{"own org", own, 200},
		{"other org", other, 403},
		{"personal", personal, 403},
	}
	for _, tt := range tests {
		for _, path := range []string{
			fmt.Sprintf("/v1/resources/%d", tt.r.id),
			"/v1/namespaces/" + tt.r.ns + "/resources/" + tt.r.name,
		} {
			t.Run(tt.name+" "+path, func(t *testing.T) {
				status, body := getJSON(t, app, restricted, path)
				require.Equal(t, tt.wantStatus, status, body)
				if tt.wantStatus == 403 {
					assert.Equal(t, "forbidden", apiErrCode(t, body))
					assert.NotContains(t, body, tt.r.name)
				}
			})
		}
	}
}

func TestWriteResourceAPI_OrgRestrictedToken(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	orgA := mkOrgWithMember(t, u, "owner")
	orgB := mkOrgWithMember(t, u, "owner")
	wr, _ := mkAPIToken(t, u, models.ScopeWrite)
	restricted, _, err := apitoken.Svc.Create(u.ID, "t", models.ScopeWrite, &orgA.ID, 30)
	require.NoError(t, err)

	tests := []struct {
		name   string
		ns     string
		status int
	}{
		{"other org", orgB.Name, 403},
		{"personal", u.Username, 403},
		{"own org", orgA.Name, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := createViaAPI(t, app, wr, tt.ns, nil)
			status, body := sendJSON(t, app, "PATCH", restricted, resPath(r.ID), `{"description":"changed"}`)
			require.Equal(t, tt.status, status, body)
			if tt.status == 403 {
				assert.Equal(t, "forbidden", apiErrCode(t, body))
				assert.NotContains(t, body, r.Name)
				assert.Equal(t, "d", loadRow(t, r.ID).Description)
			}
			status, body = sendJSON(t, app, "DELETE", restricted, resPath(r.ID), "")
			if tt.status == 403 {
				require.Equal(t, 403, status, body)
				assert.False(t, loadRow(t, r.ID).DeletedAt.Valid)
			} else {
				require.Equal(t, 204, status, body)
			}
		})
	}
}
