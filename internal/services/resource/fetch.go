package resource

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"rmbl/internal/models"
)

// Sentinel errors of the version operations.
var (
	ErrDuplicateVersion = errors.New("version already exists")
	ErrVersionNotFound  = errors.New("version not found")
	ErrVersionNotFailed = errors.New("version is not in failed state")
)

// ListVersions returns the versions of a resource, newest first, without the
// large content columns.
func (s *ResourceService) ListVersions(resourceID uint) ([]models.ResourceVersion, error) {
	var versions []models.ResourceVersion
	err := s.db.Select("id", "version", "fetch_status", "fetch_error", "created_at").
		Where("resource_id = ?", resourceID).
		Order("created_at DESC, id DESC").
		Find(&versions).Error
	if err != nil {
		return nil, fmt.Errorf("could not list versions: %w", err)
	}
	return versions, nil
}

// AddVersion authorizes the caller (a member of the owning organisation or the
// owning user), validates version and stores a new pending version. The
// resource row is locked for the duplicate check because resource_versions has
// no unique index on (resource_id, version): without the lock two concurrent
// calls could both insert.
func (s *ResourceService) AddVersion(resourceID, userID uint, version string) (*models.ResourceVersion, error) {
	version = strings.TrimSpace(version)
	if err := validateVersion(version); err != nil {
		return nil, err
	}
	if err := s.AuthorizeResourceAction(resourceID, userID, false); err != nil {
		return nil, err
	}

	created := models.ResourceVersion{ResourceID: resourceID, Version: version}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&models.NomadResource{}, resourceID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrResourceNotFound
			}
			return err
		}
		var n int64
		if err := tx.Model(&models.ResourceVersion{}).Where("resource_id = ? AND version = ?", resourceID, version).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrDuplicateVersion
		}
		return tx.Create(&created).Error
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateVersion) || errors.Is(err, ErrResourceNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("could not create version: %w", err)
	}
	return &created, nil
}

// RetryVersion resets a failed version to pending so its content can be
// fetched again. The reset is one conditional UPDATE, so of two concurrent
// retries exactly one succeeds and starts a fetch; the other gets
// ErrVersionNotFailed. A version that does not exist is ErrVersionNotFound.
func (s *ResourceService) RetryVersion(resourceID, userID uint, version string) (*models.ResourceVersion, error) {
	if err := s.AuthorizeResourceAction(resourceID, userID, false); err != nil {
		return nil, err
	}

	res := s.db.Model(&models.ResourceVersion{}).
		Where("resource_id = ? AND version = ? AND fetch_status = ?", resourceID, version, models.FetchStatusFailed).
		Updates(map[string]any{
			"fetch_status":       models.FetchStatusPending,
			"fetch_error":        "",
			"fetch_completed_at": nil,
		})
	if res.Error != nil {
		return nil, fmt.Errorf("could not reset version: %w", res.Error)
	}

	var v models.ResourceVersion
	q := s.db.Select("id", "resource_id", "version", "fetch_status", "fetch_error", "created_at").
		Where("resource_id = ? AND version = ?", resourceID, version).
		Order("id DESC").Limit(1).Find(&v)
	if q.Error != nil {
		return nil, fmt.Errorf("could not load version: %w", q.Error)
	}
	if res.RowsAffected == 0 {
		if v.ID == 0 {
			return nil, ErrVersionNotFound
		}
		return nil, ErrVersionNotFailed
	}
	return &v, nil
}

// NOTE: fetchVersionContent stays in the handlers package. It depends on
// downloadFileWithRetry (HTTP utility), isPermanentError (retry logic) and
// errgroup patterns (goroutine lifecycle), and moving it risks breaking the
// background fetch lifecycle. Handlers start it after the service call that
// created or reset the version returns.
