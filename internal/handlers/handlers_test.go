package handlers

import (
	"fmt"
	"net/http/httptest"
	"os"
	"rmbl/internal/database"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"github.com/stretchr/testify/assert"
)

// Global test app setup
func TestMain(m *testing.M) {
	// Setup Test DB (Container)
	_, cleanup := database.SetupTestDB()
	
	// Run tests
	code := m.Run()

	// Cleanup container
	cleanup()

	os.Exit(code)
}

func setupTestApp() *fiber.App {
	InitSession()

	// Setup View Engine
	engine := html.New("../../views", ".html")
	
	// Add required functions
	engine.AddFunc("dict", func(values ...interface{}) (map[string]interface{}, error) {
		if len(values)%2 != 0 { return nil, fmt.Errorf("invalid dict call") }
		dict := make(map[string]interface{}, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string); if !ok { return nil, fmt.Errorf("dict keys must be strings") }
			dict[key] = values[i+1]
		}
		return dict, nil
	})
	engine.AddFunc("add", func(a, b int) int { return a + b })
	engine.AddFunc("upper", func(s string) string { return strings.ToUpper(s) })
	engine.AddFunc("capitalize", func(s string) string { return strings.ToUpper(s[:1]) + s[1:] })

	app := fiber.New(fiber.Config{
		Views: engine,
	})
	
	// Mock Locals middleware
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("CSRFToken", "test-token")
		return c.Next()
	})

	return app
}

func TestHome(t *testing.T) {
	app := setupTestApp()
	app.Get("/", Home)

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestSearch(t *testing.T) {
	app := setupTestApp()
	app.Get("/search", Search)

	req := httptest.NewRequest("GET", "/search?q=test", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestSignupAndLogin(t *testing.T) {
	app := setupTestApp()
	app.Post("/signup", PostSignup)
	app.Post("/login", PostLogin)

	// 1. Test Signup (using strong password meeting all requirements)
	username := fmt.Sprintf("user%d",  os.Getpid()) // Unique username per run if needed
	strongPassword := "SecurePass123!"  // Meets all requirements: 12+ chars, upper, lower, number, special
	payload := strings.NewReader(fmt.Sprintf("username=%s&name=Test+User&email=%s@example.com&password=%s", username, username, strongPassword))
	req := httptest.NewRequest("POST", "/signup", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// 2. Test Login
	loginPayload := strings.NewReader(fmt.Sprintf("email=%s@example.com&password=%s", username, strongPassword))
	reqLogin := httptest.NewRequest("POST", "/login", loginPayload)
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	respLogin, err := app.Test(reqLogin)
	assert.Nil(t, err)
	assert.Equal(t, 200, respLogin.StatusCode)
}