package authhelpers

import (
	appconfig "rmbl/pkg/config"

	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/jwt/v2"
)

// Protected protect routes
// Protected returns a middleware handler that protects routes by requiring a valid JWT token.
// It uses the JWT secret from the app configuration to validate the token.
// If the token is invalid or missing, it will invoke the jwtError function to handle the error.
func Protected() fiber.Handler {
	return jwtware.New(jwtware.Config{
		SigningKey:   []byte(appconfig.Config.Server.JWTSecret),
		ErrorHandler: jwtError,
	})
}

// jwtError handles JWT-related errors and returns an appropriate response.
// If the error is "Missing or malformed JWT", it returns a Bad Request response.
// Otherwise, it returns an Unauthorized response indicating an invalid or expired JWT.
func jwtError(c *fiber.Ctx, err error) error {
	if err.Error() == "Missing or malformed JWT" {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"Status": "Error", "Message": "Missing or malformed JWT", "Data": nil})
	}
	return c.Status(fiber.StatusUnauthorized).
		JSON(fiber.Map{"Status": "Error", "Message": "Invalid or expired JWT", "Data": nil})
}
