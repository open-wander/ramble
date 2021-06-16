package models

import (
	"rmbl/pkg/database"
)

// User struct
type User struct {
	database.DefaultModel
	Username     string       `json:"username" gorm:"unique"`
	Email        string       `json:"email" gorm:"unique"`
	Password     string       `json:"-"`
	Organization Organization `json:"organization" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}
