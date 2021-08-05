package models

import (
	"rmbl/pkg/database"

	"github.com/google/uuid"
)

// Repository Struct
type Repository struct {
	database.DefaultModel
	Name           string    `json:"name" validate:"required,min=5,max=32" gorm:"index"`
	Version        string    `json:"version" validate:"required,min=2,max=32"`
	Description    string    `json:"description" validate:"required,min=5,max=256"`
	URL            string    `json:"url" validate:"required,url,min=5,max=256"`
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
