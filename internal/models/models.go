package models

import (
	"time"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username     string `gorm:"uniqueIndex;not null"`
	Email        string `gorm:"uniqueIndex;not null"`
	PasswordHash string // Optional for OAuth users
	Name         string
	AvatarURL    string
	Provider     string // e.g., github, gitlab
	ProviderID   string // Unique ID from the provider
	AccessToken  string // OAuth token for API calls // #nosec G117 -- GORM model field, not exposed via JSON API
	IsAdmin      bool   `gorm:"default:false"`
	// Relations
	Memberships []Membership    `gorm:"foreignKey:UserID"`
	Resources   []NomadResource `gorm:"foreignKey:UserID"`
	Starred     []NomadResource `gorm:"many2many:user_stars;"`

	// Password Reset
	ResetToken        string
	ResetTokenExpires time.Time
	ResetTokenUsedAt  *time.Time // NULL = unused, set on first use (for single-use enforcement)

	// Session Security
	PasswordChangedAt time.Time // Set on password change, used for session invalidation

	// Email Verification
	EmailVerified            bool      `gorm:"default:false"`
	VerificationToken        string
	VerificationTokenExpires time.Time

	// Brute Force Protection
	FailedLoginAttempts int       `gorm:"default:0"`
	LockedUntil         time.Time // Account locked until this time after too many failed attempts
}

type Organization struct {
	gorm.Model
	Name        string `gorm:"uniqueIndex;not null"`
	Description string
	// Relations
	Memberships []Membership    `gorm:"foreignKey:OrganizationID"`
	Resources   []NomadResource `gorm:"foreignKey:OrganizationID"`
}

type Membership struct {
	gorm.Model
	UserID         uint   `gorm:"uniqueIndex:idx_user_org"`
	OrganizationID uint   `gorm:"uniqueIndex:idx_user_org"`
	Role           string `gorm:"default:'member'"` // 'owner' or 'member'
	// Relations
	User         User         `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Organization Organization `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type ResourceType string

const (
	ResourceTypeJob  ResourceType = "job"
	ResourceTypePack ResourceType = "pack"
)

type FetchStatus string

const (
	FetchStatusPending   FetchStatus = "pending"
	FetchStatusFetching  FetchStatus = "fetching"
	FetchStatusCompleted FetchStatus = "completed"
	FetchStatusFailed    FetchStatus = "failed"
)

type NomadResource struct {
	gorm.Model
	Name           string       `gorm:"not null;uniqueIndex:idx_user_res_name"`
	Description    string
	Type           ResourceType `gorm:"default:'job'"` // job or pack
	License        string       // e.g., MIT, Apache-2.0
	RepositoryURL  string       // Link to GitHub/GitLab
	FilePath       string       // Path to the main .nomad.hcl or pack directory
	WebhookSecret  string       // Secret for validating incoming webhooks
	LastWebhookDelivery time.Time
	LastWebhookStatus   string // 'success', 'failure'
	LastWebhookError    string // Error message if failed
	StarCount      int          `gorm:"default:0"` // Denormalized count for sorting
	DownloadCount  int          `gorm:"default:0"` // Count of raw HCL fetches
	OrganizationID *uint        `gorm:"uniqueIndex:idx_user_res_name"`
	UserID         uint         `gorm:"uniqueIndex:idx_user_res_name"`
	// Relations
	Organization Organization      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	User         User              `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Tags         []Tag             `gorm:"many2many:resource_tags;"`
	Versions     []ResourceVersion `gorm:"foreignKey:ResourceID"`
	StarredBy    []User            `gorm:"many2many:user_stars;"`
}

type ResourceVersion struct {
	gorm.Model
	ResourceID       uint        `gorm:"index;not null"`
	Version          string      `gorm:"not null"`
	Readme           string      `gorm:"type:text"`
	Content          string      `gorm:"type:text"` // Stores the actual .nomad.hcl content
	Variables        string      `gorm:"type:text"` // JSON string of variables
	FetchStatus      FetchStatus `gorm:"type:varchar(20);default:'pending'"`
	FetchError       string      `gorm:"type:text"`
	FetchStartedAt   *time.Time
	FetchCompletedAt *time.Time
}

type Tag struct {
	gorm.Model
	Name      string          `gorm:"uniqueIndex;not null"`
	Resources []NomadResource `gorm:"many2many:resource_tags;"`
}

// RequestStatus represents the status of a pack/job request
type RequestStatus string

const (
	RequestStatusOpen       RequestStatus = "open"
	RequestStatusInProgress RequestStatus = "in_progress"
	RequestStatusCompleted  RequestStatus = "completed"
	RequestStatusClosed     RequestStatus = "closed"
)

// PackRequest represents a community request for a new pack or job
type PackRequest struct {
	gorm.Model
	Title          string        `gorm:"not null"`
	Description    string        `gorm:"type:text"`
	Type           ResourceType  `gorm:"default:'pack'"` // pack or job
	Status         RequestStatus `gorm:"default:'open'"`
	UserID         uint          `gorm:"not null;index"`
	GitHubIssueURL string `gorm:"column:github_issue_url"` // URL to the created GitHub issue
	GitHubIssueNum int    `gorm:"column:github_issue_num"` // GitHub issue number
	VoteCount      int           `gorm:"default:0"`
	// Relations
	User   User   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Voters []User `gorm:"many2many:request_votes;"`
}

// SiteSetting stores configurable site-wide settings
type SiteSetting struct {
	gorm.Model
	Key   string `gorm:"uniqueIndex;not null"`
	Value string `gorm:"type:text"`
}

// AuditLog records security-relevant actions for compliance and debugging
// Uses explicit fields instead of gorm.Model to prevent soft deletes and ensure immutability
type AuditLog struct {
	ID         uint      `gorm:"primaryKey"`
	CreatedAt  time.Time `gorm:"index"`
	Action     string    `gorm:"not null;index"` // e.g., "user.login", "admin.delete_user"
	ActorID    uint      `gorm:"index"`          // User who performed action (0 for system/anonymous)
	ActorName  string    // Username at time of action
	TargetType string    `gorm:"index"` // e.g., "user", "resource", "organization"
	TargetID   uint      // ID of affected entity
	TargetName string    // Name/identifier at time of action
	Details    string    `gorm:"type:text"` // JSON with additional context
	IPAddress  string
	UserAgent  string
	RequestID  string `gorm:"index"` // Request ID for tracing
	Checksum   string `gorm:"index"` // SHA-256 hash of all fields for integrity verification
}
