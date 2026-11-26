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
	"rmbl/internal/services/resource"
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
		if err := purgeDeletedOrgs(tx, name); err != nil {
			return err
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

// FindUser looks a user up by username, ignoring case. A missing user returns
// ErrUserNotFound.
func FindUser(db *gorm.DB, username string) (models.User, error) {
	var user models.User
	if err := db.Where("LOWER(username) = LOWER(?)", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.User{}, ErrUserNotFound
		}
		return models.User{}, fmt.Errorf("org: load user: %w", err)
	}
	return user, nil
}

// Member is one live member of an organisation.
type Member struct {
	Username string
	Role     string
}

// ListMembers returns the live members of the organisation ordered by
// username, ignoring case. Deleted users are left out. The result is never nil.
func ListMembers(db *gorm.DB, orgID uint) ([]Member, error) {
	members := make([]Member, 0)
	err := db.Model(&models.Membership{}).
		Select("users.username AS username, memberships.role AS role").
		Joins("JOIN users ON users.id = memberships.user_id AND users.deleted_at IS NULL").
		Where("memberships.organization_id = ?", orgID).
		Order("LOWER(users.username), users.id").
		Scan(&members).Error
	if err != nil {
		return nil, fmt.Errorf("org: list members: %w", err)
	}
	return members, nil
}

// SetRoleResult describes what SetMemberRole did. OldRole is empty when the
// membership was created or restored.
type SetRoleResult struct {
	Username string
	Role     string
	OldRole  string
	Created  bool
}

// SetMemberRole adds the user to the organisation with the role, restoring a
// soft-deleted membership, or changes the role of a live member. The actor
// must be an owner, checked under the organisation row lock so a concurrently
// demoted owner cannot still change members. Demoting the only owner returns
// ErrLastOwner. The role is checked strictly with ParseRole.
func SetMemberRole(db *gorm.DB, orgID, actorID uint, username, role string) (SetRoleResult, error) {
	var res SetRoleResult
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&models.Organization{}, orgID).Error; err != nil {
			return mapNotFound(err)
		}
		if err := RequireOwner(tx, orgID, actorID); err != nil {
			return err
		}
		parsed, err := ParseRole(role)
		if err != nil {
			return err
		}
		role := parsed
		user, err := FindUser(tx, username)
		if err != nil {
			return err
		}
		res = SetRoleResult{Username: user.Username, Role: role}

		var m models.Membership
		err = tx.Unscoped().Where("user_id = ? AND organization_id = ?", user.ID, orgID).First(&m).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			res.Created = true
			if err := tx.Create(&models.Membership{UserID: user.ID, OrganizationID: orgID, Role: role}).Error; err != nil {
				return fmt.Errorf("org: create membership: %w", err)
			}
			return nil
		case err != nil:
			return fmt.Errorf("org: load membership: %w", err)
		case m.DeletedAt.Valid:
			res.Created = true
			if err := tx.Unscoped().Model(&m).Updates(map[string]any{"deleted_at": nil, "role": role}).Error; err != nil {
				return fmt.Errorf("org: restore membership: %w", err)
			}
			return nil
		}

		res.OldRole = m.Role
		if m.Role == role {
			return nil
		}
		if m.Role == RoleOwner {
			if err := keepsAnOwner(tx, orgID, m.UserID); err != nil {
				return err
			}
		}
		if err := tx.Model(&m).Update("role", role).Error; err != nil {
			return fmt.Errorf("org: update role: %w", err)
		}
		return nil
	})
	if err != nil {
		return SetRoleResult{}, err
	}
	return res, nil
}

// AddMember adds a user by username, restoring a soft-deleted membership.
// The role is checked strictly with ParseRole; callers that want the web
// form's lenient mapping normalise it first.
func AddMember(db *gorm.DB, orgID uint, username, role string) (models.Membership, error) {
	role, err := ParseRole(role)
	if err != nil {
		return models.Membership{}, err
	}

	user, err := FindUser(db, username)
	if err != nil {
		return models.Membership{}, err
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
			if err := keepsAnOwner(tx, orgID, m.UserID); err != nil {
				return err
			}
		}
		if err := tx.Delete(&m).Error; err != nil {
			return fmt.Errorf("org: delete membership: %w", err)
		}
		return nil
	})
}

// DeleteOrg soft-deletes an empty organisation and its memberships. The
// caller must be an owner, checked again under the row lock. Live resources
// block the delete with a *NotEmptyError. The lock conflicts with the share
// lock CreateResource takes, so a resource cannot appear in an organisation
// being deleted.
func DeleteOrg(db *gorm.DB, orgID, userID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var org models.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&org, orgID).Error; err != nil {
			return mapNotFound(err)
		}
		if err := RequireOwner(tx, orgID, userID); err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&models.NomadResource{}).Where("organization_id = ?", orgID).Count(&n).Error; err != nil {
			return fmt.Errorf("org: count resources: %w", err)
		}
		if n > 0 {
			return &NotEmptyError{Count: n}
		}
		if err := tx.Where("organization_id = ?", orgID).Delete(&models.Membership{}).Error; err != nil {
			return fmt.Errorf("org: delete memberships: %w", err)
		}
		if err := tx.Delete(&org).Error; err != nil {
			return fmt.Errorf("org: delete: %w", err)
		}
		return nil
	})
}

// purgeDeletedOrgs hard-deletes soft-deleted organisations that hold name so
// it can be reused, with their memberships and soft-deleted resources. An
// organisation deleted by an admin can still own live resources; then the name
// stays taken. Tokens restricted to a purged organisation go with it through
// the foreign key; they could no longer authenticate anyway.
func purgeDeletedOrgs(tx *gorm.DB, name string) error {
	var ids []uint
	if err := tx.Unscoped().Model(&models.Organization{}).
		Where(nameClause, name).Where("deleted_at IS NOT NULL").
		Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("org: find deleted: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	var live int64
	if err := tx.Model(&models.NomadResource{}).Where("organization_id IN ?", ids).Count(&live).Error; err != nil {
		return fmt.Errorf("org: count resources of deleted: %w", err)
	}
	if live > 0 {
		return ErrNameTaken
	}
	// Resources first: their organization_id is ON DELETE SET NULL, which
	// would otherwise move them into a personal namespace.
	if err := resource.PurgeDeletedInOrgs(tx, ids); err != nil {
		return err
	}
	if err := tx.Unscoped().Where("organization_id IN ?", ids).Delete(&models.Membership{}).Error; err != nil {
		return fmt.Errorf("org: purge memberships: %w", err)
	}
	if err := tx.Unscoped().Where("id IN ?", ids).Delete(&models.Organization{}).Error; err != nil {
		return fmt.Errorf("org: purge: %w", err)
	}
	return nil
}

// keepsAnOwner returns ErrLastOwner unless the organisation has another
// owner, besides userID, whose user account is live. A soft-deleted user's
// owner membership does not count: such a user cannot act for the org.
func keepsAnOwner(tx *gorm.DB, orgID, userID uint) error {
	var others int64
	err := tx.Model(&models.Membership{}).
		Joins("JOIN users ON users.id = memberships.user_id AND users.deleted_at IS NULL").
		Where("memberships.organization_id = ? AND memberships.role = ? AND memberships.user_id <> ?", orgID, RoleOwner, userID).
		Count(&others).Error
	if err != nil {
		return fmt.Errorf("org: count owners: %w", err)
	}
	if others == 0 {
		return ErrLastOwner
	}
	return nil
}
