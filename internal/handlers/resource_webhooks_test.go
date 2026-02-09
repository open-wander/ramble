package handlers

import (
	"net/http/httptest"
	"os"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

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
	assert.Equal(t, 401, resp.StatusCode)

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
	assert.Equal(t, 401, resp.StatusCode)
}

func TestHandleWebhook_ValidSecret(t *testing.T) {
	defer cleanupTestData(t)

	// Enable legacy auth for this test
	os.Setenv("ALLOW_LEGACY_WEBHOOK_AUTH", "true")
	defer os.Unsetenv("ALLOW_LEGACY_WEBHOOK_AUTH")

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

	// GitHub tag create event with new signature format
	body := []byte(`{"ref": "v2.0.0", "ref_type": "tag"}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := computeTestSignature(body, timestamp, "github-secret")

	req := httptest.NewRequest("POST", "/webhook/"+toString(resource.ID), strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "create")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-Webhook-Timestamp", timestamp)

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestHandleWebhook_TimestampReplayProtection(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "webhookuser5")
	resource := models.NomadResource{
		Name:          "webhook-pack5",
		Type:          models.ResourceTypePack,
		UserID:        user.ID,
		WebhookSecret: "test-secret",
	}
	database.DB.Create(&resource)
	database.DB.Create(&models.ResourceVersion{ResourceID: resource.ID, Version: "v1.0.0"})

	app := setupTestApp()
	app.Post("/webhook/:id", HandleWebhook)

	body := []byte(`{"ref": "main"}`)

	// Test 1: Timestamp 6 minutes old -> expect 401
	oldTimestamp := strconv.FormatInt(time.Now().Unix()-360, 10)
	oldSignature := computeTestSignature(body, oldTimestamp, "test-secret")

	req := httptest.NewRequest("POST", "/webhook/"+toString(resource.ID), strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", oldSignature)
	req.Header.Set("X-Webhook-Timestamp", oldTimestamp)

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	// Test 2: Current timestamp -> expect 200
	currentTimestamp := strconv.FormatInt(time.Now().Unix(), 10)
	currentSignature := computeTestSignature(body, currentTimestamp, "test-secret")

	req = httptest.NewRequest("POST", "/webhook/"+toString(resource.ID), strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", currentSignature)
	req.Header.Set("X-Webhook-Timestamp", currentTimestamp)

	resp, err = app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestHandleWebhook_NewSignatureFormat(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "webhookuser6")
	resource := models.NomadResource{
		Name:          "webhook-pack6",
		Type:          models.ResourceTypePack,
		UserID:        user.ID,
		WebhookSecret: "signature-test-secret",
	}
	database.DB.Create(&resource)
	database.DB.Create(&models.ResourceVersion{ResourceID: resource.ID, Version: "v1.0.0"})

	app := setupTestApp()
	app.Post("/webhook/:id", HandleWebhook)

	// Test new signature format: HMAC-SHA256 over "timestamp.body"
	body := []byte(`{"event": "push"}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := computeTestSignature(body, timestamp, "signature-test-secret")

	req := httptest.NewRequest("POST", "/webhook/"+toString(resource.ID), strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-Webhook-Timestamp", timestamp)

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify success was logged
	var updated models.NomadResource
	database.DB.First(&updated, resource.ID)
	assert.Equal(t, "success", updated.LastWebhookStatus)
}

func TestHandleWebhook_LegacyAuthDeprecationWarning(t *testing.T) {
	defer cleanupTestData(t)

	// Enable legacy auth
	os.Setenv("ALLOW_LEGACY_WEBHOOK_AUTH", "true")
	defer os.Unsetenv("ALLOW_LEGACY_WEBHOOK_AUTH")

	user := createTestUser(t, "webhookuser7")
	resource := models.NomadResource{
		Name:          "webhook-pack7",
		Type:          models.ResourceTypePack,
		UserID:        user.ID,
		WebhookSecret: "legacy-secret",
	}
	database.DB.Create(&resource)
	database.DB.Create(&models.ResourceVersion{ResourceID: resource.ID, Version: "v1.0.0"})

	app := setupTestApp()
	app.Post("/webhook/:id", HandleWebhook)

	// Send request with query param secret (no headers)
	req := httptest.NewRequest("POST", "/webhook/"+toString(resource.ID)+"?secret=legacy-secret", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify X-Deprecation-Warning header is present
	deprecationHeader := resp.Header.Get("X-Deprecation-Warning")
	assert.NotEmpty(t, deprecationHeader)
	assert.Contains(t, deprecationHeader, "deprecated")
	assert.Contains(t, deprecationHeader, "X-Hub-Signature-256")
}

func TestHandleWebhook_LegacyAuthDisabled(t *testing.T) {
	defer cleanupTestData(t)

	// Ensure legacy auth is disabled (unset or false)
	os.Unsetenv("ALLOW_LEGACY_WEBHOOK_AUTH")

	user := createTestUser(t, "webhookuser8")
	resource := models.NomadResource{
		Name:          "webhook-pack8",
		Type:          models.ResourceTypePack,
		UserID:        user.ID,
		WebhookSecret: "legacy-disabled-secret",
	}
	database.DB.Create(&resource)

	app := setupTestApp()
	app.Post("/webhook/:id", HandleWebhook)

	// Send request with only query param secret (legacy auth)
	req := httptest.NewRequest("POST", "/webhook/"+toString(resource.ID)+"?secret=legacy-disabled-secret", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode) // Legacy auth not allowed
}

// PostNewVersion Tests

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
