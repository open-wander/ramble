package models

import (
	"rmbl/pkg/database"

	"github.com/google/uuid"
)

// User struct
type User struct {
	database.DefaultModel
	Username     string       `json:"username" gorm:"uniqueIndex"`
	Email        string       `json:"email" gorm:"uniqueIndex"`
	FirstName    string       `json:"first_name"`
	LastName     string       `json:"last_name"`
	Password     string       `json:"-"`
	SiteAdmin    bool         `json:"-"`
	Organization Organization `json:"organization"`
}

type NewUser struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
}

type UserLoginInput struct {
	Identity string `json:"identity" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type UserSignupData struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username" validate:"required,min=5,max=32"`
	Email     string    `json:"email" validate:"required,email,min=6,max=32"`
	FirstName string    `json:"first_name" validate:"required,min=3,max=256"`
	LastName  string    `json:"last_name" validate:"required,min=3,max=256"`
	Password  string    `json:"password" validate:"required"`
}
