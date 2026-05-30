package handlers

import (
	"context"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/resource"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func GetNewResource(c *fiber.Ctx) error {
	isLoggedIn := c.Locals("UserID") != nil
	var user models.User
	if isLoggedIn {
		userID := c.Locals("UserID").(uint)
		database.DB.Preload("Memberships.Organization").First(&user, userID)
	}
	var orgs []models.Organization
	for _, m := range user.Memberships {
		orgs = append(orgs, m.Organization)
	}
	return c.Render("new_resource", MergeContext(BaseContext(c), fiber.Map{
		"Provider":      user.Provider,
		"Organizations": orgs,
	}), "layouts/main")
}

// PostNewResource godoc
// @Summary Create a new resource
// @Description Register a new Nomad job or pack in the registry.
// @Tags resources
// @Accept x-www-form-urlencoded
// @Param name formData string true "Resource name"
// @Param type formData string true "Resource type (job, pack)"
// @Param owner formData string true "Namespace owner (user or org:ID)"
// @Param repository_url formData string true "Git repository URL"
// @Param version formData string true "Initial version"
// @Success 302 {string} string "Redirect to new resource"
// @Failure 400 {string} string "Bad Request"
// @Router /new [post]
func PostNewResource(c *fiber.Ctx) error {
	sess, err := Store.Get(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	userID := sess.Get("user_id").(uint)

	type ResourceInput struct {
		Name          string `form:"name"`
		Type          string `form:"type"`
		Owner         string `form:"owner"`
		Description   string `form:"description"`
		RepositoryURL string `form:"repository_url"`
		FilePath      string `form:"file_path"`
		Version       string `form:"version"`
		License       string `form:"license"`
		Tags          string `form:"tags"`
	}
	var input ResourceInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid input")
	}

	// Parse organization ID from owner field
	var orgID *uint
	if strings.HasPrefix(input.Owner, "org:") {
		id, _ := strconv.ParseUint(strings.TrimPrefix(input.Owner, "org:"), 10, 32)
		val := uint(id)
		orgID = &val
	}

	// Authorization: when an org is specified, verify the org exists and that the
	// authenticated user is a member (role 'member' or 'owner').
	if orgID != nil {
		var org models.Organization
		if err := database.DB.First(&org, *orgID).Error; err != nil {
			return fiber.NewError(fiber.StatusForbidden, "organization not found")
		}
		var membership models.Membership
		if err := database.DB.Where(
			"user_id = ? AND organization_id = ? AND role IN ('member','owner')",
			userID, *orgID,
		).First(&membership).Error; err != nil {
			return fiber.NewError(fiber.StatusForbidden, "you must be a member of this organization to create resources in it")
		}
	}

	// Download and detect license if not provided
	license := input.License
	if license == "" {
		if licBody, err := downloadFile(input.RepositoryURL, "LICENSE"); err == nil && licBody != "" {
			lines := strings.Split(licBody, "\n")
			if len(lines) > 0 {
				license = strings.TrimPrefix(strings.TrimSpace(lines[0]), "The ")
				if len(license) > 25 {
					license = license[:25] + "..."
				}
			}
		}
	}
	if license == "" {
		license = "Unknown"
	}

	// Create resource via service
	serviceInput := resource.CreateInput{
		Name:          input.Name,
		Type:          input.Type,
		Owner:         input.Owner,
		Description:   input.Description,
		RepositoryURL: input.RepositoryURL,
		FilePath:      input.FilePath,
		Version:       input.Version,
		License:       license,
		Tags:          input.Tags,
	}

	res, err := resource.Service.CreateResource(serviceInput, userID, orgID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}

	// Spawn background fetch for version content
	go fetchVersionContent(context.Background(), res.ID, input.Version, input.RepositoryURL, string(res.Type), res.Name, res.FilePath)

	// Build redirect path
	redirectPath := "/"
	var user models.User
	database.DB.First(&user, userID)
	if orgID != nil {
		var org models.Organization
		database.DB.First(&org, *orgID)
		redirectPath = "/" + org.Name + "/" + res.Name
	} else {
		redirectPath = "/" + user.Username + "/" + res.Name
	}

	AuditLog(c, "resource.create", "resource", res.ID, res.Name, nil)
	SetFlash(c, "success", "Resource '"+res.Name+"' created!")
	c.Set("HX-Redirect", redirectPath)
	return c.SendStatus(fiber.StatusOK)
}

