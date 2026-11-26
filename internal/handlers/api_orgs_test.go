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
