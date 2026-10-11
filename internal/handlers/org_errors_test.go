package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"rmbl/internal/database"
	"rmbl/internal/models"
)

// failingDB swaps database.DB for a separate gorm instance on the same pool
// whose callbacks fail every statement on table. The instance has its own
// callback registry, so the shared one is untouched. Restored in t.Cleanup.
func failingDB(t *testing.T, table string, op string) {
	t.Helper()
	sqlDB, err := database.DB.DB()
	require.NoError(t, err)
	fdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	fail := func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			_ = tx.AddError(errors.New("forced failure"))
		}
	}
	switch op {
	case "query":
		require.NoError(t, fdb.Callback().Query().Before("gorm:query").Register("test:fail", fail))
	case "create":
		require.NoError(t, fdb.Callback().Create().Before("gorm:create").Register("test:fail", fail))
	}
	orig := database.DB
	database.DB = fdb
	t.Cleanup(func() { database.DB = orig })
}

func logHasEvent(buf *bytes.Buffer, msg, level string) (map[string]any, bool) {
	for line := range bytes.SplitSeq(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var m map[string]any
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		if m[zerolog.MessageFieldName] == msg && m[zerolog.LevelFieldName] == level {
			return m, true
		}
	}
	return nil, false
}

func orgAppAs(t *testing.T, user models.User, buf *bytes.Buffer) *fiber.App {
	t.Helper()
	InitSession()
	app := newRequestLogApp(zerolog.New(buf))
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})
	return app
}

func TestRequireOrgOwner_DBErrorIs500(t *testing.T) {
	defer cleanupOrgTestData(t)
	owner := createTestUser(t, "orgerrowner")
	org := models.Organization{Name: "testorgerr"}
	require.NoError(t, database.DB.Create(&org).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"}).Error)

	var buf bytes.Buffer
	app := orgAppAs(t, owner, &buf)
	app.Get("/orgs/:orgname/settings", RequireOrgOwner, GetOrgSettings)

	failingDB(t, "organizations", "query")
	req := httptest.NewRequest("GET", "/orgs/testorgerr/settings", nil)
	req.Header.Set("X-Request-ID", "rid-org-err")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)

	m, ok := logHasEvent(&buf, "org.load_failed", "error")
	require.True(t, ok, "expected org.load_failed ERROR line: %s", buf.String())
	assert.Equal(t, "rid-org-err", m["request_id"])
}

