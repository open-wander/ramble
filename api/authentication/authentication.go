package authentication

import (
	"strings"
	"time"

	"rmbl/models"
	"rmbl/pkg/apperr"
	appconfig "rmbl/pkg/config"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"

	appvalid "rmbl/pkg/validator"

	jwt "github.com/form3tech-oss/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type UserSignupData struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username" validate:"required,min=5,max=32"`
	Email    string    `json:"email" validate:"required,email,min=6,max=32"`
	Password string    `json:"password" validate:"required"`
}

type UserLoginInput struct {
	Identity string `json:"identity" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// Signup create user and userorg
func Signup(c *fiber.Ctx) error {
	c.Accepts("application/json")
	type NewUser struct {
		ID       uuid.UUID `json:"id"`
		Username string    `json:"username"`
		Email    string    `json:"email"`
	}

	var userdata UserSignupData
	if !helpers.ValidRequestHeader(c) {
		return apperr.UnsupportedMediaType("Not Valid Content Type, Expect application/json")
	}

	if err := c.BodyParser(&userdata); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Status": "Error", "Message": "Error on signup request", "Data": err})
	}
	validate := appvalid.NewValidator()
	if validateerr := validate.Struct(userdata); validateerr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"Status":  "validation-error",
			"Message": appvalid.ValidationErrors(validateerr),
		})
	}

	db := database.DB
	user := new(models.User)
	org := new(models.Organization)
	uname := strings.ToLower(userdata.Username)

	org.OrgName = uname

	hash, err := helpers.HashPassword(userdata.Password)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Couldn't hash password", "Data": err})
	}

	user.Email = userdata.Email
	user.Username = uname
	user.Password = hash
	user.Organization = *org
	if err := db.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"Status": "Error", "Message": "Couldn't create user", "Data": "Username or Email Already Exists"})
	}
	newUser := NewUser{
		ID:       user.ID,
		Email:    user.Email,
		Username: user.Username,
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"Status": "Success", "Message": "Created User", "Data": newUser})
}

// Login get user and password
func Login(c *fiber.Ctx) error {
	type UserData struct {
		ID        uuid.UUID `json:"id"`
		Username  string    `json:"username"`
		Email     string    `json:"email"`
		Password  string    `json:"password"`
		SiteAdmin bool      `json:"-"`
	}
	var input UserLoginInput
	var userdata UserData

	if !helpers.ValidRequestHeader(c) {
		return apperr.UnsupportedMediaType("Not Valid Content Type, Expect application/json")
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Status": "Error", "Message": "Error on login request", "Data": err})
	}

	validate := appvalid.NewValidator()
	if validateerr := validate.Struct(input); validateerr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"Status":  "validation-error",
			"Message": appvalid.ValidationErrors(validateerr),
		})
	}
	identity := strings.ToLower(input.Identity)
	pass := input.Password

	email, err := helpers.GetUserByEmail(identity)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Error on email", "Data": err})
	}

	if email == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Invalid Credentials", "Data": err})
	}

	userdata = UserData{
		ID:        email.ID,
		Username:  email.Username,
		Email:     email.Email,
		SiteAdmin: email.SiteAdmin,
		Password:  email.Password,
	}

	orgid := helpers.GetORGIDByUserid(userdata.ID)

	if !helpers.CheckPasswordHash(pass, userdata.Password) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Invalid Credentials", "Data": nil})
	}

	token := jwt.New(jwt.SigningMethodHS256)

	claims := token.Claims.(jwt.MapClaims)
	claims["username"] = userdata.Username
	claims["user_id"] = userdata.ID
	claims["userorg_id"] = orgid
	claims["site_admin"] = userdata.SiteAdmin
	claims["expires"] = time.Now().Add(time.Hour * 72).Unix()

	t, err := token.SignedString([]byte(appconfig.Config.Server.JWTSecret))
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	var authtoken models.JWTToken
	authtoken.Token = t
	return c.JSON(fiber.Map{"Status": "Success", "Message": "Success login", "Data": authtoken})
}
