package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
	"rmbl/internal/services/logger"
	orgsvc "rmbl/internal/services/org"
)

func newOrgAPIApp(t *testing.T, writeLimiter fiber.Handler) *fiber.App {
	t.Helper()
	apitoken.Init(database.DB)
	app := fiber.New(fiber.Config{ErrorHandler: APIErrorHandler})
	app.Use(requestid.New())
	app.Use(logger.RequestLogger(logger.NewJSON(&lockedBuffer{})))
	if writeLimiter == nil {
		writeLimiter = func(c *fiber.Ctx) error { return c.Next() }
	}
	app.Post("/v1/orgs", RequireAPIToken(models.ScopeWrite), writeLimiter, CreateOrgAPI)
	app.Get("/v1/orgs/:name", RequireAPIToken(models.ScopeRead), GetOrgAPI)
	app.Patch("/v1/orgs/:name", RequireAPIToken(models.ScopeWrite), writeLimiter, UpdateOrgAPI)
	app.Delete("/v1/orgs/:name", RequireAPIToken(models.ScopeWrite), writeLimiter, DeleteOrgAPI)
	app.Get("/v1/orgs/:name/members", RequireAPIToken(models.ScopeRead), ListOrgMembersAPI)
	app.Put("/v1/orgs/:name/members/:username", RequireAPIToken(models.ScopeWrite), writeLimiter, PutOrgMemberAPI)
	app.Delete("/v1/orgs/:name/members/:username", RequireAPIToken(models.ScopeWrite), writeLimiter, RemoveOrgMemberAPI)
	return app
}

func orgAPIApp(t *testing.T) *fiber.App { return newOrgAPIApp(t, nil) }

func orgNamed(t *testing.T, name string) models.Organization {
	t.Helper()
	var o models.Organization
	require.NoError(t, database.DB.Where("name = ?", name).First(&o).Error)
	return o
}

func countOrgsNamed(t *testing.T, name string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, database.DB.Model(&models.Organization{}).Where("LOWER(name) = LOWER(?)", name).Count(&n).Error)
	return n
}

func auditRows(t *testing.T, action string, targetID uint) []models.AuditLog {
	t.Helper()
	var rows []models.AuditLog
	require.NoError(t, database.DB.Where("action = ? AND target_id = ?", action, targetID).Find(&rows).Error)
	return rows
}

func TestCreateOrgAPI_Success(t *testing.T) {
	app := orgAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, tok := mkAPIToken(t, u, models.ScopeWrite)
	name := uniqueName(t, "neworg")

	status, body := sendJSON(t, app, "POST", plain, "/v1/orgs", `{"name":"`+name+`","description":"hello"}`)
	require.Equal(t, 201, status, body)

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &raw))
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"name", "description", "created_at", "member_count"}, keys)

	var got OrgResponse
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	assert.Equal(t, name, got.Name)
	assert.Equal(t, "hello", got.Description)
	assert.EqualValues(t, 1, got.MemberCount)
	assert.False(t, got.CreatedAt.IsZero())

	org := orgNamed(t, name)
	role, err := orgsvc.MemberRole(database.DB, org.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, orgsvc.RoleOwner, role)

	rows := auditRows(t, "org.create", org.ID)
	require.Len(t, rows, 1)
	assert.Equal(t, "organization", rows[0].TargetType)
	assert.Equal(t, name, rows[0].TargetName)
	assert.Contains(t, rows[0].Details, `"via":"token"`)
	assert.Contains(t, rows[0].Details, "token_id")
	_ = tok
}

// The create response and a later GET report the same created_at: the
// create response is rendered from the stored row, not the in-memory copy
// (nanoseconds on Linux, microseconds in Postgres).
func TestCreateOrgAPI_CreatedAtMatchesGet(t *testing.T) {
	app := orgAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	name := uniqueName(t, "tsorg")

	status, body := sendJSON(t, app, "POST", plain, "/v1/orgs", `{"name":"`+name+`"}`)
	require.Equal(t, 201, status, body)
	var created map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &created))

	status, body = sendJSON(t, app, "GET", plain, "/v1/orgs/"+name, "")
	require.Equal(t, 200, status, body)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &got))

	assert.Equal(t, got["created_at"], created["created_at"])
}

