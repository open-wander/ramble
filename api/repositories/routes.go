package repositories

import (
	"github.com/gofiber/fiber/v2"
)

func Routes(route fiber.Router) {
	route.Get("/_catalog", GetAllRepositories)
	route.Get("/:org", GetOrgRepositories)
	route.Get("/:org/:reponame/*", GetRepository)
	route.Put("/:org/:reponame", UpdateRepository)
	route.Post("/:org", NewRepository)
	route.Delete("/:org/:reponame", DeleteRepository)
}
