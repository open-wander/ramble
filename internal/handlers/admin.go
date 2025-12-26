package handlers

import (
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

	return c.Render("admin/dashboard", fiber.Map{
		"UserCount":       userCount,
		"OrgCount":        orgCount,
		"ResourceCount":   resourceCount,
		"StarCount":       starCount,
		"LatestUsers":     latestUsers,
		"LatestResources": latestResources,
		"IsLoggedIn":      true,
		"CurrentUser":     c.Locals("User"),
		"Page":            "admin",
		"CSRFToken":       c.Locals("CSRFToken"),
	}, "layouts/main")
}

func GetAdminUsers(c *fiber.Ctx) error {
	var users []models.User
	database.DB.Order("id asc").Find(&users)

	return c.Render("admin/users", fiber.Map{
		"Users":       users,
		"IsLoggedIn":  true,
		"CurrentUser": c.Locals("User"),
		"Page":        "admin_users",
		"CSRFToken":   c.Locals("CSRFToken"),
	}, "layouts/main")
}

func GetAdminResources(c *fiber.Ctx) error {
	var resources []models.NomadResource
	database.DB.Preload("User").Order("id asc").Find(&resources)

	return c.Render("admin/resources", fiber.Map{
		"Resources":   resources,
		"IsLoggedIn":  true,
		"CurrentUser": c.Locals("User"),
		"Page":        "admin_resources",
		"CSRFToken":   c.Locals("CSRFToken"),
	}, "layouts/main")
}
