package handlers

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"rmbl/internal/database"
)

// reqDB returns the database bound to the request's user context, so GORM's
// slow-query and error lines go through the request logger and carry
// request_id. It reads database.DB on every call. Call it once per query;
// never hand the result to a goroutine that outlives the request.
func reqDB(c *fiber.Ctx) *gorm.DB {
	return database.DB.WithContext(c.UserContext())
}
