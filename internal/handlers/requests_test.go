package handlers

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestGetRequests_DefaultParams(t *testing.T) {
	app := setupTestApp()
	app.Get("/requests", GetRequests)

	req := httptest.NewRequest("GET", "/requests", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetRequests_WithStatusFilter(t *testing.T) {
	app := setupTestApp()
	app.Get("/requests", GetRequests)

	statuses := []string{"open", "in_progress", "completed", "closed", "all"}
	for _, status := range statuses {
		req := httptest.NewRequest("GET", "/requests?status="+status, nil)
		resp, err := app.Test(req)

		assert.Nil(t, err, "Status: %s", status)
		assert.Equal(t, 200, resp.StatusCode, "Status: %s", status)
	}
}

func TestGetRequests_InvalidStatusFallback(t *testing.T) {
	app := setupTestApp()
	app.Get("/requests", GetRequests)

	// Invalid status should fall back to "open"
	req := httptest.NewRequest("GET", "/requests?status=invalid", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetRequests_WithTypeFilter(t *testing.T) {
	app := setupTestApp()
	app.Get("/requests", GetRequests)

	types := []string{"pack", "job", ""}
	for _, reqType := range types {
		req := httptest.NewRequest("GET", "/requests?type="+reqType, nil)
		resp, err := app.Test(req)

		assert.Nil(t, err, "Type: %s", reqType)
		assert.Equal(t, 200, resp.StatusCode, "Type: %s", reqType)
	}
}

func TestGetRequests_InvalidTypeFallback(t *testing.T) {
	app := setupTestApp()
	app.Get("/requests", GetRequests)

	req := httptest.NewRequest("GET", "/requests?type=invalid", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetRequests_WithSortOptions(t *testing.T) {
	app := setupTestApp()
	app.Get("/requests", GetRequests)

	sortOptions := []string{"votes", "newest", "oldest"}
	for _, sort := range sortOptions {
		req := httptest.NewRequest("GET", "/requests?sort="+sort, nil)
		resp, err := app.Test(req)

		assert.Nil(t, err, "Sort: %s", sort)
		assert.Equal(t, 200, resp.StatusCode, "Sort: %s", sort)
	}
}

func TestGetRequests_InvalidSortFallback(t *testing.T) {
	app := setupTestApp()
	app.Get("/requests", GetRequests)

	req := httptest.NewRequest("GET", "/requests?sort=invalid", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetRequests_WithPagination(t *testing.T) {
	app := setupTestApp()
	app.Get("/requests", GetRequests)

	req := httptest.NewRequest("GET", "/requests?page=2", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetRequests_InvalidPageFallback(t *testing.T) {
	app := setupTestApp()
	app.Get("/requests", GetRequests)

	// Test invalid page values
	invalidPages := []string{"0", "-1", "abc", "1001"}
	for _, page := range invalidPages {
		req := httptest.NewRequest("GET", "/requests?page="+page, nil)
		resp, err := app.Test(req)

		assert.Nil(t, err, "Page: %s", page)
		assert.Equal(t, 200, resp.StatusCode, "Page: %s", page)
	}
}

func TestGetRequests_CombinedFilters(t *testing.T) {
	app := setupTestApp()
	app.Get("/requests", GetRequests)

	req := httptest.NewRequest("GET", "/requests?status=open&type=pack&sort=newest&page=1", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetRequest_NotFound(t *testing.T) {
	app := setupTestApp()
	app.Get("/requests/:id", GetRequest)

	req := httptest.NewRequest("GET", "/requests/99999", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetRequest_Found(t *testing.T) {
	// Create a test user and request
	user := models.User{
		Username: "reqtestuser",
		Email:    "reqtest@test.com",
	}
	database.DB.Create(&user)
	defer database.DB.Delete(&user)

	request := models.PackRequest{
		Title:       "Test Request",
		Description: "Test description",
		Type:        models.ResourceTypePack,
		Status:      models.RequestStatusOpen,
		UserID:      user.ID,
	}
	database.DB.Create(&request)
	defer database.DB.Delete(&request)

	app := setupTestApp()
	app.Get("/requests/:id", GetRequest)

	req := httptest.NewRequest("GET", "/requests/"+uintToString(request.ID), nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetNewRequest(t *testing.T) {
	app := setupTestApp()
	app.Get("/requests/new", GetNewRequest)

	req := httptest.NewRequest("GET", "/requests/new", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPostNewRequest_ValidationErrors(t *testing.T) {
	// Create a test user
	user := models.User{
		Username: "postuser",
		Email:    "postuser@test.com",
	}
	database.DB.Create(&user)
	defer database.DB.Delete(&user)

	app := setupTestApp()
	// Add session middleware that sets user_id
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		return c.Next()
	})
	app.Post("/requests", PostNewRequest)

	testCases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"Empty title", "title=&description=desc&type=pack", 400},
		{"Title too short", "title=ab&description=desc&type=pack", 400},
		{"Title too long", "title=" + strings.Repeat("a", 201) + "&description=desc&type=pack", 400},
		{"Invalid type", "title=Valid+Title&description=desc&type=invalid", 400},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/requests", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			resp, err := app.Test(req)
			assert.Nil(t, err)
			assert.Equal(t, tc.wantStatus, resp.StatusCode)
		})
	}
}

func TestPostNewRequest_Success(t *testing.T) {
	// Create a test user with unique identifier
	uniqueID := fmt.Sprintf("%d", time.Now().UnixNano())
	user := models.User{
		Username: "successuser" + uniqueID,
		Email:    "successuser" + uniqueID + "@test.com",
	}
	database.DB.Create(&user)
	defer database.DB.Delete(&user)

	app := setupTestApp()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		return c.Next()
	})
	app.Post("/requests", PostNewRequest)

	testTitle := "Valid Test Request " + uniqueID
	body := url.Values{}
	body.Set("title", testTitle)
	body.Set("description", "This is a valid description")
	body.Set("type", "pack")

	req := httptest.NewRequest("POST", "/requests", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify request was created
	var created models.PackRequest
	database.DB.Where("title = ?", testTitle).First(&created)
	assert.Equal(t, testTitle, created.Title)
	defer database.DB.Delete(&created)
}

func TestGetEditRequest_NotFound(t *testing.T) {
	user := models.User{Username: "edituser1", Email: "edituser1@test.com"}
	database.DB.Create(&user)
	defer database.DB.Delete(&user)

	app := setupTestApp()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		return c.Next()
	})
	app.Get("/requests/:id/edit", GetEditRequest)

	req := httptest.NewRequest("GET", "/requests/99999/edit", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetEditRequest_NotOwner(t *testing.T) {
	owner := models.User{Username: "reqowner1", Email: "reqowner1@test.com"}
	database.DB.Create(&owner)
	defer database.DB.Delete(&owner)

	otherUser := models.User{Username: "reqother1", Email: "reqother1@test.com"}
	database.DB.Create(&otherUser)
	defer database.DB.Delete(&otherUser)

	packRequest := models.PackRequest{
		Title:  "Owned Request",
		UserID: owner.ID,
		Status: models.RequestStatusOpen,
	}
	database.DB.Create(&packRequest)
	defer database.DB.Delete(&packRequest)

	app := setupTestApp()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", otherUser.ID) // Different user
		sess.Save()
		return c.Next()
	})
	app.Get("/requests/:id/edit", GetEditRequest)

	req := httptest.NewRequest("GET", "/requests/"+uintToString(packRequest.ID)+"/edit", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestGetEditRequest_NotOpen(t *testing.T) {
	user := models.User{Username: "closedowner", Email: "closedowner@test.com"}
	database.DB.Create(&user)
	defer database.DB.Delete(&user)

	packRequest := models.PackRequest{
		Title:  "Closed Request",
		UserID: user.ID,
		Status: models.RequestStatusClosed,
	}
	database.DB.Create(&packRequest)
	defer database.DB.Delete(&packRequest)

	app := setupTestApp()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		return c.Next()
	})
	app.Get("/requests/:id/edit", GetEditRequest)

	req := httptest.NewRequest("GET", "/requests/"+uintToString(packRequest.ID)+"/edit", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestDeleteUserRequest_Success(t *testing.T) {
	user := models.User{Username: "deleteuser", Email: "deleteuser@test.com"}
	database.DB.Create(&user)
	defer database.DB.Delete(&user)

	packRequest := models.PackRequest{
		Title:  "Request to Delete",
		UserID: user.ID,
		Status: models.RequestStatusOpen,
	}
	database.DB.Create(&packRequest)
	// Note: no defer delete since we're testing deletion

	app := setupTestApp()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		return c.Next()
	})
	app.Delete("/requests/:id", DeleteUserRequest)

	req := httptest.NewRequest("DELETE", "/requests/"+uintToString(packRequest.ID), nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify deleted
	var count int64
	database.DB.Model(&models.PackRequest{}).Where("id = ?", packRequest.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestDeleteUserRequest_NotOwner(t *testing.T) {
	owner := models.User{Username: "delowner", Email: "delowner@test.com"}
	database.DB.Create(&owner)
	defer database.DB.Delete(&owner)

	otherUser := models.User{Username: "delother", Email: "delother@test.com"}
	database.DB.Create(&otherUser)
	defer database.DB.Delete(&otherUser)

	packRequest := models.PackRequest{
		Title:  "Owned Request",
		UserID: owner.ID,
		Status: models.RequestStatusOpen,
	}
	database.DB.Create(&packRequest)
	defer database.DB.Delete(&packRequest)

	app := setupTestApp()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", otherUser.ID)
		sess.Save()
		return c.Next()
	})
	app.Delete("/requests/:id", DeleteUserRequest)

	req := httptest.NewRequest("DELETE", "/requests/"+uintToString(packRequest.ID), nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestDeleteUserRequest_NotOpen(t *testing.T) {
	user := models.User{Username: "delclosed", Email: "delclosed@test.com"}
	database.DB.Create(&user)
	defer database.DB.Delete(&user)

	packRequest := models.PackRequest{
		Title:  "Closed Request",
		UserID: user.ID,
		Status: models.RequestStatusInProgress,
	}
	database.DB.Create(&packRequest)
	defer database.DB.Delete(&packRequest)

	app := setupTestApp()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		return c.Next()
	})
	app.Delete("/requests/:id", DeleteUserRequest)

	req := httptest.NewRequest("DELETE", "/requests/"+uintToString(packRequest.ID), nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

// Helper function to convert uint to string
func uintToString(id uint) string {
	return fmt.Sprintf("%d", id)
}
