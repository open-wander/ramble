package handlers

import (
	"net/http/httptest"
	"rmbl/internal/database"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

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
