package handlers

import (
	"fmt"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type PackSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PackDetail struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Versions    []PackVersion `json:"versions"`
}

type PackVersion struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// ListPacksAPI godoc
// @Summary List packs in a namespace
// @Description Fetch a list of all Nomad Packs within a specific user or organization namespace.
// @Tags nomad-pack
// @Produce json
// @Param username path string true "Namespace (user or org)"
// @Success 200 {object} map[string][]PackSummary
// @Failure 404 {object} map[string]string
// @Router /{username}/v1/packs [get]
func ListPacksAPI(c *fiber.Ctx) error {
	namespace := c.Params("username")

	var userID uint
	var orgID *uint

	var user models.User
	if err := database.DB.Where("username ILIKE ?", namespace).First(&user).Error; err == nil {
		userID = user.ID
	} else {
		var org models.Organization
		if err := database.DB.Where("name ILIKE ?", namespace).First(&org).Error; err == nil {
			orgID = &org.ID
		} else {
			return c.Status(404).JSON(fiber.Map{"error": "Namespace not found"})
		}
	}

	var resources []models.NomadResource
	dbQuery := database.DB.Where("type = ?", models.ResourceTypePack)
	if orgID != nil {
		dbQuery = dbQuery.Where("organization_id = ?", *orgID)
	} else {
		dbQuery = dbQuery.Where("user_id = ? AND organization_id IS NULL", userID)
	}

	if err := dbQuery.Find(&resources).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database error"})
	}

	summaries := make([]PackSummary, len(resources))
	for i, r := range resources {
		summaries[i] = PackSummary{
			Name:        r.Name,
			Description: r.Description,
		}
	}

	return c.JSON(fiber.Map{"packs": summaries})
}

// ListAllPacksAPI godoc
// @Summary List all packs
// @Description Fetch a list of all Nomad Packs available in the registry across all namespaces.
// @Tags nomad-pack
// @Produce json
// @Success 200 {object} map[string][]PackSummary
// @Router /v1/packs [get]
func ListAllPacksAPI(c *fiber.Ctx) error {
	var resources []models.NomadResource
	if err := database.DB.Where("type = ?", models.ResourceTypePack).Find(&resources).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database error"})
	}

	summaries := make([]PackSummary, len(resources))
	for i, r := range resources {
		summaries[i] = PackSummary{
			Name:        r.Name,
			Description: r.Description,
		}
	}

	return c.JSON(fiber.Map{"packs": summaries})
}

// GetPackAPI godoc
// @Summary Get pack details
// @Description Fetch detailed metadata and version history for a specific Nomad Pack.
// @Tags nomad-pack
// @Produce json
// @Param username path string true "Namespace (user or org)"
// @Param packname path string true "Pack name"
// @Success 200 {object} PackDetail
// @Failure 404 {object} map[string]string
// @Router /{username}/v1/packs/{packname} [get]
func GetPackAPI(c *fiber.Ctx) error {
	namespace := c.Params("username")
	packname := c.Params("packname")

	var userID uint
	var orgID *uint

	var user models.User
	if err := database.DB.Where("username ILIKE ?", namespace).First(&user).Error; err == nil {
		userID = user.ID
	} else {
		var org models.Organization
		if err := database.DB.Where("name ILIKE ?", namespace).First(&org).Error; err == nil {
			orgID = &org.ID
		} else {
			return c.Status(404).JSON(fiber.Map{"error": "Namespace not found"})
		}
	}

	var resource models.NomadResource
	dbQuery := database.DB.Preload("Versions").Where("type = ? AND name ILIKE ?", models.ResourceTypePack, packname)
	if orgID != nil {
		dbQuery = dbQuery.Where("organization_id = ?", *orgID)
	} else {
		dbQuery = dbQuery.Where("user_id = ? AND organization_id IS NULL", userID)
	}

	if err := dbQuery.First(&resource).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Pack not found"})
	}

	versions := make([]PackVersion, len(resource.Versions))
	for i, v := range resource.Versions {
		versions[i] = PackVersion{
			Version: v.Version,
			URL:     getDownloadURL(resource.RepositoryURL, v.Version),
		}
	}

	return c.JSON(PackDetail{
		Name:        resource.Name,
		Description: resource.Description,
		Versions:    versions,
	})
}

