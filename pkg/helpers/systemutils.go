package helpers

import (
	"crypto/rand"
	"fmt"
	"log"
	"rmbl/models"
	"rmbl/pkg/database"
)

const letters = "01234567890!@#$%^&*()abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

//Generate Random Password for System User
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

func CreateSystemUser(system_user_email string) string {
	db := database.DB
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
	if err := db.First(&user).Error; err != nil {
		db.Create(&user)
		fmt.Println("This is the System User Password")
		fmt.Println("Save it as it will not appear again")
		fmt.Println("System user password: " + password)
		return password
	}
	return ""
}
