package handlers

import (
	"strconv"

	"rmbl/internal/database"
	"rmbl/internal/models"

	"github.com/gofiber/fiber/v2"
)

// RequireAdmin middleware ensures the user is an admin
func RequireAdmin(c *fiber.Ctx) error {
	userLoc := c.Locals("User")
	if userLoc == nil {
		return c.Redirect("/login")
	}
	user := userLoc.(models.User)
	if !user.IsAdmin {
		SetFlash(c, "error", "Access denied: Administrator privileges required.")
		return c.Redirect("/")
	}
	return c.Next()
}

func GetAdminDashboard(c *fiber.Ctx) error {
	var userCount int64
	var orgCount int64
	var resourceCount int64
	var starCount int64

	database.DB.Model(&models.User{}).Count(&userCount)
	database.DB.Model(&models.Organization{}).Count(&orgCount)
	database.DB.Model(&models.NomadResource{}).Count(&resourceCount)
	database.DB.Table("user_stars").Count(&starCount)

	var latestUsers []models.User
	database.DB.Order("created_at desc").Limit(5).Find(&latestUsers)

	var latestResources []models.NomadResource
	database.DB.Preload("User").Order("created_at desc").Limit(5).Find(&latestResources)

	return c.Render("admin/dashboard", MergeContext(BaseContext(c), fiber.Map{
		"UserCount":       userCount,
		"OrgCount":        orgCount,
		"ResourceCount":   resourceCount,
		"StarCount":       starCount,
		"LatestUsers":     latestUsers,
		"LatestResources": latestResources,
		"Page":            "admin",
	}), "layouts/main")
}

func GetAdminUsers(c *fiber.Ctx) error {
	var users []models.User
	database.DB.Order("id asc").Find(&users)

	return c.Render("admin/users", MergeContext(BaseContext(c), fiber.Map{
		"Users": users,
		"Page":  "admin_users",
	}), "layouts/main")
}

func GetAdminResources(c *fiber.Ctx) error {
	var resources []models.NomadResource
	database.DB.Preload("User").Order("id asc").Find(&resources)

	return c.Render("admin/resources", MergeContext(BaseContext(c), fiber.Map{
		"Resources": resources,
		"Page":      "admin_resources",
	}), "layouts/main")
}

// PostToggleAdmin toggles a user's admin status
func PostToggleAdmin(c *fiber.Ctx) error {
	userID := c.Params("id")

	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}

	// Prevent admin from removing their own admin status
	currentUser := c.Locals("User").(models.User)
	if currentUser.ID == user.ID {
		return c.Status(400).SendString("Cannot modify your own admin status")
	}

	// Toggle admin status
	user.IsAdmin = !user.IsAdmin
	database.DB.Save(&user)

	AuditLog(c, "admin.toggle_admin", "user", user.ID, user.Username, map[string]interface{}{
		"new_status": user.IsAdmin,
	})

	// Return updated user row HTML
	return c.Render("partials/admin_user_row", fiber.Map{
		"User":      user,
		"CSRFToken": c.Locals("CSRFToken"),
	})
}

// DeleteUser deletes a user account
func DeleteUser(c *fiber.Ctx) error {
	userID := c.Params("id")

	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}

	// Prevent admin from deleting themselves
	currentUser := c.Locals("User").(models.User)
	if currentUser.ID == user.ID {
		return c.Status(400).SendString("Cannot delete your own account")
	}

	AuditLog(c, "admin.delete_user", "user", user.ID, user.Username, nil)

	// Delete user (this will cascade delete resources, memberships, etc. if configured in the model)
	database.DB.Delete(&user)

	// Return empty response (HTMX will swap with empty content, removing the row)
	return c.SendString("")
}

