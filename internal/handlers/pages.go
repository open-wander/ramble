package handlers

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// docsBaseDir is the base directory for documentation files
const docsBaseDir = "docs"

// validateDocsPath validates that a file path is within the docs directory
// to prevent directory traversal attacks
func validateDocsPath(filePath string) (string, error) {
	// Clean the path to resolve . and .. elements
	cleanPath := filepath.Clean(filePath)

	// Get absolute paths for comparison
	absDocsDir, err := filepath.Abs(docsBaseDir)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", err
	}

	// Ensure the path is within the docs directory
	if !strings.HasPrefix(absPath, absDocsDir+string(os.PathSeparator)) && absPath != absDocsDir {
		return "", fiber.NewError(fiber.StatusBadRequest, "invalid path")
	}

	return cleanPath, nil
}

// DocPage represents a documentation page
type DocPage struct {
	Path  string
	Title string
}

// DocSection represents a section in the docs sidebar
type DocSection struct {
	Title string
	Pages []DocPage
}

// getDocSections returns the documentation structure
func getDocSections() []DocSection {
	return []DocSection{
		{
			Title: "Overview",
			Pages: []DocPage{
				{Path: "", Title: "Introduction"},
				{Path: "getting-started", Title: "Getting Started"},
			},
		},
		{
			Title: "Web Interface",
			Pages: []DocPage{
				{Path: "web-interface", Title: "Registry Guide"},
			},
		},
		{
			Title: "CLI",
			Pages: []DocPage{
				{Path: "cli/overview", Title: "CLI Overview"},
				{Path: "cli/pack", Title: "Pack Commands"},
				{Path: "cli/job", Title: "Job Commands"},
				{Path: "cli/registry", Title: "Registry Commands"},
				{Path: "cli/cache", Title: "Cache Commands"},
			},
		},
		{
			Title: "Reference",
			Pages: []DocPage{
				{Path: "api", Title: "API Reference"},
			},
		},
	}
}

// GetDocs godoc
// @Summary Get documentation page
// @Description Renders the documentation page with sidebar navigation.
// @Tags pages
// @Produce html
// @Success 200 {string} string "HTML content"
// @Router /docs [get]
func GetDocs(c *fiber.Ctx) error {
	return getDocsPage(c, "")
}

// GetDocsPage godoc
// @Summary Get specific documentation page
// @Description Renders a specific documentation page.
// @Tags pages
// @Produce html
// @Param page path string true "Doc page path"
// @Success 200 {string} string "HTML content"
// @Router /docs/{page} [get]
func GetDocsPage(c *fiber.Ctx) error {
	page := c.Params("page")
	subpage := c.Params("subpage")
	if subpage != "" {
		page = page + "/" + subpage
	}
	return getDocsPage(c, page)
}

func getDocsPage(c *fiber.Ctx, page string) error {
	// Determine which file to load
	var filePath string
	var title string

	if page == "" || page == "index" {
		filePath = "docs/index.md"
		title = "Documentation"
	} else {
		filePath = filepath.Join("docs", page+".md")
		title = getPageTitle(page)
	}

	// Validate file path to prevent directory traversal
	validPath, err := validateDocsPath(filePath)
	if err != nil {
		// Invalid path, use empty content
		return c.Render("docs", MergeContext(BaseContext(c), fiber.Map{
			"Page":        "docs",
			"Title":       "Documentation",
			"DocsContent": "",
			"DocSections": getDocSections(),
			"CurrentPage": page,
		}), "layouts/main")
	}

	// Try to read the validated file
	// G304: Path is validated above to be within docs directory
	content, err := os.ReadFile(validPath) //#nosec G304 -- path validated to be within docs directory
	docsContent := ""
	if err == nil {
		docsContent = string(content)
	} else {
		// Fallback to REQUIREMENTS.md for backward compatibility
		// This is a hardcoded path, not user-controlled
		content, err = os.ReadFile("REQUIREMENTS.md") //#nosec G304 -- hardcoded path, not user-controlled
		if err == nil {
			docsContent = string(content)
		}
	}

	return c.Render("docs", MergeContext(BaseContext(c), fiber.Map{
		"Page":        "docs",
		"Title":       title,
		"DocsContent": docsContent,
		"DocSections": getDocSections(),
		"CurrentPage": page,
	}), "layouts/main")
}

func getPageTitle(page string) string {
	titles := map[string]string{
		"getting-started": "Getting Started",
		"web-interface":   "Web Interface",
		"cli/overview":    "CLI Overview",
		"cli/pack":        "Pack Commands",
		"cli/job":         "Job Commands",
		"cli/registry":    "Registry Commands",
		"cli/cache":       "Cache Commands",
		"api":             "API Reference",
	}
	if title, ok := titles[page]; ok {
		return title + " - Documentation"
	}
	// Convert path to title
	parts := strings.Split(page, "/")
	last := parts[len(parts)-1]
	return strings.Title(strings.ReplaceAll(last, "-", " ")) + " - Documentation"
}

// GetAbout godoc
// @Summary Get about page
// @Description Renders the about page.
// @Tags pages
// @Produce html
// @Success 200 {string} string "HTML content"
// @Router /about [get]
func GetAbout(c *fiber.Ctx) error {
	return c.Render("about", MergeContext(BaseContext(c), fiber.Map{
		"Page":  "about",
		"Title": "About RMBL",
	}), "layouts/main")
}
