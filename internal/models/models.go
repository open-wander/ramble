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
	AccessToken  string // OAuth token for API calls
	IsAdmin      bool   `gorm:"default:false"`
	// Relations
	Memberships []Membership    `gorm:"foreignKey:UserID"`
	Resources   []NomadResource `gorm:"foreignKey:UserID"`
	Starred     []NomadResource `gorm:"many2many:user_stars;"`

	// Password Reset
	ResetToken        string
	ResetTokenExpires time.Time

	// Email Verification
	EmailVerified            bool      `gorm:"default:false"`
	VerificationToken        string
	VerificationTokenExpires time.Time
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
	ResourceTypeJob    ResourceType = "job"
		ResourceTypePack ResourceType = "pack"
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
	ResourceID uint   `gorm:"index;not null"`
	Version    string `gorm:"not null"`
	Readme     string `gorm:"type:text"`
	Content    string `gorm:"type:text"` // Stores the actual .nomad.hcl content
	Variables  string `gorm:"type:text"` // JSON string of variables
}

type Tag struct {
	gorm.Model
	Name      string          `gorm:"uniqueIndex;not null"`
	Resources []NomadResource `gorm:"many2many:resource_tags;"`
}
