package org

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"rmbl/internal/models"
	"rmbl/internal/names"
)

const pgUniqueViolation = "23505"

const nameClause = "LOWER(name) = LOWER(?)"

// CreateOrg creates the organisation and its owner membership in one
// transaction.
func CreateOrg(db *gorm.DB, ownerID uint, name, description string) (models.Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return models.Organization{}, ErrNameRequired
	}
	if !names.IsValid(name) {
		return models.Organization{}, ErrNameInvalid
	}
	if names.IsReserved(name) {
		return models.Organization{}, ErrNameReserved
	}

	org := models.Organization{Name: name, Description: description}
	err := db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.User{}).Where("LOWER(username) = LOWER(?)", name).Count(&count).Error; err != nil {
			return fmt.Errorf("org: check username: %w", err)
		}
		if count > 0 {
			return ErrNameTakenByUser
		}
		if err := tx.Model(&models.Organization{}).Where(nameClause, name).Count(&count).Error; err != nil {
			return fmt.Errorf("org: check name: %w", err)
		}
		if count > 0 {
			return ErrNameTaken
		}
		if err := tx.Create(&org).Error; err != nil {
			if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgUniqueViolation {
				return ErrNameTaken
			}
			return fmt.Errorf("org: create: %w", err)
		}
		m := models.Membership{UserID: ownerID, OrganizationID: org.ID, Role: RoleOwner}
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("org: create owner membership: %w", err)
		}
		return nil
	})
	if err != nil {
		return models.Organization{}, err
	}
	return org, nil
}

// MemberCount returns the number of live memberships of the organisation.
func MemberCount(db *gorm.DB, orgID uint) (int64, error) {
	var n int64
	if err := db.Model(&models.Membership{}).Where("organization_id = ?", orgID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("org: count members: %w", err)
	}
	return n, nil
}

// GetByName finds an organisation by name, ignoring case.
func GetByName(db *gorm.DB, name string) (models.Organization, error) {
	var org models.Organization
	if err := db.Where(nameClause, name).First(&org).Error; err != nil {
		return models.Organization{}, mapNotFound(err)
	}
	return org, nil
}

// GetByID finds an organisation by id.
func GetByID(db *gorm.DB, id uint) (models.Organization, error) {
	var org models.Organization
	if err := db.First(&org, id).Error; err != nil {
		return models.Organization{}, mapNotFound(err)
	}
	return org, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return fmt.Errorf("org: load: %w", err)
}

// MemberRole returns the user's role in the organisation, or ErrNotMember
// when there is no live membership.
func MemberRole(db *gorm.DB, orgID, userID uint) (string, error) {
	var m models.Membership
	err := db.Where("user_id = ? AND organization_id = ?", userID, orgID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrNotMember
	}
	if err != nil {
		return "", fmt.Errorf("org: load membership: %w", err)
	}
	return m.Role, nil
}

// RequireOwner returns ErrNotOwner unless the user is an owner of the
// organisation.
func RequireOwner(db *gorm.DB, orgID, userID uint) error {
	role, err := MemberRole(db, orgID, userID)
	if errors.Is(err, ErrNotMember) {
		return ErrNotOwner
	}
	if err != nil {
		return err
	}
	if role != RoleOwner {
		return ErrNotOwner
	}
	return nil
}

// UpdateDescription changes only the description column.
func UpdateDescription(db *gorm.DB, org *models.Organization, description string) error {
	if err := db.Model(org).Update("description", description).Error; err != nil {
		return fmt.Errorf("org: update description: %w", err)
	}
	return nil
}

// ParseRole validates a role strictly: only owner and member are accepted.
func ParseRole(s string) (string, error) {
	switch s {
	case RoleOwner, RoleMember:
		return s, nil
	}
	return "", ErrInvalidRole
}

// AddMember adds a user by username, restoring a soft-deleted membership.
// The role is checked strictly with ParseRole; callers that want the web
// form's lenient mapping normalise it first.
func AddMember(db *gorm.DB, orgID uint, username, role string) (models.Membership, error) {
	role, err := ParseRole(role)
	if err != nil {
		return models.Membership{}, err
	}

	var user models.User
	if err := db.Where("LOWER(username) = LOWER(?)", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Membership{}, ErrUserNotFound
		}
		return models.Membership{}, fmt.Errorf("org: load user: %w", err)
	}

	// Unscoped: idx_user_org covers soft-deleted rows, so a removed member
	// is restored rather than inserted again.
	var m models.Membership
	err = db.Unscoped().Where("user_id = ? AND organization_id = ?", user.ID, orgID).First(&m).Error
	switch {
	case err == nil && !m.DeletedAt.Valid:
		return models.Membership{}, ErrAlreadyMember
	case err == nil:
		if err := db.Unscoped().Model(&m).Updates(map[string]any{"deleted_at": nil, "role": role}).Error; err != nil {
			return models.Membership{}, fmt.Errorf("org: restore membership: %w", err)
		}
		m.DeletedAt.Valid = false
		m.Role = role
		return m, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		m = models.Membership{UserID: user.ID, OrganizationID: orgID, Role: role}
		if err := db.Create(&m).Error; err != nil {
			return models.Membership{}, fmt.Errorf("org: create membership: %w", err)
		}
		return m, nil
	}
	return models.Membership{}, fmt.Errorf("org: load membership: %w", err)
}

// RemoveMember deletes a live membership. Removing the only owner returns
// ErrLastOwner. The organisation row is locked so concurrent removals cannot
// leave it without an owner.
func RemoveMember(db *gorm.DB, orgID, userID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&models.Organization{}, orgID).Error; err != nil {
			return mapNotFound(err)
		}
		var m models.Membership
		err := tx.Where("user_id = ? AND organization_id = ?", userID, orgID).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotMember
		}
		if err != nil {
			return fmt.Errorf("org: load membership: %w", err)
		}
		if m.Role == RoleOwner {
			var owners int64
			if err := tx.Model(&models.Membership{}).Where("organization_id = ? AND role = ?", orgID, RoleOwner).Count(&owners).Error; err != nil {
				return fmt.Errorf("org: count owners: %w", err)
			}
			if owners <= 1 {
				return ErrLastOwner
			}
		}
		if err := tx.Delete(&m).Error; err != nil {
			return fmt.Errorf("org: delete membership: %w", err)
		}
		return nil
	})
}
