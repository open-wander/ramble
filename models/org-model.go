package models

import (
	"rmbl/pkg/database"

	"github.com/google/uuid"
)

// Repository Struct
type Organization struct {
	database.DefaultModel
	UserID       uuid.UUID    `json:"userid"`
	OrgName      string       `json:"orgname" gorm:"uniqueIndex"`
	Repositories []Repository `json:"repositories"`
}