// GetEditUser returns the edit modal for a user
func GetEditUser(c *fiber.Ctx) error {
	userID := c.Params("id")

	var user models.User
	if err := database.DB.Preload("Memberships.Organization").First(&user, userID).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}

	return c.Render("partials/admin_user_edit_modal", fiber.Map{
		"User":      user,
		"CSRFToken": c.Locals("CSRFToken"),
	})
}

// PostEditUser saves user edits
func PostEditUser(c *fiber.Ctx) error {
	userID := c.Params("id")

	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}

	type EditInput struct {
		Username      string `form:"username"`
		Name          string `form:"name"`
		Email         string `form:"email"`
		EmailVerified string `form:"email_verified"`
	}
	var input EditInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).SendString("Invalid input")
	}

	// Update user fields
	user.Username = input.Username
	user.Name = input.Name
	user.Email = input.Email
	user.EmailVerified = input.EmailVerified == "on"

	if err := database.DB.Save(&user).Error; err != nil {
		return c.Status(500).SendString("Failed to update user")
	}

	AuditLog(c, "admin.edit_user", "user", user.ID, user.Username, nil)

	// Return updated user row
	return c.Render("partials/admin_user_row", fiber.Map{
		"User":      user,
		"CSRFToken": c.Locals("CSRFToken"),
	})
}

// GetAdminOrganizations lists all organizations
func GetAdminOrganizations(c *fiber.Ctx) error {
	var orgs []models.Organization
	database.DB.Preload("Memberships.User").Preload("Resources").Order("id asc").Find(&orgs)

	return c.Render("admin/organizations", MergeContext(BaseContext(c), fiber.Map{
		"Organizations": orgs,
		"Page":          "admin_orgs",
	}), "layouts/main")
}

// GetEditOrganization returns the edit modal for an organization
func GetEditOrganization(c *fiber.Ctx) error {
	orgID := c.Params("id")

	var org models.Organization
	if err := database.DB.Preload("Memberships.User").Preload("Resources").First(&org, orgID).Error; err != nil {
		return c.Status(404).SendString("Organization not found")
	}

	return c.Render("partials/admin_org_edit_modal", fiber.Map{
		"Org":       org,
		"CSRFToken": c.Locals("CSRFToken"),
	})
}

// PostEditOrganization saves organization edits
func PostEditOrganization(c *fiber.Ctx) error {
	orgID := c.Params("id")

	var org models.Organization
	if err := database.DB.Preload("Memberships").Preload("Resources").First(&org, orgID).Error; err != nil {
		return c.Status(404).SendString("Organization not found")
	}

	type OrgEditInput struct {
		Name        string `form:"name"`
		Description string `form:"description"`
	}
	var input OrgEditInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).SendString("Invalid input")
	}

	// Update organization fields
	org.Name = input.Name
	org.Description = input.Description

	if err := database.DB.Save(&org).Error; err != nil {
		return c.Status(500).SendString("Failed to update organization")
	}

	AuditLog(c, "admin.edit_org", "organization", org.ID, org.Name, nil)

	// Return updated org row
	return c.Render("partials/admin_org_row", fiber.Map{
		"Org":       org,
		"CSRFToken": c.Locals("CSRFToken"),
	})
}

// DeleteOrganization deletes an organization and all its resources
func DeleteOrganization(c *fiber.Ctx) error {
	orgID := c.Params("id")

	var org models.Organization
	if err := database.DB.First(&org, orgID).Error; err != nil {
		return c.Status(404).SendString("Organization not found")
	}

	AuditLog(c, "admin.delete_org", "organization", org.ID, org.Name, nil)

	// Delete organization (cascade deletes resources and memberships)
	database.DB.Delete(&org)

	// Return empty response (HTMX will swap with empty content, removing the row)
	return c.SendString("")
}

// GetAdminSettings shows the site settings page
func GetAdminSettings(c *fiber.Ctx) error {
	var settings []models.SiteSetting
	database.DB.Find(&settings)

	settingsMap := make(map[string]string)
	for _, s := range settings {
		settingsMap[s.Key] = s.Value
	}

	return c.Render("admin/settings", MergeContext(BaseContext(c), fiber.Map{
		"Settings": settingsMap,
		"Page":     "admin_settings",
	}), "layouts/main")
}

