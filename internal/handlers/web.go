package handlers

import (
	"encoding/json"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func Home(c *fiber.Ctx) error {
	isLoggedIn := c.Locals("UserID") != nil
	flash := c.Locals("Flash")
	csrfToken := c.Locals("CSRFToken")
	popularTags := GetPopularTags()
	currentUser := c.Locals("User")

	var results []models.NomadResource
	database.DB.Model(&models.NomadResource{}).Preload("User").Preload("Tags").Order("updated_at desc").Limit(12).Find(&results)

	nextPage := 0
	if len(results) == 12 {
		nextPage = 2
	}

	// Render index within the main layout
	return c.Render("index", fiber.Map{
		"IsLoggedIn":  isLoggedIn,
		"Page":        "home",
		"Flash":       flash,
		"CSRFToken":   csrfToken,
		"PopularTags": popularTags,
		"CurrentUser": currentUser,
		"Resources":   results,
		"NextPage":    nextPage,
	}, "layouts/main")
}

// Search godoc
// @Summary Search nomad resources
// @Description Search for jobs or packs with optional filtering by type and tag.
// @Tags resources
// @Produce html
// @Param q query string false "Search query"
// @Param type query string false "Resource type (job or pack)"
// @Param tag query string false "Filter by tag"
// @Param page query int false "Page number"
// @Success 200 {string} string "HTML content"
// @Router /search [get]
func Search(c *fiber.Ctx) error {
	query := c.Query("q")
	resourceType := c.Query("type")
	tag := c.Query("tag")
	sort := c.Query("sort", "latest")
	pageStr := c.Query("page", "1")
	page, _ := strconv.Atoi(pageStr)
	pageSize := 12

	isLoggedIn := c.Locals("UserID") != nil
	
	var results []models.NomadResource
	
	dbQuery := database.DB.Model(&models.NomadResource{}).Preload("User").Preload("Tags")

	if query != "" {
		searchParam := "%" + escapeLikeString(query) + "%"
		dbQuery = dbQuery.Where("name ILIKE ? ESCAPE '\\' OR description ILIKE ? ESCAPE '\\'", searchParam, searchParam)
	}
	if resourceType != "" {
		dbQuery = dbQuery.Where("type = ?", resourceType)
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
	
	offset := (page - 1) * pageSize
	dbQuery.Limit(pageSize).Offset(offset).Find(&results)

	nextPage := 0
	if len(results) == pageSize {
		nextPage = page + 1
	}

	// If it's an HTMX request, render the partial
	if c.Get("HX-Request") == "true" && c.Get("HX-Target") == "search-results" {
		return c.Render("partials/resource_list", fiber.Map{
			"Resources": results,
			"NextPage":  nextPage,
			"Query":     query,
			"Type":      resourceType,
			"Tag":       tag,
			"Sort":      sort,
		})
	}

	return c.Render("index", fiber.Map{
		"Resources":   results,
		"Query":       query,
		"Type":        resourceType,
		"Tag":         tag,
		"Sort":        sort,
		"IsLoggedIn":  isLoggedIn,
		"Flash":       c.Locals("Flash"),
		"CSRFToken":   c.Locals("CSRFToken"),
		"NextPage":    nextPage,
		"PopularTags": GetPopularTags(),
		"CurrentUser": c.Locals("User"),
	}, "layouts/main")
}

// GetResource godoc
// @Summary Get resource details
// @Description Fetch metadata for a specific Nomad resource within a user or organization namespace.
// @Tags resources
// @Produce html
// @Param username path string true "User or Organization namespace"
// @Param resourcename path string true "Resource name"
// @Success 200 {string} string "HTML content"
// @Failure 404 {string} string "Not Found"
// @Router /{username}/{resourcename} [get]
func GetResource(c *fiber.Ctx) error {
	namespace := c.Params("username") // This is the namespace (user or org)
	resourcename := c.Params("resourcename")

	// Check if namespace is a User or Organization
	var userID uint
	var orgID *uint
	var displayName string

	var user models.User
	if err := database.DB.Where("username ILIKE ?", namespace).First(&user).Error; err == nil {
		userID = user.ID
		displayName = user.Username
	} else {
		var org models.Organization
		if err := database.DB.Where("name ILIKE ?", namespace).First(&org).Error; err == nil {
			orgID = &org.ID
			displayName = org.Name
		} else {
			return c.Status(404).SendString("Namespace not found")
		}
	}

	var resource models.NomadResource
	dbQuery := database.DB.Preload("User").Preload("Tags").Preload("StarredBy").Preload("Versions", func(db *gorm.DB) *gorm.DB {
		return db.Order("resource_versions.created_at DESC")
	})

	if orgID != nil {
		dbQuery = dbQuery.Where("organization_id = ? AND name ILIKE ?", *orgID, resourcename)
	} else {
		dbQuery = dbQuery.Where("user_id = ? AND organization_id IS NULL AND name ILIKE ?", userID, resourcename)
	}

	if err := dbQuery.First(&resource).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}

	isLoggedIn := c.Locals("UserID") != nil
	var isOwner bool
	var isStarred bool
	if isLoggedIn {
		currentUserID := c.Locals("UserID").(uint)
		
		// If it's a personal repo, current user must be owner
		// If it's an org repo, current user must be member of that org
		if orgID != nil {
			var membership models.Membership
			if err := database.DB.Where("user_id = ? AND organization_id = ?", currentUserID, *orgID).First(&membership).Error; err == nil {
				isOwner = true // members can edit
			}
		} else {
			isOwner = currentUserID == resource.UserID
		}

		for _, u := range resource.StarredBy {
			if u.ID == currentUserID {
				isStarred = true
				break
			}
		}
	}

	var latestVariables []PackVariable
	if len(resource.Versions) > 0 && resource.Versions[0].Variables != "" {
		json.Unmarshal([]byte(resource.Versions[0].Variables), &latestVariables)
	}

	return c.Render("resource_detail", fiber.Map{
		"Resource":    resource,
		"IsLoggedIn":  isLoggedIn,
		"IsOwner":     isOwner,
		"IsStarred":   isStarred,
		"StarCount":   len(resource.StarredBy),
		"DisplayName": displayName,
		"Flash":       c.Locals("Flash"),
		"CSRFToken":   c.Locals("CSRFToken"),
		"Host":        c.Hostname(),
		"CurrentUser": c.Locals("User"),
		"LatestVersionVariables": latestVariables,
	}, "layouts/main")
}

// GetResourceVersion godoc
// @Summary Get specific resource version details
// @Description Fetch metadata and README for a specific version of a resource. Usually called via HTMX.
// @Tags resources
// @Produce html
// @Param username path string true "User or Organization namespace"
// @Param resourcename path string true "Resource name"
// @Param version query string true "Version string (e.g., v1.0.0)"
// @Success 200 {string} string "HTML fragment"
// @Failure 404 {string} string "Not Found"
// @Router /{username}/{resourcename}/v [get]
func GetResourceVersion(c *fiber.Ctx) error {
	username := c.Params("username")
	resourcename := c.Params("resourcename")
	versionStr := c.Query("version")

	var user models.User
	if err := database.DB.Where("username ILIKE ?", username).First(&user).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}

	var resource models.NomadResource
	if err := database.DB.Where("user_id = ? AND name ILIKE ?", user.ID, resourcename).First(&resource).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}

	var version models.ResourceVersion
	if err := database.DB.Where("resource_id = ? AND version = ?", resource.ID, versionStr).First(&version).Error; err != nil {
		return c.Status(404).SendString("Version not found")
	}

	var variables []PackVariable
	if version.Variables != "" {
		json.Unmarshal([]byte(version.Variables), &variables)
	}

	return c.Render("partials/version_content", fiber.Map{
		"Version":   version,
		"Resource":  resource,
		"Host":      c.Hostname(),
		"Variables": variables,
	})
}

