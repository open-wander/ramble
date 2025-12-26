package database

import (
	"log"
	"os"
	"rmbl/internal/models"

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
		log.Println("Initial user already exists, skipping seed.")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash password for initial user: %v", err)
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
		log.Printf("Failed to create initial user: %v", err)
	} else {
		log.Printf("Successfully seeded initial user: %s (%s)", username, email)
	}
}
