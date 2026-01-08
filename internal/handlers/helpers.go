package handlers

import "github.com/gofiber/fiber/v2"

// BaseContext returns a fiber.Map with common template values
// Use this as a starting point and add page-specific values
func BaseContext(c *fiber.Ctx) fiber.Map {
	return fiber.Map{
		"IsLoggedIn":    c.Locals("UserID") != nil,
		"Flash":         c.Locals("Flash"),
		"CSRFToken":     c.Locals("CSRFToken"),
		"CurrentUser":   c.Locals("User"),
		"LatestVersion": c.Locals("LatestVersion"),
	}
}

// MergeContext merges additional values into a base context
func MergeContext(base fiber.Map, extra fiber.Map) fiber.Map {
	for k, v := range extra {
		base[k] = v
	}
	return base
}
