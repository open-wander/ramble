package handlers

import (
	"net/http/httptest"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsReservedName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"requests", true},
		{"Requests", true},
		{"ADMIN", true},
		{"  login  ", true},
		{"favicon.ico", true},
		{"alice", false},
		{"", false},
		{"requests2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsReservedName(tt.name))
		})
	}
}

func postForm(t *testing.T, app *fiber.App, path, body string) int {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	return resp.StatusCode
}

func TestPostSignup_ReservedUsername(t *testing.T) {
	defer cleanupTestData(t)

	app := setupTestApp()
	app.Post("/signup", PostSignup)

	status := postForm(t, app, "/signup", "username=Requests&name=R&email=reserved@test.com&password=SecurePass123!")
	assert.Equal(t, 400, status)

	var count int64
	database.DB.Model(&models.User{}).Where("email = ?", "reserved@test.com").Count(&count)
	assert.Zero(t, count, "no user row should be created")

	// Control: a non-reserved name still signs up.
	status = postForm(t, app, "/signup", "username=notreservedname&name=R&email=notreserved@test.com&password=SecurePass123!")
	assert.Equal(t, 200, status)
}

func TestPostCreateOrg_ReservedName(t *testing.T) {
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "orgreservedcreator")
	app := fiber.New()
	InitSession()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})
	app.Post("/orgs/new", PostCreateOrg)

	for _, name := range []string{"admin", "ADMIN"} {
		assert.Equal(t, 400, postForm(t, app, "/orgs/new", "name="+name+"&description=x"), name)
	}
	var count int64
	database.DB.Model(&models.Organization{}).Where("LOWER(name) = ?", "admin").Count(&count)
	assert.Zero(t, count, "no org should be created")
}

func adminEditApp(admin models.User) *fiber.App {
	app := setupTestApp()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})
	app.Post("/admin/users/:id/edit", RequireAdmin, PostEditUser)
	app.Post("/admin/organizations/:id/edit", RequireAdmin, PostEditOrganization)
	return app
}

func TestPostEditUser_ReservedUsername(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "reservededitadmin")
	target := createTestUser(t, "reservedtarget")
	app := adminEditApp(admin)

	status := postForm(t, app, "/admin/users/"+toString(target.ID)+"/edit", "username=Login&name=X&email=reservedtarget@test.com")
	assert.Equal(t, 400, status)

	var after models.User
	database.DB.First(&after, target.ID)
	assert.Equal(t, "reservedtarget", after.Username)
}

func TestPostEditUser_GrandfatheredReservedUsername(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "grandadmin")
	target := createTestUser(t, "admin")
	app := adminEditApp(admin)

	status := postForm(t, app, "/admin/users/"+toString(target.ID)+"/edit", "username=Admin&name=New+Name&email=newgrand@test.com")
	assert.Equal(t, 200, status)

	var after models.User
	database.DB.First(&after, target.ID)
	assert.Equal(t, "New Name", after.Name)
	assert.Equal(t, "newgrand@test.com", after.Email)
}

func TestPostEditOrganization_ReservedName(t *testing.T) {
	defer cleanupOrgTestData(t)

	admin := createAdminUser(t, "reservedorgadmin")
	org := models.Organization{Name: "testreservedorg", Description: "d"}
	require.NoError(t, database.DB.Create(&org).Error)
	app := adminEditApp(admin)

	status := postForm(t, app, "/admin/organizations/"+toString(org.ID)+"/edit", "name=Login&description=d")
	assert.Equal(t, 400, status)

	var after models.Organization
	database.DB.First(&after, org.ID)
	assert.Equal(t, "testreservedorg", after.Name)
}

func TestPostEditOrganization_GrandfatheredReservedName(t *testing.T) {
	defer cleanupOrgTestData(t)

	admin := createAdminUser(t, "grandorgadmin")
	org := models.Organization{Name: "support", Description: "old"}
	require.NoError(t, database.DB.Create(&org).Error)
	defer database.DB.Exec("DELETE FROM organizations WHERE id = ?", org.ID)
	app := adminEditApp(admin)

	status := postForm(t, app, "/admin/organizations/"+toString(org.ID)+"/edit", "name=Support&description=new")
	assert.Equal(t, 200, status)

	var after models.Organization
	database.DB.First(&after, org.ID)
	assert.Equal(t, "new", after.Description)
}

func TestOAuthNewUser_ReservedNickname(t *testing.T) {
	defer cleanupTestData(t)

	gu := func(id string) goth.User {
		return goth.User{Provider: "github", UserID: id, Email: id + "@test.com", Name: "N", NickName: "login", AccessToken: "tok"}
	}

	u1, err := processOAuthUserDB(database.DB, gu("rsv1"))
	require.NoError(t, err)
	assert.Equal(t, "login-1", u1.Username)

	// login-1 is now taken, so the next reserved login gets login-2.
	u2, err := processOAuthUserDB(database.DB, gu("rsv2"))
	require.NoError(t, err)
	assert.Equal(t, "login-2", u2.Username)
}
