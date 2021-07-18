package helpers

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

func ValidRequestHeader(c *fiber.Ctx) bool {
	wantedtype := "application/json"
	headertype := c.Get("Content-Type")

	if headertype != "" {
		if strings.Contains(headertype, wantedtype) {
			return true
		} else if c.Get("Content-Type") != "application/json" {
			return false
		}
	}
	return false
}