func TestCreateOrgAPI_Rejections(t *testing.T) {
	app := orgAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	other := mkAPIUser(t, "bob", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	readTok, _ := mkAPIToken(t, u, models.ScopeRead)
	taken := orgsvcCreate(t, other, uniqueName(t, "taken"))

	tests := []struct {
		name   string
		token  string
		body   string
		status int
		code   string
	}{
		{"no token", "", `{"name":"x1"}`, 401, "unauthorized"},
		{"read scope", readTok, `{"name":"x1"}`, 403, "insufficient_scope"},
		{"unknown field", plain, `{"name":"x1","bogus":1}`, 400, "bad_request"},
		{"malformed", plain, `{"name":`, 400, "bad_request"},
		{"empty body", plain, ``, 400, "bad_request"},
		{"empty name", plain, `{"name":""}`, 422, "invalid_name"},
		{"blank name", plain, `{"name":"   "}`, 422, "invalid_name"},
		{"reserved", plain, `{"name":"admin"}`, 422, "invalid_name"},
		{"bad pattern dash", plain, `{"name":"-x"}`, 422, "invalid_name"},
		{"bad pattern space", plain, `{"name":"a b"}`, 422, "invalid_name"},
		{"bad pattern unicode", plain, `{"name":"ü"}`, 422, "invalid_name"},
		{"too long", plain, `{"name":"` + strings.Repeat("a", 65) + `"}`, 422, "invalid_name"},
		{"taken by org other case", plain, `{"name":"` + strings.ToUpper(taken.Name) + `"}`, 409, "conflict"},
		{"taken by user", plain, `{"name":"` + other.Username + `"}`, 409, "conflict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := sendJSON(t, app, "POST", tt.token, "/v1/orgs", tt.body)
			require.Equal(t, tt.status, status, body)
			assert.Equal(t, tt.code, apiErrCode(t, body))
			assert.NotContains(t, body, "org:")
		})
	}
}

func orgsvcCreate(t *testing.T, owner models.User, name string) models.Organization {
	t.Helper()
	o, err := orgsvc.CreateOrg(database.DB, owner.ID, name, "")
	require.NoError(t, err)
	return o
}

func TestCreateOrgAPI_UnverifiedEmail(t *testing.T) {
	app := orgAPIApp(t)
	u := mkAPIUser(t, "alice", false, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	name := uniqueName(t, "unv")
	status, body := sendJSON(t, app, "POST", plain, "/v1/orgs", `{"name":"`+name+`"}`)
	assert.Equal(t, 403, status, body)
	assert.Equal(t, "email_unverified", apiErrCode(t, body))
	assert.Zero(t, countOrgsNamed(t, name))
}

func TestGetOrgAPI(t *testing.T) {
	app := orgAPIApp(t)
	owner := mkAPIUser(t, "alice", true, "")
	member := mkAPIUser(t, "bob", true, "")
	outsider := mkAPIUser(t, "carol", true, "")
	name := uniqueName(t, "MixedOrg")
	org, err := orgsvc.CreateOrg(database.DB, owner.ID, name, "about")
	require.NoError(t, err)
	outTok, _ := mkAPIToken(t, outsider, models.ScopeRead)

	get := func() OrgResponse {
		status, body := getJSON(t, app, outTok, "/v1/orgs/"+name)
		require.Equal(t, 200, status, body)
		var raw map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &raw))
		assert.Len(t, raw, 4)
		for _, k := range []string{"name", "description", "created_at", "member_count"} {
			assert.Contains(t, raw, k)
		}
		var r OrgResponse
		require.NoError(t, json.Unmarshal([]byte(body), &r))
		return r
	}

	r := get()
	assert.Equal(t, name, r.Name)
	assert.Equal(t, "about", r.Description)
	assert.EqualValues(t, 1, r.MemberCount)

	_, err = orgsvc.AddMember(database.DB, org.ID, member.Username, orgsvc.RoleMember)
	require.NoError(t, err)
	assert.EqualValues(t, 2, get().MemberCount)
	require.NoError(t, orgsvc.RemoveMember(database.DB, org.ID, member.ID))
	assert.EqualValues(t, 1, get().MemberCount)

	t.Run("case insensitive", func(t *testing.T) {
		status, body := getJSON(t, app, outTok, "/v1/orgs/"+strings.ToLower(name))
		assert.Equal(t, 200, status, body)
	})
	t.Run("unknown", func(t *testing.T) {
		status, body := getJSON(t, app, outTok, "/v1/orgs/"+uniqueName(t, "nope"))
		assert.Equal(t, 404, status, body)
		assert.Equal(t, "not_found", apiErrCode(t, body))
	})
	t.Run("no token", func(t *testing.T) {
		status, body := getJSON(t, app, "", "/v1/orgs/"+name)
		assert.Equal(t, 401, status, body)
	})
}

