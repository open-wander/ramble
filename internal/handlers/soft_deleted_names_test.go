package handlers

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
)

// softDeletedFixtures creates a soft-deleted user and a soft-deleted org.
// Rows are hard-deleted on cleanup because the unique indexes include them.
func softDeletedFixtures(t *testing.T, userName, orgName string) {
	t.Helper()
	u := createTestUser(t, userName)
	require.NoError(t, database.DB.Delete(&u).Error)
	o := models.Organization{Name: orgName}
	require.NoError(t, database.DB.Create(&o).Error)
	require.NoError(t, database.DB.Delete(&o).Error)
	t.Cleanup(func() {
		if err := database.DB.Exec("DELETE FROM organizations WHERE id = ?", o.ID).Error; err != nil {
			t.Errorf("cleanup organizations: %v", err)
		}
		if err := database.DB.Exec("DELETE FROM users WHERE id = ?", u.ID).Error; err != nil {
			t.Errorf("cleanup users: %v", err)
		}
	})
}

func TestPostSignup_SoftDeletedNameRejected(t *testing.T) {
	defer cleanupTestData(t)
	softDeletedFixtures(t, "testsdgone", "testsdgoneorg")

	app := setupTestApp()
	app.Post("/signup", PostSignup)

	assert.Equal(t, 400, postForm(t, app, "/signup", "username=testsdgone&name=A&email=sd1@test.com&password=SecurePass123!"))
	assert.Equal(t, 400, postForm(t, app, "/signup", "username=testsdgoneorg&name=A&email=sd2@test.com&password=SecurePass123!"))
}

func TestAdminRename_SoftDeletedNameRejected(t *testing.T) {
	defer cleanupOrgTestData(t)
	admin := createAdminUser(t, "testsdadmin")
	target := createTestUser(t, "testsdtarget")
	org := models.Organization{Name: "testsdtargetorg"}
	require.NoError(t, database.DB.Create(&org).Error)
	softDeletedFixtures(t, "testsdgone2", "testsdgoneorg2")

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

	userPath := "/admin/users/" + toString(target.ID) + "/edit"
	orgPath := "/admin/organizations/" + toString(org.ID) + "/edit"

	assert.Equal(t, 400, postForm(t, app, userPath, "username=testsdgone2&name=T&email=testsdtarget@test.com"))
	assert.Equal(t, 400, postForm(t, app, userPath, "username=testsdgoneorg2&name=T&email=testsdtarget@test.com"))
	assert.Equal(t, 400, postForm(t, app, orgPath, "name=testsdgone2&description=d"))
	assert.Equal(t, 400, postForm(t, app, orgPath, "name=testsdgoneorg2&description=d"))
}
