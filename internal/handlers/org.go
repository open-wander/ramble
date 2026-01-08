package handlers

import (
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strconv"
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

    // Check if name is taken by a User or another Org
    var count int64
    database.DB.Model(&models.User{}).Where("username ILIKE ?", orgName).Count(&count)
    if count > 0 {
        return c.Status(400).SendString("This name is already taken by a user")
    }
    database.DB.Model(&models.Organization{}).Where("name ILIKE ?", orgName).Count(&count)
    if count > 0 {
        return c.Status(400).SendString("This organization name is already taken")
    }

    // Create Organization
    org := models.Organization{
        Name:        orgName,
        Description: input.Description,
    }

    if err := database.DB.Create(&org).Error; err != nil {
        return c.Status(500).SendString("Could not create organization")
    }

    // Create Membership as Owner
    membership := models.Membership{
        UserID:         userID,
        OrganizationID: org.ID,
        Role:           "owner",
    }
    database.DB.Create(&membership)

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
	if err := database.DB.Where("name ILIKE ?", orgName).First(&org).Error; err != nil {
		return c.Status(404).SendString("Organization not found")
	}

	var membership models.Membership
	if err := database.DB.Where("user_id = ? AND organization_id = ? AND role = 'owner'", userID, org.ID).First(&membership).Error; err != nil {
		return c.Status(403).SendString("You must be an owner of this organization")
	}

	c.Locals("OrgID", org.ID)
	return c.Next()
}

func GetOrgSettings(c *fiber.Ctx) error {
	orgName := c.Params("orgname")
	var org models.Organization
	database.DB.Preload("Memberships.User").Where("name ILIKE ?", orgName).First(&org)

	return c.Render("org_settings", MergeContext(BaseContext(c), fiber.Map{
		"Organization": org,
	}), "layouts/main")
}

func PostUpdateOrg(c *fiber.Ctx) error {
	orgID := c.Locals("OrgID").(uint)
	var org models.Organization
	database.DB.First(&org, orgID)

	org.Description = c.FormValue("description")
	database.DB.Save(&org)

	SetFlash(c, "success", "Organization settings updated.")
	c.Set("HX-Redirect", "/"+org.Name+"/settings")
	return c.SendStatus(200)
}

func PostAddMember(c *fiber.Ctx) error {
	orgID := c.Locals("OrgID").(uint)
	username := c.FormValue("username")
	role := c.FormValue("role")

	var user models.User
	if err := database.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}

	// Check if already a member
	var existing models.Membership
	if err := database.DB.Where("user_id = ? AND organization_id = ?", user.ID, orgID).First(&existing).Error; err == nil {
		return c.Status(400).SendString("User is already a member")
	}

	membership := models.Membership{
		UserID:         user.ID,
		OrganizationID: orgID,
		Role:           role,
	}
	database.DB.Create(&membership)

	SetFlash(c, "success", "Member added successfully.")
	c.Set("HX-Refresh", "true")
	return c.SendStatus(200)
}

func PostRemoveMember(c *fiber.Ctx) error {
	orgID := c.Locals("OrgID").(uint)
	memberIDStr := c.Params("member_id")
	memberID, _ := strconv.ParseUint(memberIDStr, 10, 32)

	// Don't allow removing yourself if you're the only owner
	sess, _ := Store.Get(c)
	currentUserID := sess.Get("user_id").(uint)
	if uint(memberID) == currentUserID {
		var ownerCount int64
		database.DB.Model(&models.Membership{}).Where("organization_id = ? AND role = 'owner'", orgID).Count(&ownerCount)
		if ownerCount <= 1 {
			return c.Status(400).SendString("You cannot remove yourself as you are the only owner")
		}
	}

	database.DB.Where("user_id = ? AND organization_id = ?", uint(memberID), orgID).Delete(&models.Membership{})

	SetFlash(c, "success", "Member removed.")
	c.Set("HX-Refresh", "true")
	return c.SendStatus(200)
}