package repositories

import (
	"rmbl/pkg/authhelpers"

	"github.com/gofiber/fiber/v2"
)

func Routes(route fiber.Router) {
	route.Get("/", GetAllRepositories)
	route.Get("/:org", GetOrgRepositories)
	route.Get("/:org/:reponame/*", GetRepository)
	route.Put("/:org/:reponame", authhelpers.Protected(), UpdateRepository)
	route.Post("/:org", authhelpers.Protected(), NewRepository)
	route.Delete("/:org/:reponame", authhelpers.Protected(), DeleteRepository)
}
