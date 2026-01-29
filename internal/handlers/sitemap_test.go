package handlers

import (
	"io"
	"net/http/httptest"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestGenerateSitemap_Structure(t *testing.T) {
	app := fiber.New()
	app.Get("/sitemap.xml", GenerateSitemap)

	req := httptest.NewRequest("GET", "/sitemap.xml", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "application/xml", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	xmlStr := string(body)

	// Verify XML structure
	assert.True(t, strings.HasPrefix(xmlStr, `<?xml version="1.0" encoding="UTF-8"?>`))
	assert.Contains(t, xmlStr, `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	assert.Contains(t, xmlStr, `</urlset>`)
	assert.Contains(t, xmlStr, `<url>`)
	assert.Contains(t, xmlStr, `<loc>`)
	assert.Contains(t, xmlStr, `<lastmod>`)
	assert.Contains(t, xmlStr, `<changefreq>`)
	assert.Contains(t, xmlStr, `<priority>`)
}

func TestGenerateSitemap_StaticPages(t *testing.T) {
	app := fiber.New()
	app.Get("/sitemap.xml", GenerateSitemap)

	req := httptest.NewRequest("GET", "/sitemap.xml", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	xmlStr := string(body)

	// Verify static pages are included
	assert.Contains(t, xmlStr, "/docs")
	assert.Contains(t, xmlStr, "/about")
	assert.Contains(t, xmlStr, "/registries")
	assert.Contains(t, xmlStr, "/packs")
	assert.Contains(t, xmlStr, "/jobs")
}

func TestGenerateSitemap_IncludesResources(t *testing.T) {
	// Create a test user and resource
	user := models.User{
		Username: "sitemapuser",
		Email:    "sitemap@test.com",
	}
	database.DB.Create(&user)
	defer database.DB.Delete(&user)

	resource := models.NomadResource{
		Name:        "sitemap-pack",
		UserID:      user.ID,
		Description: "Test pack for sitemap",
		Type:        "pack",
	}
	database.DB.Create(&resource)
	defer database.DB.Delete(&resource)

	app := fiber.New()
	app.Get("/sitemap.xml", GenerateSitemap)

	req := httptest.NewRequest("GET", "/sitemap.xml", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	xmlStr := string(body)

	// Verify resource URL is included
	assert.Contains(t, xmlStr, "/sitemapuser/sitemap-pack")
}

func TestGenerateSitemap_IncludesUsers(t *testing.T) {
	// Create a test user
	user := models.User{
		Username: "sitemaptestuser",
		Email:    "sitemaptest@test.com",
	}
	database.DB.Create(&user)
	defer database.DB.Delete(&user)

	app := fiber.New()
	app.Get("/sitemap.xml", GenerateSitemap)

	req := httptest.NewRequest("GET", "/sitemap.xml", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	xmlStr := string(body)

	// Verify user profile URL is included
	assert.Contains(t, xmlStr, "/sitemaptestuser")
}

func TestGenerateSitemap_IncludesOrganizations(t *testing.T) {
	// Create a test user and organization
	user := models.User{
		Username: "orgowner",
		Email:    "orgowner@test.com",
	}
	database.DB.Create(&user)
	defer database.DB.Delete(&user)

	org := models.Organization{
		Name: "sitemaptestorg",
	}
	database.DB.Create(&org)
	defer database.DB.Delete(&org)

	// Create owner membership
	membership := models.Membership{
		UserID:         user.ID,
		OrganizationID: org.ID,
		Role:           "owner",
	}
	database.DB.Create(&membership)
	defer database.DB.Delete(&membership)

	app := fiber.New()
	app.Get("/sitemap.xml", GenerateSitemap)

	req := httptest.NewRequest("GET", "/sitemap.xml", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	xmlStr := string(body)

	// Verify organization URL is included
	assert.Contains(t, xmlStr, "/sitemaptestorg")
}

func TestGenerateSitemap_OrgResource(t *testing.T) {
	// Create a test user, organization, and resource owned by org
	user := models.User{
		Username: "orgresowner",
		Email:    "orgresowner@test.com",
	}
	database.DB.Create(&user)
	defer database.DB.Delete(&user)

	org := models.Organization{
		Name: "resourceorg",
	}
	database.DB.Create(&org)
	defer database.DB.Delete(&org)

	// Create owner membership
	membership := models.Membership{
		UserID:         user.ID,
		OrganizationID: org.ID,
		Role:           "owner",
	}
	database.DB.Create(&membership)
	defer database.DB.Delete(&membership)

	resource := models.NomadResource{
		Name:           "org-owned-pack",
		UserID:         user.ID,
		OrganizationID: &org.ID,
		Description:    "Pack owned by org",
		Type:           "pack",
	}
	database.DB.Create(&resource)
	defer database.DB.Delete(&resource)

	app := fiber.New()
	app.Get("/sitemap.xml", GenerateSitemap)

	req := httptest.NewRequest("GET", "/sitemap.xml", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	xmlStr := string(body)

	// Verify resource is under org name, not user name
	assert.Contains(t, xmlStr, "/resourceorg/org-owned-pack")
}

func TestSitemapURL(t *testing.T) {
	// Test with lastmod
	xml := sitemapURL("https://example.com/page", "2024-01-15", "weekly", "0.8")

	assert.Contains(t, xml, "<url>")
	assert.Contains(t, xml, "<loc>https://example.com/page</loc>")
	assert.Contains(t, xml, "<lastmod>2024-01-15</lastmod>")
	assert.Contains(t, xml, "<changefreq>weekly</changefreq>")
	assert.Contains(t, xml, "<priority>0.8</priority>")
	assert.Contains(t, xml, "</url>")
}

func TestSitemapURL_NoLastmod(t *testing.T) {
	// Test without lastmod - should use current date
	xml := sitemapURL("https://example.com/page", "", "daily", "1.0")

	assert.Contains(t, xml, "<url>")
	assert.Contains(t, xml, "<loc>https://example.com/page</loc>")
	assert.Contains(t, xml, "<lastmod>") // Should have a lastmod with today's date
	assert.Contains(t, xml, "<changefreq>daily</changefreq>")
	assert.Contains(t, xml, "<priority>1.0</priority>")
	assert.Contains(t, xml, "</url>")
}

func TestSitemapURL_Priorities(t *testing.T) {
	testCases := []struct {
		priority string
	}{
		{"0.0"},
		{"0.5"},
		{"1.0"},
	}

	for _, tc := range testCases {
		xml := sitemapURL("https://example.com", "", "monthly", tc.priority)
		assert.Contains(t, xml, "<priority>"+tc.priority+"</priority>")
	}
}

func TestSitemapURL_ChangeFrequencies(t *testing.T) {
	freqs := []string{"always", "hourly", "daily", "weekly", "monthly", "yearly", "never"}

	for _, freq := range freqs {
		xml := sitemapURL("https://example.com", "", freq, "0.5")
		assert.Contains(t, xml, "<changefreq>"+freq+"</changefreq>")
	}
}
