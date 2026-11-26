package resource

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"rmbl/internal/models"
	"rmbl/internal/security"
)

// Sentinel errors returned by CreateResource. Their messages are the
// user-facing text shown by the web UI.
var (
	ErrInvalidName          = errors.New("name must be 1-64 characters: letters, digits, dot, dash or underscore, starting with a letter or digit")
	ErrInvalidType          = errors.New("type must be pack or job")
	ErrInvalidRepositoryURL = errors.New("repository URL must be an https URL on an allowed git host")
	ErrInvalidVersion       = errors.New("version is required and must be at most 64 characters")
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

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

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
	if !nameRegex.MatchString(input.Name) {
		return ErrInvalidName
	}
	if t := models.ResourceType(input.Type); t != models.ResourceTypePack && t != models.ResourceTypeJob {
		return ErrInvalidType
	}
	if err := security.ValidateRepositoryURL(input.RepositoryURL); err != nil {
		return ErrInvalidRepositoryURL
	}
	if v := strings.TrimSpace(input.Version); v == "" || len(input.Version) > maxVersionLen {
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
		if len(tn) < 2 || len(tn) > 20 || !tagRegex.MatchString(tn) {
			continue
		}
		var tag models.Tag
		s.db.Where(models.Tag{Name: tn}).FirstOrCreate(&tag)
		out = append(out, tag)
	}
	return out
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

	// Case-insensitive: resource URLs are resolved with LOWER(name) = LOWER(?).
	dupQuery := s.db.Model(&models.NomadResource{}).Where("LOWER(name) = LOWER(?)", input.Name)
	if orgID != nil {
		dupQuery = dupQuery.Where("organization_id = ?", *orgID)
	} else {
		dupQuery = dupQuery.Where("user_id = ? AND organization_id IS NULL", userID)
	}
	var dups int64
	if err := dupQuery.Count(&dups).Error; err != nil {
		return nil, fmt.Errorf("could not check for duplicates: %w", err)
	}
	if dups > 0 {
		return nil, ErrDuplicateName
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

	tags := s.resolveTags(input.Tags)

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
		Tags:           tags,
		Versions:       []models.ResourceVersion{{Version: input.Version}},
	}

	// A unique violation is reported as a duplicate. idx_user_res_name only
	// covers part of the namespace rules (NULL organization_id values are
	// distinct), so the pre-check above is the main guard.
	if err := s.db.Create(&resource).Error; err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgUniqueViolationSQL {
			return nil, ErrDuplicateName
		}
		return nil, fmt.Errorf("could not create resource: %w", err)
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