func TestRequireOrgOwner_MembershipDBErrorIs500(t *testing.T) {
	defer cleanupOrgTestData(t)
	owner := createTestUser(t, "orgmemerr")
	org := models.Organization{Name: "testorgmemerr"}
	require.NoError(t, database.DB.Create(&org).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"}).Error)

	var buf bytes.Buffer
	app := orgAppAs(t, owner, &buf)
	app.Get("/orgs/:orgname/settings", RequireOrgOwner, GetOrgSettings)

	failingDB(t, "memberships", "query")
	req := httptest.NewRequest("GET", "/orgs/testorgmemerr/settings", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	_, ok := logHasEvent(&buf, "org.membership_load_failed", "error")
	assert.True(t, ok, buf.String())
}

func TestOrgHandlers_LoadErrorIs500(t *testing.T) {
	defer cleanupOrgTestData(t)
	owner := createTestUser(t, "orghloaderr")
	other := createTestUser(t, "orghloadother")
	org := models.Organization{Name: "testorghload"}
	require.NoError(t, database.DB.Create(&org).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"}).Error)

	setOrgID := func(c *fiber.Ctx) error { c.Locals("OrgID", org.ID); return c.Next() }

	cases := []struct {
		name    string
		method  string
		path    string
		body    string
		handler fiber.Handler
		route   string
	}{
		{"GetOrgSettings", "GET", "/orgs/testorghload/settings", "", GetOrgSettings, "/orgs/:orgname/settings"},
		{"PostUpdateOrg", "POST", "/orgs/testorghload/update", "description=x", PostUpdateOrg, "/orgs/:orgname/update"},
		{"PostAddMember", "POST", "/orgs/testorghload/members/add", "username=" + other.Username, PostAddMember, "/orgs/:orgname/members/add"},
		{"PostRemoveMember", "POST", "/orgs/testorghload/members/:member_id/remove", "", PostRemoveMember, "/orgs/:orgname/members/:member_id/remove"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			app := orgAppAs(t, owner, &buf)
			app.Add(tc.method, tc.route, setOrgID, tc.handler)

			failingDB(t, "organizations", "query")
			path := strings.Replace(tc.path, ":member_id", "1", 1)
			req := httptest.NewRequest(tc.method, path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("X-Request-ID", "rid-"+tc.name)
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, 500, resp.StatusCode)
			m, ok := logHasEvent(&buf, "org.load_failed", "error")
			require.True(t, ok, buf.String())
			assert.Equal(t, "rid-"+tc.name, m["request_id"])
		})
	}
}

func addMemberApp(t *testing.T, owner models.User, buf *bytes.Buffer) *fiber.App {
	t.Helper()
	app := orgAppAs(t, owner, buf)
	app.Post("/orgs/:orgname/members/add", RequireOrgOwner, PostAddMember)
	return app
}

func postAddMember(t *testing.T, app *fiber.App, orgName, body string) int {
	t.Helper()
	return postForm(t, app, "/orgs/"+orgName+"/members/add", body)
}

func TestPostAddMember_CaseInsensitiveUsername(t *testing.T) {
	defer cleanupOrgTestData(t)
	owner := createTestUser(t, "ciaddowner")
	target := createTestUser(t, "ciaddalice")
	org := models.Organization{Name: "testciadd"}
	require.NoError(t, database.DB.Create(&org).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"}).Error)

	var buf bytes.Buffer
	app := addMemberApp(t, owner, &buf)
	assert.Equal(t, 200, postAddMember(t, app, org.Name, "username=CIADDALICE"))

	var n int64
	database.DB.Model(&models.Membership{}).Where("user_id = ? AND organization_id = ?", target.ID, org.ID).Count(&n)
	assert.EqualValues(t, 1, n)
}

func TestPostAddMember_CreateErrorIs500(t *testing.T) {
	defer cleanupOrgTestData(t)
	owner := createTestUser(t, "createerrowner")
	target := createTestUser(t, "createerrtarget")
	org := models.Organization{Name: "testcreateerr"}
	require.NoError(t, database.DB.Create(&org).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"}).Error)

	var buf bytes.Buffer
	app := addMemberApp(t, owner, &buf)
	failingDB(t, "memberships", "create")
	assert.Equal(t, 500, postAddMember(t, app, org.Name, "username="+target.Username))
	_, ok := logHasEvent(&buf, "org.member_add_failed", "error")
	assert.True(t, ok, buf.String())

	var n int64
	database.DB.Model(&models.Membership{}).Where("user_id = ?", target.ID).Count(&n)
	assert.EqualValues(t, 0, n)
}

func TestPostAddMember_ReaddSoftDeletedMember(t *testing.T) {
	defer cleanupOrgTestData(t)
	owner := createTestUser(t, "readdowner")
	target := createTestUser(t, "readdtarget")
	org := models.Organization{Name: "testreadd"}
	require.NoError(t, database.DB.Create(&org).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"}).Error)
	m := models.Membership{UserID: target.ID, OrganizationID: org.ID, Role: "member"}
	require.NoError(t, database.DB.Create(&m).Error)
	require.NoError(t, database.DB.Delete(&m).Error)

	var buf bytes.Buffer
	app := addMemberApp(t, owner, &buf)
	assert.Equal(t, 200, postAddMember(t, app, org.Name, "username="+target.Username+"&role=owner"))

	var live models.Membership
	require.NoError(t, database.DB.Where("user_id = ? AND organization_id = ?", target.ID, org.ID).First(&live).Error)
	assert.Equal(t, "owner", live.Role)

	// A live membership is still rejected.
	assert.Equal(t, 400, postAddMember(t, app, org.Name, "username="+target.Username))
}

func TestPostAddMember_UnknownRoleBecomesMember(t *testing.T) {
	defer cleanupOrgTestData(t)
	owner := createTestUser(t, "roleowner")
	target := createTestUser(t, "roletarget")
	org := models.Organization{Name: "testrolecheck"}
	require.NoError(t, database.DB.Create(&org).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"}).Error)

	var buf bytes.Buffer
	app := addMemberApp(t, owner, &buf)
	assert.Equal(t, 200, postAddMember(t, app, org.Name, "username="+target.Username+"&role=superadmin"))

	var m models.Membership
	require.NoError(t, database.DB.Where("user_id = ? AND organization_id = ?", target.ID, org.ID).First(&m).Error)
	assert.Equal(t, "member", m.Role)
}
