package users

import (
	"rmbl/pkg/auth"

	"github.com/gofiber/fiber/v2"
)

func Routes(route fiber.Router) {
	route.Get("/", auth.Protected(), GetAllUsers)
	route.Get("/:id", auth.Protected(), GetUser)
	route.Get("/:username", auth.Protected(), GetUser)
	route.Patch("/:id", auth.Protected(), UpdateUser)
	route.Delete("/:id", auth.Protected(), DeleteUser)
}