// PostAdminSettings saves site settings
func PostAdminSettings(c *fiber.Ctx) error {
	type SettingsInput struct {
		GitHubRequestsRepo string `form:"github_requests_repo"`
		GitHubReleasesURL  string `form:"github_releases_url"`
	}
	var input SettingsInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).SendString("Invalid input")
	}

	// Note: GITHUB_REQUESTS_TOKEN is configured via environment variable, not here
	settings := map[string]string{
		"github_requests_repo": input.GitHubRequestsRepo,
		"github_releases_url":  input.GitHubReleasesURL,
	}

	for key, value := range settings {
		database.DB.Where(models.SiteSetting{Key: key}).
			Assign(models.SiteSetting{Value: value}).
			FirstOrCreate(&models.SiteSetting{})
	}

	AuditLog(c, "admin.update_settings", "settings", 0, "", nil)

	SetFlash(c, "success", "Settings updated successfully!")
	c.Set("HX-Redirect", "/admin/settings")
	return c.SendStatus(200)
}

// GetAdminRequests lists all pack/job requests for admin management
func GetAdminRequests(c *fiber.Ctx) error {
	var requests []models.PackRequest
	database.DB.Preload("User").Order("created_at desc").Find(&requests)

	return c.Render("admin/requests", MergeContext(BaseContext(c), fiber.Map{
		"Requests": requests,
		"Page":     "admin_requests",
	}), "layouts/main")
}

// PostUpdateRequestStatus updates a request's status
func PostUpdateRequestStatus(c *fiber.Ctx) error {
	id := c.Params("id")
	status := c.FormValue("status")

	// Validate status value
	validStatuses := map[string]bool{
		string(models.RequestStatusOpen):       true,
		string(models.RequestStatusInProgress): true,
		string(models.RequestStatusCompleted):  true,
		string(models.RequestStatusClosed):     true,
	}
	if !validStatuses[status] {
		return c.Status(400).SendString("Invalid status value")
	}

	var request models.PackRequest
	if err := database.DB.Preload("User").First(&request, id).Error; err != nil {
		return c.Status(404).SendString("Request not found")
	}

	request.Status = models.RequestStatus(status)
	database.DB.Save(&request)

	AuditLog(c, "admin.update_request_status", "request", request.ID, request.Title, map[string]interface{}{
		"new_status": status,
	})

	return c.Render("partials/admin_request_row", fiber.Map{
		"Request":   request,
		"CSRFToken": c.Locals("CSRFToken"),
	})
}

// GetAdminAudit shows the audit log viewer
func GetAdminAudit(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	if page < 1 {
		page = 1
	}
	actionFilter := c.Query("action", "")
	pageSize := 50

	var logs []models.AuditLog
	query := database.DB.Order("created_at desc").Limit(pageSize).Offset((page - 1) * pageSize)

	if actionFilter != "" {
		query = query.Where("action LIKE ?", actionFilter+"%")
	}

	query.Find(&logs)

	// Check if there are more pages
	var totalCount int64
	countQuery := database.DB.Model(&models.AuditLog{})
	if actionFilter != "" {
		countQuery = countQuery.Where("action LIKE ?", actionFilter+"%")
	}
	countQuery.Count(&totalCount)

	hasNextPage := int64(page*pageSize) < totalCount
	hasPrevPage := page > 1

	return c.Render("admin/audit", MergeContext(BaseContext(c), fiber.Map{
		"Logs":         logs,
		"CurrentPage":  page,
		"ActionFilter": actionFilter,
		"HasNextPage":  hasNextPage,
		"HasPrevPage":  hasPrevPage,
		"Page":         "admin_audit",
	}), "layouts/main")
}
