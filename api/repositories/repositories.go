package repositories

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"rmbl/models"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RepoService represents a repository service.
type RepoService struct {
	db *gorm.DB
}

// NewRepoService creates a new instance of RepoService with the given database connection.
// It performs checks on the db parameter to ensure it is not empty and can establish a connection.
// If the db parameter is nil, it returns an error indicating that the database connection cannot be empty.
// If the database connection fails, it returns an error along with the RepoService instance.
// Otherwise, it returns the RepoService instance with the provided database connection and no error.
func NewRepoService(dbconn *gorm.DB) (*RepoService, error) {
	// do some checks on the db parameter in case there's an error to return
	if dbconn == nil {
		return nil, errors.New("database connection cannot be empty")
	}
	sqlDB, _ := dbconn.DB()
	if sqlDBerr := sqlDB.Ping(); sqlDBerr != nil {
		return &RepoService{
			db: dbconn,
		}, sqlDBerr
	}

	return &RepoService{
		db: dbconn,
	}, nil
}

// paginate is a higher-order function that returns a function used for pagination in database queries.
// The returned function takes a *gorm.DB object as input and returns a modified *gorm.DB object with the specified offset and limit.
// The offset parameter determines the number of records to skip, while the limit parameter determines the maximum number of records to retrieve.
// Example usage: paginate(10, 20)(db) will skip the first 10 records and retrieve the next 20 records from the database.
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

// getOrganizationIDByUserName retrieves the organization ID associated with the given username.
// It queries the database for the organization with the matching org_name and returns its ID.
func (s *RepoService) getOrganizationIDByUserName(username string) (id uuid.UUID) {
	var org models.Organization
	s.db.Where("org_name = ?", username).Find(&org)
	return uuid.UUID(org.ID)
}

// GetUserIDByUserName retrieves the user ID associated with the given username.
// It queries the database for a user with the matching username and returns their ID.
func (s *RepoService) getUserIDByUserName(username string) (id uuid.UUID) {
	var user models.User
	s.db.Where("username = ?", username).Find(&user)
	return uuid.UUID(user.ID)
}

// GetAllRepositories retrieves all repositories based on the provided parameters.
// It validates the incoming parameters and uses the Options pattern to ensure sane defaults.
// The repositories are ordered by organization name in the specified order.
// The search parameter is used to filter repositories based on a case-insensitive search.
// Pagination is applied using the limit and offset parameters.
// The function returns a models.RepoData struct containing the retrieved repositories.
func (s *RepoService) GetAllRepositories(order string, search string, limit int, offset int) models.RepoData {
	// TODO: We may want to validate the incoming parameters, and in the future
	// consider using the Options pattern to ensure sane defaults.

	search = strings.ToLower(search)

	var repositories []models.RepositoryViewStruct
	var data models.RepoData

	dbQuery := s.db.Model(&models.Repository{}).Joins("inner join organizations on organizations.id = repositories.organization_id")
	dbQuery.Order("org_name " + order)
	dbQuery.Scopes(searchterm(search))
	dbQuery.Select("repositories.id, repositories.name, repositories.version, repositories.description, repositories.url, organizations.org_name")
	dbQuery.Count(&data.TotalRecords)
	dbQuery.Scopes(paginate(offset, limit))
	dbQuery.Scan(&repositories)

	data.Status = "Success"
	data.Message = "Records found"
	data.Data = repositories

	// Override the message if there were no records.
	if dbQuery.RowsAffected == 0 {
		data.Message = "No Records found"
	}

	return data
}

// GetOrgRepositories retrieves a list of repositories belonging to a specific organization.
// It takes the following parameters:
// - order: the order in which the repositories should be sorted (ascending or descending).
// - search: a search string to filter the repositories by name or description.
// - limit: the maximum number of repositories to retrieve.
// - offset: the number of repositories to skip before retrieving the results.
// - orgid: the ID of the organization to which the repositories belong.
// It returns a models.RepoData struct containing the retrieved repositories, along with the total number of records.
// If no repositories are found, the "Message" field in the returned struct will indicate that no records were found.
func (s *RepoService) GetOrgRepositories(order string, search string, limit int, offset int, orgid uuid.UUID) models.RepoData {
	var repositories []models.RepositoryViewStruct
	var data models.RepoData

	dbquery := s.db.Model(&models.Repository{}).Joins("inner join organizations on organizations.id = repositories.organization_id")
	dbquery.Where("organization_id = ?", orgid)
	dbquery.Order("repositories.name " + order)
	dbquery.Scopes(searchterm(search))
	dbquery.Select("repositories.id, repositories.name, repositories.version, repositories.description, repositories.url, organizations.org_name")
	dbquery.Count(&data.TotalRecords)
	dbquery.Scopes(paginate(offset, limit))
	dbquery.Scan(&repositories)

	data.Status = "Success"
	data.Message = "Records found"
	data.Data = repositories

	if dbquery.RowsAffected == 0 {
		data.Message = "No Records found"
	}
	return data
}

