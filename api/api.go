package api

import (
	"rmbl/api/authentication"
	"rmbl/api/repositories"
	"rmbl/api/users"

	"github.com/gofiber/fiber/v2"
)

func version(c *fiber.Ctx) error {
	return c.SendString("v1")
}

func Setup(app *fiber.App) {
	v1 := app.Group("/v1")
	v1.Get("/", version)
	repositories.Routes(v1)
	user := app.Group("/user")
	users.Routes(user)
	auth := app.Group("/auth")
	authentication.Routes(auth)
}