// GetRawResourceVersion godoc
// @Summary Get raw HCL content for a specific version
// @Description Get the raw .nomad.hcl or template content for a specific version of a resource. Used by Nomad CLI.
// @Tags resources
// @Produce text/plain
// @Param username path string true "User or Organization namespace"
// @Param resourcename path string true "Resource name"
// @Param version path string true "Version string"
// @Success 200 {string} string "Raw content"
// @Failure 404 {string} string "Not Found"
// @Router /{username}/{resourcename}/v/{version}/raw [get]
func GetRawResourceVersion(c *fiber.Ctx) error {
	username := c.Params("username")
	resourcename := c.Params("resourcename")
	versionStr := c.Params("version")

	var user models.User
	if err := database.DB.Where("username ILIKE ?", username).First(&user).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}

	var resource models.NomadResource
	if err := database.DB.Where("user_id = ? AND name ILIKE ?", user.ID, resourcename).First(&resource).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}

	var version models.ResourceVersion
	if err := database.DB.Where("resource_id = ? AND version = ?", resource.ID, versionStr).First(&version).Error; err != nil {
		return c.Status(404).SendString("Version not found")
	}

	if version.Content == "" {
		return c.Status(404).SendString("No content available for this version")
	}

	// Increment download count
	database.DB.Model(&resource).Update("download_count", gorm.Expr("download_count + ?", 1))

	c.Set("Content-Type", "text/plain")
	return c.SendString(version.Content)
}

