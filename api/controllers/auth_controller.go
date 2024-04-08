package controllers

import (
	"rmbl/api/authentication"
	"rmbl/models"
	"rmbl/pkg/apperr"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"

	appvalid "rmbl/pkg/validator"

	"github.com/gofiber/fiber/v2"
)

// AuthRoutes registers the authentication routes to the provided fiber.Router.
// It adds the "/login" and "/signup" routes with their corresponding handlers.
func AuthRoutes(route fiber.Router) {
	route.Post("/login", Login)
	route.Post("/signup", Signup)
}

// Signup handles the signup request and creates a new user.
// It expects the request body to be in JSON format with the following fields:
// - username (string): the username of the new user
// - email (string): the email address of the new user
// - password (string): the password of the new user
// If the request body is not in the expected format, it returns a 400 Bad Request response.
// If the provided data fails validation, it returns a 400 Bad Request response with validation errors.
// If there is an error creating the user, it returns a 409 Conflict response with an error message.
// If the user is created successfully, it returns a 200 OK response with the created user data.
func Signup(c *fiber.Ctx) error {
	c.Accepts("application/json")

	if !helpers.ValidRequestHeader(c) {
		return apperr.UnsupportedMediaType("Not Valid Content Type, Expect application/json")
	}
	var userdata models.UserSignupData
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

	authservice, err := authentication.NewAuthService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}

	newUser, err := authservice.SignUp(userdata)
	if err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"Status": "Error", "Message": "Couldn't create user", "Data": "Username or Email Already Exists"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"Status": "Success", "Message": "Created User", "Data": newUser})
}

// Login handles the login request and returns a JWT token upon successful authentication.
// It expects the request header to have a valid content type of "application/json".
// The request body should contain a JSON object representing the user's login credentials.
// If the request is valid and the login is successful, it returns a JSON response with a JWT token.
// If there is an error during the login process, it returns an appropriate error response.
func Login(c *fiber.Ctx) error {
	// Validate the header type
	if !helpers.ValidRequestHeader(c) {
		return apperr.UnsupportedMediaType("Not Valid Content Type, Expect application/json")
	}

	var loginInput models.UserLoginInput

	// parse the request body to validate it and to populate the input struct
	if err := c.BodyParser(&loginInput); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Status": "Error", "Message": "Error on login request", "Data": err})
	}
	authservice, err := authentication.NewAuthService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}

	validate := appvalid.NewValidator()
	if validateerr := validate.Struct(loginInput); validateerr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"Status":  "validation-error",
			"Message": appvalid.ValidationErrors(validateerr),
		})
	}

	jwtToken, error := authservice.Login(loginInput)

	if error != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Couldn't login", "Data": error})
	}
	return c.JSON(fiber.Map{"Status": "Success", "Message": "Success login", "Data": jwtToken})
}