func TestUpdateOrgAPI(t *testing.T) {
	app := orgAPIApp(t)
	owner := mkAPIUser(t, "alice", true, "")
	member := mkAPIUser(t, "bob", true, "")
	outsider := mkAPIUser(t, "carol", true, "")
	name := uniqueName(t, "upd")
	org, err := orgsvc.CreateOrg(database.DB, owner.ID, name, "first")
	require.NoError(t, err)
	_, err = orgsvc.AddMember(database.DB, org.ID, member.Username, orgsvc.RoleMember)
	require.NoError(t, err)
	ownerTok, _ := mkAPIToken(t, owner, models.ScopeWrite)
	memberTok, _ := mkAPIToken(t, member, models.ScopeWrite)
	outTok, _ := mkAPIToken(t, outsider, models.ScopeWrite)
	readTok, _ := mkAPIToken(t, owner, models.ScopeRead)
	path := "/v1/orgs/" + name

	desc := func() string { return orgNamed(t, name).Description }

	t.Run("owner updates description", func(t *testing.T) {
		status, body := sendJSON(t, app, "PATCH", ownerTok, path, `{"description":"second"}`)
		require.Equal(t, 200, status, body)
		var r OrgResponse
		require.NoError(t, json.Unmarshal([]byte(body), &r))
		assert.Equal(t, "second", r.Description)
		assert.Equal(t, name, r.Name)
		assert.EqualValues(t, 2, r.MemberCount)
		assert.Equal(t, "second", desc())
		rows := auditRows(t, "org.update", org.ID)
		require.Len(t, rows, 1)
		assert.Equal(t, "organization", rows[0].TargetType)
		assert.Contains(t, rows[0].Details, `"via":"token"`)
	})
	t.Run("empty object and null leave unchanged", func(t *testing.T) {
		for _, b := range []string{`{}`, `{"description":null}`} {
			status, body := sendJSON(t, app, "PATCH", ownerTok, path, b)
			require.Equal(t, 200, status, body)
			assert.Equal(t, "second", desc())
		}
		assert.Len(t, auditRows(t, "org.update", org.ID), 1)
	})
	t.Run("empty string clears", func(t *testing.T) {
		status, body := sendJSON(t, app, "PATCH", ownerTok, path, `{"description":""}`)
		require.Equal(t, 200, status, body)
		assert.Empty(t, desc())
	})
	t.Run("case insensitive path", func(t *testing.T) {
		status, body := sendJSON(t, app, "PATCH", ownerTok, "/v1/orgs/"+strings.ToUpper(name), `{"description":"up"}`)
		assert.Equal(t, 200, status, body)
		assert.Equal(t, "up", desc())
	})

	rejections := []struct {
		name   string
		token  string
		path   string
		body   string
		status int
		code   string
	}{
		{"no token", "", path, `{}`, 401, "unauthorized"},
		{"read scope", readTok, path, `{"description":"x"}`, 403, "insufficient_scope"},
		{"member", memberTok, path, `{"description":"x"}`, 403, "forbidden"},
		{"non-member", outTok, path, `{"description":"x"}`, 403, "forbidden"},
		{"unknown org", ownerTok, "/v1/orgs/" + uniqueName(t, "nope"), `{"description":"x"}`, 404, "not_found"},
		{"unknown field", ownerTok, path, `{"bogus":1}`, 400, "bad_request"},
		{"malformed", ownerTok, path, `{`, 400, "bad_request"},
		{"name immutable", ownerTok, path, `{"name":"x"}`, 422, "immutable_field"},
		{"name null immutable", ownerTok, path, `{"name":null}`, 422, "immutable_field"},
		{"name with description", ownerTok, path, `{"name":"x","description":"zzz"}`, 422, "immutable_field"},
	}
	for _, tt := range rejections {
		t.Run(tt.name, func(t *testing.T) {
			before := desc()
			status, body := sendJSON(t, app, "PATCH", tt.token, tt.path, tt.body)
			require.Equal(t, tt.status, status, body)
			assert.Equal(t, tt.code, apiErrCode(t, body))
			assert.Equal(t, before, desc())
			assert.NotContains(t, body, "org:")
		})
	}
	assert.Len(t, auditRows(t, "org.update", org.ID), 3)
}