// GetRepository retrieves a single repository based on the provided repository name and organization ID.
// It returns a models.SingleRepoData struct containing the repository information.
func (s *RepoService) GetRepository(reponame string, orgname string) models.SingleRepoData {
	repository := &models.RepositoryViewStruct{}
	var data models.SingleRepoData
	helperservice, err := helpers.NewHelperService(database.DB)
	if err != nil {
		data.Status = "Error"
		data.Message = "Internal Server Error"
		return data
	}
	orgid, err := helperservice.GetOrganizationIDByOrgName(orgname)
	if err != nil {
		data.Status = "Error"
		data.Message = "Record Not Found"
		return data
	}
	dbquery := s.db.Model(&models.Repository{}).Joins("inner join organizations on organizations.id = repositories.organization_id")
	dbquery.Where("organization_id = ? and name = ?", orgid, reponame)
	dbquery.Select("repositories.id, repositories.name, repositories.version, repositories.description, repositories.url, organizations.org_name")
	dbquery.Scan(&repository)
	data.Status = "Success"
	data.Message = "Records found"
	data.Data = *repository

	if dbquery.RowsAffected == 0 {
		data.Message = "No repository found with that name"
	}
	return data
}

// NewRepository creates a new repository and associates it with the specified organization.
// It takes a repository model, organization name, and a flag indicating if the user is a site admin.
// It returns the created repository model and an error, if any.
func (s *RepoService) NewRepository(repository models.Repository, orgname string, is_siteadmin bool) (models.Repository, error) {
	var organization models.Organization
	// find the org in the organization table
	err := s.db.Where("org_name = ?", orgname).Find(&organization).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("organization not found")
		return repository, fmt.Errorf("organization not found")
	} else if err != nil {
		return repository, fmt.Errorf("")
	}
	repository.OrganizationID = organization.ID

	// db.Model(&organization).Association("Repositories")

	m := s.db.Model(&organization)
	if m == nil {
		log.Println("model was nil")
		fmt.Println("model was nil")
	} else if m.Error != nil {
		log.Printf("model error: %v", m.Error)
		fmt.Printf("model error: %v", m.Error)
	}

	w := m.Where("org_name = ?", orgname)
	if w == nil {
		log.Println("where was nil")
		fmt.Println("where was nil")
	} else if w.Error != nil {
		log.Printf("where error: %v", w.Error)
		fmt.Printf("where error: %v", w.Error)
	}

	a := w.Association("Repositories")
	if a == nil {
		log.Println("assoc was nil")
		fmt.Println("assoc was nil")
	} else if a.Error != nil {
		log.Printf("assoc error: %v", err)
		fmt.Printf("assoc error: %v", err)
	}

	err = a.Append(&repository)
	if err != nil {
		log.Printf("append error: %v", err)
		fmt.Printf("append error: %v", err)
	}

	// repoErr := db.Model(&organization).Where("org_name = ?", orgname).Association("Repositories").Append(&repository).Error
	// if err != nil {
	// 	return repository, fmt.Errorf("unable to create repository: %s ", repoErr())
	// }
	return repository, nil
}

// UpdateRepository updates a repository with the given organization ID, repository name, and updated repository data.
// It returns the updated repository and an error, if any.
func (s *RepoService) UpdateRepository(orgid uuid.UUID, reponame string, updatedRepository models.Repository) (models.Repository, error) {
	repository := new(models.Repository)

	err := s.db.Where(&models.Repository{OrganizationID: orgid}).Where("name = ?", reponame).First(&repository).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return *repository, fmt.Errorf("repository not found: %w", err)
	} else if err != nil {
		return *repository, err
	}
	if err = s.db.Where(&models.Repository{OrganizationID: orgid}).Model(&repository).Where("name = ?", reponame).Updates(updatedRepository).Error; err != nil {
		return *repository, err
	}
	// return c.SendStatus(204)
	return updatedRepository, err
}

// DeleteRepository deletes a repository from the database based on the organization ID and repository name.
// It returns an error if the repository is not found or if there is an error during the deletion process.
func (s *RepoService) DeleteRepository(orgid uuid.UUID, reponame string) error {
	repository := new(models.Repository)
	err := s.db.Where(&models.Repository{OrganizationID: orgid}).Where("name = ?", reponame).First(&repository).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("repository not found")
	} else if err != nil {
		return fmt.Errorf(err.Error())
	}

	if err = s.db.Delete(&repository).Error; err != nil {
		return fmt.Errorf(err.Error())
	}
	return err
}
