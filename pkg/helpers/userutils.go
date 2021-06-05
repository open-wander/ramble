package helpers

import (
	"rmbl/models"
	"rmbl/pkg/database"

	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

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
