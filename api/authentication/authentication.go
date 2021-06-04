package authentication

import (
	"errors"
	"fmt"
	"rmbl/models"
	appconfig "rmbl/pkg/config"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"
	"strings"

	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func getUserByEmail(e string) (*models.User, error) {
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

func getUserByUsername(u string) (*models.User, error) {
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

// Login get user and password
func Login(c *fiber.Ctx) error {
	type LoginInput struct {
		Identity string `json:"identity"`
		Password string `json:"password"`
	}
	type UserData struct {
		ID       uuid.UUID `json:"id"`
		Username string    `json:"username"`
		Email    string    `json:"email"`
		Password string    `json:"password"`
	}
	var input LoginInput
	var ud UserData

	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Error on login request", "Data": err})
	}
	identity := strings.ToLower(input.Identity)
	pass := input.Password

	email, err := getUserByEmail(identity)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"status": "error", "message": "Error on email", "Data": err})
	}

	user, err := getUserByUsername(identity)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"status": "error", "message": "Error on username", "Data": err})
	}

	if email == nil && user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"status": "error", "message": "Invalid Credentials", "Data": err})
	}

	if email == nil {
		ud = UserData{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Password: user.Password,
		}
	} else {
		ud = UserData{
			ID:       email.ID,
			Username: email.Username,
			Email:    email.Email,
			Password: email.Password,
		}
	}

	if !helpers.CheckPasswordHash(pass, ud.Password) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"status": "error", "message": "Invalid Credentials", "Data": nil})
	}

	token := jwt.New(jwt.SigningMethodHS256)

	claims := token.Claims.(jwt.MapClaims)
	claims["username"] = ud.Username
	claims["user_id"] = ud.ID
	claims["exp"] = time.Now().Add(time.Hour * 72).Unix()

	t, err := token.SignedString([]byte(appconfig.Config.Server.JWTSecret))
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	return c.JSON(fiber.Map{"status": "success", "message": "Success login", "Data": t})
}

// Login get user and password
func Signup(c *fiber.Ctx) error {
	c.Accepts("application/json")
	type NewUser struct {
		ID       uuid.UUID `json:"id"`
		Username string    `json:"username"`
		Email    string    `json:"email"`
	}
	type UserData struct {
		ID       uuid.UUID `json:"id"`
		Username string    `json:"username"`
		Email    string    `json:"email"`
		Password string    `json:"password"`
	}

	var userdata UserData

	if err := c.BodyParser(&userdata); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Error on signup request", "Data": err})
	}

	db := database.DB
	user := new(models.User)
	org := new(models.Organization)
	uname := strings.ToLower(userdata.Username)

	org.Name = uname

	hash, err := helpers.HashPassword(userdata.Password)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"status": "error", "message": "Couldn't hash password", "Data": err})

	}

	user.Email = userdata.Email
	user.Username = uname
	user.Password = hash
	user.Organization = *org
	if err := db.Create(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"status": "error", "message": "Couldn't create user", "Data": err})
	}
	fmt.Println("UserID")
	fmt.Println(user.ID)
	newUser := NewUser{
		ID:       user.ID,
		Email:    user.Email,
		Username: user.Username,
	}

	return c.JSON(fiber.Map{"status": "success", "message": "Created user", "Data": newUser})
}
