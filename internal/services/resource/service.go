package resource

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"rmbl/internal/models"
)

var tagRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

// Sentinel errors for authorization checks.
var (
	ErrUnauthorized    = errors.New("unauthorized")
	ErrResourceNotFound = errors.New("resource not found")
)

// Role constants matching models.Membership.Role values.
const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

// ResourceServicer defines business logic operations for resources
type ResourceServicer interface {
	CreateResource(input CreateInput, userID uint, orgID *uint) (*models.NomadResource, error)
	UpdateResource(id uint, input UpdateInput, userID uint) error
	DeleteResource(id uint, userID uint) error
	ToggleStar(resourceID, userID uint) (isStarred bool, starCount int64, err error)
	ValidateOwnership(resourceID, userID uint) (bool, error)
	ValidateOrgMembership(userID, orgID uint) (bool, error)
	GenerateWebhookSecret() (string, error)
	ResetWebhookSecret(resourceID, userID uint) error
	// AuthorizeResourceAction checks whether userID may perform an action on the
	// given resource. Pass requireOwner=true for destructive actions (delete).
	// Returns ErrResourceNotFound or ErrUnauthorized as appropriate.
	AuthorizeResourceAction(resourceID, userID uint, requireOwner bool) error
	// RecordFetchFailed marks a resource version fetch as FAILED with the supplied
	// error message. It updates only FetchStatus, FetchError, and FetchCompletedAt;
	// it never clears content columns.
	RecordFetchFailed(resourceID uint, version, errMsg string) error
}

// ResourceService implements ResourceServicer
type ResourceService struct {
	db *gorm.DB
}

// NewResourceService creates a new resource service
func NewResourceService(db *gorm.DB) *ResourceService {
	return &ResourceService{db: db}
}

// Package-level service variable for global access
var Service *ResourceService

// Init initializes the resource service
func Init(db *gorm.DB) {
	Service = NewResourceService(db)
}

// CreateInput defines the input for creating a resource
type CreateInput struct {
	Name          string
	Type          string
	Owner         string
	Description   string
	RepositoryURL string
	FilePath      string
	Version       string
	License       string
	Tags          string
}

// UpdateInput defines the input for updating a resource
type UpdateInput struct {
	Name          string
	Type          string
	Owner         string
	Description   string
	RepositoryURL string
	FilePath      string
	License       string
	Tags          string
}

// CreateResource creates a new resource with business validation
func (s *ResourceService) CreateResource(input CreateInput, userID uint, orgID *uint) (*models.NomadResource, error) {
	// Business validation
	if input.Name == "" || input.Version == "" {
		return nil, fmt.Errorf("name and version are required")
	}

	// Check for duplicate resource name in namespace
	var existing models.NomadResource
	dbQuery := s.db.Where("name = ?", input.Name)
	if orgID != nil {
		dbQuery = dbQuery.Where("organization_id = ?", *orgID)
	} else {
		dbQuery = dbQuery.Where("user_id = ? AND organization_id IS NULL", userID)
	}
	if err := dbQuery.First(&existing).Error; err == nil {
		return nil, fmt.Errorf("a resource with this name already exists in this namespace")
	}

	// License is accepted as-is (handler's responsibility to set default "Unknown")
	license := input.License
	if license == "" {
		license = "Unknown"
	}

	// Process and validate tags
	var tags []models.Tag
	if input.Tags != "" {
		for _, tn := range strings.Split(input.Tags, ",") {
			tn = strings.ToLower(strings.TrimSpace(tn))
			if len(tn) < 2 || len(tn) > 20 || !tagRegex.MatchString(tn) {
				continue
			}
			var tag models.Tag
			s.db.Where(models.Tag{Name: tn}).FirstOrCreate(&tag)
			tags = append(tags, tag)
		}
	}

	// Generate webhook secret
	secret, err := s.GenerateWebhookSecret()
	if err != nil {
		return nil, fmt.Errorf("could not generate webhook secret: %w", err)
	}

	// Create resource
	resource := models.NomadResource{
		Name:          input.Name,
		Type:          models.ResourceType(input.Type),
		Description:   input.Description,
		License:       license,
		RepositoryURL: input.RepositoryURL,
		FilePath:      input.FilePath,
		WebhookSecret: secret,
		UserID:        userID,
		OrganizationID: orgID,
		Tags:          tags,
		Versions:      []models.ResourceVersion{{Version: input.Version}},
	}

	if err := s.db.Create(&resource).Error; err != nil {
		return nil, fmt.Errorf("could not create resource: %w", err)
	}

	return &resource, nil
}

