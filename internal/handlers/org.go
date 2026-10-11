package handlers

import (
	"rmbl/internal/models"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func GetCreateOrg(c *fiber.Ctx) error {
	return c.Render("new_org", BaseContext(c), "layouts/main")
}

func PostCreateOrg(c *fiber.Ctx) error {
	sess, _ := Store.Get(c)
	userID := sess.Get("user_id").(uint)

	type OrgInput struct {
		Name        string `form:"name"`
		Description string `form:"description"`
	}

	var input OrgInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).SendString("Invalid input")
	}

	orgName := strings.TrimSpace(input.Name)
	if orgName == "" {
		return c.Status(400).SendString("Organization name is required")
	}

	if IsReservedName(orgName) {
		return c.Status(400).SendString("This name is reserved")
	}

	// Check if name is taken by a User or another Org
	var count int64
	reqDB(c).Model(&models.User{}).Where(iEq("username"), orgName).Count(&count)
	if count > 0 {
		return c.Status(400).SendString("This name is already taken by a user")
	}
	reqDB(c).Model(&models.Organization{}).Where(iEq("name"), orgName).Count(&count)
	if count > 0 {
		return c.Status(400).SendString("This organization name is already taken")
	}

	// Create Organization
	org := models.Organization{
		Name:        orgName,
		Description: input.Description,
	}

	if err := reqDB(c).Create(&org).Error; err != nil {
		return c.Status(500).SendString("Could not create organization")
	}

	// Create Membership as Owner
	membership := models.Membership{
		UserID:         userID,
		OrganizationID: org.ID,
		Role:           "owner",
	}
	reqDB(c).Create(&membership)

	AuditLog(c, "org.create", "organization", org.ID, org.Name, nil)

	SetFlash(c, "success", "Organization '"+org.Name+"' created successfully!")
	c.Set("HX-Redirect", "/"+org.Name)
	return c.SendStatus(200)
}

// Middleware to check if user is an owner of the organization
func RequireOrgOwner(c *fiber.Ctx) error {
	orgName := c.Params("orgname")
	sess, _ := Store.Get(c)
	userID := sess.Get("user_id").(uint)

	var org models.Organization
	if err := reqDB(c).Where(iEq("name"), orgName).First(&org).Error; err != nil {
		return c.Status(404).SendString("Organization not found")
	}

	var membership models.Membership
	if err := reqDB(c).Where("user_id = ? AND organization_id = ? AND role = 'owner'", userID, org.ID).First(&membership).Error; err != nil {
		return c.Status(403).SendString("You must be an owner of this organization")
	}

	c.Locals("OrgID", org.ID)
	return c.Next()
}

func GetOrgSettings(c *fiber.Ctx) error {
	orgName := c.Params("orgname")
	var org models.Organization
	if err := reqDB(c).Preload("Memberships.User").Where(iEq("name"), orgName).First(&org).Error; err != nil {
		return c.Status(404).SendString("Organization not found")
	}

	return c.Render("org_settings", MergeContext(BaseContext(c), fiber.Map{
		"Organization": org,
	}), "layouts/main")
}

func PostUpdateOrg(c *fiber.Ctx) error {
	orgID := c.Locals("OrgID").(uint)
	var org models.Organization
	if err := reqDB(c).First(&org, orgID).Error; err != nil {
		return c.Status(404).SendString("Organization not found")
	}

	org.Description = c.FormValue("description")
	if err := reqDB(c).Save(&org).Error; err != nil {
		return c.Status(500).SendString("Failed to update organization")
	}

	SetFlash(c, "success", "Organization settings updated.")
	c.Set("HX-Redirect", "/"+org.Name+"/settings")
	return c.SendStatus(200)
}

func PostAddMember(c *fiber.Ctx) error {
	orgID := c.Locals("OrgID").(uint)
	username := c.FormValue("username")
	role := c.FormValue("role")

	var org models.Organization
	if err := reqDB(c).First(&org, orgID).Error; err != nil {
		return c.Status(404).SendString("Organization not found")
	}

	var user models.User
	if err := reqDB(c).Where("username = ?", username).First(&user).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}

	// Check if already a member
	var existing models.Membership
	if err := reqDB(c).Where("user_id = ? AND organization_id = ?", user.ID, orgID).First(&existing).Error; err == nil {
		return c.Status(400).SendString("User is already a member")
	}

	membership := models.Membership{
		UserID:         user.ID,
		OrganizationID: orgID,
		Role:           role,
	}
	reqDB(c).Create(&membership)

	AuditLog(c, "org.add_member", "organization", org.ID, org.Name, map[string]interface{}{
		"member": username,
		"role":   role,
	})

	SetFlash(c, "success", "Member added successfully.")
	c.Set("HX-Refresh", "true")
	return c.SendStatus(200)
}

func PostRemoveMember(c *fiber.Ctx) error {
	orgID := c.Locals("OrgID").(uint)
	memberID, idErr := paramID(c, "member_id")
	if idErr != nil {
		return c.Status(404).SendString("Member not found")
	}

	var org models.Organization
	if err := reqDB(c).First(&org, orgID).Error; err != nil {
		return c.Status(404).SendString("Organization not found")
	}

	// Don't allow removing yourself if you're the only owner
	sess, _ := Store.Get(c)
	currentUserID := sess.Get("user_id").(uint)
	if memberID == currentUserID {
		var ownerCount int64
		reqDB(c).Model(&models.Membership{}).Where("organization_id = ? AND role = 'owner'", orgID).Count(&ownerCount)
		if ownerCount <= 1 {
			return c.Status(400).SendString("You cannot remove yourself as you are the only owner")
		}
	}

	AuditLog(c, "org.remove_member", "organization", org.ID, org.Name, map[string]interface{}{
		"member_id": memberID,
	})

	reqDB(c).Where("user_id = ? AND organization_id = ?", memberID, orgID).Delete(&models.Membership{})

	SetFlash(c, "success", "Member removed.")
	c.Set("HX-Refresh", "true")
	return c.SendStatus(200)
}
