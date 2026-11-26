package handlers

import (
	"errors"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	"rmbl/internal/services/resource"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// resourceService returns the resource service bound to the request context
// and wired with the SSRF-protected file downloader.
func resourceService(c *fiber.Ctx) *resource.ResourceService {
	return resource.Service.WithContext(c.UserContext()).WithFileFetcher(downloadFile)
}

func GetNewResource(c *fiber.Ctx) error {
	isLoggedIn := c.Locals("UserID") != nil
	var user models.User
	if isLoggedIn {
		userID := c.Locals("UserID").(uint)
		reqDB(c).Preload("Memberships.Organization").First(&user, userID)
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
	if after, ok := strings.CutPrefix(input.Owner, "org:"); ok {
		id, _ := strconv.ParseUint(after, 10, 32)
		val := uint(id)
		orgID = &val
	}

	serviceInput := resource.CreateInput{
		Name:          input.Name,
		Type:          input.Type,
		Owner:         input.Owner,
		Description:   input.Description,
		RepositoryURL: input.RepositoryURL,
		FilePath:      input.FilePath,
		Version:       input.Version,
		License:       input.License,
		Tags:          input.Tags,
	}

	res, err := resourceService(c).CreateResource(serviceInput, userID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, resource.ErrOrgNotFound), errors.Is(err, resource.ErrNotOrgMember):
			return fiber.NewError(fiber.StatusForbidden, err.Error())
		case resource.IsInputError(err):
			return c.Status(fiber.StatusBadRequest).SendString(err.Error())
		}
		logger.FromContext(c.UserContext()).Error("resource.create_failed", logger.Err(err))
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Spawn background fetch for version content
	go fetchVersionContent(detachedLogCtx(c), res.ID, input.Version, input.RepositoryURL, string(res.Type), res.Name, res.FilePath)

	redirectPath := "/" + res.User.Username + "/" + res.Name
	if res.OrganizationID != nil {
		redirectPath = "/" + res.Organization.Name + "/" + res.Name
	}

	AuditLog(c, "resource.create", "resource", res.ID, res.Name, nil)
	SetFlash(c, "success", "Resource '"+res.Name+"' created!")
	c.Set("HX-Redirect", redirectPath)
	return c.SendStatus(fiber.StatusOK)
}

func GetNewVersion(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return c.Status(404).SendString("Resource not found")
	}
	return c.Render("partials/new_version_modal", fiber.Map{"ResourceID": id, "CSRFToken": c.Locals("CSRFToken")})
}

