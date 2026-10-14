package database

import (
	"os"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func SeedInitialUser(db *gorm.DB) {
	username := os.Getenv("INITIAL_USER_USERNAME")
	email := os.Getenv("INITIAL_USER_EMAIL")
	password := os.Getenv("INITIAL_USER_PASSWORD")

	if username == "" || email == "" || password == "" {
		return
	}

	var count int64
	db.Model(&models.User{}).Where("email = ? OR username = ?", email, username).Count(&count)

	if count > 0 {
		logger.Log.Info("seed.initial_user_exists")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		logger.Log.Error("seed.hash_password_failed", logger.Err(err))
		return
	}

	user := models.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
		Name:         "Initial Admin User",
		IsAdmin:      true,
	}

	if err := db.Create(&user).Error; err != nil {
		logger.Log.Error("seed.create_user_failed", logger.Err(err))
	} else {
		logger.Log.Info("seed.initial_user_created")
	}
}