// GetUserProfile godoc
// @Summary Get user or organization profile
// @Description Fetch all resources belonging to a specific user or organization namespace.
// @Tags resources
// @Produce html
// @Param username path string true "User or Organization namespace"
// @Param q query string false "Search query"
// @Param sort query string false "Sort order (latest, stars, alpha, downloads)"
// @Param page query int false "Page number"
// @Success 200 {string} string "HTML content"
// @Failure 404 {string} string "Not Found"
// @Router /{username} [get]
func GetUserProfile(c *fiber.Ctx) error {
	namespace := c.Params("username")
	query := c.Query("q")
	sort := c.Query("sort", "latest")
	pageStr := c.Query("page", "1")
	page, _ := strconv.Atoi(pageStr)
	pageSize := 12
	offset := (page - 1) * pageSize

	isLoggedIn := c.Locals("UserID") != nil



	var profileUser *models.User

	var profileOrg *models.Organization



	var user models.User

	if err := database.DB.Preload("Memberships.Organization").Where("username ILIKE ?", namespace).First(&user).Error; err == nil {

		profileUser = &user

	} else {

		var org models.Organization

		if err := database.DB.Preload("Memberships.User").Where("name ILIKE ?", namespace).First(&org).Error; err == nil {

			profileOrg = &org

		} else {

			return c.Status(404).SendString("Profile not found")

		}

	}



	var results []models.NomadResource

	dbQuery := database.DB.Model(&models.NomadResource{}).

		Preload("User").

		Preload("Tags")



	if profileUser != nil {

		dbQuery = dbQuery.Where("user_id = ? AND organization_id IS NULL", profileUser.ID)

	} else {

		dbQuery = dbQuery.Where("organization_id = ?", profileOrg.ID)

	}



	if query != "" {

		searchParam := "%" + escapeLikeString(query) + "%"

		dbQuery = dbQuery.Where("name ILIKE ? ESCAPE '\\' OR description ILIKE ? ESCAPE '\\'", searchParam, searchParam)

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



	// If it's an HTMX request, render the partial

	if c.Get("HX-Request") == "true" && c.Get("HX-Target") == "search-results" {

		return c.Render("partials/resource_list", fiber.Map{

			"Resources": results,

			"NextPage":  nextPage,

			"Query":     query,

			"Sort":      sort,

		})

	}



	title := namespace + "'s Resources"

	var isOwner bool

	if profileOrg != nil {

		title = "Organization: " + profileOrg.Name

		if isLoggedIn {

			currentUserID := c.Locals("UserID").(uint)

			var count int64

			database.DB.Model(&models.Membership{}).Where("user_id = ? AND organization_id = ?", currentUserID, profileOrg.ID).Count(&count)

			isOwner = count > 0

		}

	} else if profileUser != nil && isLoggedIn {

		isOwner = c.Locals("UserID").(uint) == profileUser.ID

	}



	return c.Render("profile", fiber.Map{

		"ProfileUser": profileUser,

		"ProfileOrg":  profileOrg,

		"Resources":   results,

		"IsLoggedIn":  isLoggedIn,

		"IsOwner":     isOwner,

		"Title":       title,

		"Query":       query,

		"Sort":        sort,

		"Page":        "profile",

		"Flash":       c.Locals("Flash"),

		"CSRFToken":   c.Locals("CSRFToken"),

		"NextPage":    nextPage,

		"PopularTags": GetPopularTags(),

		"CurrentUser": c.Locals("User"),

	}, "layouts/main")

}

// GetRawResource godoc
// @Summary Get raw HCL content
// @Description Get the raw .nomad.hcl content for the latest version of a resource.
// @Tags resources
// @Produce text/plain
// @Param username path string true "User namespace"
// @Param resourcename path string true "Resource name"
// @Success 200 {string} string "Raw HCL content"
// @Router /{username}/{resourcename}/raw [get]
func GetRawResource(c *fiber.Ctx) error {
	username := c.Params("username")
	resourcename := c.Params("resourcename")

	var user models.User
	if err := database.DB.Where("username ILIKE ?", username).First(&user).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}

	var resource models.NomadResource
	if err := database.DB.Preload("Versions", func(db *gorm.DB) *gorm.DB {
		return db.Order("resource_versions.created_at DESC")
	}).Where("user_id = ? AND name ILIKE ?", user.ID, resourcename).First(&resource).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}

	if len(resource.Versions) == 0 || resource.Versions[0].Content == "" {
		return c.Status(404).SendString("No content available for this resource")
	}

	// Increment download count
	database.DB.Model(&resource).Update("download_count", gorm.Expr("download_count + ?", 1))

	c.Set("Content-Type", "text/plain")
	return c.SendString(resource.Versions[0].Content)
}

