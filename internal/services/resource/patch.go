package resource

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"rmbl/internal/models"
	"rmbl/internal/security"
)

// PatchInput is a partial update of a resource. A nil field is left unchanged;
// an empty string clears description and file path, an empty license becomes
// "Unknown" and an empty Tags list removes every tag.
type PatchInput struct {
	Description   *string
	RepositoryURL *string
	FilePath      *string
	License       *string
	Tags          *[]string
}

// PatchResource applies in to resource id. It authorizes (any org member, or
// the owning user of a personal resource) and validates before the first
// write, then writes tags and columns in one transaction. Only the named
// columns are written, so concurrent star, download and webhook updates are
// not overwritten.
func (s *ResourceService) PatchResource(id, userID uint, in PatchInput) error {
	if err := s.AuthorizeResourceAction(id, userID, false); err != nil {
		return err
	}
	if in.RepositoryURL != nil {
		if err := security.ValidateRepositoryURL(*in.RepositoryURL); err != nil {
			return ErrInvalidRepositoryURL
		}
	}

	// updated_at keeps the statement non-empty for a tags-only or empty patch.
	cols := map[string]any{"updated_at": gorm.Expr("NOW()")}
	if in.Description != nil {
		cols["description"] = *in.Description
	}
	if in.RepositoryURL != nil {
		cols["repository_url"] = *in.RepositoryURL
	}
	if in.FilePath != nil {
		cols["file_path"] = *in.FilePath
	}
	if in.License != nil {
		license := strings.TrimSpace(*in.License)
		if license == "" {
			license = unknownLicense
		}
		cols["license"] = license
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if in.Tags != nil {
			tags := (&ResourceService{db: tx}).resolveTags(strings.Join(*in.Tags, ","))
			if err := tx.Model(&models.NomadResource{ID: id}).Association("Tags").Replace(tags); err != nil {
				return fmt.Errorf("failed to update tags: %w", err)
			}
		}
		if err := tx.Model(&models.NomadResource{}).Where("id = ?", id).Updates(cols).Error; err != nil {
			return fmt.Errorf("failed to update resource: %w", err)
		}
		return nil
	})
}
