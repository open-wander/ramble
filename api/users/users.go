package users

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"rmbl/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserService represents the authentication service.
type UserService struct {
	db *gorm.DB
}

// NewUserService creates a new instance of UserService with the provided database connection.
// It performs checks on the db parameter and returns an error if it is empty or if there is an error pinging the database.
// Parameters:
// - dbconn: The database connection to be used by the UserService.
// Returns:
// - *UserService: The newly created UserService instance.
// - error: An error if the database connection is empty or if there is an error pinging the database.
func NewUserService(dbconn *gorm.DB) (*UserService, error) {
	// do some checks on the db parameter in case there's an error to return
	if dbconn == nil {
		return nil, errors.New("database connection cannot be empty")
	}
	sqlDB, _ := dbconn.DB()
	if sqlDBerr := sqlDB.Ping(); sqlDBerr != nil {
		return &UserService{
			db: dbconn,
		}, sqlDBerr
	}

	return &UserService{
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

// userSearch is a higher-order function that returns a function used for searching users in the database.
// The returned function takes a *gorm.DB object as input and returns a modified *gorm.DB object.
// If the search string is not empty, the returned function adds a WHERE clause to the query to filter users by username.
// The search string is matched against the username using a LIKE operator with wildcard characters.
func userSearch(search string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if search != "" {
			db = db.Where("username LIKE ?", "%"+search+"%")
		}
		return db
	}
}

// GetAllUsers retrieves all users from the database based on the specified parameters.
// It returns a UserData struct containing the retrieved users, along with the total number of records.
// The order parameter specifies the order in which the users should be sorted.
// The search parameter is used to filter the users based on a search string.
// The limit and offset parameters are used for pagination.
func (s *UserService) GetAllUsers(order string, search string, limit int, offset int) models.UserData {
	search = strings.ToLower(search)

	var users []models.User
	var data models.UserData

	dbquery := s.db.Model(&users).Preload("Organization")
	dbquery.Order("username " + order)
	dbquery.Scopes(userSearch(search))
	dbquery.Count(&data.TotalRecords)
	dbquery.Scopes(paginate(offset, limit))
	dbquery.Find(&users)
	if dbquery.Error != nil {
		data.Status = "Error"
		data.Message = "Error Retrieving Users"
	} else {
		data.Status = "Success"
		data.Message = "Records found"

	}
	data.Data = users

	// Override the message if there were no records.
	if dbquery.RowsAffected == 0 {
		data.Message = "No Records found"
	}

	return data
}

// GetUser retrieves a user from the database based on the provided user ID.
// If includeRepos is set to true, the user's associated repositories will also be loaded.
// It returns a SingleUserData struct containing the user information.
func (s *UserService) GetUser(userId uuid.UUID, includeRepos bool) models.SingleUserData {
	var user models.User
	var data models.SingleUserData
	dbquery := s.db.Model(&user).Preload("Organization")
	if includeRepos {
		dbquery.Preload("Organization.Repositories")
	}

	dbquery.Find(&user, userId)
	data.Status = "Success"
	data.Message = "Records found"
	data.Data = user
	if dbquery.RowsAffected == 0 {
		data.Status = "Error"
		data.Message = "No user found with that id"
	}
	return data
}

// UpdateUser updates the user with the specified user_id in the database.
// It updates the user's password and email address if provided.
// If the user is not found, it returns an error with "user not found" message.
// If any other error occurs during the update process, it returns an error with "something went wrong" message.
// If the update is successful, it returns the updated user object and nil error.
func (s *UserService) UpdateUser(user_id uuid.UUID, password string, emailAddress string) (models.User, error) {
	var user models.User
	err := s.db.First(&user, user_id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return user, fmt.Errorf("user not found")
	} else if err != nil {
		log.Printf("updateuser find query failed %s", err.Error())
		return user, fmt.Errorf("something went wrong")
	}
	if password != "" {
		user.Password = password
	}
	if emailAddress != "" {
		user.Email = emailAddress
	}

	if err = s.db.Save(&user).Error; err != nil {
		log.Printf("updateuser save query failed %s", err.Error())
		return user, fmt.Errorf("unable to save new user details")
	}
	return user, err
}

// DeleteUser deletes a user from the database.
// It takes a user_id parameter of type uuid.UUID, representing the ID of the user to be deleted.
// If the user is not found, it returns an error with the message "user not found".
// If there is a problem finding the user to delete, it returns an error with the message "problem finding user to delete".
// If the organization associated with the user is not found, it returns an error with the message "organization not found".
// If there is an error while deleting the user, it returns an error with the message "unable to delete user".
// Otherwise, it returns nil.
func (s *UserService) DeleteUser(user_id uuid.UUID) error {
	var user models.User
	var org models.Organization
	err := s.db.First(&user, user_id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("user not found")
	} else if err != nil {
		log.Printf("deleteuser find query failed %s", err.Error())
		return fmt.Errorf("problem finding user to delete")
	}
	// find the org in the organization table
	orgerr := s.db.Where("org_name = ?", &user.Username).Find(&org).Error
	if errors.Is(orgerr, gorm.ErrRecordNotFound) {
		log.Printf("organization not found")
		return fmt.Errorf("organization not found")
	} else if err != nil {
		return fmt.Errorf("")
	}
	// Delete organizations repositories
	s.db.Select("Repositories").Delete(&org)
	// // Delete Users Organization
	// db.Select("Organization").Delete(&user)
	// When Users are Deleted their repositories are deleted at the same time.
	if err := s.db.Select("Organization").Delete(&user).Error; err != nil {
		log.Printf("deleteuser delete query failed %s", err.Error())
		return fmt.Errorf("unable to delete user")
	}
	return err
}
