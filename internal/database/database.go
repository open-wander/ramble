package database

import (
	"fmt"
	"log"
	"os"

	"rmbl/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

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
			dsn = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
				host, user, password, dbname, port)
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
	)
	if err != nil {
		log.Fatal("Migration failed: ", err)
	}
	log.Println("Migrations completed")
}