func FetchReadme(c *fiber.Ctx) error {
	id := c.Params("id")
	var resource models.NomadResource
	if err := database.DB.Preload("Versions", func(db *gorm.DB) *gorm.DB {
		return db.Order("resource_versions.created_at DESC")
	}).First(&resource, id).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}

	readme, _ := downloadFile(resource.RepositoryURL, "README.md")
	var content string
	if resource.Type == models.ResourceTypeJob {
		fetchPath := resource.FilePath
		if fetchPath == "" {
			fetchPath = resource.Name
			if !strings.HasSuffix(fetchPath, ".nomad.hcl") {
				fetchPath = fetchPath + ".nomad.hcl"
			}
		}
		content, _ = downloadFile(resource.RepositoryURL, fetchPath)
	} else if resource.Type == models.ResourceTypePack {
		content, _ = downloadFile(resource.RepositoryURL, "metadata.hcl")
	}

	// Update License if unknown
	if resource.License == "" || resource.License == "Unknown" {
		if licBody, err := downloadFile(resource.RepositoryURL, "LICENSE"); err == nil && licBody != "" {
			lines := strings.Split(licBody, "\n")
			if len(lines) > 0 {
				lic := strings.TrimSpace(lines[0])
				lic = strings.TrimPrefix(lic, "The ")
				if len(lic) > 25 {
					lic = lic[:25] + "..."
				}
				resource.License = lic
				database.DB.Save(&resource)
			}
		}
	}

	// Update the latest version
	if len(resource.Versions) > 0 {
		version := resource.Versions[0]
		version.Readme = readme
		version.Content = content
		database.DB.Save(&version)
	}

	if readme == "" {
		return c.SendString("<p class='text-red-500'>Failed to fetch README. Please check the repository URL.</p>")
	}

	return c.SendString("<div id='readme-content' _='on load call renderMarkdown(my.textContent) then set my.innerHTML to it then call hljs.highlightAll()'>" + readme + "</div>")
}

func GetPopularTags() []models.Tag {
	var tags []models.Tag
	database.DB.Table("tags").
		Select("tags.*, COUNT(resource_tags.nomad_resource_id) as usage_count").
		Joins("JOIN resource_tags ON resource_tags.tag_id = tags.id").
		Group("tags.id").
		Order("usage_count DESC").
		Limit(12).
		Find(&tags)
	return tags
}
