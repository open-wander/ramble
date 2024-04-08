package helpers

import (
	"errors"
	"fmt"

	"rmbl/models"
	"rmbl/pkg/apperr"

	jwt "github.com/form3tech-oss/jwt-go"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// GetUserByEmail retrieves a user from the database based on their email address.
// It takes an email string as input and returns a pointer to a models.User struct and an error.
// If the user is found, the function returns a pointer to the user struct and a nil error.
// If the user is not found, the function returns nil and a nil error.
// If an error occurs during the database query, the function returns nil and the error.
func (s *HelperService) GetUserByEmail(e string) (*models.User, error) {
	var user models.User
	if err := s.db.Where(&models.User{Email: e}).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.EntityNotFound(e)
		}
		return nil, apperr.EntityNotFound(e)
	}
	return &user, nil
}

// GetUserByUsername retrieves a user from the database based on their username.
// It takes a string parameter 'u' representing the username.
// It returns a pointer to a models.User struct and an error.
// If the user is found, the pointer to the user is returned along with a nil error.
// If the user is not found, nil is returned for the user and a nil error.
// If an error occurs during the database query, nil is returned for the user and the error is returned.
func (s *HelperService) GetUserByUsername(u string) (*models.User, error) {
	var user models.User
	if err := s.db.Where(&models.User{Username: u}).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.EntityNotFound(u)
		}
		return nil, apperr.EntityNotFound(u)
	}
	return &user, nil
}

// GetUserIDByUserName retrieves the user ID associated with the given username.
// It queries the database for a user with the matching username and returns their ID.
func (s *HelperService) GetUserIDByUserName(username string) (uuid.UUID, error) {
	var user models.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return user.ID, apperr.EntityNotFound(username)
		}
		return user.ID, apperr.EntityNotFound(username)
	}
	return uuid.UUID(user.ID), nil
}

// GetORGIDByUserid retrieves the organization ID associated with the given user ID.
// It queries the database for the organization record that matches the provided user ID.
// If a matching organization is found, it returns the organization ID.
// Otherwise, it returns an empty UUID.
func (s *HelperService) GetORGIDByUserid(id uuid.UUID) (uuid.UUID, error) {
	var organization models.Organization
	if err := s.db.Where("user_id = ?", id).Find(&organization).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return organization.ID, apperr.EntityNotFound(id.String())
		}
		return organization.ID, apperr.EntityNotFound(id.String())
	}
	return uuid.UUID(organization.ID), nil
}

// HashPassword takes a password string and returns its hashed representation.
// It uses bcrypt algorithm with a cost factor of 14.
// The returned string is the hashed password and an error if any occurred.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

// CheckPasswordHash compares a password with its corresponding hash and returns true if they match.
// It uses bcrypt.CompareHashAndPassword to perform the comparison.
// The password and hash parameters should be strings.
// It returns a boolean value indicating whether the password matches the hash.
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// ValidToken checks if a given JWT token is valid for a specific user ID.
// It compares the user ID extracted from the token claims with the provided ID.
// If the IDs match, it returns true; otherwise, it returns false.
func ValidToken(t *jwt.Token, id uuid.UUID) bool {
	n := id

	claims := t.Claims.(jwt.MapClaims)
	uid, err := uuid.Parse(claims["user_id"].(string))
	// TODO deal with this error in a better way
	if err != nil {
		fmt.Println("Not a Valid UUID")
	}
	if uid != n {
		return false
	}

	return true
}

// ValidUser checks if the user with the given ID and password is valid.
// It retrieves the user from the database using the provided ID and verifies
// if the user's email is not empty and the password matches the hashed password
// stored in the database.
// Returns true if the user is valid, otherwise returns false.
func (s *HelperService) ValidUser(id uuid.UUID, p string) bool {
	var user models.User
	s.db.First(&user, id)
	if user.Email == "" {
		return false
	}
	if !CheckPasswordHash(p, user.Password) {
		return false
	}
	return true
}
