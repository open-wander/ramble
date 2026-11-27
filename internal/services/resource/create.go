package resource

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"rmbl/internal/models"
	"rmbl/internal/names"
	"rmbl/internal/security"
)

// Sentinel errors returned by CreateResource. Their messages are the
// user-facing text shown by the web UI.
var (
	ErrInvalidName          = errors.New("name must be 1-64 characters: letters, digits, dot, dash or underscore, starting with a letter or digit")
	ErrInvalidType          = errors.New("type must be pack or job")
	ErrInvalidRepositoryURL = errors.New("repository URL must be an https URL on an allowed git host")
	ErrInvalidVersion       = errors.New("version is required, at most 64 characters, and may contain only letters, digits, dots, dashes, underscores and plus signs")
	ErrDuplicateName        = errors.New("a resource with this name already exists in this namespace")
	ErrOrgNotFound          = errors.New("organization not found")
	ErrNotOrgMember         = errors.New("you must be a member of this organization to create resources in it")
)

const (
	maxVersionLen        = 64
	maxLicenseLen        = 25
	unknownLicense       = "Unknown"
	licenseFileName      = "LICENSE"
	pgUniqueViolationSQL = "23505"
)

// FileFetcher downloads fileName from the repository at repoURL. It is
// injected so the service does not depend on the SSRF-protected HTTP client.
type FileFetcher func(repoURL, fileName string) (string, error)

// WithFileFetcher returns a copy of the service that uses f to download
// repository files (for license detection).
func (s *ResourceService) WithFileFetcher(f FileFetcher) *ResourceService {
	cp := *s
	cp.fetchFile = f
	return &cp
}

func validateCreateInput(input CreateInput) error {
	if !names.IsValid(input.Name) {
		return ErrInvalidName
	}
	if t := models.ResourceType(input.Type); t != models.ResourceTypePack && t != models.ResourceTypeJob {
		return ErrInvalidType
	}
	if err := security.ValidateRepositoryURL(input.RepositoryURL); err != nil {
		return ErrInvalidRepositoryURL
	}
	return validateVersion(input.Version)
}

// validateVersion checks a version string that has already been trimmed.
func validateVersion(v string) error {
	if strings.TrimSpace(v) == "" || len(v) > maxVersionLen || !names.IsValidVersion(v) {
		return ErrInvalidVersion
	}
	return nil
}

// detectLicense derives a short license label from the first line of a
// LICENSE file body.
func detectLicense(body string) string {
	first, _, _ := strings.Cut(body, "\n")
	license := strings.TrimPrefix(strings.TrimSpace(first), "The ")
	if len(license) > maxLicenseLen {
		license = license[:maxLicenseLen] + "..."
	}
	if license == "" {
		return unknownLicense
	}
	return license
}

// resolveTags normalises a comma-separated tag string and returns the stored
// tags, creating missing ones. Invalid tags are skipped.
func (s *ResourceService) resolveTags(tags string) []models.Tag {
	var out []models.Tag
	if tags == "" {
		return out
	}
	for tn := range strings.SplitSeq(tags, ",") {
		tn = strings.ToLower(strings.TrimSpace(tn))
		if len(tn) < minTagLen || len(tn) > maxTagLen || !tagRegex.MatchString(tn) {
			continue
		}
		var tag models.Tag
		s.db.Where(models.Tag{Name: tn}).FirstOrCreate(&tag)
		out = append(out, tag)
	}
	return out
}

// namespaceNameScope restricts q to resources named name (case-insensitive:
// resource URLs are resolved with LOWER(name) = LOWER(?)) in the namespace of
// the caller: the organization when orgID is set, else the user's personal
// namespace. The duplicate check and the purge both use it.
func namespaceNameScope(q *gorm.DB, name string, userID uint, orgID *uint) *gorm.DB {
	q = q.Where("LOWER(name) = LOWER(?)", name)
	if orgID != nil {
		return q.Where("organization_id = ?", *orgID)
	}
	return q.Where("user_id = ? AND organization_id IS NULL", userID)
}

// purgeDeletedName permanently removes soft-deleted resources that hold name
// in the namespace, with their versions, tag links and stars, so the name can
// be reused. Live resources are never touched.
func purgeDeletedName(tx *gorm.DB, name string, userID uint, orgID *uint) error {
	var ids []uint
	q := namespaceNameScope(tx.Unscoped().Model(&models.NomadResource{}), name, userID, orgID).
		Where("deleted_at IS NOT NULL")
	if err := q.Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("could not find deleted resources: %w", err)
	}
	return purgeIDs(tx, ids)
}

