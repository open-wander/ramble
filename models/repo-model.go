package models

import "rmbl/pkg/database"

// Repository Struct
type Repository struct {
	database.DefaultModel
	Name        string `json:"name"`
	UserID      uint   `json:"username" gorm:"not null" gorm:"index"`
	Version     string `json:"version"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

type RepositoryViewStruct struct {
	database.DefaultModel
	Name        string `json:"name"`
	UserName    string `json:"username" gorm:"column:username"`
	Version     string `json:"version"`
	Description string `json:"description"`
	URL         string `json:"url"`
}
