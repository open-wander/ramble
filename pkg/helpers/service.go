package helpers

import (
	"errors"

	"gorm.io/gorm"
)

// HelperService represents a repository service.
type HelperService struct {
	db *gorm.DB
}

// NewHelperService creates a new instance of HelperService.
// It takes a *gorm.DB parameter representing the database connection.
// It returns a pointer to HelperService and an error.
// If the database connection is nil, it returns an error with message "database connection cannot be empty".
// If there is an error while pinging the database, it returns the error.
// Otherwise, it returns a pointer to HelperService and nil error.
func NewHelperService(dbconn *gorm.DB) (*HelperService, error) {
	// do some checks on the db parameter in case there's an error to return
	if dbconn == nil {
		return nil, errors.New("database connection cannot be empty")
	}
	sqlDB, _ := dbconn.DB()
	if sqlDBerr := sqlDB.Ping(); sqlDBerr != nil {
		return &HelperService{
			db: dbconn,
		}, sqlDBerr
	}

	return &HelperService{
		db: dbconn,
	}, nil
}
