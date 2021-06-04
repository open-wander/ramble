package models

import (
	"rmbl/pkg/database"

	"github.com/google/uuid"
)

// Repository Struct
type Repository struct {
	database.DefaultModel
	Name           string    `json:"name"`
	Version        string    `json:"version"`
	Description    string    `json:"description"`
	URL            string    `json:"url"`
	OrganizationID uuid.UUID `json:"orgid"`
}

type RepositoryViewStruct struct {
	database.DefaultModel
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	URL         string `json:"url"`
	OrgName     string `json:"orgname"`
}