// UpdateResource updates an existing resource with authorization checks
func (s *ResourceService) UpdateResource(id uint, input UpdateInput, userID uint) error {
	// Authorize first (returns ErrResourceNotFound or ErrUnauthorized as appropriate)
	if err := s.AuthorizeResourceAction(id, userID, false); err != nil {
		return err
	}

	var resource models.NomadResource
	if err := s.db.First(&resource, id).Error; err != nil {
		return ErrResourceNotFound
	}

	// Parse new owner if transferring
	var newOrgID *uint
	originalOrgID := resource.OrganizationID
	if strings.HasPrefix(input.Owner, "org:") {
		var oid uint
		if _, err := fmt.Sscanf(input.Owner, "org:%d", &oid); err != nil {
			return fmt.Errorf("invalid organization ID")
		}

		// Verify organization exists
		var org models.Organization
		if err := s.db.First(&org, oid).Error; err != nil {
			return fmt.Errorf("organization not found")
		}
		newOrgID = &oid
	}

	// Validate new org membership if transferring
	if newOrgID != nil && (originalOrgID == nil || *originalOrgID != *newOrgID) {
		isMember, err := s.ValidateOrgMembership(userID, *newOrgID)
		if err != nil {
			return err
		}
		if !isMember {
			return fmt.Errorf("you must be a member of the target organization")
		}
	}

	// Check name collision in target namespace
	if input.Name != resource.Name || (resource.OrganizationID != newOrgID) {
		var count int64
		collideQuery := s.db.Model(&models.NomadResource{}).Where("name = ? AND id != ?", input.Name, id)
		if newOrgID != nil {
			collideQuery = collideQuery.Where("organization_id = ?", *newOrgID)
		} else {
			collideQuery = collideQuery.Where("user_id = ? AND organization_id IS NULL", resource.UserID)
		}
		collideQuery.Count(&count)
		if count > 0 {
			return fmt.Errorf("a resource with this name already exists in that namespace")
		}
	}

	// Update resource fields
	resource.Name = input.Name
	resource.Type = models.ResourceType(input.Type)
	resource.OrganizationID = newOrgID
	resource.Description = input.Description
	resource.RepositoryURL = input.RepositoryURL
	resource.FilePath = input.FilePath
	resource.License = input.License

	// Update tags
	var tags []models.Tag
	if input.Tags != "" {
		for _, tn := range strings.Split(input.Tags, ",") {
			tn = strings.ToLower(strings.TrimSpace(tn))
			if len(tn) < 2 || len(tn) > 20 || !tagRegex.MatchString(tn) {
				continue
			}
			var tag models.Tag
			s.db.Where(models.Tag{Name: tn}).FirstOrCreate(&tag)
			tags = append(tags, tag)
		}
	}

	if err := s.db.Model(&resource).Association("Tags").Replace(tags); err != nil {
		return fmt.Errorf("failed to update tags: %w", err)
	}

	if err := s.db.Save(&resource).Error; err != nil {
		return fmt.Errorf("failed to save resource: %w", err)
	}

	return nil
}

// DeleteResource deletes a resource after authorization check.
// For org-owned resources, only the org owner may delete (requireOwner=true).
// For personal resources, only the owning user may delete.
func (s *ResourceService) DeleteResource(id uint, userID uint) error {
	if err := s.AuthorizeResourceAction(id, userID, true); err != nil {
		return err
	}

	var resource models.NomadResource
	if err := s.db.First(&resource, id).Error; err != nil {
		// AuthorizeResourceAction already loaded the resource; this is a safety net.
		return ErrResourceNotFound
	}

	if err := s.db.Delete(&resource).Error; err != nil {
		return fmt.Errorf("failed to delete resource: %w", err)
	}

	return nil
}

// ToggleStar toggles the star status for a resource and returns new state
func (s *ResourceService) ToggleStar(resourceID, userID uint) (isStarred bool, starCount int64, err error) {
	var resource models.NomadResource
	if err := s.db.Preload("StarredBy").First(&resource, resourceID).Error; err != nil {
		return false, 0, fmt.Errorf("resource not found")
	}

	// Check if already starred
	currentlyStarred := false
	for _, u := range resource.StarredBy {
		if u.ID == userID {
			currentlyStarred = true
			break
		}
	}

	// Use transaction to prevent race condition
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if currentlyStarred {
			// Unstar
			if err := tx.Model(&resource).Association("StarredBy").Delete(&models.User{Model: gorm.Model{ID: userID}}); err != nil {
				return err
			}
			return tx.Model(&resource).Update("star_count", gorm.Expr("star_count - ?", 1)).Error
		}
		// Star
		if err := tx.Model(&resource).Association("StarredBy").Append(&models.User{Model: gorm.Model{ID: userID}}); err != nil {
			return err
		}
		return tx.Model(&resource).Update("star_count", gorm.Expr("star_count + ?", 1)).Error
	})

	if err != nil {
		return false, 0, fmt.Errorf("failed to update star status: %w", err)
	}

	// Get updated count
	var count int64
	s.db.Table("user_stars").Where("nomad_resource_id = ?", resourceID).Count(&count)

	// Return new starred state (opposite of what it was)
	return !currentlyStarred, count, nil
}

