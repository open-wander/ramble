package users

import (
	"rmbl/pkg/authhelpers"

	"github.com/gofiber/fiber/v2"
)

func Routes(route fiber.Router) {
	route.Get("/", authhelpers.Protected(), GetAllUsers)
	route.Get("/:id", authhelpers.Protected(), GetUser)
	route.Get("/:username", authhelpers.Protected(), GetUser)
	route.Put("/:id", authhelpers.Protected(), UpdateUser)
	route.Delete("/:id", authhelpers.Protected(), DeleteUser)
}
