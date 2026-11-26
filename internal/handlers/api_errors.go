package handlers

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Error codes of the /v1 JSON API. Every /v1 endpoint reports failures through
// APIError so clients see one shape: {"error":{"code":"...","message":"..."}}.
const (
	codeUnauthorized      = "unauthorized"
	codeInsufficientScope = "insufficient_scope"
	codeForbidden         = "forbidden"
	codeEmailUnverified   = "email_unverified"
	codeRateLimited       = "rate_limited"
	codeNotFound          = "not_found"
	codeMethodNotAllowed  = "method_not_allowed"

	codeInternal = "internal_error"

	msgUnauthenticated = "invalid or expired token"
)

// APIErrorBody is the error object inside every /v1 error response.
type APIErrorBody struct {
	Code    string `json:"code" example:"not_found"`
	Message string `json:"message" example:"pack not found"`
}

// APIErrorResponse is the body of every /v1 error response.
type APIErrorResponse struct {
	Error APIErrorBody `json:"error"`
}

// APIError writes the standard /v1 error body with the given status.
func APIError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(APIErrorResponse{Error: APIErrorBody{Code: code, Message: message}})
}

// IsAPIPath reports whether p is under /v1. It is case-insensitive because
// Fiber routing is, and only selects the error format; it is not a security
// check (see isTokenAPIRequest for the CSRF rule).
func IsAPIPath(p string) bool {
	p = strings.ToLower(p)
	return p == "/v1" || strings.HasPrefix(p, "/v1/")
}

// APINotFound answers any /v1 path no route matched.
func APINotFound(c *fiber.Ctx) error {
	return APIError(c, fiber.StatusNotFound, codeNotFound, "not found")
}

// APIRateLimited writes the standard 429 for /v1 rate limiters.
func APIRateLimited(c *fiber.Ctx) error {
	return APIError(c, fiber.StatusTooManyRequests, codeRateLimited, "too many requests")
}

// APIErrorHandler is the Fiber ErrorHandler. /v1 paths get the JSON error
// shape with fixed messages (never err.Error(), which may carry internals);
// every other path keeps Fiber's default handling.
func APIErrorHandler(c *fiber.Ctx, err error) error {
	if !IsAPIPath(c.Path()) {
		return fiber.DefaultErrorHandler(c, err)
	}
	status, code, msg := fiber.StatusInternalServerError, codeInternal, "internal error"
	var fe *fiber.Error
	if errors.As(err, &fe) {
		switch fe.Code {
		case fiber.StatusBadRequest:
			status, code, msg = fe.Code, "bad_request", "bad request"
		case fiber.StatusForbidden:
			status, code, msg = fe.Code, codeForbidden, "forbidden"
		case fiber.StatusNotFound:
			status, code, msg = fe.Code, codeNotFound, "not found"
		case fiber.StatusMethodNotAllowed:
			status, code, msg = fe.Code, codeMethodNotAllowed, "method not allowed"
		case fiber.StatusTooManyRequests:
			status, code, msg = fe.Code, codeRateLimited, "too many requests"
		}
	}
	return APIError(c, status, code, msg)
}
