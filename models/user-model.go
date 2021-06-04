package models

import (
	"rmbl/pkg/database"
)

// User struct
type User struct {
	database.DefaultModel
	Username     string       `json:"username"`
	Email        string       `json:"email"`
	Password     string       `json:"-"`
	Organization Organization `json:"organization" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}
