package repositories

import (
	"github.com/gofiber/fiber/v2"
)

func Routes(route fiber.Router) {
	route.Get("/_catalog", GetAllRepositories)
	route.Get("/:user", GetUserRepositories)
	route.Get("/:user/:name/*", GetRepository)
	route.Put("/:user/:name", UpdateRepository)
	route.Post("/:user", NewRepository)
	route.Delete("/:user/:name", DeleteRepository)
}
