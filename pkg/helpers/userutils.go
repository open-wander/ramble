package helpers

import (
	"errors"
	"rmbl/models"
	"rmbl/pkg/database"

	jwt "github.com/form3tech-oss/jwt-go"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func GetUserByEmail(e string) (*models.User, error) {
	db := database.DB
	var user models.User
	if err := db.Where(&models.User{Email: e}).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func GetUserByUsername(u string) (*models.User, error) {
	db := database.DB
	var user models.User
	if err := db.Where(&models.User{Username: u}).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// GetUserID by name
func GetUserIDByUserName(username string) (id uuid.UUID) {
	db := database.DB
	var user models.User
	db.Where("username = ?", username).Find(&user)
	return uuid.UUID(user.ID)
}

// GetUserORGID by userid
func GetORGIDByUserid(id uuid.UUID) (orgid uuid.UUID) {
	db := database.DB
	var organization models.Organization
	db.Where("user_id = ?", id).Find(&organization)
	return uuid.UUID(organization.ID)
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

// CheckPasswordHash compare password with hash
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func ValidToken(t *jwt.Token, id uuid.UUID) bool {
	n := id

	claims := t.Claims.(jwt.MapClaims)
	uid := claims["user_id"].(uuid.UUID)

	if uid != n {
		return false
	}

	return true
}

func ValidUser(id uuid.UUID, p string) bool {
	db := database.DB
	var user models.User
	db.First(&user, id)
	if user.Email == "" {
		return false
	}
	if !CheckPasswordHash(p, user.Password) {
		return false
	}
	return true
}
