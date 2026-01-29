package handlers

import (
	"net/http/httptest"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestComputeAuditChecksum(t *testing.T) {
	audit := &models.AuditLog{
		CreatedAt:  time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		ActorID:    123,
		ActorName:  "testuser",
		Action:     "test.action",
		TargetID:   456,
		TargetName: "target",
		TargetType: "resource",
		Details:    `{"key":"value"}`,
		IPAddress:  "192.168.1.1",
		UserAgent:  "Test-Agent",
		RequestID:  "req-123",
	}

	checksum := computeAuditChecksum(audit)

	// Checksum should be a 64-character hex string (SHA-256)
	assert.Equal(t, 64, len(checksum))

	// Same input should produce same checksum
	checksum2 := computeAuditChecksum(audit)
	assert.Equal(t, checksum, checksum2)
}

func TestComputeAuditChecksum_DifferentInputs(t *testing.T) {
	audit1 := &models.AuditLog{
		CreatedAt:  time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		ActorID:    123,
		ActorName:  "testuser",
		Action:     "test.action",
		TargetID:   456,
		TargetName: "target",
		TargetType: "resource",
	}

	audit2 := &models.AuditLog{
		CreatedAt:  time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		ActorID:    124, // Different actor
		ActorName:  "testuser",
		Action:     "test.action",
		TargetID:   456,
		TargetName: "target",
		TargetType: "resource",
	}

	checksum1 := computeAuditChecksum(audit1)
	checksum2 := computeAuditChecksum(audit2)

	assert.NotEqual(t, checksum1, checksum2)
}

func TestAuditLog_WithUser(t *testing.T) {
	logger.Init()

	app := fiber.New()

	// Setup middleware to set user locals
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("UserID", uint(123))
		c.Locals("User", models.User{Username: "testuser"})
		c.Locals("requestid", "test-request-id")
		return c.Next()
	})

	var auditCalled bool
	app.Get("/test", func(c *fiber.Ctx) error {
		AuditLog(c, "test.action", "resource", 456, "test-resource", map[string]interface{}{
			"key": "value",
		})
		auditCalled = true
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("User-Agent", "Test-Agent")

	_, err := app.Test(req)
	assert.Nil(t, err)
	assert.True(t, auditCalled)

	// Verify audit log was created in database
	var audit models.AuditLog
	result := database.DB.Where("action = ?", "test.action").First(&audit)
	assert.Nil(t, result.Error)
	assert.Equal(t, "test.action", audit.Action)
	assert.Equal(t, uint(123), audit.ActorID)
	assert.Equal(t, "testuser", audit.ActorName)
	assert.Equal(t, uint(456), audit.TargetID)
	assert.Equal(t, "test-resource", audit.TargetName)
	assert.Equal(t, "resource", audit.TargetType)
	assert.Contains(t, audit.Details, "value")
	assert.NotEmpty(t, audit.Checksum)
}

func TestAuditLog_WithoutUser(t *testing.T) {
	logger.Init()

	app := fiber.New()

	// No user middleware
	var auditCalled bool
	app.Get("/test", func(c *fiber.Ctx) error {
		AuditLog(c, "anonymous.action", "system", 0, "", nil)
		auditCalled = true
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, err := app.Test(req)
	assert.Nil(t, err)
	assert.True(t, auditCalled)

	// Verify audit log was created
	var audit models.AuditLog
	result := database.DB.Where("action = ?", "anonymous.action").First(&audit)
	assert.Nil(t, result.Error)
	assert.Equal(t, uint(0), audit.ActorID)
	assert.Empty(t, audit.ActorName)
}

func TestAuditLog_NilDetails(t *testing.T) {
	logger.Init()

	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		AuditLog(c, "no.details", "system", 0, "", nil)
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, err := app.Test(req)
	assert.Nil(t, err)

	// Verify audit log was created with empty details
	var audit models.AuditLog
	result := database.DB.Where("action = ?", "no.details").First(&audit)
	assert.Nil(t, result.Error)
	assert.Empty(t, audit.Details)
}

func TestAuditLogNoContext(t *testing.T) {
	logger.Init()

	AuditLogNoContext("background.action", 999, "system", "job", 1, "cleanup", map[string]interface{}{
		"processed": 100,
	})

	// Verify audit log was created
	var audit models.AuditLog
	result := database.DB.Where("action = ?", "background.action").First(&audit)
	assert.Nil(t, result.Error)
	assert.Equal(t, "background.action", audit.Action)
	assert.Equal(t, uint(999), audit.ActorID)
	assert.Equal(t, "system", audit.ActorName)
	assert.Equal(t, "job", audit.TargetType)
	assert.Equal(t, uint(1), audit.TargetID)
	assert.Equal(t, "cleanup", audit.TargetName)
	assert.Contains(t, audit.Details, "100")
	assert.Empty(t, audit.IPAddress)
	assert.Empty(t, audit.UserAgent)
	assert.Empty(t, audit.RequestID)
	assert.NotEmpty(t, audit.Checksum)
}

func TestAuditLogNoContext_NilDetails(t *testing.T) {
	logger.Init()

	AuditLogNoContext("simple.action", 1, "admin", "config", 0, "", nil)

	var audit models.AuditLog
	result := database.DB.Where("action = ?", "simple.action").First(&audit)
	assert.Nil(t, result.Error)
	assert.Empty(t, audit.Details)
}

func TestAuditLog_CapturesIP(t *testing.T) {
	logger.Init()

	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		AuditLog(c, "ip.capture", "system", 0, "", nil)
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.195")

	_, err := app.Test(req)
	assert.Nil(t, err)

	var audit models.AuditLog
	result := database.DB.Where("action = ?", "ip.capture").First(&audit)
	assert.Nil(t, result.Error)
	// IP may vary based on Fiber's IP resolution
	assert.NotEmpty(t, audit.IPAddress)
}

func TestAuditLog_CapturesUserAgent(t *testing.T) {
	logger.Init()

	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		AuditLog(c, "ua.capture", "system", 0, "", nil)
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("User-Agent", "CustomAgent/1.0")

	_, err := app.Test(req)
	assert.Nil(t, err)

	var audit models.AuditLog
	result := database.DB.Where("action = ?", "ua.capture").First(&audit)
	assert.Nil(t, result.Error)
	assert.Equal(t, "CustomAgent/1.0", audit.UserAgent)
}
