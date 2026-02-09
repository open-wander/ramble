package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"rmbl/internal/models"
)

// computeTestSignature computes HMAC-SHA256 signature over "timestamp.body" format
func computeTestSignature(body []byte, timestamp, secret string) string {
	payload := fmt.Sprintf("%s.%s", timestamp, body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

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

// toString converts uint to string for URL params
func toString(id uint) string {
	return fmt.Sprintf("%d", id)
}
