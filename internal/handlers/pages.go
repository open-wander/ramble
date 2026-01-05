package handlers

import (
	"os"

	"github.com/gofiber/fiber/v2"
)

// GetDocs godoc
// @Summary Get documentation page
// @Description Renders the documentation page, optionally loading content from REQUIREMENTS.md.
// @Tags pages
// @Produce html
// @Success 200 {string} string "HTML content"
// @Router /docs [get]
func GetDocs(c *fiber.Ctx) error {
	content, err := os.ReadFile("REQUIREMENTS.md")
	docsContent := ""
	if err == nil {
		docsContent = string(content)
	}

	return c.Render("docs", fiber.Map{
		"IsLoggedIn":  c.Locals("UserID") != nil,
		"Page":        "docs",
		"Title":       "Documentation",
		"DocsContent": docsContent,
		"Flash":       c.Locals("Flash"),
		"CSRFToken":   c.Locals("CSRFToken"),
		"CurrentUser": c.Locals("User"),
	}, "layouts/main")
}

// GetAbout godoc
// @Summary Get about page
// @Description Renders the about page.
// @Tags pages
// @Produce html
// @Success 200 {string} string "HTML content"
// @Router /about [get]
func GetAbout(c *fiber.Ctx) error {
	return c.Render("about", fiber.Map{
		"IsLoggedIn":  c.Locals("UserID") != nil,
		"Page":        "about",
		"Title":       "About RMBL",
		"Flash":       c.Locals("Flash"),
		"CSRFToken":   c.Locals("CSRFToken"),
		"CurrentUser": c.Locals("User"),
	}, "layouts/main")
}
