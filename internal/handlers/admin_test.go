package handlers

import (
	"net/http/httptest"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

// createAdminUser creates an admin user for testing
func createAdminUser(t *testing.T, username string) models.User {
	user := models.User{
		Username:      username,
		Email:         username + "@test.com",
		Name:          "Admin " + username,
		EmailVerified: true,
		IsAdmin:       true,
	}
	err := database.DB.Create(&user).Error
	assert.NoError(t, err)
	return user
}

func TestRequireAdmin_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	app.Get("/admin", RequireAdmin, GetAdminDashboard)

	req := httptest.NewRequest("GET", "/admin", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Should redirect to login
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

func TestRequireAdmin_NonAdmin(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "regularuser")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Get("/admin", RequireAdmin, GetAdminDashboard)

	req := httptest.NewRequest("GET", "/admin", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Should redirect to home (not authorized)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/", resp.Header.Get("Location"))
}

func TestRequireAdmin_IsAdmin(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "adminuser")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Get("/admin", RequireAdmin, GetAdminDashboard)

	req := httptest.NewRequest("GET", "/admin", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetAdminUsers(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "adminuser2")
	createTestUser(t, "user1")
	createTestUser(t, "user2")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Get("/admin/users", RequireAdmin, GetAdminUsers)

	req := httptest.NewRequest("GET", "/admin/users", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPostToggleAdmin_NonAdmin(t *testing.T) {
	defer cleanupTestData(t)

	regularUser := createTestUser(t, "regular")
	targetUser := createTestUser(t, "target")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", regularUser.ID)
		sess.Save()
		c.Locals("UserID", regularUser.ID)
		c.Locals("User", regularUser)
		return c.Next()
	})

	app.Post("/admin/users/:id/toggle-admin", RequireAdmin, PostToggleAdmin)

	req := httptest.NewRequest("POST", "/admin/users/"+toString(targetUser.ID)+"/toggle-admin", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Non-admin should be redirected
	assert.Equal(t, 302, resp.StatusCode)
}

func TestPostToggleAdmin_Success(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "adminuser3")
	targetUser := createTestUser(t, "targetuser")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Post("/admin/users/:id/toggle-admin", RequireAdmin, PostToggleAdmin)

	req := httptest.NewRequest("POST", "/admin/users/"+toString(targetUser.ID)+"/toggle-admin", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify user is now admin
	var updated models.User
	database.DB.First(&updated, targetUser.ID)
	assert.True(t, updated.IsAdmin)
}

func TestPostToggleAdmin_CannotToggleSelf(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "selfadmin")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Post("/admin/users/:id/toggle-admin", RequireAdmin, PostToggleAdmin)

	// Try to toggle own admin status
	req := httptest.NewRequest("POST", "/admin/users/"+toString(admin.ID)+"/toggle-admin", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)

	// Verify admin status unchanged
	var updated models.User
	database.DB.First(&updated, admin.ID)
	assert.True(t, updated.IsAdmin)
}

func TestGetAdminResources(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "resourceadmin")
	user := createTestUser(t, "resourceowner")
	createTestPack(t, user.ID, "admin-test-pack")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Get("/admin/resources", RequireAdmin, GetAdminResources)

	req := httptest.NewRequest("GET", "/admin/resources", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetAdminOrganizations(t *testing.T) {
	defer cleanupOrgTestData(t)

	admin := createAdminUser(t, "orgadmin")

	// Create a test org
	org := models.Organization{Name: "testadminorg", Description: "Test"}
	database.DB.Create(&org)

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Get("/admin/organizations", RequireAdmin, GetAdminOrganizations)

	req := httptest.NewRequest("GET", "/admin/organizations", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// DeleteUser Tests

func TestDeleteUser_NonAdmin(t *testing.T) {
	defer cleanupTestData(t)

	regularUser := createTestUser(t, "regulardelete")
	targetUser := createTestUser(t, "targetdelete")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", regularUser.ID)
		sess.Save()
		c.Locals("UserID", regularUser.ID)
		c.Locals("User", regularUser)
		return c.Next()
	})

	app.Delete("/admin/users/:id", RequireAdmin, DeleteUser)

	req := httptest.NewRequest("DELETE", "/admin/users/"+toString(targetUser.ID), nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Non-admin should be redirected
	assert.Equal(t, 302, resp.StatusCode)

	// Verify user wasn't deleted
	var count int64
	database.DB.Model(&models.User{}).Where("id = ?", targetUser.ID).Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestDeleteUser_CannotDeleteSelf(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "selfdeleteadmin")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Delete("/admin/users/:id", RequireAdmin, DeleteUser)

	req := httptest.NewRequest("DELETE", "/admin/users/"+toString(admin.ID), nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)

	// Verify admin wasn't deleted
	var count int64
	database.DB.Model(&models.User{}).Where("id = ?", admin.ID).Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestDeleteUser_Success(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "deleteadmin")
	targetUser := createTestUser(t, "deletable")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Delete("/admin/users/:id", RequireAdmin, DeleteUser)

	req := httptest.NewRequest("DELETE", "/admin/users/"+toString(targetUser.ID), nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify user was deleted
	var count int64
	database.DB.Model(&models.User{}).Where("id = ?", targetUser.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestDeleteUser_NotFound(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "deleteadmin2")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Delete("/admin/users/:id", RequireAdmin, DeleteUser)

	req := httptest.NewRequest("DELETE", "/admin/users/99999", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

// DeleteOrganization Tests

func TestDeleteOrganization_NonAdmin(t *testing.T) {
	defer cleanupOrgTestData(t)

	regularUser := createTestUser(t, "orgdeleteregular")
	org := models.Organization{Name: "testdeleteorg", Description: "Test"}
	database.DB.Create(&org)

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", regularUser.ID)
		sess.Save()
		c.Locals("UserID", regularUser.ID)
		c.Locals("User", regularUser)
		return c.Next()
	})

	app.Delete("/admin/organizations/:id", RequireAdmin, DeleteOrganization)

	req := httptest.NewRequest("DELETE", "/admin/organizations/"+toString(org.ID), nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)

	// Verify org wasn't deleted
	var count int64
	database.DB.Model(&models.Organization{}).Where("id = ?", org.ID).Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestDeleteOrganization_Success(t *testing.T) {
	defer cleanupOrgTestData(t)

	admin := createAdminUser(t, "orgdeleteadmin")
	org := models.Organization{Name: "testdeleteorg2", Description: "Test"}
	database.DB.Create(&org)

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Delete("/admin/organizations/:id", RequireAdmin, DeleteOrganization)

	req := httptest.NewRequest("DELETE", "/admin/organizations/"+toString(org.ID), nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify org was deleted
	var count int64
	database.DB.Model(&models.Organization{}).Where("id = ?", org.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestDeleteOrganization_NotFound(t *testing.T) {
	defer cleanupOrgTestData(t)

	admin := createAdminUser(t, "orgdeleteadmin2")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Delete("/admin/organizations/:id", RequireAdmin, DeleteOrganization)

	req := httptest.NewRequest("DELETE", "/admin/organizations/99999", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

// PostEditUser Tests

func TestPostEditUser_NonAdmin(t *testing.T) {
	defer cleanupTestData(t)

	regularUser := createTestUser(t, "editregular")
	targetUser := createTestUser(t, "edittarget")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", regularUser.ID)
		sess.Save()
		c.Locals("UserID", regularUser.ID)
		c.Locals("User", regularUser)
		return c.Next()
	})

	app.Post("/admin/users/:id/edit", RequireAdmin, PostEditUser)

	payload := strings.NewReader("username=newname&name=New+Name&email=new@test.com")
	req := httptest.NewRequest("POST", "/admin/users/"+toString(targetUser.ID)+"/edit", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
}

func TestPostEditUser_NotFound(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "editadmin1")

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

	payload := strings.NewReader("username=newname&name=New+Name&email=new@test.com")
	req := httptest.NewRequest("POST", "/admin/users/99999/edit", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPostEditUser_Success(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "editadmin2")
	targetUser := createTestUser(t, "editabletarget")

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

	payload := strings.NewReader("username=updateduser&name=Updated+Name&email=updated@test.com&email_verified=on")
	req := httptest.NewRequest("POST", "/admin/users/"+toString(targetUser.ID)+"/edit", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify user was updated
	var updated models.User
	database.DB.First(&updated, targetUser.ID)
	assert.Equal(t, "updateduser", updated.Username)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, "updated@test.com", updated.Email)
	assert.True(t, updated.EmailVerified)
}

// PostEditOrganization Tests

func TestPostEditOrganization_NonAdmin(t *testing.T) {
	defer cleanupOrgTestData(t)

	regularUser := createTestUser(t, "orgeditregular")
	org := models.Organization{Name: "testeditorg", Description: "Test"}
	database.DB.Create(&org)

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", regularUser.ID)
		sess.Save()
		c.Locals("UserID", regularUser.ID)
		c.Locals("User", regularUser)
		return c.Next()
	})

	app.Post("/admin/organizations/:id/edit", RequireAdmin, PostEditOrganization)

	payload := strings.NewReader("name=neworgname&description=New+Description")
	req := httptest.NewRequest("POST", "/admin/organizations/"+toString(org.ID)+"/edit", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
}

func TestPostEditOrganization_NotFound(t *testing.T) {
	defer cleanupOrgTestData(t)

	admin := createAdminUser(t, "orgeditadmin1")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Post("/admin/organizations/:id/edit", RequireAdmin, PostEditOrganization)

	payload := strings.NewReader("name=neworgname&description=New+Description")
	req := httptest.NewRequest("POST", "/admin/organizations/99999/edit", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPostEditOrganization_Success(t *testing.T) {
	defer cleanupOrgTestData(t)

	admin := createAdminUser(t, "orgeditadmin2")
	org := models.Organization{Name: "testeditorg2", Description: "Original"}
	database.DB.Create(&org)

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Post("/admin/organizations/:id/edit", RequireAdmin, PostEditOrganization)

	payload := strings.NewReader("name=updatedorg&description=Updated+Description")
	req := httptest.NewRequest("POST", "/admin/organizations/"+toString(org.ID)+"/edit", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify org was updated
	var updated models.Organization
	database.DB.First(&updated, org.ID)
	assert.Equal(t, "updatedorg", updated.Name)
	assert.Equal(t, "Updated Description", updated.Description)
}