func GetNewVersion(c *fiber.Ctx) error {
	id := c.Params("id")
	return c.Render("partials/new_version_modal", fiber.Map{"ResourceID": id, "CSRFToken": c.Locals("CSRFToken")})
}

func PostNewVersion(c *fiber.Ctx) error {
	idStr := c.Params("id")
	versionStr := c.FormValue("version")
	id, _ := strconv.ParseUint(idStr, 10, 32)
	if versionStr == "" {
		return c.Status(400).SendString("Version is required")
	}
	var resource models.NomadResource
	if err := database.DB.First(&resource, uint(id)).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}
	version := models.ResourceVersion{ResourceID: uint(id), Version: versionStr}
	if err := database.DB.Create(&version).Error; err != nil {
		return c.Status(500).SendString("Could not add version")
	}
	go fetchVersionContent(context.Background(), resource.ID, versionStr, resource.RepositoryURL, string(resource.Type), resource.Name, resource.FilePath)
	SetFlash(c, "success", "Version "+version.Version+" added!")
	c.Set("HX-Refresh", "true")
	return c.SendStatus(200)
}

func GetEditResource(c *fiber.Ctx) error {
	namespace := c.Params("username")
	resourcename := c.Params("resourcename")
	sess, _ := Store.Get(c)
	currentUserID := sess.Get("user_id").(uint)
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
			return c.Status(404).SendString("Namespace not found")
		}
	}
	var isAllowed bool
	if orgID != nil {
		var m models.Membership
		if err := database.DB.Where("user_id = ? AND organization_id = ?", currentUserID, *orgID).First(&m).Error; err == nil {
			isAllowed = true
		}
	} else {
		isAllowed = currentUserID == userID
	}
	if !isAllowed {
		return c.Status(403).SendString("You don't have permission to edit this resource")
	}
	var resource models.NomadResource
	dbQuery := database.DB.Preload("User").Preload("Tags")
	if orgID != nil {
		dbQuery = dbQuery.Where("organization_id = ? AND name ILIKE ?", *orgID, resourcename)
	} else {
		dbQuery = dbQuery.Where("user_id = ? AND organization_id IS NULL AND name ILIKE ?", userID, resourcename)
	}
	if err := dbQuery.First(&resource).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}
	var currentUser models.User
	database.DB.Preload("Memberships.Organization").First(&currentUser, currentUserID)
	var orgs []models.Organization
	for _, m := range currentUser.Memberships {
		orgs = append(orgs, m.Organization)
	}
	var tagNames []string
	for _, t := range resource.Tags {
		tagNames = append(tagNames, t.Name)
	}
	return c.Render("edit_resource", MergeContext(BaseContext(c), fiber.Map{
		"Resource":      resource,
		"TagsString":    strings.Join(tagNames, ", "),
		"Organizations": orgs,
	}), "layouts/main")
}

