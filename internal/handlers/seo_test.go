package handlers

import (
	"net/http/httptest"
	"rmbl/internal/models"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestGetHomeSEO(t *testing.T) {
	app := fiber.New()

	var seoData SEOData
	app.Get("/", func(c *fiber.Ctx) error {
		seoData = GetHomeSEO(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "rmbl.test"
	_, err := app.Test(req)

	assert.NoError(t, err)
	assert.Contains(t, seoData.Title, "RMBL")
	assert.Contains(t, seoData.Description, "Nomad")
	assert.NotEmpty(t, seoData.CanonicalURL)
}

func TestGetProfileSEO_User(t *testing.T) {
	app := fiber.New()

	var seoData SEOData
	app.Get("/", func(c *fiber.Ctx) error {
		seoData = GetProfileSEO(c, "testuser", 5, false)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "rmbl.test"
	_, err := app.Test(req)

	assert.NoError(t, err)
	assert.Contains(t, seoData.Title, "testuser")
	assert.Contains(t, seoData.Title, "User")
	assert.Contains(t, seoData.Description, "5 published resources")
	assert.Contains(t, seoData.CanonicalURL, "testuser")
}

func TestGetProfileSEO_Organization(t *testing.T) {
	app := fiber.New()

	var seoData SEOData
	app.Get("/", func(c *fiber.Ctx) error {
		seoData = GetProfileSEO(c, "testorg", 10, true)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "rmbl.test"
	_, err := app.Test(req)

	assert.NoError(t, err)
	assert.Contains(t, seoData.Title, "testorg")
	assert.Contains(t, seoData.Title, "Organization")
	assert.Contains(t, seoData.Description, "10 published resources")
}

func TestGetResourceSEO(t *testing.T) {
	app := fiber.New()

	resource := models.NomadResource{
		Name:        "test-pack",
		Description: "A test pack for Nomad",
		Type:        models.ResourceTypePack,
	}

	var seoData SEOData
	app.Get("/", func(c *fiber.Ctx) error {
		seoData = GetResourceSEO(c, resource, "testuser")
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "rmbl.test"
	_, err := app.Test(req)

	assert.NoError(t, err)
	assert.Contains(t, seoData.Title, "test-pack")
	assert.Contains(t, seoData.Title, "testuser")
	assert.Contains(t, seoData.Description, "A test pack for Nomad")
	assert.Contains(t, seoData.CanonicalURL, "testuser/test-pack")
}

func TestGetResourceSEO_EmptyDescription(t *testing.T) {
	app := fiber.New()

	resource := models.NomadResource{
		Name:        "empty-desc-pack",
		Description: "",
		Type:        models.ResourceTypePack,
	}

	var seoData SEOData
	app.Get("/", func(c *fiber.Ctx) error {
		seoData = GetResourceSEO(c, resource, "someuser")
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "rmbl.test"
	_, err := app.Test(req)

	assert.NoError(t, err)
	// Should generate a default description
	assert.Contains(t, seoData.Description, "empty-desc-pack")
	assert.NotEmpty(t, seoData.Description)
}

func TestGetResourceSEO_LongDescription(t *testing.T) {
	app := fiber.New()

	// Create a very long description
	longDesc := ""
	for i := 0; i < 50; i++ {
		longDesc += "This is a very long description. "
	}

	resource := models.NomadResource{
		Name:        "long-desc-pack",
		Description: longDesc,
		Type:        models.ResourceTypePack,
	}

	var seoData SEOData
	app.Get("/", func(c *fiber.Ctx) error {
		seoData = GetResourceSEO(c, resource, "someuser")
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "rmbl.test"
	_, err := app.Test(req)

	assert.NoError(t, err)
	// Description should be truncated to 160 characters
	assert.LessOrEqual(t, len(seoData.Description), 160)
	assert.True(t, len(seoData.Description) > 100) // Should have content
}

func TestGetSearchSEO(t *testing.T) {
	app := fiber.New()

	var seoData SEOData
	app.Get("/", func(c *fiber.Ctx) error {
		seoData = GetSearchSEO(c, "traefik", 15)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "rmbl.test"
	_, err := app.Test(req)

	assert.NoError(t, err)
	assert.Contains(t, seoData.Title, "traefik")
	assert.Contains(t, seoData.Description, "15")
	assert.Contains(t, seoData.Description, "traefik")
	assert.Contains(t, seoData.CanonicalURL, "search")
	assert.Contains(t, seoData.CanonicalURL, "traefik")
}

func TestGetBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		proto    string
		expected string
	}{
		{
			name:     "HTTP localhost",
			host:     "localhost:3000",
			proto:    "",
			expected: "http://localhost:3000",
		},
		{
			name:     "HTTPS with X-Forwarded-Proto",
			host:     "rmbl.io",
			proto:    "https",
			expected: "https://rmbl.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()

			var baseURL string
			app.Get("/", func(c *fiber.Ctx) error {
				baseURL = GetBaseURL(c)
				return c.SendString("ok")
			})

			req := httptest.NewRequest("GET", "/", nil)
			req.Host = tt.host
			if tt.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.proto)
			}

			_, err := app.Test(req)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, baseURL)
		})
	}
}
