package handlers

import (
	"fmt"
	"net/http/httptest"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAuthenticatedApp creates an app with session middleware that authenticates as the given user
func setupAuthenticatedApp(user models.User) *fiber.App {
	app := setupTestApp()

	// Insert middleware to set up session and locals
	originalHandlers := app.Stack()
	app = setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	// Re-add any handlers if needed
	_ = originalHandlers

	return app
}

func TestGetRawResource(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "rawuser")

	// Create resource with content
	resource := models.NomadResource{
		Name:          "raw-test-job",
		Description:   "Test job for raw content",
		Type:          models.ResourceTypeJob,
		UserID:        user.ID,
		RepositoryURL: "https://github.com/test/raw-test-job",
	}
	err := database.DB.Create(&resource).Error
	require.NoError(t, err)

	version := models.ResourceVersion{
		ResourceID: resource.ID,
		Version:    "v1.0.0",
		Content:    "job \"test\" {\n  type = \"service\"\n}",
	}
	err = database.DB.Create(&version).Error
	require.NoError(t, err)

	app := setupTestApp()
	app.Get("/:username/:resourcename/raw", GetRawResource)

	req := httptest.NewRequest("GET", "/rawuser/raw-test-job/raw", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Check content type is plain text
	contentType := resp.Header.Get("Content-Type")
	assert.Contains(t, contentType, "text/plain")
}

func TestGetRawResource_NotFound(t *testing.T) {
	defer cleanupTestData(t)

	createTestUser(t, "rawuser2")

	app := setupTestApp()
	app.Get("/:username/:resourcename/raw", GetRawResource)

	req := httptest.NewRequest("GET", "/rawuser2/nonexistent/raw", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestToggleStar_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	// Route should be behind RequireAuth middleware
	app.Post("/resource/:id/star", RequireAuth, ToggleStar)

	req := httptest.NewRequest("POST", "/resource/1/star", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// RequireAuth redirects to login with 302
	assert.Equal(t, 302, resp.StatusCode)
}

func TestToggleStar_Authenticated(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "staruser")
	resource := createTestPack(t, user.ID, "starrable-pack")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		return c.Next()
	})

	app.Post("/resource/:id/star", ToggleStar)

	// Star the resource
	req := httptest.NewRequest("POST", "/resource/"+toString(resource.ID)+"/star", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify star was added
	var count int64
	database.DB.Table("user_stars").Where("user_id = ? AND nomad_resource_id = ?", user.ID, resource.ID).Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestDeleteResource_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	// Route should be behind RequireAuth middleware
	app.Delete("/resource/:id", RequireAuth, DeleteResource)

	req := httptest.NewRequest("DELETE", "/resource/1", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// RequireAuth redirects to login with 302
	assert.Equal(t, 302, resp.StatusCode)
}

func TestDeleteResource_NotOwner(t *testing.T) {
	defer cleanupTestData(t)

	owner := createTestUser(t, "owner1")
	otherUser := createTestUser(t, "other1")
	resource := createTestPack(t, owner.ID, "owned-pack")

	app := fiber.New()
	InitSession()

	// Authenticate as other user (not owner)
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", otherUser.ID)
		sess.Save()
		c.Locals("UserID", otherUser.ID)
		c.Locals("User", otherUser)
		return c.Next()
	})

	app.Delete("/resource/:id", DeleteResource)

	req := httptest.NewRequest("DELETE", "/resource/"+toString(resource.ID), nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestDeleteResource_Owner(t *testing.T) {
	defer cleanupTestData(t)

	owner := createTestUser(t, "deleteowner")
	resource := createTestPack(t, owner.ID, "deletable-pack")
	resourceID := resource.ID

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", owner.ID)
		sess.Save()
		c.Locals("UserID", owner.ID)
		c.Locals("User", owner)
		return c.Next()
	})

	app.Delete("/resource/:id", DeleteResource)

	req := httptest.NewRequest("DELETE", "/resource/"+toString(resourceID), nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify resource was deleted
	var count int64
	database.DB.Model(&models.NomadResource{}).Where("id = ?", resourceID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestGetResourceVersion(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "versionuser")

	resource := models.NomadResource{
		Name:          "versioned-pack",
		Description:   "Pack with versions",
		Type:          models.ResourceTypePack,
		UserID:        user.ID,
		RepositoryURL: "https://github.com/test/versioned",
	}
	err := database.DB.Create(&resource).Error
	require.NoError(t, err)

	// Create multiple versions
	v1 := models.ResourceVersion{ResourceID: resource.ID, Version: "v1.0.0", Readme: "# Version 1"}
	v2 := models.ResourceVersion{ResourceID: resource.ID, Version: "v2.0.0", Readme: "# Version 2"}
	database.DB.Create(&v1)
	database.DB.Create(&v2)

	app := setupTestApp()
	app.Get("/:username/:resourcename/v", GetResourceVersion)

	req := httptest.NewRequest("GET", "/versionuser/versioned-pack/v?version=v1.0.0", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// Helper to convert uint to string
func toString(id uint) string {
	return fmt.Sprintf("%d", id)
}

func TestEscapeLikeString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal", "normal"},
		{"with%percent", "with\\%percent"},
		{"with_underscore", "with\\_underscore"},
		{"with\\backslash", "with\\\\backslash"},
		{"combo%_\\test", "combo\\%\\_\\\\test"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeLikeString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Resource Creation Tests

func TestPostNewResource_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	app.Post("/new", RequireAuth, PostNewResource)

	payload := strings.NewReader("name=test-resource&type=pack&version=v1.0.0")
	req := httptest.NewRequest("POST", "/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	// RequireAuth redirects to login
	assert.Equal(t, 302, resp.StatusCode)
}

func TestPostNewResource_MissingFields(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "resourcecreator")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Post("/new", PostNewResource)

	// Missing version
	payload := strings.NewReader("name=test-resource&type=pack")
	req := httptest.NewRequest("POST", "/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostNewResource_Success(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "packcreator")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Post("/new", PostNewResource)

	payload := strings.NewReader("name=my-new-pack&type=pack&version=v1.0.0&description=A+test+pack&repository_url=https://github.com/test/my-new-pack")
	req := httptest.NewRequest("POST", "/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify resource was created
	var resource models.NomadResource
	err = database.DB.Where("name = ? AND user_id = ?", "my-new-pack", user.ID).First(&resource).Error
	assert.NoError(t, err)
	assert.Equal(t, "A test pack", resource.Description)
	assert.Equal(t, models.ResourceTypePack, resource.Type)
}

func TestPostNewResource_DuplicateName(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "dupcreator")
	createTestPack(t, user.ID, "existing-pack")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Post("/new", PostNewResource)

	// Try to create resource with same name
	payload := strings.NewReader("name=existing-pack&type=pack&version=v1.0.0")
	req := httptest.NewRequest("POST", "/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostNewResource_JobType(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "jobcreator")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Post("/new", PostNewResource)

	payload := strings.NewReader("name=my-new-job&type=job&version=v1.0.0&description=A+test+job")
	req := httptest.NewRequest("POST", "/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify job was created
	var resource models.NomadResource
	err = database.DB.Where("name = ? AND user_id = ?", "my-new-job", user.ID).First(&resource).Error
	assert.NoError(t, err)
	assert.Equal(t, models.ResourceTypeJob, resource.Type)
}

func TestPostNewResource_WithTags(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "tagcreator")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Post("/new", PostNewResource)

	payload := strings.NewReader("name=tagged-pack&type=pack&version=v1.0.0&tags=database,redis,cache")
	req := httptest.NewRequest("POST", "/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify tags were created
	var resource models.NomadResource
	err = database.DB.Preload("Tags").Where("name = ?", "tagged-pack").First(&resource).Error
	assert.NoError(t, err)
	assert.Len(t, resource.Tags, 3)
}

func TestGetRawResourceVersion(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "rawversionuser")

	resource := models.NomadResource{
		Name:          "versioned-raw-job",
		Description:   "Test job with versions",
		Type:          models.ResourceTypeJob,
		UserID:        user.ID,
		RepositoryURL: "https://github.com/test/versioned-raw",
	}
	err := database.DB.Create(&resource).Error
	require.NoError(t, err)

	// Create multiple versions with different content
	v1 := models.ResourceVersion{ResourceID: resource.ID, Version: "v1.0.0", Content: "job v1 content"}
	v2 := models.ResourceVersion{ResourceID: resource.ID, Version: "v2.0.0", Content: "job v2 content"}
	database.DB.Create(&v1)
	database.DB.Create(&v2)

	app := setupTestApp()
	app.Get("/:username/:resourcename/raw/:version", GetRawResourceVersion)

	// Get v1 content
	req := httptest.NewRequest("GET", "/rawversionuser/versioned-raw-job/raw/v1.0.0", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/plain")
}

// Webhook Tests

func TestHandleWebhook_NotFound(t *testing.T) {
	app := setupTestApp()
	app.Post("/webhook/:id", HandleWebhook)

	req := httptest.NewRequest("POST", "/webhook/99999?secret=test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestHandleWebhook_InvalidSecret(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "webhookuser")
	resource := models.NomadResource{
		Name:          "webhook-pack",
		Type:          models.ResourceTypePack,
		UserID:        user.ID,
		WebhookSecret: "correct-secret",
	}
	database.DB.Create(&resource)

	app := setupTestApp()
	app.Post("/webhook/:id", HandleWebhook)

	req := httptest.NewRequest("POST", "/webhook/"+toString(resource.ID)+"?secret=wrong-secret", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)

	// Verify failure was logged
	var updated models.NomadResource
	database.DB.First(&updated, resource.ID)
	assert.Equal(t, "failure", updated.LastWebhookStatus)
}

func TestHandleWebhook_MissingSecret(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "webhookuser2")
	resource := models.NomadResource{
		Name:          "webhook-pack2",
		Type:          models.ResourceTypePack,
		UserID:        user.ID,
		WebhookSecret: "some-secret",
	}
	database.DB.Create(&resource)

	app := setupTestApp()
	app.Post("/webhook/:id", HandleWebhook)

	req := httptest.NewRequest("POST", "/webhook/"+toString(resource.ID), nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestHandleWebhook_ValidSecret(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "webhookuser3")
	resource := models.NomadResource{
		Name:          "webhook-pack3",
		Type:          models.ResourceTypePack,
		UserID:        user.ID,
		WebhookSecret: "valid-secret-123",
	}
	database.DB.Create(&resource)
	database.DB.Create(&models.ResourceVersion{ResourceID: resource.ID, Version: "v1.0.0"})

	app := setupTestApp()
	app.Post("/webhook/:id", HandleWebhook)

	// Valid secret but no tag event (ping)
	req := httptest.NewRequest("POST", "/webhook/"+toString(resource.ID)+"?secret=valid-secret-123", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify success was logged
	var updated models.NomadResource
	database.DB.First(&updated, resource.ID)
	assert.Equal(t, "success", updated.LastWebhookStatus)
}

func TestHandleWebhook_GitHubTagEvent(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "webhookuser4")
	resource := models.NomadResource{
		Name:          "webhook-pack4",
		Type:          models.ResourceTypePack,
		UserID:        user.ID,
		WebhookSecret: "github-secret",
		RepositoryURL: "https://github.com/test/webhook-pack4",
	}
	database.DB.Create(&resource)
	database.DB.Create(&models.ResourceVersion{ResourceID: resource.ID, Version: "v1.0.0"})

	app := setupTestApp()
	app.Post("/webhook/:id", HandleWebhook)

	// GitHub tag create event
	payload := strings.NewReader(`{"ref": "v2.0.0", "ref_type": "tag"}`)
	req := httptest.NewRequest("POST", "/webhook/"+toString(resource.ID)+"?secret=github-secret", payload)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "create")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// PostNewVersion Tests

func TestPostNewVersion_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	app.Post("/resource/:id/version", RequireAuth, PostNewVersion)

	payload := strings.NewReader("version=v2.0.0")
	req := httptest.NewRequest("POST", "/resource/1/version", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode) // Redirect to login
}

func TestPostNewVersion_MissingVersion(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "versioncreator")
	resource := createTestPack(t, user.ID, "version-test-pack")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Post("/resource/:id/version", PostNewVersion)

	// Missing version field
	payload := strings.NewReader("")
	req := httptest.NewRequest("POST", "/resource/"+toString(resource.ID)+"/version", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostNewVersion_ResourceNotFound(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "versioncreator2")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Post("/resource/:id/version", PostNewVersion)

	payload := strings.NewReader("version=v2.0.0")
	req := httptest.NewRequest("POST", "/resource/99999/version", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPostNewVersion_Success(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "versioncreator3")
	resource := createTestPack(t, user.ID, "version-success-pack")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Post("/resource/:id/version", PostNewVersion)

	payload := strings.NewReader("version=v2.0.0")
	req := httptest.NewRequest("POST", "/resource/"+toString(resource.ID)+"/version", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify version was created
	var version models.ResourceVersion
	err = database.DB.Where("resource_id = ? AND version = ?", resource.ID, "v2.0.0").First(&version).Error
	assert.NoError(t, err)
}

// PostEditResource Tests

func TestPostEditResource_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	app.Post("/resource/:id/edit", RequireAuth, PostEditResource)

	payload := strings.NewReader("name=newname&type=pack")
	req := httptest.NewRequest("POST", "/resource/1/edit", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
}

func TestPostEditResource_NotOwner(t *testing.T) {
	defer cleanupTestData(t)

	owner := createTestUser(t, "editowner")
	otherUser := createTestUser(t, "editother")
	resource := createTestPack(t, owner.ID, "edit-test-pack")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", otherUser.ID)
		sess.Save()
		c.Locals("UserID", otherUser.ID)
		c.Locals("User", otherUser)
		return c.Next()
	})

	app.Post("/resource/:id/edit", PostEditResource)

	payload := strings.NewReader("name=hacked&type=pack&description=hacked")
	req := httptest.NewRequest("POST", "/resource/"+toString(resource.ID)+"/edit", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestPostEditResource_Success(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "edituser")
	resource := createTestPack(t, user.ID, "editable-pack")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Post("/resource/:id/edit", PostEditResource)

	payload := strings.NewReader("name=editable-pack&type=pack&description=Updated+description&license=MIT")
	req := httptest.NewRequest("POST", "/resource/"+toString(resource.ID)+"/edit", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify resource was updated
	var updated models.NomadResource
	database.DB.First(&updated, resource.ID)
	assert.Equal(t, "Updated description", updated.Description)
	assert.Equal(t, "MIT", updated.License)
}

// PostResetWebhookSecret Tests

func TestPostResetWebhookSecret_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	app.Post("/resource/:id/reset-webhook", RequireAuth, PostResetWebhookSecret)

	req := httptest.NewRequest("POST", "/resource/1/reset-webhook", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
}

func TestPostResetWebhookSecret_NotOwner(t *testing.T) {
	defer cleanupTestData(t)

	owner := createTestUser(t, "webhookowner")
	otherUser := createTestUser(t, "webhookother")
	resource := models.NomadResource{
		Name:          "webhook-reset-pack",
		Type:          models.ResourceTypePack,
		UserID:        owner.ID,
		WebhookSecret: "old-secret",
	}
	database.DB.Create(&resource)

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", otherUser.ID)
		sess.Save()
		c.Locals("UserID", otherUser.ID)
		c.Locals("User", otherUser)
		return c.Next()
	})

	app.Post("/resource/:id/reset-webhook", PostResetWebhookSecret)

	req := httptest.NewRequest("POST", "/resource/"+toString(resource.ID)+"/reset-webhook", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)

	// Verify secret wasn't changed
	var updated models.NomadResource
	database.DB.First(&updated, resource.ID)
	assert.Equal(t, "old-secret", updated.WebhookSecret)
}

func TestPostResetWebhookSecret_Success(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "webhookreset")
	resource := models.NomadResource{
		Name:          "webhook-reset-pack2",
		Type:          models.ResourceTypePack,
		UserID:        user.ID,
		WebhookSecret: "old-secret-123",
	}
	database.DB.Create(&resource)

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Post("/resource/:id/reset-webhook", PostResetWebhookSecret)

	req := httptest.NewRequest("POST", "/resource/"+toString(resource.ID)+"/reset-webhook", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify secret was changed
	var updated models.NomadResource
	database.DB.First(&updated, resource.ID)
	assert.NotEqual(t, "old-secret-123", updated.WebhookSecret)
	assert.NotEmpty(t, updated.WebhookSecret)
}

// GetNewVersion Page Tests

func TestGetNewVersion_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	app.Get("/resource/:id/new-version", RequireAuth, GetNewVersion)

	req := httptest.NewRequest("GET", "/resource/1/new-version", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

func TestGetNewVersion_Authenticated(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "newversionpage")
	resource := createTestPack(t, user.ID, "newversion-pack")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Get("/resource/:id/new-version", GetNewVersion)

	req := httptest.NewRequest("GET", "/resource/"+toString(resource.ID)+"/new-version", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// GetEditResource Page Tests

func TestGetEditResource_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	app.Get("/:username/:resourcename/edit", RequireAuth, GetEditResource)

	req := httptest.NewRequest("GET", "/someuser/somepack/edit", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

func TestGetEditResource_NotOwner(t *testing.T) {
	defer cleanupTestData(t)

	owner := createTestUser(t, "editpageowner")
	otherUser := createTestUser(t, "editpageother")
	createTestPack(t, owner.ID, "editpage-pack")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", otherUser.ID)
		sess.Save()
		c.Locals("UserID", otherUser.ID)
		c.Locals("User", otherUser)
		return c.Next()
	})

	app.Get("/:username/:resourcename/edit", GetEditResource)

	req := httptest.NewRequest("GET", "/editpageowner/editpage-pack/edit", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestGetEditResource_Owner(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "editpageuser")
	createTestPack(t, user.ID, "myeditpack")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Get("/:username/:resourcename/edit", GetEditResource)

	req := httptest.NewRequest("GET", "/editpageuser/myeditpack/edit", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetEditResource_NotFound(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "editpagenotfound")

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Get("/:username/:resourcename/edit", GetEditResource)

	req := httptest.NewRequest("GET", "/editpagenotfound/nonexistent/edit", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

// Input validation edge cases

func TestPostNewResource_EmptyVersion(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "emptyversion")

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

	app.Post("/new", PostNewResource)

	// Empty version
	payload := strings.NewReader("name=testres&type=pack&version=")
	req := httptest.NewRequest("POST", "/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostNewResource_EmptyName(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "emptyname")

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

	app.Post("/new", PostNewResource)

	// Empty name
	payload := strings.NewReader("name=&type=pack&version=v1.0.0")
	req := httptest.NewRequest("POST", "/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostNewResource_InvalidTags(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "invalidtags")

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

	app.Post("/new", PostNewResource)

	// Tags with invalid characters are silently ignored
	payload := strings.NewReader("name=invalidtagsres&type=pack&version=v1.0.0&tags=x,valid-tag,ab@cd")
	req := httptest.NewRequest("POST", "/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode) // Resource created, invalid tags ignored

	// Verify only valid tag was added
	var resource models.NomadResource
	database.DB.Preload("Tags").Where("name = ?", "invalidtagsres").First(&resource)
	// "x" is too short (< 2 chars), "ab@cd" has invalid char
	// Only "valid-tag" should be added
	assert.Equal(t, 1, len(resource.Tags))
	assert.Equal(t, "valid-tag", resource.Tags[0].Name)
}

func TestPostNewVersion_EmptyVersion(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "emptyversionuser")

	resource := models.NomadResource{
		Name:   "emptyversionres",
		Type:   models.ResourceTypePack,
		UserID: user.ID,
	}
	database.DB.Create(&resource)

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

	app.Post("/resource/:id/versions", PostNewVersion)

	payload := strings.NewReader("version=")
	req := httptest.NewRequest("POST", "/resource/"+toString(resource.ID)+"/versions", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestGetRawResource_InvalidFilePath(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "rawfileuser")

	resource := models.NomadResource{
		Name:   "rawfileres",
		Type:   models.ResourceTypeJob,
		UserID: user.ID,
	}
	database.DB.Create(&resource)

	version := models.ResourceVersion{
		ResourceID: resource.ID,
		Version:    "v1.0.0",
		Content:    "job content",
	}
	database.DB.Create(&version)

	app := fiber.New()

	app.Get("/:username/:resourcename/raw", GetRawResource)

	// Test without version query param
	req := httptest.NewRequest("GET", "/"+user.Username+"/rawfileres/raw", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestToggleStar_ResourceNotFound(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "starnotfound")

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

	app.Post("/resource/:id/star", ToggleStar)

	req := httptest.NewRequest("POST", "/resource/99999/star", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestDeleteResource_ResourceNotFound(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "deletenotfound")

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

	app.Delete("/resource/:id", DeleteResource)

	req := httptest.NewRequest("DELETE", "/resource/99999", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}
