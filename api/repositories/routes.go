package repositories

import (
	"github.com/gofiber/fiber/v2"
)

func Routes(route fiber.Router) {
	route.Get("/_catalog", GetAllRepositories)
	route.Get("/:org", GetOrgRepositories)
	route.Get("/:org/:name", GetRepository)
	route.Put("/:org/:name", UpdateRepository)
	route.Post("/:org", NewRepository)
	route.Delete("/:org/:name", DeleteRepository)
}
