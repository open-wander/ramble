package models

import "rmbl/pkg/database"

// Repository Struct
type Repository struct {
	database.DefaultModel
	Name        string `json:"name"`
	Org         string `json:"org"`
	Version     string `json:"version"`
	Description string `json:"description"`
	URL         string `json:"url"`
}