// PostEditResource godoc
// @Summary Update resource details
// @Description Update the metadata, repository URL, or tags for an existing resource.
// @Tags resources
// @Accept x-www-form-urlencoded
// @Param id path string true "Resource ID"
// @Param name formData string true "New resource name"
// @Param type formData string true "New resource type"
// @Param description formData string false "New description"
// @Success 200 {string} string "OK"
// @Failure 403 {string} string "Unauthorized"
// @Router /resource/{id}/edit [post]
func PostEditResource(c *fiber.Ctx) error {
	idStr := c.Params("id")
	sess, _ := Store.Get(c)
	currentUserID := sess.Get("user_id").(uint)
	id, _ := strconv.ParseUint(idStr, 10, 32)

	type EditInput struct {
		Name          string `form:"name"`
		Type          string `form:"type"`
		Owner         string `form:"owner"`
		Description   string `form:"description"`
		RepositoryURL string `form:"repository_url"`
		FilePath      string `form:"file_path"`
		License       string `form:"license"`
		Tags          string `form:"tags"`
	}
	var input EditInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).SendString("Invalid input")
	}

	// Update resource via service
	serviceInput := resource.UpdateInput{
		Name:          input.Name,
		Type:          input.Type,
		Owner:         input.Owner,
		Description:   input.Description,
		RepositoryURL: input.RepositoryURL,
		FilePath:      input.FilePath,
		License:       input.License,
		Tags:          input.Tags,
	}

	if err := resource.Service.UpdateResource(uint(id), serviceInput, currentUserID); err != nil {
		if err.Error() == "unauthorized" {
			return c.Status(403).SendString("Unauthorized")
		}
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}

	// Get updated resource for redirect
	var res models.NomadResource
	database.DB.First(&res, uint(id))

	newNamespace := ""
	if res.OrganizationID != nil {
		var o models.Organization
		database.DB.First(&o, *res.OrganizationID)
		newNamespace = o.Name
	} else {
		var u models.User
		database.DB.First(&u, res.UserID)
		newNamespace = u.Username
	}

	AuditLog(c, "resource.edit", "resource", res.ID, res.Name, nil)
	SetFlash(c, "success", "Resource updated successfully!")
	c.Set("HX-Redirect", "/"+newNamespace+"/"+res.Name)
	return c.SendStatus(200)
}

// DeleteResource godoc
// @Summary Delete a resource
// @Description Permantently remove a resource and all its versions from the registry.
// @Tags resources
// @Param id path string true "Resource ID"
// @Success 200 {string} string "OK"
// @Failure 403 {string} string "Unauthorized"
// @Router /resource/{id} [delete]
func PostRetryFetch(c *fiber.Ctx) error {
	idStr := c.Params("id")
	sess, err := Store.Get(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	currentUserID := sess.Get("user_id").(uint)

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(404).SendString("Resource not found")
	}

	var res models.NomadResource
	if err := database.DB.Preload("Versions", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at DESC")
	}).First(&res, uint(id)).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}

	// Check ownership: user is resource owner, org member, or admin
	var currentUser models.User
	database.DB.First(&currentUser, currentUserID)

	isAllowed := currentUser.IsAdmin
	if !isAllowed {
		if res.OrganizationID != nil {
			var m models.Membership
			if err := database.DB.Where("user_id = ? AND organization_id = ?", currentUserID, *res.OrganizationID).First(&m).Error; err == nil {
				isAllowed = true
			}
		} else {
			isAllowed = currentUserID == res.UserID
		}
	}
	if !isAllowed {
		return c.Status(403).SendString("You don't have permission to retry this fetch")
	}

	if len(res.Versions) == 0 {
		return c.Status(400).SendString("No versions found")
	}

	latest := res.Versions[0]
	if latest.FetchStatus != models.FetchStatusFailed {
		return c.Status(400).SendString("Latest version is not in failed state")
	}

	// Reset fetch status and clear error
	database.DB.Model(&latest).Updates(map[string]interface{}{
		"fetch_status": models.FetchStatusPending,
		"fetch_error":  "",
	})

	go fetchVersionContent(context.Background(), res.ID, latest.Version, res.RepositoryURL, string(res.Type), res.Name, res.FilePath)

	AuditLog(c, "resource.retry_fetch", "resource", res.ID, res.Name, nil)
	SetFlash(c, "success", "Fetch retry started for version "+latest.Version)
	c.Set("HX-Refresh", "true")
	return c.SendStatus(200)
}

func DeleteResource(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, _ := Store.Get(c)
	userID := sess.Get("user_id").(uint)

	// Parse resource ID
	resourceIDUint64, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return c.Status(404).SendString("Resource not found")
	}
	resourceID := uint(resourceIDUint64)

	// Get resource name before deletion for audit log
	var res models.NomadResource
	if err := database.DB.First(&res, resourceID).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}
	resourceName := res.Name

	// Delete via service
	if err := resource.Service.DeleteResource(resourceID, userID); err != nil {
		if err.Error() == "unauthorized" {
			return c.Status(403).SendString("Unauthorized")
		}
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}

	AuditLog(c, "resource.delete", "resource", resourceID, resourceName, nil)
	SetFlash(c, "success", "Resource deleted successfully.")
	c.Set("HX-Redirect", "/")
	return c.SendStatus(200)
}
