//go:build security

package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
	orgsvc "rmbl/internal/services/org"

	"rmbl/internal/database"
)

func TestCreateOrgAPI_OrgRestrictedTokenCannotCreate(t *testing.T) {
	app := orgAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	orgA := mkOrgWithMember(t, u, "owner")
	plain, _, err := apitoken.Svc.Create(u.ID, "t", models.ScopeWrite, &orgA.ID, 30)
	require.NoError(t, err)

	for _, body := range []string{`{"name":"` + uniqueName(t, "rc") + `"}`, `{"name":"-invalid"}`, `{`} {
		status, resp := sendJSON(t, app, "POST", plain, "/v1/orgs", body)
		assert.Equal(t, 403, status, resp)
		assert.Equal(t, "forbidden", apiErrCode(t, resp))
	}
	assert.Zero(t, countOrgsNamed(t, "-invalid"))
}

func TestOrgAPI_OrgRestrictedTokenOnlyOwnOrg(t *testing.T) {
	app := orgAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	orgA := mkOrgWithMember(t, u, "owner")
	orgB := mkOrgWithMember(t, u, "owner")
	w, _, err := apitoken.Svc.Create(u.ID, "w", models.ScopeWrite, &orgA.ID, 30)
	require.NoError(t, err)
	r, _, err := apitoken.Svc.Create(u.ID, "r", models.ScopeRead, &orgA.ID, 30)
	require.NoError(t, err)

	status, body := getJSON(t, app, r, "/v1/orgs/"+orgA.Name)
	assert.Equal(t, 200, status, body)
	status, body = getJSON(t, app, r, "/v1/orgs/"+orgB.Name)
	assert.Equal(t, 403, status, body)
	assert.Equal(t, "forbidden", apiErrCode(t, body))

	status, body = sendJSON(t, app, "PATCH", w, "/v1/orgs/"+orgA.Name, `{"description":"mine"}`)
	assert.Equal(t, 200, status, body)
	status, body = sendJSON(t, app, "PATCH", w, "/v1/orgs/"+orgB.Name, `{"description":"theirs"}`)
	assert.Equal(t, 403, status, body)
	assert.Equal(t, "forbidden", apiErrCode(t, body))
	assert.Empty(t, orgNamed(t, orgB.Name).Description)

	// Unknown orgs are 404 for restricted tokens as for everyone.
	status, _ = getJSON(t, app, r, "/v1/orgs/"+uniqueName(t, "nope"))
	assert.Equal(t, 404, status)
}

func TestUpdateOrgAPI_AdminFlagDoesNotBypassOwnership(t *testing.T) {
	app := orgAPIApp(t)
	owner := mkAPIUser(t, "alice", true, "")
	admin := mkAPIUser(t, "root", true, "")
	require.NoError(t, database.DB.Model(&admin).Update("is_admin", true).Error)
	org, err := orgsvc.CreateOrg(database.DB, owner.ID, uniqueName(t, "adm"), "keep")
	require.NoError(t, err)
	plain, _ := mkAPIToken(t, admin, models.ScopeWrite)

	status, body := sendJSON(t, app, "PATCH", plain, "/v1/orgs/"+org.Name, `{"description":"hijack"}`)
	assert.Equal(t, 403, status, body)
	assert.Equal(t, "keep", orgNamed(t, org.Name).Description)
}

func TestDeleteOrgAPI_OrgRestrictedTokenOnlyOwnOrg(t *testing.T) {
	app := orgAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	orgA := mkOrgWithMember(t, u, "owner")
	orgB := mkOrgWithMember(t, u, "owner")
	w, _, err := apitoken.Svc.Create(u.ID, "w", models.ScopeWrite, &orgA.ID, 30)
	require.NoError(t, err)

	status, body := sendJSON(t, app, "DELETE", w, "/v1/orgs/"+orgB.Name, "")
	assert.Equal(t, 403, status, body)
	assert.Equal(t, "forbidden", apiErrCode(t, body))
	assert.Equal(t, int64(1), countOrgsNamed(t, orgB.Name))

	status, _ = sendJSON(t, app, "DELETE", w, "/v1/orgs/"+uniqueName(t, "nope"), "")
	assert.Equal(t, 404, status)
}

func TestDeleteOrgAPI_AdminFlagDoesNotBypassOwnership(t *testing.T) {
	app := orgAPIApp(t)
	owner := mkAPIUser(t, "alice", true, "")
	admin := mkAPIUser(t, "root", true, "")
	require.NoError(t, database.DB.Model(&admin).Update("is_admin", true).Error)
	org := orgsvcCreate(t, owner, uniqueName(t, "adm"))
	plain, _ := mkAPIToken(t, admin, models.ScopeWrite)

	status, body := sendJSON(t, app, "DELETE", plain, "/v1/orgs/"+org.Name, "")
	assert.Equal(t, 403, status, body)
	assert.Equal(t, int64(1), countOrgsNamed(t, org.Name))
}

func TestOrgMembersAPI_AdminFlagDoesNotBypassOwnership(t *testing.T) {
	app := orgAPIApp(t)
	owner := mkAPIUser(t, "alice", true, "")
	target := mkAPIUser(t, "bob", true, "")
	admin := mkAPIUser(t, "root", true, "")
	require.NoError(t, database.DB.Model(&admin).Update("is_admin", true).Error)
	org := orgsvcCreate(t, owner, uniqueName(t, "adm"))
	_, err := orgsvc.AddMember(database.DB, org.ID, target.Username, orgsvc.RoleMember)
	require.NoError(t, err)
	w, _ := mkAPIToken(t, admin, models.ScopeWrite)
	r, _ := mkAPIToken(t, admin, models.ScopeRead)
	base := "/v1/orgs/" + org.Name + "/members"

	status, body := sendJSON(t, app, "PUT", w, base+"/"+admin.Username, `{"role":"owner"}`)
	assert.Equal(t, 403, status, body)
	status, body = sendJSON(t, app, "DELETE", w, base+"/"+target.Username, "")
	assert.Equal(t, 403, status, body)
	status, body = getJSON(t, app, r, base)
	assert.Equal(t, 403, status, body)
	assert.Equal(t, "member", roleOf(t, org.ID, target))
	assert.Empty(t, roleOf(t, org.ID, admin))
}

func TestOrgMembersAPI_OrgRestrictedTokenOnlyOwnOrg(t *testing.T) {
	app := orgAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	target := mkAPIUser(t, "bob", true, "")
	orgA := mkOrgWithMember(t, u, "owner")
	orgB := mkOrgWithMember(t, u, "owner")
	w, _, err := apitoken.Svc.Create(u.ID, "w", models.ScopeWrite, &orgA.ID, 30)
	require.NoError(t, err)

	status, body := sendJSON(t, app, "PUT", w, "/v1/orgs/"+orgB.Name+"/members/"+target.Username, `{"role":"member"}`)
	assert.Equal(t, 403, status, body)
	assert.Empty(t, roleOf(t, orgB.ID, target))
	status, body = sendJSON(t, app, "DELETE", w, "/v1/orgs/"+orgB.Name+"/members/"+u.Username, "")
	assert.Equal(t, 403, status, body)
	status, body = sendJSON(t, app, "PUT", w, "/v1/orgs/"+orgA.Name+"/members/"+target.Username, `{"role":"member"}`)
	assert.Equal(t, 200, status, body)
}