func TestOrgAPI_WriteLimiter(t *testing.T) {
	app := newOrgAPIApp(t, NewAPIWriteLimiter())
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	var last int
	for range 121 {
		last, _ = sendJSON(t, app, "POST", plain, "/v1/orgs", `{`)
	}
	assert.Equal(t, 429, last)
}

func TestDeleteOrgAPI(t *testing.T) {
	app := orgAPIApp(t)
	owner := mkAPIUser(t, "alice", true, "")
	member := mkAPIUser(t, "bob", true, "")
	outsider := mkAPIUser(t, "carol", true, "")
	unverified := mkAPIUser(t, "dave", false, "")
	name := uniqueName(t, "del")
	org := orgsvcCreate(t, owner, name)
	_, err := orgsvc.AddMember(database.DB, org.ID, member.Username, orgsvc.RoleMember)
	require.NoError(t, err)
	_, err = orgsvc.AddMember(database.DB, org.ID, unverified.Username, orgsvc.RoleOwner)
	require.NoError(t, err)
	ownerTok, _ := mkAPIToken(t, owner, models.ScopeWrite)
	memberTok, _ := mkAPIToken(t, member, models.ScopeWrite)
	outTok, _ := mkAPIToken(t, outsider, models.ScopeWrite)
	readTok, _ := mkAPIToken(t, owner, models.ScopeRead)
	unvTok, _ := mkAPIToken(t, unverified, models.ScopeWrite)
	path := "/v1/orgs/" + name

	rejections := []struct {
		name   string
		token  string
		path   string
		status int
		code   string
	}{
		{"no token", "", path, 401, "unauthorized"},
		{"read scope", readTok, path, 403, "insufficient_scope"},
		{"member", memberTok, path, 403, "forbidden"},
		{"non-member", outTok, path, 403, "forbidden"},
		{"unverified email", unvTok, path, 403, "email_unverified"},
		{"unknown org", ownerTok, "/v1/orgs/" + uniqueName(t, "nope"), 404, "not_found"},
	}
	for _, tt := range rejections {
		t.Run(tt.name, func(t *testing.T) {
			status, body := sendJSON(t, app, "DELETE", tt.token, tt.path, "")
			require.Equal(t, tt.status, status, body)
			assert.Equal(t, tt.code, apiErrCode(t, body))
			assert.Equal(t, int64(1), countOrgsNamed(t, name))
		})
	}
	assert.Empty(t, auditRows(t, "org.delete", org.ID))

	t.Run("not empty", func(t *testing.T) {
		for i, want := range []string{"organization still has 1 resource", "organization still has 2 resources"} {
			r := models.NomadResource{Name: uniqueName(t, "res"), UserID: owner.ID, OrganizationID: &org.ID}
			require.NoError(t, database.DB.Create(&r).Error)
			t.Cleanup(func() { database.DB.Unscoped().Delete(&r) })
			status, body := sendJSON(t, app, "DELETE", ownerTok, path, "")
			require.Equal(t, 409, status, "%d %s", i, body)
			assert.Equal(t, "org_not_empty", apiErrCode(t, body))
			assert.Contains(t, body, want+`"`)
		}
		assert.Equal(t, int64(1), countOrgsNamed(t, name))
		assert.Empty(t, auditRows(t, "org.delete", org.ID))
		require.NoError(t, database.DB.Exec("DELETE FROM nomad_resources WHERE organization_id = ?", org.ID).Error)
	})

	t.Run("owner deletes", func(t *testing.T) {
		status, body := sendJSON(t, app, "DELETE", ownerTok, "/v1/orgs/"+strings.ToUpper(name), "")
		require.Equal(t, 204, status, body)
		assert.Empty(t, body)
		assert.Zero(t, countOrgsNamed(t, name))
		rows := auditRows(t, "org.delete", org.ID)
		require.Len(t, rows, 1)
		assert.Equal(t, "organization", rows[0].TargetType)

		status, body = getJSON(t, app, ownerTok, path)
		assert.Equal(t, 404, status, body)
		status, body = sendJSON(t, app, "DELETE", ownerTok, path, "")
		assert.Equal(t, 404, status, body)
		assert.Len(t, auditRows(t, "org.delete", org.ID), 1)
	})
}

