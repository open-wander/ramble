package handlers

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"rmbl/internal/services/resource"
)

// ToggleStar godoc
// @Summary Toggle star status
// @Description Star or unstar a resource for the authenticated user. Returns updated star button HTML.
// @Tags resources
// @Produce html
// @Param id path string true "Resource ID"
// @Success 200 {string} string "HTML fragment"
// @Failure 401 {string} string "Unauthorized"
// @Router /resource/{id}/star [post]
func ToggleStar(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, _ := Store.Get(c)
	userID := sess.Get("user_id").(uint)

	// Parse resource ID
	resourceIDUint64, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return c.Status(404).SendString("Resource not found")
	}
	resourceID := uint(resourceIDUint64)

	// Toggle star via service
	isStarred, starCount, err := resource.Service.ToggleStar(resourceID, userID)
	if err != nil {
		if errors.Is(err, resource.ErrResourceNotFound) {
			return c.Status(404).SendString("Resource not found")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update star status")
	}

	// Return partial button
	return c.Render("partials/star_button", fiber.Map{
		"Resource":  fiber.Map{"ID": resourceID},
		"IsStarred": isStarred,
		"StarCount": starCount,
	})
}
