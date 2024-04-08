package api

import (
	"rmbl/api/controllers"

	"github.com/gofiber/fiber/v2"
)

// Setup sets up the routes for the API endpoints.
// It takes an instance of the fiber.App and configures the routes for authentication,
// user management, organization management, and repository management.
// The routes are grouped under different paths ("/auth", "/user", "/org", and "/").
func Setup(app *fiber.App) {
	auth := app.Group("/auth")
	controllers.AuthRoutes(auth)
	user := app.Group("/user")
	controllers.UserRoutes(user)
	org := app.Group("/org")
	controllers.OrgRoutes(org)
	v1 := app.Group("/")
	controllers.RepoRoutes(v1)
}