type memberFixture struct {
	app                                    *fiber.App
	org                                    models.Organization
	owner, member, outsider, unverified    models.User
	ownerW, ownerR, memberW, memberR, outW string
	unvW                                   string
}

func newMemberFixture(t *testing.T) memberFixture {
	t.Helper()
	f := memberFixture{app: orgAPIApp(t)}
	f.owner = mkAPIUser(t, "alice", true, "")
	f.member = mkAPIUser(t, "bob", true, "")
	f.outsider = mkAPIUser(t, "carol", true, "")
	f.unverified = mkAPIUser(t, "dave", false, "")
	f.org = orgsvcCreate(t, f.owner, uniqueName(t, "mem"))
	_, err := orgsvc.AddMember(database.DB, f.org.ID, f.member.Username, orgsvc.RoleMember)
	require.NoError(t, err)
	_, err = orgsvc.AddMember(database.DB, f.org.ID, f.unverified.Username, orgsvc.RoleOwner)
	require.NoError(t, err)
	f.ownerW, _ = mkAPIToken(t, f.owner, models.ScopeWrite)
	f.ownerR, _ = mkAPIToken(t, f.owner, models.ScopeRead)
	f.memberW, _ = mkAPIToken(t, f.member, models.ScopeWrite)
	f.memberR, _ = mkAPIToken(t, f.member, models.ScopeRead)
	f.outW, _ = mkAPIToken(t, f.outsider, models.ScopeWrite)
	f.unvW, _ = mkAPIToken(t, f.unverified, models.ScopeWrite)
	return f
}

func (f memberFixture) path(username string) string {
	return "/v1/orgs/" + f.org.Name + "/members/" + username
}

func roleOf(t *testing.T, orgID uint, u models.User) string {
	t.Helper()
	role, err := orgsvc.MemberRole(database.DB, orgID, u.ID)
	if err != nil {
		return ""
	}
	return role
}

func TestListOrgMembersAPI(t *testing.T) {
	f := newMemberFixture(t)
	path := "/v1/orgs/" + f.org.Name + "/members"
	otherOrg := orgsvcCreate(t, f.owner, uniqueName(t, "oth"))
	restricted, _, err := apitoken.Svc.Create(f.owner.ID, "r", models.ScopeRead, &otherOrg.ID, 30)
	require.NoError(t, err)
	outR, _ := mkAPIToken(t, f.outsider, models.ScopeRead)

	rejections := []struct {
		name   string
		token  string
		path   string
		status int
		code   string
	}{
		{"no token", "", path, 401, "unauthorized"},
		{"non-member", outR, path, 403, "forbidden"},
		{"restricted token", restricted, path, 403, "forbidden"},
		{"unknown org", f.ownerR, "/v1/orgs/" + uniqueName(t, "nope") + "/members", 404, "not_found"},
	}
	for _, tt := range rejections {
		t.Run(tt.name, func(t *testing.T) {
			status, body := getJSON(t, f.app, tt.token, tt.path)
			require.Equal(t, tt.status, status, body)
			assert.Equal(t, tt.code, apiErrCode(t, body))
		})
	}

	for name, tok := range map[string]string{"owner": f.ownerR, "member": f.memberR} {
		t.Run(name+" lists", func(t *testing.T) {
			status, body := getJSON(t, f.app, tok, path)
			require.Equal(t, 200, status, body)
			var got []OrgMemberResponse
			require.NoError(t, json.Unmarshal([]byte(body), &got))
			assert.Equal(t, []OrgMemberResponse{
				{f.owner.Username, "owner"}, {f.member.Username, "member"}, {f.unverified.Username, "owner"},
			}, got)
			assert.NotContains(t, body, "email")
		})
	}
}

