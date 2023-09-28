package controllers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"rmbl/api/repositories"
	"rmbl/pkg/authhelpers"
)

// Routes defines all the API routes available for the repositories controller.
func Routes(route fiber.Router) {
	route.Get("/", GetAllRepositories)
	route.Get("/:org", repositories.GetOrgRepositories)
	route.Get("/:org/:reponame/*", repositories.GetRepository)
	route.Put("/:org/:reponame", authhelpers.Protected(), repositories.UpdateRepository)
	route.Post("/:org", authhelpers.Protected(), repositories.NewRepository)
	route.Delete("/:org/:reponame", authhelpers.Protected(), repositories.DeleteRepository)
}

// GetAllRepositories is called by the API and should handle obtaining the repositories
// stored in the application's database.
func GetAllRepositories(c *fiber.Ctx) error {
	// Parse the context for the params or use defaults.
	search := strings.ToLower(c.Query("search"))

	order, err := strconv.ParseBool(c.Query("order", "true"))
	if err != nil {
		return fmt.Errorf("unable to parse 'order' expected boolean: %w", err)
	}

	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil {
		return fmt.Errorf("unable to parse 'offset': %w", err)
	}

	limit, err := strconv.Atoi(c.Query("limit", "25"))
	if err != nil {
		return fmt.Errorf("unable to parse 'limit': %w", err)
	}

	// Call the repository layer for the data
	data := repositories.GetAllRepositories(order, search, limit, offset)

	return c.Status(200).JSON(data)
}