// GetRegistries godoc
// @Summary List all namespaces acting as registries
// @Description Fetch a list of all users and organizations that have published at least one Nomad Pack.
// @Tags registries
// @Produce html
// @Success 200 {string} string "HTML content"
// @Router /registries [get]
func GetRegistries(c *fiber.Ctx) error {
	var registries []struct {
		Name string
		Type string
	}

	// Query for unique owners of resources of type 'pack'
	err := database.DB.Table("nomad_resources").
		Select("DISTINCT COALESCE(organizations.name, users.username) as name, CASE WHEN organization_id IS NOT NULL THEN 'organization' ELSE 'user' END as type").
		Joins("LEFT JOIN users ON users.id = nomad_resources.user_id").
		Joins("LEFT JOIN organizations ON organizations.id = nomad_resources.organization_id").
		Where("nomad_resources.type = ?", models.ResourceTypePack).
		Scan(&registries).Error

	if err != nil {
		return c.Status(500).SendString("Database error")
	}

	return c.Render("registries", fiber.Map{
		"IsLoggedIn":  c.Locals("UserID") != nil,
		"Page":        "registries",
		"Title":       "User Registries",
		"Registries": registries,
		"Flash":       c.Locals("Flash"),
		"CSRFToken":   c.Locals("CSRFToken"),
		"CurrentUser": c.Locals("User"),
	}, "layouts/main")
}

// ListUserRegistriesAPI godoc
// @Summary List all registry namespaces (JSON)
// @Description Returns a JSON list of all namespaces that contain at least one Nomad Pack.
// @Tags nomad-pack
// @Produce json
// @Success 200 {object} map[string][]string
// @Router /v1/registries [get]
func ListUserRegistriesAPI(c *fiber.Ctx) error {
	var names []string
	err := database.DB.Table("nomad_resources").
		Select("DISTINCT COALESCE(organizations.name, users.username)").
		Joins("LEFT JOIN users ON users.id = nomad_resources.user_id").
		Joins("LEFT JOIN organizations ON organizations.id = nomad_resources.organization_id").
		Where("nomad_resources.type = ?", models.ResourceTypePack).
		Pluck("coalesce", &names).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database error"})
	}

	return c.JSON(fiber.Map{"registries": names})
}

// SearchPacksAPI godoc
// @Summary Search packs
// @Description Search for Nomad Packs across all namespaces by name or description.
// @Tags nomad-pack
// @Produce json
// @Param q query string true "Search query"
// @Success 200 {object} map[string][]PackSummary
// @Router /v1/packs/search [get]
func SearchPacksAPI(c *fiber.Ctx) error {
	query := c.Query("q")
	if query == "" {
		return c.JSON(fiber.Map{"packs": []PackSummary{}})
	}

	var resources []models.NomadResource
	searchParam := "%" + escapeLikeString(query) + "%"
	err := database.DB.Where("type = ? AND (name ILIKE ? ESCAPE '\\' OR description ILIKE ? ESCAPE '\\')", models.ResourceTypePack, searchParam, searchParam).
		Limit(20).Find(&resources).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database error"})
	}

	summaries := make([]PackSummary, len(resources))
	for i, r := range resources {
		summaries[i] = PackSummary{
			Name:        r.Name,
			Description: r.Description,
		}
	}

	return c.JSON(fiber.Map{"packs": summaries})
}

// getDownloadURL constructs a tarball download URL for GitHub or GitLab.
func getDownloadURL(repoURL string, version string) string {
	repoURL = strings.TrimSuffix(repoURL, ".git")
	repoURL = strings.TrimSuffix(repoURL, "/")

	if strings.Contains(repoURL, "github.com") {
		// https://github.com/owner/repo/archive/refs/tags/v1.0.0.tar.gz
		return fmt.Sprintf("%s/archive/refs/tags/%s.tar.gz", repoURL, version)
	}

	if strings.Contains(repoURL, "gitlab.com") {
		// https://gitlab.com/owner/repo/-/archive/v1.0.0/repo-v1.0.0.tar.gz
		parts := strings.Split(repoURL, "/")
		repoName := parts[len(parts)-1]
		return fmt.Sprintf("%s/-/archive/%s/%s-%s.tar.gz", repoURL, version, repoName, version)
	}

	return repoURL // Fallback to repo URL
}
