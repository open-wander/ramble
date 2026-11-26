package handlers

import (
	"errors"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	orgsvc "rmbl/internal/services/org"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
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

	org, err := orgsvc.CreateOrg(reqDB(c), userID, input.Name, input.Description)
	switch {
	case err == nil:
	case errors.Is(err, orgsvc.ErrNameRequired):
		return c.Status(400).SendString("Organization name is required")
	case errors.Is(err, orgsvc.ErrNameReserved):
		return c.Status(400).SendString("This name is reserved")
	case errors.Is(err, orgsvc.ErrNameTakenByUser):
		return c.Status(400).SendString("This name is already taken by a user")
	case errors.Is(err, orgsvc.ErrNameTaken):
		return c.Status(400).SendString("This organization name is already taken")
	default:
		logger.FromContext(c.UserContext()).Error("org.create_failed", logger.Err(err))
		return c.Status(500).SendString("Could not create organization")
	}

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

	org, err := orgsvc.GetByName(reqDB(c), orgName)
	if err != nil {
		return orgLoadError(c, err)
	}

	if err := orgsvc.RequireOwner(reqDB(c), org.ID, userID); err != nil {
		if errors.Is(err, orgsvc.ErrNotOwner) {
			return c.Status(403).SendString("You must be an owner of this organization")
		}
		logger.FromContext(c.UserContext()).Error("org.membership_load_failed", logger.Err(err))
		return c.Status(500).SendString("Internal server error")
	}

	c.Locals("OrgID", org.ID)
	return c.Next()
}

// orgLoadError answers a failed organisation lookup: 404 when the row is
// absent, otherwise 500 with an org.load_failed ERROR line.
func orgLoadError(c *fiber.Ctx, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, orgsvc.ErrNotFound) {
		return c.Status(404).SendString("Organization not found")
	}
	logger.FromContext(c.UserContext()).Error("org.load_failed", logger.Err(err))
	return c.Status(500).SendString("Internal server error")
}

func GetOrgSettings(c *fiber.Ctx) error {
	orgName := c.Params("orgname")
	var org models.Organization
	if err := reqDB(c).Preload("Memberships.User").Where(iEq("name"), orgName).First(&org).Error; err != nil {
		return orgLoadError(c, err)
	}

	return c.Render("org_settings", MergeContext(BaseContext(c), fiber.Map{
		"Organization": org,
	}), "layouts/main")
}

func PostUpdateOrg(c *fiber.Ctx) error {
	orgID := c.Locals("OrgID").(uint)
	org, err := orgsvc.GetByID(reqDB(c), orgID)
	if err != nil {
		return orgLoadError(c, err)
	}

	if err := orgsvc.UpdateDescription(reqDB(c), &org, c.FormValue("description")); err != nil {
		logger.FromContext(c.UserContext()).Error("org.update_failed", logger.Err(err))
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
	if role != "owner" {
		role = "member" // only two roles exist; same rule as the admin handlers
	}

	org, err := orgsvc.GetByID(reqDB(c), orgID)
	if err != nil {
		return orgLoadError(c, err)
	}

	if _, err := orgsvc.AddMember(reqDB(c), orgID, username, role); err != nil {
		switch {
		case errors.Is(err, orgsvc.ErrUserNotFound):
			return c.Status(404).SendString("User not found")
		case errors.Is(err, orgsvc.ErrAlreadyMember):
			return c.Status(400).SendString("User is already a member")
		}
		logger.FromContext(c.UserContext()).Error("org.member_add_failed", logger.Err(err))
		return c.Status(500).SendString("Failed to add member")
	}

	AuditLog(c, "org.add_member", "organization", org.ID, org.Name, map[string]any{
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

	org, err := orgsvc.GetByID(reqDB(c), orgID)
	if err != nil {
		return orgLoadError(c, err)
	}

	// An already-missing membership counts as removed, but is not audited.
	err = orgsvc.RemoveMember(reqDB(c), orgID, memberID)
	switch {
	case err == nil:
		AuditLog(c, "org.remove_member", "organization", org.ID, org.Name, map[string]any{
			"member_id": memberID,
		})
	case errors.Is(err, orgsvc.ErrNotMember):
	case errors.Is(err, orgsvc.ErrLastOwner):
		return c.Status(400).SendString("You cannot remove yourself as you are the only owner")
	case errors.Is(err, orgsvc.ErrNotFound):
		return c.Status(404).SendString("Organization not found")
	default:
		logger.FromContext(c.UserContext()).Error("org.member_remove_failed", logger.Err(err))
		return c.Status(500).SendString("Internal server error")
	}

	SetFlash(c, "success", "Member removed.")
	c.Set("HX-Refresh", "true")
	return c.SendStatus(200)
}
