package authhelpers

import (
	appconfig "rmbl/pkg/config"

	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/jwt/v2"
)

// Protected protect routes
func Protected() fiber.Handler {
	return jwtware.New(jwtware.Config{
		SigningKey:   []byte(appconfig.Config.Server.JWTSecret),
		ErrorHandler: jwtError,
	})
}

func jwtError(c *fiber.Ctx, err error) error {
	if err.Error() == "Missing or malformed JWT" {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"Status": "Error", "Message": "Missing or malformed JWT", "Data": nil})
	}
	return c.Status(fiber.StatusUnauthorized).
		JSON(fiber.Map{"Status": "Error", "Message": "Invalid or expired JWT", "Data": nil})
}
