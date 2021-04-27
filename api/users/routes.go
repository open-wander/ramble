package users

import (
	"github.com/gofiber/fiber/v2"
)

func Routes(route fiber.Router) {
	route.Get("/", GetAllUsers)
	route.Get("/:id", GetUser)
	route.Get("/:username", GetUser)
	route.Post("/", CreateUser)
	route.Patch("/:id", UpdateUser)
	route.Delete("/:id", DeleteUser)
	// route.Post("/", middleware.Protected(), CreateUser)
	// route.Patch("/:id", middleware.Protected(), UpdateUser)
	// route.Delete("/:id", middleware.Protected(), DeleteUser)
}
