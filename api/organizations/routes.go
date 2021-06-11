package organizations

import (
	"rmbl/pkg/authhelpers"

	"github.com/gofiber/fiber/v2"
)

func Routes(route fiber.Router) {
	route.Get("/", authhelpers.Protected(), GetAllOrgs)
}
