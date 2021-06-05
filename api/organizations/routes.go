package organizations

import (
	"rmbl/pkg/auth"

	"github.com/gofiber/fiber/v2"
)

func Routes(route fiber.Router) {
	route.Get("/", auth.Protected(), GetAllOrgs)
}
