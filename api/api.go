package api

import (
	"rmbl/api/authentication"
	"rmbl/api/repositories"
	"rmbl/api/users"

	"github.com/gofiber/fiber/v2"
)

func Setup(app *fiber.App) {
	v1 := app.Group("/")
	repositories.Routes(v1)
	user := app.Group("/user")
	users.Routes(user)
	auth := app.Group("/auth")
	authentication.Routes(auth)
}
