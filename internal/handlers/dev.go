package handlers

import (
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
)

type DevPack struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// GetDevelopmentPacks returns example packs for local development.
// This endpoint is disabled in production and not included in API docs.
func GetDevelopmentPacks(c *fiber.Ctx) error {
	packsDir := "examples"
	if _, err := os.Stat(packsDir); os.IsNotExist(err) {
		// Fallback for tests running in internal/handlers
		packsDir = "../../examples"
	}

	entries, err := os.ReadDir(packsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return c.Status(404).JSON(fiber.Map{"error": "Examples directory not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to read examples directory"})
	}

	var packs []DevPack
	for _, entry := range entries {
		if entry.IsDir() {
			// Basic validation: Check if metadata.hcl exists
			if _, err := os.Stat(filepath.Join(packsDir, entry.Name(), "metadata.hcl")); err == nil {
				packs = append(packs, DevPack{
					Name: entry.Name(),
					Path: filepath.Join(packsDir, entry.Name()),
				})
			}
		}
	}

	return c.JSON(packs)
}