// PurgeDeletedInOrgs permanently removes the soft-deleted resources of the
// organisations, with their versions, tag links and stars. Live resources are
// never touched.
func PurgeDeletedInOrgs(tx *gorm.DB, orgIDs []uint) error {
	if len(orgIDs) == 0 {
		return nil
	}
	var ids []uint
	if err := tx.Unscoped().Model(&models.NomadResource{}).
		Where("organization_id IN ? AND deleted_at IS NOT NULL", orgIDs).
		Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("could not find deleted resources: %w", err)
	}
	return purgeIDs(tx, ids)
}

func purgeIDs(tx *gorm.DB, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Unscoped().Where("resource_id IN ?", ids).Delete(&models.ResourceVersion{}).Error; err != nil {
		return fmt.Errorf("could not purge versions: %w", err)
	}
	for _, table := range []string{"resource_tags", "user_stars"} {
		if err := tx.Exec("DELETE FROM "+table+" WHERE nomad_resource_id IN ?", ids).Error; err != nil {
			return fmt.Errorf("could not purge %s: %w", table, err)
		}
	}
	if err := tx.Unscoped().Where("id IN ?", ids).Delete(&models.NomadResource{}).Error; err != nil {
		return fmt.Errorf("could not purge deleted resources: %w", err)
	}
	return nil
}

// IsInputError reports whether err is a rejection of the caller's input
// (validation or a duplicate name) rather than a server fault.
func IsInputError(err error) bool {
	for _, e := range []error{ErrInvalidName, ErrInvalidType, ErrInvalidRepositoryURL, ErrInvalidVersion, ErrDuplicateName} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// CreateResource validates input, authorizes the caller for the target
// namespace, detects the license when none is given and persists the
// resource. The returned resource has User and Organization populated.
func (s *ResourceService) CreateResource(input CreateInput, userID uint, orgID *uint) (*models.NomadResource, error) {
	input.Version = strings.TrimSpace(input.Version)
	if err := validateCreateInput(input); err != nil {
		return nil, err
	}

	var org *models.Organization
	if orgID != nil {
		var o models.Organization
		if err := s.db.Limit(1).Find(&o, *orgID).Error; err != nil {
			return nil, fmt.Errorf("could not load organization: %w", err)
		} else if o.ID == 0 {
			return nil, ErrOrgNotFound
		}
		org = &o

		var members int64
		if err := s.db.Model(&models.Membership{}).Where(
			"user_id = ? AND organization_id = ? AND role IN ('member','owner')",
			userID, *orgID,
		).Count(&members).Error; err != nil {
			return nil, fmt.Errorf("could not check membership: %w", err)
		}
		if members == 0 {
			return nil, ErrNotOrgMember
		}
	}

	license := input.License
	if license == "" {
		license = unknownLicense
		if s.fetchFile != nil {
			if body, err := s.fetchFile(input.RepositoryURL, licenseFileName); err == nil && body != "" {
				license = detectLicense(body)
			}
		}
	}

	secret, err := s.GenerateWebhookSecret()
	if err != nil {
		return nil, fmt.Errorf("could not generate webhook secret: %w", err)
	}

	resource := models.NomadResource{
		Name:           input.Name,
		Type:           models.ResourceType(input.Type),
		Description:    input.Description,
		License:        license,
		RepositoryURL:  input.RepositoryURL,
		FilePath:       input.FilePath,
		WebhookSecret:  secret,
		UserID:         userID,
		OrganizationID: orgID,
		Versions:       []models.ResourceVersion{{Version: input.Version}},
	}

	// The duplicate check, the purge of soft-deleted holders of the name and
	// the insert share one transaction: a failed insert restores the purged
	// rows. A unique violation is reported as a duplicate. idx_user_res_name
	// only covers part of the namespace rules (NULL organization_id values are
	// distinct), so the live check is the main guard.
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if orgID != nil {
			// Share lock: conflicts with the row lock of DeleteOrg, so the
			// organisation cannot be deleted under this insert.
			var o models.Organization
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Find(&o, *orgID).Error; err != nil {
				return fmt.Errorf("could not lock organization: %w", err)
			}
			if o.ID == 0 {
				return ErrOrgNotFound
			}
		}
		var dups int64
		if err := namespaceNameScope(tx.Model(&models.NomadResource{}), input.Name, userID, orgID).Count(&dups).Error; err != nil {
			return fmt.Errorf("could not check for duplicates: %w", err)
		}
		if dups > 0 {
			return ErrDuplicateName
		}
		if err := purgeDeletedName(tx, input.Name, userID, orgID); err != nil {
			return err
		}
		resource.Tags = (&ResourceService{db: tx}).resolveTags(input.Tags)
		if err := tx.Create(&resource).Error; err != nil {
			if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgUniqueViolationSQL {
				return ErrDuplicateName
			}
			return fmt.Errorf("could not create resource: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("could not load user: %w", err)
	}
	resource.User = user
	if org != nil {
		resource.Organization = *org
	}
	return &resource, nil
}