func PostNewVersion(c *fiber.Ctx) error {
	id, idErr := paramID(c, "id")
	if idErr != nil {
		return c.Status(404).SendString("Resource not found")
	}
	versionStr := c.FormValue("version")
	if versionStr == "" {
		return c.Status(400).SendString("Version is required")
	}

	// Authorization: read current user from session (same pattern as sibling handlers)
	sess, err := Store.Get(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	currentUserID, ok := sess.Get("user_id").(uint)
	if !ok || currentUserID == 0 {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}

	svc := resource.Service.WithContext(c.UserContext())
	version, err := svc.AddVersion(id, currentUserID, versionStr)
	if err != nil {
		switch {
		case errors.Is(err, resource.ErrResourceNotFound):
			return c.Status(404).SendString("Resource not found")
		case errors.Is(err, resource.ErrUnauthorized):
			return c.Status(403).SendString("You don't have permission to add versions to this resource")
		case errors.Is(err, resource.ErrInvalidVersion):
			return c.Status(400).SendString("Version is required and must be at most 64 characters")
		case errors.Is(err, resource.ErrDuplicateVersion):
			return c.Status(400).SendString("Version " + strings.TrimSpace(versionStr) + " already exists")
		}
		return c.Status(500).SendString("Could not add version")
	}

	var res models.NomadResource
	if err := reqDB(c).First(&res, id).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}
	AuditLog(c, "resource.version_create", "resource", res.ID, res.Name, map[string]any{"version": version.Version})
	go fetchVersionContent(detachedLogCtx(c), res.ID, version.Version, res.RepositoryURL, string(res.Type), res.Name, res.FilePath)
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
	if err := reqDB(c).Where(iEq("username"), namespace).First(&user).Error; err == nil {
		userID = user.ID
	} else {
		var org models.Organization
		if err := reqDB(c).Where(iEq("name"), namespace).First(&org).Error; err == nil {
			orgID = &org.ID
		} else {
			return c.Status(404).SendString("Namespace not found")
		}
	}
	var isAllowed bool
	if orgID != nil {
		var m models.Membership
		if err := reqDB(c).Where("user_id = ? AND organization_id = ?", currentUserID, *orgID).First(&m).Error; err == nil {
			isAllowed = true
		}
	} else {
		isAllowed = currentUserID == userID
	}
	if !isAllowed {
		return c.Status(403).SendString("You don't have permission to edit this resource")
	}
	var resource models.NomadResource
	dbQuery := reqDB(c).Preload("User").Preload("Tags")
	if orgID != nil {
		dbQuery = dbQuery.Where("organization_id = ?", *orgID).Where(iEq("name"), resourcename)
	} else {
		dbQuery = dbQuery.Where("user_id = ? AND organization_id IS NULL", userID).Where(iEq("name"), resourcename)
	}
	if err := dbQuery.First(&resource).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}
	var currentUser models.User
	reqDB(c).Preload("Memberships.Organization").First(&currentUser, currentUserID)
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
	id, idErr := paramID(c, "id")
	if idErr != nil {
		return c.Status(404).SendString("Resource not found")
	}
	sess, _ := Store.Get(c)
	currentUserID := sess.Get("user_id").(uint)

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

	if err := resource.Service.WithContext(c.UserContext()).UpdateResource(id, serviceInput, currentUserID); err != nil {
		if errors.Is(err, resource.ErrUnauthorized) {
			return c.Status(403).SendString("Unauthorized")
		}
		if errors.Is(err, resource.ErrResourceNotFound) {
			return c.Status(404).SendString("Resource not found")
		}
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}

	// Get updated resource for redirect
	var res models.NomadResource
	reqDB(c).First(&res, id)

	newNamespace := ""
	if res.OrganizationID != nil {
		var o models.Organization
		reqDB(c).First(&o, *res.OrganizationID)
		newNamespace = o.Name
	} else {
		var u models.User
		reqDB(c).First(&u, res.UserID)
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
	id, idErr := paramID(c, "id")
	if idErr != nil {
		return c.Status(404).SendString("Resource not found")
	}
	sess, err := Store.Get(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	currentUserID := sess.Get("user_id").(uint)

	var res models.NomadResource
	if err := reqDB(c).Preload("Versions", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at DESC")
	}).First(&res, id).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}

	// Check ownership: admin bypass preserved; otherwise use centralized authorization.
	var currentUser models.User
	reqDB(c).First(&currentUser, currentUserID)

	if !currentUser.IsAdmin {
		if authErr := resource.Service.WithContext(c.UserContext()).AuthorizeResourceAction(id, currentUserID, false); authErr != nil {
			if errors.Is(authErr, resource.ErrResourceNotFound) {
				return c.Status(404).SendString("Resource not found")
			}
			return c.Status(403).SendString("You don't have permission to retry this fetch")
		}
	}

	if len(res.Versions) == 0 {
		return c.Status(400).SendString("No versions found")
	}

	latest := res.Versions[0]
	if latest.FetchStatus != models.FetchStatusFailed {
		return c.Status(400).SendString("Latest version is not in failed state")
	}

	// Reset fetch status and clear error
	reqDB(c).Model(&latest).Updates(map[string]any{
		"fetch_status": models.FetchStatusPending,
		"fetch_error":  "",
	})

	go fetchVersionContent(detachedLogCtx(c), res.ID, latest.Version, res.RepositoryURL, string(res.Type), res.Name, res.FilePath)

	AuditLog(c, "resource.retry_fetch", "resource", res.ID, res.Name, nil)
	SetFlash(c, "success", "Fetch retry started for version "+latest.Version)
	c.Set("HX-Refresh", "true")
	return c.SendStatus(200)
}

func DeleteResource(c *fiber.Ctx) error {
	resourceID, err := paramID(c, "id")
	if err != nil {
		return c.Status(404).SendString("Resource not found")
	}
	sess, _ := Store.Get(c)
	userID := sess.Get("user_id").(uint)

	// Get resource name before deletion for audit log
	var res models.NomadResource
	if err := reqDB(c).First(&res, resourceID).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}
	resourceName := res.Name

	// Delete via service
	if err := resource.Service.WithContext(c.UserContext()).DeleteResource(resourceID, userID); err != nil {
		if errors.Is(err, resource.ErrUnauthorized) {
			return c.Status(403).SendString("Unauthorized")
		}
		if errors.Is(err, resource.ErrResourceNotFound) {
			return c.Status(404).SendString("Resource not found")
		}
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}

	AuditLog(c, "resource.delete", "resource", resourceID, resourceName, nil)
	SetFlash(c, "success", "Resource deleted successfully.")
	c.Set("HX-Redirect", "/")
	return c.SendStatus(200)
}
