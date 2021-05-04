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

// Data is mainly generated for filtering and pagination
type Data struct {
	TotalData    int64
	FilteredData int64
	Data         []RepositoryViewStruct
}
