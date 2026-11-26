package handlers

import "github.com/gofiber/fiber/v2"

// Error codes of the /v1 JSON API. Every /v1 endpoint reports failures through
// APIError so clients see one shape: {"error":{"code":"...","message":"..."}}.
const (
	codeUnauthorized      = "unauthorized"
	codeInsufficientScope = "insufficient_scope"
	codeForbidden         = "forbidden"
	codeEmailUnverified   = "email_unverified"
	codeRateLimited       = "rate_limited"

	codeInternal = "internal_error"

	msgUnauthenticated = "invalid or expired token"
)

// APIError writes the standard /v1 error body with the given status.
func APIError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{"error": fiber.Map{"code": code, "message": message}})
}
