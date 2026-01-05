package database

import (
	"context"
	"log"
	"rmbl/internal/models"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	postgresDriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SetupTestDB initializes a Postgres container for testing
// Returns the GORM DB instance and a cleanup function
func SetupTestDB() (*gorm.DB, func()) {
	ctx := context.Background()

	dbName := "rmbl_test"
	dbUser := "postgres"
	dbPassword := "password"

	pgContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		log.Fatalf("failed to start postgres container: %s", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("failed to get connection string: %s", err)
	}

	// Connect with GORM
	db, err := gorm.Open(postgresDriver.Open(connStr), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("failed to connect to test db: %s", err)
	}

	// Auto Migrate
	err = db.AutoMigrate(
		&models.User{},
		&models.Organization{},
		&models.Membership{},
		&models.NomadResource{},
		&models.ResourceVersion{},
		&models.Tag{},
	)
	if err != nil {
		log.Fatalf("Migration failed: %s", err)
	}

	DB = db // Set global DB

	cleanup := func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			log.Printf("failed to terminate container: %s", err)
		}
	}

	return db, cleanup
}