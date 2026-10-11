package database

import (
	"fmt"
	"os"
	"time"

	"rmbl/internal/models"
	"rmbl/internal/services/logger"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

// exit is a seam so tests can observe fatal startup failures. Startup
// failures log at ERROR (zerolog Fatal is level FATAL, outside the
// DEBUG|INFO|WARN|ERROR standard) and then exit 1.
var exit = os.Exit

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

// gormConfig is the production GORM configuration; Connect opens the database with it.
func gormConfig() *gorm.Config { return &gorm.Config{Logger: newDefaultGormLogger()} }

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
			logger.Log.Error().Msg("db.credentials_missing")
			exit(1)
			return
		} else {
			// Fallback to local default for development
			dsn = "host=localhost user=postgres password=postgres dbname=rmbl port=5432 sslmode=disable"
		}
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), gormConfig())

	if err != nil {
		logger.Log.Error().Err(err).Msg("db.connect_failed")
		exit(1)
		return
	}

	logger.Log.Info().Msg("db.connected")

	// Auto Migrate
	logger.Log.Info().Msg("db.migrate_started")
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
		logger.Log.Error().Err(err).Msg("db.migrate_failed")
		exit(1)
		return
	}
	logger.Log.Info().Msg("db.migrate_completed")

	// Backfill fetch status for existing resource versions
	backfillFetchStatus(DB)

	// Clean up any existing reset tokens (security: fresh start on deploy)
	cleanupExpiredTokens(DB)
}

// cleanupExpiredTokens invalidates all existing password reset tokens on startup.
// Per security requirements: "Expire all existing tokens immediately on deploy (clean slate)"
func cleanupExpiredTokens(db *gorm.DB) {
	result := db.Model(&models.User{}).
		Where("reset_token != ?", "").
		Updates(map[string]interface{}{
			"reset_token":         "",
			"reset_token_expires": time.Time{},
			"reset_token_used_at": nil,
		})
	if result.RowsAffected > 0 {
		logger.Log.Info().Int64("count", result.RowsAffected).Msg("db.reset_tokens_cleared")
	}
}

// backfillFetchStatus sets FetchStatus to "completed" for any existing
// ResourceVersions that have no FetchStatus set. This handles the migration
// from before error tracking was added - existing versions are assumed complete.
func backfillFetchStatus(db *gorm.DB) {
	result := db.Model(&models.ResourceVersion{}).
		Where("fetch_status = '' OR fetch_status IS NULL").
		Update("fetch_status", models.FetchStatusCompleted)
	if result.RowsAffected > 0 {
		logger.Log.Info().Int64("count", result.RowsAffected).Msg("db.fetch_status_backfilled")
	}
}
