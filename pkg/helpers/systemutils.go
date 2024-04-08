package helpers

import (
	"crypto/rand"
	"fmt"
	"log"

	"rmbl/models"
)

const letters = "01234567890!@#$%^&*()abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// generateRandomString generates a random string of the specified length.
// It uses cryptographic random number generation to ensure randomness.
// The generated string may contain alphanumeric characters.
//
// Parameters:
// - length: The length of the random string to generate.
//
// Returns:
// - string: The generated random string.
// - error: An error if the random number generation fails.
func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	for i, b := range bytes {
		bytes[i] = letters[b%byte(len(letters))]
	}

	return string(bytes), nil
}

// CreateSystemUser creates a system user with the given system_user_email.
// It generates a random password, hashes it, and saves the user to the database.
// If the user already exists in the database, it returns an empty string.
// Otherwise, it returns the generated password.
func (s *HelperService) CreateSystemUser(system_user_email string) string {
	user := new(models.User)
	org := new(models.Organization)
	uname := "system"

	org.OrgName = uname

	password, err := generateRandomString(20)
	if err != nil {
		log.Fatal(err)
	}

	hash, pwerr := HashPassword(password)
	if err != nil {
		fmt.Println(pwerr)
	}

	user.Email = system_user_email
	user.Username = uname
	user.Password = hash
	user.SiteAdmin = true
	user.Organization = *org
	if err := s.db.First(&user).Error; err != nil {
		s.db.Create(&user)
		fmt.Println("This is the System User Password")
		fmt.Println("Save it as it will not appear again")
		fmt.Println("System user password: " + password)
		return password
	}
	return ""
}