func TestPutOrgMemberAPI(t *testing.T) {
	f := newMemberFixture(t)

	rejections := []struct {
		name   string
		token  string
		path   string
		body   string
		status int
		code   string
	}{
		{"no token", "", f.path(f.outsider.Username), `{"role":"member"}`, 401, "unauthorized"},
		{"read scope", f.ownerR, f.path(f.outsider.Username), `{"role":"member"}`, 403, "insufficient_scope"},
		{"unverified email", f.unvW, f.path(f.outsider.Username), `{"role":"member"}`, 403, "email_unverified"},
		{"member", f.memberW, f.path(f.outsider.Username), `{"role":"member"}`, 403, "forbidden"},
		{"non-member", f.outW, f.path(f.outsider.Username), `{"role":"owner"}`, 403, "forbidden"},
		{"unknown org", f.ownerW, "/v1/orgs/" + uniqueName(t, "nope") + "/members/x", `{"role":"member"}`, 404, "not_found"},
		{"unknown user", f.ownerW, f.path("nobody-" + uniqueName(t, "u")), `{"role":"member"}`, 404, "not_found"},
		{"role admin", f.ownerW, f.path(f.outsider.Username), `{"role":"admin"}`, 422, "invalid_role"},
		{"role empty", f.ownerW, f.path(f.outsider.Username), `{"role":""}`, 422, "invalid_role"},
		{"role missing", f.ownerW, f.path(f.outsider.Username), `{}`, 422, "invalid_role"},
		{"role wrong case", f.ownerW, f.path(f.outsider.Username), `{"role":"Owner"}`, 422, "invalid_role"},
		{"unknown field", f.ownerW, f.path(f.outsider.Username), `{"role":"member","x":1}`, 400, "bad_request"},
		{"malformed", f.ownerW, f.path(f.outsider.Username), `{"role":`, 400, "bad_request"},
		{"empty body", f.ownerW, f.path(f.outsider.Username), ``, 400, "bad_request"},
	}
	for _, tt := range rejections {
		t.Run(tt.name, func(t *testing.T) {
			status, body := sendJSON(t, f.app, "PUT", tt.token, tt.path, tt.body)
			require.Equal(t, tt.status, status, body)
			assert.Equal(t, tt.code, apiErrCode(t, body))
		})
	}
	assert.Empty(t, roleOf(t, f.org.ID, f.outsider))
	assert.Equal(t, "member", roleOf(t, f.org.ID, f.member))
	assert.Empty(t, auditRows(t, "org.add_member", f.org.ID))

	t.Run("unknown user message names the user", func(t *testing.T) {
		_, body := sendJSON(t, f.app, "PUT", f.ownerW, f.path("nobody-x"), `{"role":"member"}`)
		assert.Contains(t, body, "user not found")
	})

	t.Run("add by mixed-case name", func(t *testing.T) {
		status, body := sendJSON(t, f.app, "PUT", f.ownerW, f.path(strings.ToUpper(f.outsider.Username)), `{"role":"member"}`)
		require.Equal(t, 200, status, body)
		var got OrgMemberResponse
		require.NoError(t, json.Unmarshal([]byte(body), &got))
		assert.Equal(t, OrgMemberResponse{f.outsider.Username, "member"}, got)
		rows := auditRows(t, "org.add_member", f.org.ID)
		require.Len(t, rows, 1)
		assert.Contains(t, rows[0].Details, `"member":"`+f.outsider.Username+`"`)
		assert.Contains(t, rows[0].Details, `"role":"member"`)
	})

	t.Run("same role again is 200 without audit", func(t *testing.T) {
		status, body := sendJSON(t, f.app, "PUT", f.ownerW, f.path(f.outsider.Username), `{"role":"member"}`)
		require.Equal(t, 200, status, body)
		assert.Len(t, auditRows(t, "org.add_member", f.org.ID), 1)
		assert.Empty(t, auditRows(t, "org.change_member_role", f.org.ID))
	})

	t.Run("promote audits the change", func(t *testing.T) {
		status, body := sendJSON(t, f.app, "PUT", f.ownerW, f.path(f.outsider.Username), `{"role":"owner"}`)
		require.Equal(t, 200, status, body)
		assert.Equal(t, "owner", roleOf(t, f.org.ID, f.outsider))
		rows := auditRows(t, "org.change_member_role", f.org.ID)
		require.Len(t, rows, 1)
		assert.Contains(t, rows[0].Details, `"old_role":"member"`)
		assert.Contains(t, rows[0].Details, `"new_role":"owner"`)
		assert.Contains(t, rows[0].Details, `"member":"`+f.outsider.Username+`"`)
	})

	t.Run("restores a removed member", func(t *testing.T) {
		require.NoError(t, orgsvc.RemoveMember(database.DB, f.org.ID, f.member.ID))
		status, body := sendJSON(t, f.app, "PUT", f.ownerW, f.path(f.member.Username), `{"role":"member"}`)
		require.Equal(t, 200, status, body)
		assert.Equal(t, "member", roleOf(t, f.org.ID, f.member))
		assert.Len(t, auditRows(t, "org.add_member", f.org.ID), 2)
	})

	t.Run("last owner demotion is 409", func(t *testing.T) {
		// Leave a single owner.
		for _, u := range []models.User{f.outsider, f.unverified} {
			require.NoError(t, orgsvc.RemoveMember(database.DB, f.org.ID, u.ID))
		}
		status, body := sendJSON(t, f.app, "PUT", f.ownerW, f.path(f.owner.Username), `{"role":"member"}`)
		require.Equal(t, 409, status, body)
		assert.Equal(t, "last_owner", apiErrCode(t, body))
		assert.Equal(t, "owner", roleOf(t, f.org.ID, f.owner))
		// Re-asserting the owner role is still fine.
		status, body = sendJSON(t, f.app, "PUT", f.ownerW, f.path(f.owner.Username), `{"role":"owner"}`)
		assert.Equal(t, 200, status, body)
	})
}

