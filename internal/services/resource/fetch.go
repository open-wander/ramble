package resource

import (
	"fmt"

	"rmbl/internal/models"
)

// CreateVersion creates a new version record for a resource
func (s *ResourceService) CreateVersion(resourceID uint, version string) (*models.ResourceVersion, error) {
	// Check if version already exists
	var existing models.ResourceVersion
	if err := s.db.Where("resource_id = ? AND version = ?", resourceID, version).First(&existing).Error; err == nil {
		return nil, fmt.Errorf("version already exists")
	}

	// Create new version
	newVersion := models.ResourceVersion{
		ResourceID: resourceID,
		Version:    version,
	}

	if err := s.db.Create(&newVersion).Error; err != nil {
		return nil, fmt.Errorf("could not create version: %w", err)
	}

	return &newVersion, nil
}

// NOTE: fetchVersionContent will be extracted here in a future iteration
// Currently it has deep dependencies on:
// - downloadFileWithRetry (HTTP utility)
// - isPermanentError (retry logic)
// - errgroup patterns (goroutine lifecycle)
// These work correctly in background.go and moving them risks breaking
// the background fetch lifecycle. For now, handlers continue calling
// fetchVersionContent directly after service.CreateResource returns.
