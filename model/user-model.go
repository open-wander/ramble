package model

import (
	"gorm.io/gorm"
)

//User Struct
type User struct {
	gorm.Model
	// CreatedBy
	UserName string `gorm:"type:varchar(255)"`
	// Email
	Email string `gorm:"type:varchar(255)"`
	// Password
	Password string `gorm:"type:varchar(255)"`
}
