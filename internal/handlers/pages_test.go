package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

// Helper functions are defined in api_test.go and admin_test.go

func TestGetDocs(t *testing.T) {
	app := setupTestApp()
	app.Get("/docs", GetDocs)

	req := httptest.NewRequest("GET", "/docs", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetAbout(t *testing.T) {
	app := setupTestApp()
	app.Get("/about", GetAbout)

	req := httptest.NewRequest("GET", "/about", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetLogin(t *testing.T) {
	app := setupTestApp()
	app.Get("/login", GetLogin)

	req := httptest.NewRequest("GET", "/login", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetSignup(t *testing.T) {
	app := setupTestApp()
	app.Get("/signup", GetSignup)

	req := httptest.NewRequest("GET", "/signup", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetForgotPassword(t *testing.T) {
	app := setupTestApp()
	app.Get("/forgot-password", GetForgotPassword)

	req := httptest.NewRequest("GET", "/forgot-password", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetPacks(t *testing.T) {
	app := setupTestApp()
	app.Get("/packs", GetPacks)

	req := httptest.NewRequest("GET", "/packs", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetJobs(t *testing.T) {
	app := setupTestApp()
	app.Get("/jobs", GetJobs)

	req := httptest.NewRequest("GET", "/jobs", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetRegistries(t *testing.T) {
	app := setupTestApp()
	app.Get("/registries", GetRegistries)

	req := httptest.NewRequest("GET", "/registries", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGenerateSitemap(t *testing.T) {
	app := setupTestApp()
	app.Get("/sitemap.xml", GenerateSitemap)

	req := httptest.NewRequest("GET", "/sitemap.xml", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "xml")
}

func TestGetResetPassword_InvalidToken(t *testing.T) {
	app := setupTestApp()
	app.Get("/reset-password", GetResetPassword)

	req := httptest.NewRequest("GET", "/reset-password?token=invalidtoken", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Should redirect to forgot-password with invalid token
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/forgot-password", resp.Header.Get("Location"))
}

func TestGetResetPassword_ValidToken(t *testing.T) {
	defer cleanupTestData(t)

	token := "validresetpagetoken"
	hashedToken := sha256.Sum256([]byte(token))
	user := models.User{
		Username:          "resetpageuser",
		Email:             "resetpageuser@test.com",
		Name:              "Reset Page User",
		EmailVerified:     true,
		ResetToken:        hex.EncodeToString(hashedToken[:]),
		ResetTokenExpires: time.Now().Add(1 * time.Hour),
	}
	err := database.DB.Create(&user).Error
	assert.NoError(t, err)

	app := setupTestApp()
	app.Get("/reset-password", GetResetPassword)

	req := httptest.NewRequest("GET", "/reset-password?token="+token, nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetNewResource_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	app.Get("/new", RequireAuth, GetNewResource)

	req := httptest.NewRequest("GET", "/new", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

func TestGetCreateOrg_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	app.Get("/orgs/new", RequireAuth, GetCreateOrg)

	req := httptest.NewRequest("GET", "/orgs/new", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

func TestGetAdminDashboard_Admin(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "dashboardadmin")

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

func TestGetEditUser_Admin(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "edituseradmin")
	target := createTestUser(t, "edittarget2")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", admin.ID)
		sess.Save()
		c.Locals("UserID", admin.ID)
		c.Locals("User", admin)
		return c.Next()
	})

	app.Get("/admin/users/:id", RequireAdmin, GetEditUser)

	req := httptest.NewRequest("GET", "/admin/users/"+toString(target.ID), nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetEditOrganization_Admin(t *testing.T) {
	defer cleanupOrgTestData(t)

	admin := createAdminUser(t, "editorgadmin")
	org := models.Organization{Name: "testeditorgpage", Description: "Test"}
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

	app.Get("/admin/organizations/:id", RequireAdmin, GetEditOrganization)

	req := httptest.NewRequest("GET", "/admin/organizations/"+toString(org.ID), nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetNewResource_Authenticated(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "newresourceuser")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Get("/new", GetNewResource)

	req := httptest.NewRequest("GET", "/new", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetCreateOrg_Authenticated(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "createorguser")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Get("/orgs/new", GetCreateOrg)

	req := httptest.NewRequest("GET", "/orgs/new", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetUserProfile_User(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "profileuser")
	createTestPack(t, user.ID, "profile-pack")

	app := setupTestApp()
	app.Get("/:username", GetUserProfile)

	req := httptest.NewRequest("GET", "/profileuser", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetUserProfile_NotFound(t *testing.T) {
	app := setupTestApp()
	app.Get("/:username", GetUserProfile)

	req := httptest.NewRequest("GET", "/nonexistentuser12345", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetUserProfile_Organization(t *testing.T) {
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "orgprofileowner")
	org := models.Organization{Name: "testprofileorg", Description: "Test org"}
	database.DB.Create(&org)
	database.DB.Create(&models.Membership{UserID: user.ID, OrganizationID: org.ID, Role: "owner"})

	// Create a pack under the org
	resource := models.NomadResource{
		Name:           "org-profile-pack",
		Description:    "Org pack",
		Type:           models.ResourceTypePack,
		UserID:         user.ID,
		OrganizationID: &org.ID,
		RepositoryURL:  "https://github.com/test/org-pack",
	}
	database.DB.Create(&resource)

	app := setupTestApp()
	app.Get("/:username", GetUserProfile)

	req := httptest.NewRequest("GET", "/testprofileorg", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetResource_Success(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "resourcepageuser")
	createTestPack(t, user.ID, "viewable-pack")

	app := setupTestApp()
	app.Get("/:username/:resourcename", GetResource)

	req := httptest.NewRequest("GET", "/resourcepageuser/viewable-pack", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetResource_NotFound(t *testing.T) {
	defer cleanupTestData(t)

	createTestUser(t, "resourcenotfounduser")

	app := setupTestApp()
	app.Get("/:username/:resourcename", GetResource)

	req := httptest.NewRequest("GET", "/resourcenotfounduser/nonexistent", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetResource_NamespaceNotFound(t *testing.T) {
	app := setupTestApp()
	app.Get("/:username/:resourcename", GetResource)

	req := httptest.NewRequest("GET", "/nonexistentuser/somepack", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetResourceVersion_Page(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "versionpageuser")
	resource := createTestPack(t, user.ID, "versioned-pack")

	// Add another version
	database.DB.Create(&models.ResourceVersion{ResourceID: resource.ID, Version: "v2.0.0", Readme: "# Test v2"})

	app := setupTestApp()
	app.Get("/:username/:resourcename/version", GetResourceVersion)

	// Version is passed as query parameter
	req := httptest.NewRequest("GET", "/versionpageuser/versioned-pack/version?version=v1.0.0", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetResourceVersion_VersionNotFound(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "versionnotfounduser")
	createTestPack(t, user.ID, "version-test-pack2")

	app := setupTestApp()
	app.Get("/:username/:resourcename/version", GetResourceVersion)

	// Version is passed as query parameter
	req := httptest.NewRequest("GET", "/versionnotfounduser/version-test-pack2/version?version=v99.0.0", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetUserProfile_WithPagination(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "paginationuser")
	// Create multiple packs to test pagination
	for i := 0; i < 15; i++ {
		createTestPack(t, user.ID, fmt.Sprintf("pagination-pack-%d", i))
	}

	app := setupTestApp()
	app.Get("/:username", GetUserProfile)

	req := httptest.NewRequest("GET", "/paginationuser?page=2", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetUserProfile_WithSearch(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "searchprofileuser")
	createTestPack(t, user.ID, "traefik-pack")
	createTestPack(t, user.ID, "redis-pack")

	app := setupTestApp()
	app.Get("/:username", GetUserProfile)

	req := httptest.NewRequest("GET", "/searchprofileuser?q=traefik", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetUserProfile_WithSort(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "sortprofileuser")
	createTestPack(t, user.ID, "sort-pack-a")
	createTestPack(t, user.ID, "sort-pack-b")

	app := setupTestApp()
	app.Get("/:username", GetUserProfile)

	// Test different sort orders
	sortOrders := []string{"latest", "popular", "name"}
	for _, sort := range sortOrders {
		req := httptest.NewRequest("GET", "/sortprofileuser?sort="+sort, nil)
		resp, err := app.Test(req)

		assert.NoError(t, err, "Sort order: %s", sort)
		assert.Equal(t, 200, resp.StatusCode, "Sort order: %s", sort)
	}
}

func TestGetUserProfile_JSON(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "jsonprofileuser")
	createTestPack(t, user.ID, "json-pack")

	app := setupTestApp()
	app.Get("/:username", GetUserProfile)

	req := httptest.NewRequest("GET", "/jsonprofileuser", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}

func TestGetResource_JSON(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "jsonresourceuser")
	createTestPack(t, user.ID, "json-resource-pack")

	app := setupTestApp()
	app.Get("/:username/:resourcename", GetResource)

	req := httptest.NewRequest("GET", "/jsonresourceuser/json-resource-pack", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}