// ValidateOwnership checks if a user has ownership or membership for a resource
func (s *ResourceService) ValidateOwnership(resourceID, userID uint) (bool, error) {
	var resource models.NomadResource
	if err := s.db.First(&resource, resourceID).Error; err != nil {
		return false, fmt.Errorf("resource not found")
	}

	if resource.OrganizationID != nil {
		// Check org membership
		var membership models.Membership
		if err := s.db.Where("user_id = ? AND organization_id = ?", userID, *resource.OrganizationID).First(&membership).Error; err == nil {
			return true, nil
		}
		return false, nil
	}

	// Check user ownership
	return resource.UserID == userID, nil
}

// ValidateOrgMembership checks if a user is a member of an organization
func (s *ResourceService) ValidateOrgMembership(userID, orgID uint) (bool, error) {
	var membership models.Membership
	if err := s.db.Where("user_id = ? AND organization_id = ?", userID, orgID).First(&membership).Error; err != nil {
		return false, nil
	}
	return true, nil
}

// AuthorizeResourceAction is the single, canonical authorization check for any
// mutating action on a resource.
//
// Rules:
//   - If the resource belongs to an organization (OrganizationID != nil), the
//     user must have a Membership row for that org. When requireOwner is true the
//     membership role must be RoleOwner; any membership suffices otherwise.
//   - If the resource is personal (OrganizationID == nil), the user must be the
//     owning user (resource.UserID == userID). requireOwner is ignored for
//     personal resources: the owning user always has full control including delete.
//
// Returns ErrResourceNotFound when the resource does not exist, ErrUnauthorized
// when the user is not permitted, nil on success.
func (s *ResourceService) AuthorizeResourceAction(resourceID, userID uint, requireOwner bool) error {
	var resource models.NomadResource
	if err := s.db.First(&resource, resourceID).Error; err != nil {
		return ErrResourceNotFound
	}

	if resource.OrganizationID != nil {
		// Org-owned resource: org branch takes priority regardless of UserID.
		var membership models.Membership
		if err := s.db.Where("user_id = ? AND organization_id = ?", userID, *resource.OrganizationID).First(&membership).Error; err != nil {
			return ErrUnauthorized
		}
		if requireOwner && membership.Role != RoleOwner {
			return ErrUnauthorized
		}
		return nil
	}

	// Personal resource: only the owning user is allowed.
	if resource.UserID != userID {
		return ErrUnauthorized
	}
	return nil
}

// GenerateWebhookSecret generates a cryptographically secure random webhook secret
func (s *ResourceService) GenerateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate webhook secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// RecordFetchFailed marks a resource version fetch as FAILED with the supplied
// error message. It never stores content alongside the failure: the caller is
// responsible for not writing partial content on error. This helper centralises
// the DB update so that all fetch-failure paths (background job, webhook
// handler, retry) share the same invariant:
//
//	FetchStatus == failed, FetchError set, FetchCompletedAt set.
func (s *ResourceService) RecordFetchFailed(resourceID uint, version, errMsg string) error {
	now := time.Now()
	result := s.db.Model(&models.ResourceVersion{}).
		Where("resource_id = ? AND version = ?", resourceID, version).
		Updates(map[string]interface{}{
			"fetch_status":       models.FetchStatusFailed,
			"fetch_error":        errMsg,
			"fetch_completed_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("failed to record fetch failure: %w", result.Error)
	}
	return nil
}

// ResetWebhookSecret resets the webhook secret for a resource.
// Org members (not just owners) may reset the secret; personal resources
// require the owning user.
func (s *ResourceService) ResetWebhookSecret(resourceID, userID uint) error {
	if err := s.AuthorizeResourceAction(resourceID, userID, false); err != nil {
		return err
	}

	var resource models.NomadResource
	if err := s.db.First(&resource, resourceID).Error; err != nil {
		return ErrResourceNotFound
	}

	// Generate new secret
	secret, err := s.GenerateWebhookSecret()
	if err != nil {
		return fmt.Errorf("could not generate webhook secret: %w", err)
	}

	resource.WebhookSecret = secret
	if err := s.db.Save(&resource).Error; err != nil {
		return fmt.Errorf("failed to save resource: %w", err)
	}

	return nil
}