func TestRemoveOrgMemberAPI(t *testing.T) {
	f := newMemberFixture(t)

	rejections := []struct {
		name   string
		token  string
		path   string
		status int
		code   string
	}{
		{"no token", "", f.path(f.member.Username), 401, "unauthorized"},
		{"read scope", f.ownerR, f.path(f.member.Username), 403, "insufficient_scope"},
		{"unverified email", f.unvW, f.path(f.member.Username), 403, "email_unverified"},
		{"member", f.memberW, f.path(f.member.Username), 403, "forbidden"},
		{"non-member", f.outW, f.path(f.member.Username), 403, "forbidden"},
		{"unknown org", f.ownerW, "/v1/orgs/" + uniqueName(t, "nope") + "/members/x", 404, "not_found"},
		{"unknown user", f.ownerW, f.path("nobody-x"), 404, "not_found"},
		{"user not a member", f.ownerW, f.path(f.outsider.Username), 404, "not_found"},
	}
	for _, tt := range rejections {
		t.Run(tt.name, func(t *testing.T) {
			status, body := sendJSON(t, f.app, "DELETE", tt.token, tt.path, "")
			require.Equal(t, tt.status, status, body)
			assert.Equal(t, tt.code, apiErrCode(t, body))
		})
	}
	assert.Equal(t, "member", roleOf(t, f.org.ID, f.member))
	assert.Empty(t, auditRows(t, "org.remove_member", f.org.ID))

	t.Run("owner removes", func(t *testing.T) {
		status, body := sendJSON(t, f.app, "DELETE", f.ownerW, f.path(strings.ToUpper(f.member.Username)), "")
		require.Equal(t, 204, status, body)
		assert.Empty(t, body)
		assert.Empty(t, roleOf(t, f.org.ID, f.member))
		rows := auditRows(t, "org.remove_member", f.org.ID)
		require.Len(t, rows, 1)
		assert.Contains(t, rows[0].Details, `"member":"`+f.member.Username+`"`)
		assert.Contains(t, rows[0].Details, `"member_id"`)

		status, _ = sendJSON(t, f.app, "DELETE", f.ownerW, f.path(f.member.Username), "")
		assert.Equal(t, 404, status)
	})

	t.Run("last owner is 409", func(t *testing.T) {
		require.NoError(t, orgsvc.RemoveMember(database.DB, f.org.ID, f.unverified.ID))
		status, body := sendJSON(t, f.app, "DELETE", f.ownerW, f.path(f.owner.Username), "")
		require.Equal(t, 409, status, body)
		assert.Equal(t, "last_owner", apiErrCode(t, body))
		assert.Equal(t, "owner", roleOf(t, f.org.ID, f.owner))
	})
}

func TestOrgMembersAPI_WriteLimiter(t *testing.T) {
	app := newOrgAPIApp(t, NewAPIWriteLimiter())
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	var last int
	for range 121 {
		last, _ = sendJSON(t, app, "PUT", plain, "/v1/orgs/x/members/y", `{`)
	}
	assert.Equal(t, 429, last)
}
