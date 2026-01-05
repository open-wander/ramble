package handlers

import (
	"fmt"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// GenerateSitemap generates an XML sitemap
func GenerateSitemap(c *fiber.Ctx) error {
	baseURL := GetBaseURL(c)

	var xml strings.Builder
	xml.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	xml.WriteString("\n")
	xml.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	xml.WriteString("\n")

	// Homepage
	xml.WriteString(sitemapURL(baseURL, "", "daily", "1.0"))

	// Static pages
	xml.WriteString(sitemapURL(baseURL+"/docs", "", "monthly", "0.7"))
	xml.WriteString(sitemapURL(baseURL+"/about", "", "monthly", "0.7"))
	xml.WriteString(sitemapURL(baseURL+"/registries", "", "weekly", "0.8"))
	xml.WriteString(sitemapURL(baseURL+"/packs", "", "weekly", "0.8"))
	xml.WriteString(sitemapURL(baseURL+"/jobs", "", "weekly", "0.8"))

	// All resources
	var resources []models.NomadResource
	database.DB.Preload("User").Preload("Organization").Find(&resources)

	for _, r := range resources {
		displayName := r.User.Username
		if r.OrganizationID != nil && r.Organization.Name != "" {
			displayName = r.Organization.Name
		}

		url := fmt.Sprintf("%s/%s/%s", baseURL, displayName, r.Name)
		lastMod := r.UpdatedAt.Format("2006-01-02")
		xml.WriteString(sitemapURL(url, lastMod, "weekly", "0.9"))
	}

	// All user profiles
	var users []models.User
	database.DB.Find(&users)
	for _, u := range users {
		url := fmt.Sprintf("%s/%s", baseURL, u.Username)
		xml.WriteString(sitemapURL(url, "", "weekly", "0.6"))
	}

	// All organization profiles
	var orgs []models.Organization
	database.DB.Find(&orgs)
	for _, org := range orgs {
		url := fmt.Sprintf("%s/%s", baseURL, org.Name)
		xml.WriteString(sitemapURL(url, "", "weekly", "0.6"))
	}

	xml.WriteString("</urlset>")

	c.Set("Content-Type", "application/xml")
	return c.SendString(xml.String())
}

// sitemapURL generates a single URL entry for the sitemap
func sitemapURL(loc, lastmod, changefreq, priority string) string {
	var xml strings.Builder
	xml.WriteString("  <url>\n")
	xml.WriteString(fmt.Sprintf("    <loc>%s</loc>\n", loc))

	if lastmod != "" {
		xml.WriteString(fmt.Sprintf("    <lastmod>%s</lastmod>\n", lastmod))
	} else {
		xml.WriteString(fmt.Sprintf("    <lastmod>%s</lastmod>\n", time.Now().Format("2006-01-02")))
	}

	xml.WriteString(fmt.Sprintf("    <changefreq>%s</changefreq>\n", changefreq))
	xml.WriteString(fmt.Sprintf("    <priority>%s</priority>\n", priority))
	xml.WriteString("  </url>\n")

	return xml.String()
}
