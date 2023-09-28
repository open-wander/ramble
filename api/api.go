package api

import (
	"rmbl/api/authentication"
	"rmbl/api/controllers"
	"rmbl/api/organizations"
	"rmbl/api/users"

	"github.com/gofiber/fiber/v2"
)

func Setup(app *fiber.App) {
	auth := app.Group("/auth")
	authentication.Routes(auth)
	user := app.Group("/user")
	users.Routes(user)
	org := app.Group("/org")
	organizations.Routes(org)
	v1 := app.Group("/")
	controllers.Routes(v1)
}
