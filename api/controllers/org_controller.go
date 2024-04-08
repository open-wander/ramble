package controllers

import (
	"fmt"
	"strconv"
	"strings"

	"rmbl/api/organizations"
	"rmbl/pkg/authhelpers"
	"rmbl/pkg/database"

	jwt "github.com/form3tech-oss/jwt-go"
	"github.com/gofiber/fiber/v2"
)

// OrgRoutes registers the organization routes on the provided fiber.Router.
// It adds a GET route for retrieving all organizations, protected by authentication.
// The GetAllOrgs handler function is invoked when the route is accessed.
func OrgRoutes(route fiber.Router) {
	route.Get("/", authhelpers.Protected(), GetAllOrgs)
}

// GetAllOrgs retrieves all organizations.
// It requires the user to be a site admin.
// The function accepts optional query parameters for filtering and pagination.
// The function returns a JSON response containing the organizations.
func GetAllOrgs(c *fiber.Ctx) error {
	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	is_site_admin := claims["site_admin"].(bool)
	repos := c.Query("repositories", "false")
	var includeRepos bool
	if !is_site_admin {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}

	// Parse the context for the params or use defaults.
	search := strings.ToLower(c.Query("search"))

	order := strings.ToUpper(c.Query("order", "DESC"))

	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil {
		return fmt.Errorf("unable to parse 'offset': %w", err)
	}

	limit, err := strconv.Atoi(c.Query("limit", "25"))
	if err != nil {
		return fmt.Errorf("unable to parse 'limit': %w", err)
	}

	if repos == "true" {
		includeRepos = true
	}
	orgservice, err := organizations.NewOrgService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}

	data := orgservice.GetAllOrgs(order, search, limit, offset, includeRepos)
	return c.JSON(data)
}
