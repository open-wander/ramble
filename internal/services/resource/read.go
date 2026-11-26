package resource

import (
	"fmt"

	"gorm.io/gorm"
	"rmbl/internal/models"
)

// withAPIPreloads loads what the API view of a resource needs: owner, organisation,
// tags ordered by name and versions newest first. Versions are loaded without
// their content and readme. The id tie-break keeps the order stable when rows
// share a timestamp.
func withAPIPreloads(db *gorm.DB) *gorm.DB {
	return db.Preload("User").
		Preload("Organization").
		Preload("Tags", func(db *gorm.DB) *gorm.DB {
			return db.Order("tags.name")
		}).
		Preload("Versions", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "resource_id", "version", "created_at", "updated_at").
				Order("resource_versions.created_at DESC, resource_versions.id DESC")
		})
}

// GetResource loads a resource by ID with the API preloads. A missing or
// soft-deleted resource is ErrResourceNotFound; any other failure is wrapped.
func (s *ResourceService) GetResource(id uint) (*models.NomadResource, error) {
	var r models.NomadResource
	if err := withAPIPreloads(s.db).Limit(1).Find(&r, id).Error; err != nil {
		return nil, fmt.Errorf("could not load resource: %w", err)
	}
	if r.ID == 0 {
		return nil, ErrResourceNotFound
	}
	return &r, nil
}

// GetResourceByNamespace loads a resource by namespace and name, both matched
// case-insensitively. The namespace is a username first, then an organisation
// name, as in the web lookup. An unknown namespace and an unknown name both
// return ErrResourceNotFound.
func (s *ResourceService) GetResourceByNamespace(namespace, name string) (*models.NomadResource, error) {
	q := s.db.Where("LOWER(nomad_resources.name) = LOWER(?)", name)

	var user models.User
	if err := s.db.Where("LOWER(username) = LOWER(?)", namespace).Order("id").Limit(1).Find(&user).Error; err != nil {
		return nil, fmt.Errorf("could not look up user: %w", err)
	}
	if user.ID != 0 {
		q = q.Where("nomad_resources.user_id = ? AND nomad_resources.organization_id IS NULL", user.ID)
	} else {
		var org models.Organization
		if err := s.db.Where("LOWER(name) = LOWER(?)", namespace).Order("id").Limit(1).Find(&org).Error; err != nil {
			return nil, fmt.Errorf("could not look up organization: %w", err)
		}
		if org.ID == 0 {
			return nil, ErrResourceNotFound
		}
		q = q.Where("nomad_resources.organization_id = ?", org.ID)
	}

	var r models.NomadResource
	if err := withAPIPreloads(q).Order("nomad_resources.id").Limit(1).Find(&r).Error; err != nil {
		return nil, fmt.Errorf("could not load resource: %w", err)
	}
	if r.ID == 0 {
		return nil, ErrResourceNotFound
	}
	return &r, nil
}
