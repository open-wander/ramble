package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"rmbl/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// getSSLMode returns the appropriate SSL mode based on environment
func getSSLMode() string {
	if mode := os.Getenv("DB_SSLMODE"); mode != "" {
		return mode
	}
	if os.Getenv("ENV") == "production" {
		return "require"
	}
	return "disable"
}

func Connect() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		// Try building from individual components provided by Nomad
		host := os.Getenv("DB_HOST")
		user := os.Getenv("DB_USER")
		password := os.Getenv("DB_PASSWORD")
		dbname := os.Getenv("DB_NAME")
		port := os.Getenv("DB_PORT")

		if host != "" && user != "" && dbname != "" {
			sslmode := getSSLMode()
			dsn = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
				host, user, password, dbname, port, sslmode)
		} else if os.Getenv("ENV") == "production" {
			log.Fatal("Database credentials required in production")
		} else {
			// Fallback to local default for development
			dsn = "host=localhost user=postgres password=postgres dbname=rmbl port=5432 sslmode=disable"
		}
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})

	if err != nil {
		log.Fatal("Failed to connect to database: ", err)
	}

	log.Println("Database connection successfully opened")

	// Auto Migrate
	log.Println("Running Migrations...")
	err = DB.AutoMigrate(
		&models.User{},
		&models.Organization{},
		&models.Membership{},
		&models.NomadResource{},
		&models.ResourceVersion{},
		&models.Tag{},
		&models.PackRequest{},
		&models.SiteSetting{},
		&models.AuditLog{},
	)
	if err != nil {
		log.Fatal("Migration failed: ", err)
	}
	log.Println("Migrations completed")

	// Clean up any existing reset tokens (security: fresh start on deploy)
	cleanupExpiredTokens(DB)
}

// cleanupExpiredTokens invalidates all existing password reset tokens on startup.
// Per security requirements: "Expire all existing tokens immediately on deploy (clean slate)"
func cleanupExpiredTokens(db *gorm.DB) {
	result := db.Model(&models.User{}).
		Where("reset_token != ?", "").
		Updates(map[string]interface{}{
			"reset_token":          "",
			"reset_token_expires":  time.Time{},
			"reset_token_used_at":  nil,
		})
	if result.RowsAffected > 0 {
		log.Printf("Cleared %d existing password reset tokens", result.RowsAffected)
	}
}