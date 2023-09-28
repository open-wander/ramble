package models

import (
	"rmbl/pkg/database"
)

// User struct
type User struct {
	database.DefaultModel
	Username     string       `json:"username" gorm:"uniqueIndex"`
	Email        string       `json:"email" gorm:"uniqueIndex"`
	Password     string       `json:"-"`
	SiteAdmin    bool         `json:"-"`
	Organization Organization `json:"organization" gorm:"foreignKey:UserID"`
}
