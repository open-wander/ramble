package handlers

import (
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// GetPacks godoc
// @Summary List nomad packs
// @Description Fetch a searchable and paginated list of all Nomad Packs.
// @Tags resources
// @Produce html
// @Param q query string false "Search query"
// @Param tag query string false "Filter by tag"
// @Param sort query string false "Sort order"
// @Param page query int false "Page number"
// @Success 200 {string} string "HTML content"
// @Router /packs [get]
func GetPacks(c *fiber.Ctx) error {
	isLoggedIn := c.Locals("UserID") != nil
	var results []models.NomadResource
	
	query := c.Query("q")
	tag := c.Query("tag")
	sort := c.Query("sort", "latest")
	pageStr := c.Query("page", "1")
	page, _ := strconv.Atoi(pageStr)
	pageSize := 12
	offset := (page - 1) * pageSize

	// Build Query
	dbQuery := database.DB.Preload("User").Preload("Tags").
		Where("type = ?", models.ResourceTypePack)

	if query != "" {
		searchParam := "%" + escapeLikeString(query) + "%"
		dbQuery = dbQuery.Where("name ILIKE ? ESCAPE '\\' OR description ILIKE ? ESCAPE '\\'", searchParam, searchParam)
	}
	if tag != "" {
		dbQuery = dbQuery.Joins("JOIN resource_tags ON resource_tags.nomad_resource_id = nomad_resources.id").
			Joins("JOIN tags ON tags.id = resource_tags.tag_id").
			Where("tags.name = ?", tag)
	}

	// Apply Sorting
	switch sort {
	case "stars":
		dbQuery = dbQuery.Order("star_count desc, updated_at desc")
	case "downloads":
		dbQuery = dbQuery.Order("download_count desc, updated_at desc")
	case "alpha":
		dbQuery = dbQuery.Order("name asc")
	default:
		dbQuery = dbQuery.Order("updated_at desc")
	}

	dbQuery.Limit(pageSize).Offset(offset).Find(&results)

	nextPage := 0
	if len(results) == pageSize {
		nextPage = page + 1
	}

	return c.Render("index", fiber.Map{
		"Resources":   results,
		"IsLoggedIn":  isLoggedIn,
		"Title":       "Nomad Packs",
		"Page":        "packs",
		"Type":        "pack",
		"Query":       query,
		"Tag":         tag,
		"Sort":        sort,
		"NextPage":    nextPage,
		"Flash":       c.Locals("Flash"),
		"CSRFToken":   c.Locals("CSRFToken"),
		"PopularTags": GetPopularTags(),
		"CurrentUser": c.Locals("User"),
	}, "layouts/main")
}

// GetJobs godoc
// @Summary List nomad jobs
// @Description Fetch a searchable and paginated list of all Nomad Jobs.
// @Tags resources
// @Produce html
// @Param q query string false "Search query"
// @Param tag query string false "Filter by tag"
// @Param sort query string false "Sort order"
// @Param page query int false "Page number"
// @Success 200 {string} string "HTML content"
// @Router /jobs [get]
func GetJobs(c *fiber.Ctx) error {
	isLoggedIn := c.Locals("UserID") != nil
	var results []models.NomadResource
	
	query := c.Query("q")
	tag := c.Query("tag")
	sort := c.Query("sort", "latest")
	pageStr := c.Query("page", "1")
	page, _ := strconv.Atoi(pageStr)
	pageSize := 12
	offset := (page - 1) * pageSize

	// Build Query
	dbQuery := database.DB.Preload("User").Preload("Tags").
		Where("type = ?", models.ResourceTypeJob)

	if query != "" {
		searchParam := "%" + escapeLikeString(query) + "%"
		dbQuery = dbQuery.Where("name ILIKE ? ESCAPE '\\' OR description ILIKE ? ESCAPE '\\'", searchParam, searchParam)
	}
	if tag != "" {
		dbQuery = dbQuery.Joins("JOIN resource_tags ON resource_tags.nomad_resource_id = nomad_resources.id").
			Joins("JOIN tags ON tags.id = resource_tags.tag_id").
			Where("tags.name = ?", tag)
	}

	// Apply Sorting
	switch sort {
	case "stars":
		dbQuery = dbQuery.Order("star_count desc, updated_at desc")
	case "downloads":
		dbQuery = dbQuery.Order("download_count desc, updated_at desc")
	case "alpha":
		dbQuery = dbQuery.Order("name asc")
	default:
		dbQuery = dbQuery.Order("updated_at desc")
	}

	dbQuery.Limit(pageSize).Offset(offset).Find(&results)

	nextPage := 0
	if len(results) == pageSize {
		nextPage = page + 1
	}

	return c.Render("index", fiber.Map{
		"Resources":   results,
		"IsLoggedIn":  isLoggedIn,
		"Title":       "Nomad Jobs",
		"Page":        "jobs",
		"Type":        "job",
		"Query":       query,
		"Tag":         tag,
		"Sort":        sort,
		"NextPage":    nextPage,
		"Flash":       c.Locals("Flash"),
		"CSRFToken":   c.Locals("CSRFToken"),
		"PopularTags": GetPopularTags(),
		"CurrentUser": c.Locals("User"),
		}, "layouts/main")
	}
	