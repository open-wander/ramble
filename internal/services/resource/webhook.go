package resource

import (
	"fmt"
	"time"

	"rmbl/internal/models"
)

// ProcessTagEvent checks if a version exists and creates it if new
// Returns true if a new version was created, false if it already existed
func (s *ResourceService) ProcessTagEvent(resourceID uint, newVersion string) (created bool, err error) {
	// Check if version already exists
	var exists int64
	s.db.Model(&models.ResourceVersion{}).Where("resource_id = ? AND version = ?", resourceID, newVersion).Count(&exists)

	if exists > 0 {
		return false, nil
	}

	// Create new version
	version := models.ResourceVersion{
		ResourceID: resourceID,
		Version:    newVersion,
	}

	if err := s.db.Create(&version).Error; err != nil {
		return false, fmt.Errorf("could not create version: %w", err)
	}

	return true, nil
}

// RecordWebhookSuccess updates the webhook status to success
func (s *ResourceService) RecordWebhookSuccess(resourceID uint) error {
	return s.db.Model(&models.NomadResource{}).
		Where("id = ?", resourceID).
		Updates(map[string]any{
			"last_webhook_delivery": time.Now(),
			"last_webhook_status":   "success",
			"last_webhook_error":    "",
		}).Error
}

// RecordWebhookFailure updates the webhook status to failure with error message
func (s *ResourceService) RecordWebhookFailure(resourceID uint, errMsg string) error {
	return s.db.Model(&models.NomadResource{}).
		Where("id = ?", resourceID).
		Updates(map[string]any{
			"last_webhook_delivery": time.Now(),
			"last_webhook_status":   "failure",
			"last_webhook_error":    errMsg,
		}).Error
}
