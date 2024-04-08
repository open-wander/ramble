package organizations

import (
	"errors"
	"strings"

	"rmbl/models"

	"gorm.io/gorm"
)

// OrgService represents a service for managing organizations.
type OrgService struct {
	db *gorm.DB
}

// NewOrgService creates a new instance of OrgService with the provided database connection.
// It performs checks on the db parameter to ensure it is not empty and can establish a connection.
// If the db parameter is nil, it returns an error indicating that the database connection cannot be empty.
// If the database connection fails, it returns an error along with the OrgService instance.
// Otherwise, it returns a new OrgService instance with the provided database connection and no error.
func NewOrgService(dbconn *gorm.DB) (*OrgService, error) {
	// do some checks on the db parameter in case there's an error to return
	if dbconn == nil {
		return nil, errors.New("database connection cannot be empty")
	}
	sqlDB, _ := dbconn.DB()
	if sqlDBerr := sqlDB.Ping(); sqlDBerr != nil {
		return &OrgService{
			db: dbconn,
		}, sqlDBerr
	}

	return &OrgService{
		db: dbconn,
	}, nil
}

// paginate is a higher-order function that returns a function used for pagination in database queries.
// The returned function takes a *gorm.DB object as input and returns a modified *gorm.DB object with the specified offset and limit.
// The offset parameter determines the number of records to skip, while the limit parameter determines the maximum number of records to retrieve.
// Example usage: db := paginate(10, 20)(db)
func paginate(offset int, limit int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Offset(offset).Limit(limit)
	}
}

// Search is a higher-order function that returns a function used for searching records in a database.
// The returned function takes a *gorm.DB object as input and returns a modified *gorm.DB object.
// The search parameter is used to filter records based on a name field using the LIKE operator.
// If the search parameter is empty, the returned function does not apply any filtering.
func searchterm(search string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if search != "" {
			db = db.Where("name LIKE ?", "%"+search+"%")
		}
		return db
	}
}

// GetAllOrgs retrieves all organizations based on the specified parameters.
// It returns an OrgData struct containing the retrieved organizations, total record count, status, and message.
// The order parameter specifies the order in which the organizations should be sorted.
// The search parameter is used to filter organizations based on a search string.
// The limit parameter specifies the maximum number of organizations to retrieve.
// The offset parameter specifies the number of organizations to skip before retrieving.
// The includeRepos parameter determines whether to include repositories in the retrieved organizations.
func (s *OrgService) GetAllOrgs(order string, search string, limit int, offset int, includeRepos bool) models.OrgData {
	search = strings.ToLower(search)

	var organizations []models.Organization
	var data models.OrgData

	dbquery := s.db.Model(&organizations)
	if includeRepos {
		dbquery.Preload("Repositories")
	}
	dbquery.Order("org_name " + order)
	dbquery.Scopes(searchterm(search))
	dbquery.Count(&data.TotalRecords)
	dbquery.Scopes(paginate(offset, limit))
	dbquery.Find(&organizations)
	data.Data = organizations
	data.Status = "Success"
	data.Message = "Records found"
	return data
}
