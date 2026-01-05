package handlers

import (
	"encoding/json"
	"fmt"
	"rmbl/internal/models"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// GetBaseURL returns the base URL for the application
func GetBaseURL(c *fiber.Ctx) string {
	scheme := "https"
	if c.Protocol() != "https" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, c.Hostname())
}

// SEOData holds metadata for SEO
type SEOData struct {
	Title          string
	Description    string
	CanonicalURL   string
	OGImage        string
	StructuredData string
}

// GetResourceSEO generates SEO data for a resource
func GetResourceSEO(c *fiber.Ctx, resource models.NomadResource, displayName string) SEOData {
	baseURL := GetBaseURL(c)
	canonicalURL := fmt.Sprintf("%s/%s/%s", baseURL, displayName, resource.Name)

	title := fmt.Sprintf("%s/%s - RMBL Nomad Registry", displayName, resource.Name)
	description := resource.Description
	if description == "" {
		description = fmt.Sprintf("HashiCorp Nomad %s for %s. Download and deploy on RMBL.", resource.Type, resource.Name)
	}
	// Limit description to 160 characters for SEO
	if len(description) > 160 {
		description = description[:157] + "..."
	}

	// Generate JSON-LD structured data
	structuredData := generateResourceStructuredData(resource, displayName, canonicalURL)

	return SEOData{
		Title:          title,
		Description:    description,
		CanonicalURL:   canonicalURL,
		StructuredData: structuredData,
	}
}

// GetProfileSEO generates SEO data for a user/org profile
func GetProfileSEO(c *fiber.Ctx, displayName string, resourceCount int, isOrg bool) SEOData {
	baseURL := GetBaseURL(c)
	canonicalURL := fmt.Sprintf("%s/%s", baseURL, displayName)

	entityType := "User"
	if isOrg {
		entityType = "Organization"
	}

	title := fmt.Sprintf("%s - %s Profile - RMBL", displayName, entityType)
	description := fmt.Sprintf("%s %s on RMBL Nomad Registry. %d published resources.", entityType, displayName, resourceCount)

	return SEOData{
		Title:        title,
		Description:  description,
		CanonicalURL: canonicalURL,
	}
}

// GetHomeSEO generates SEO data for the home page
func GetHomeSEO(c *fiber.Ctx) SEOData {
	baseURL := GetBaseURL(c)

	return SEOData{
		Title:        "RMBL - Community Nomad Registry for Jobs and Packs",
		Description:  "Discover, share, and deploy HashiCorp Nomad job specifications and packs. Community-driven registry for Nomad infrastructure.",
		CanonicalURL: baseURL,
	}
}

// GetSearchSEO generates SEO data for search results
func GetSearchSEO(c *fiber.Ctx, query string, resultCount int) SEOData {
	baseURL := GetBaseURL(c)
	canonicalURL := fmt.Sprintf("%s/search?q=%s", baseURL, query)

	title := fmt.Sprintf("Search: %s - RMBL Nomad Registry", query)
	description := fmt.Sprintf("Found %d Nomad resources matching '%s'. Browse jobs and packs on RMBL.", resultCount, query)

	return SEOData{
		Title:        title,
		Description:  description,
		CanonicalURL: canonicalURL,
	}
}

// generateResourceStructuredData creates JSON-LD structured data for a resource
func generateResourceStructuredData(resource models.NomadResource, displayName, url string) string {
	data := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "SoftwareSourceCode",
		"name":     fmt.Sprintf("%s/%s", displayName, resource.Name),
		"description": resource.Description,
		"url":      url,
		"codeRepository": resource.RepositoryURL,
		"programmingLanguage": "HCL",
	}

	if len(resource.Tags) > 0 {
		keywords := make([]string, len(resource.Tags))
		for i, tag := range resource.Tags {
			keywords[i] = tag.Name
		}
		data["keywords"] = strings.Join(keywords, ", ")
	}

	if resource.CreatedAt.IsZero() == false {
		data["datePublished"] = resource.CreatedAt.Format("2006-01-02")
	}

	if resource.UpdatedAt.IsZero() == false {
		data["dateModified"] = resource.UpdatedAt.Format("2006-01-02")
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return ""
	}

	return string(jsonBytes)
}
