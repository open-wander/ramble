package model

import (
	"gorm.io/gorm"
)

// Repository Represents the model for a Repository
// Default table name will be 'repositories'
type Repository struct {
	gorm.Model
	// // docrepo Owner
	// RepoOwner string `json:"repoOwner"`
	// CreatedBy
	CreatedBy string `json:"-" gorm:"type:varchar(255)"`
	// UpdatedBy
	UpdatedBy string `json:"-" gorm:"type:varchar(255)"`
	// UpdatedBy
	DeletedBy string `json:"-" gorm:"type:varchar(255)"`
	// doc license
	License string `json:"license" gorm:"type:varchar(255)"`
	// doc readme
	Readme string `json:"readme" gorm:"type:text"`
	// repo Name
	Name string `json:"name" gorm:"unique"`
	// repo URL
	URL string `json:"url" gorm:"type:varchar(255)"`
	// Current version Tag
	CurrentVersion string `json:"current_version" gorm:"type:text"`
	// list of versions released
	RelVersions []RelVersion
	// description
	Description string `json:"description" gorm:"type:varchar(255)"`
	// Stars
	Stars int `json:"stars" gorm:"type:text"`
}

// RelVersion Repository releases
type RelVersion struct {
	gorm.Model
	// Release Name
	ReleaseName string `json:"release_name" gorm:"type:varchar(255)"`
	// Release Tag
	ReleaseTag string `json:"release_tag" gorm:"type:varchar(255)"`
	// URL for release
	URL string `json:"release_url" gorm:"type:varchar(255)"`
	// Tar Url
	TarURL string `json:"tar_url" gorm:"type:varchar(255)"`
	// Zip Url
	ZipURL string `json:"zip_url" gorm:"type:varchar(255)"`
	// RepositoryID Foreign Key
	RepositoryID uint
}

// CreateRep struct for API repo creation
type CreateRep struct {
	// repo URL
	Name string `json:"name" gorm:"type:varchar(255)"`
}
