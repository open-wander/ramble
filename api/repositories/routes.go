package repositories

import (
	"rmbl/pkg/auth"

	"github.com/gofiber/fiber/v2"
)

func Routes(route fiber.Router) {
	route.Get("/", GetAllRepositories)
	route.Get("/:org", GetOrgRepositories)
	route.Get("/:org/:reponame/*", GetRepository)
	route.Put("/:org/:reponame", auth.Protected(), UpdateRepository)
	route.Post("/:org", auth.Protected(), NewRepository)
	route.Delete("/:org/:reponame", auth.Protected(), DeleteRepository)
}
