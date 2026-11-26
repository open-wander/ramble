//go:build security

package handlers

import (
	"encoding/json"
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
