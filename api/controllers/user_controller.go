package controllers

import (
	"fmt"
	"strconv"
	"strings"

	"rmbl/api/users"
	"rmbl/models"
	"rmbl/pkg/authhelpers"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"

	jwt "github.com/form3tech-oss/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// UserRoutes registers the user routes on the provided fiber.Router.
// It sets up the necessary middleware for authentication and defines the following routes:
// - GET /: Retrieves all users (protected route)
// - GET /:id: Retrieves a user by ID (protected route)
// - GET /:username: Retrieves a user by username (protected route)
// - PUT /:id: Updates a user by ID (protected route)
// - DELETE /:id: Deletes a user by ID (protected route)
func UserRoutes(route fiber.Router) {
	route.Get("/", authhelpers.Protected(), GetAllUsers)
	route.Get("/:id", authhelpers.Protected(), GetUser)
	route.Get("/:username", authhelpers.Protected(), GetUser)
	route.Put("/:id", authhelpers.Protected(), UpdateUser)
	route.Delete("/:id", authhelpers.Protected(), DeleteUser)
}

// GetAllUsers retrieves all users based on the provided search query, order, offset, and limit.
// It takes a fiber.Ctx object as a parameter and returns an error.
// It requires the user to be a site admin.
// It returns a JSON response containing the retrieved user data.
func GetAllUsers(c *fiber.Ctx) error {
	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	is_site_admin := claims["site_admin"].(bool)

	if !is_site_admin {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}
	// Parse the context for the params or use defaults.
	search := strings.ToLower(c.Query("search"))

	order := strings.ToUpper(c.Query("order", "DESC"))

	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil {
		return fmt.Errorf("unable to parse 'offset': %w", err)
	}

	limit, err := strconv.Atoi(c.Query("limit", "25"))
	if err != nil {
		return fmt.Errorf("unable to parse 'limit': %w", err)
	}
	userservice, err := users.NewUserService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	// Call the repository layer for the data
	data := userservice.GetAllUsers(order, search, limit, offset)
	return c.JSON(data)
}

// GetUser retrieves user information based on the provided ID.
// It takes a fiber.Ctx object as a parameter and returns an error.
// It requires a valid user token with site admin privileges.
// The user's ID is extracted from the token claims and compared with the provided ID.
// If the IDs do not match or the user does not have site admin privileges, an unauthorized error is returned.
// The "repositories" query parameter can be used to include the user's repositories in the response.
// The user service is used to fetch the user data from the database.
// The retrieved user data is returned as a JSON response.
func GetUser(c *fiber.Ctx) error {
	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	is_site_admin := claims["site_admin"].(bool)
	username := claims["username"].(string)
	repos := c.Query("repositories", "false")
	var id uuid.UUID
	var includeRepos bool

	helperservice, herr := helpers.NewHelperService(database.DB)
	if herr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	user_id, user_id_err := helperservice.GetUserIDByUserName(username)
	if user_id_err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"Status": "Error", "Message": "Record not Found"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Invalid ID", "Data": nil})
	}

	if user_id != id || !is_site_admin {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}
	if repos == "true" {
		includeRepos = true
	}
	userservice, err := users.NewUserService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	// Call the user layer for the data
	data := userservice.GetUser(user_id, includeRepos)
	return c.Status(200).JSON(data)
}

// UpdateUser updates a user's information based on the provided JSON input.
// It takes a fiber.Ctx object as a parameter and returns an error.
// It requires the user to be authenticated or have site admin privileges.
// The user ID in the URL must match the user ID in the authentication token.
// If the user is not authorized or the input is invalid, appropriate error responses are returned.
// If the update is successful, the updated user information is returned.
func UpdateUser(c *fiber.Ctx) error {
	c.Accepts("application/json")
	var id uuid.UUID
	var user_id uuid.UUID
	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	token_user_id := claims["user_id"].(string)
	is_site_admin := claims["site_admin"].(bool)
	user_id, tokenerr := uuid.Parse(token_user_id)
	if tokenerr != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Invalid User ID", "Data": nil})
	}

	helperservice, herr := helpers.NewHelperService(database.DB)
	if herr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	// Convert the id parameter to a UUID for later use
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Invalid ID", "Data": nil})
	}

	// Check ID in the url against the ID in the Claim
	if id != user_id || !is_site_admin {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}

	// JSON Input for Userupdate.
	type UpdateUserInput struct {
		EmailAddress    string `json:"email"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	var uui UpdateUserInput
	var hash string
	if err := c.BodyParser(&uui); err != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Review your input", "data": err})
	}

	if !helpers.ValidToken(user_token, id) {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Invalid token id", "data": nil})
	}
	if !helperservice.ValidUser(id, uui.CurrentPassword) || !is_site_admin {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Invalid Credentials", "Data": err})
	}
	if uui.NewPassword != "" {
		hash, err = helpers.HashPassword(uui.NewPassword)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Couldn't hash password", "Data": err})
		}
	}
	userservice, err := users.NewUserService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	user, err := userservice.UpdateUser(id, hash, uui.EmailAddress)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Could not update user: " + uui.EmailAddress, "Data": err})
	}

	return c.JSON(fiber.Map{"Status": "Success", "Message": "User successfully updated", "data": user})
}

// DeleteUser is a handler function that deletes a user.
// It takes a fiber.Ctx object as a parameter and returns an error.
// The function retrieves the user ID from the JWT token and the ID parameter from the URL.
// If the user is a site admin, it expects a JSON object with a "password" field in the request body.
// If the user is not a site admin, it checks if the ID in the URL matches the ID in the JWT token.
// It also expects a JSON object with a "password" field in the request body.
// The function then validates the token ID and the user ID using helper functions.
// Finally, it deletes the user using the UserService and returns a JSON response indicating success or failure.
func DeleteUser(c *fiber.Ctx) error {
	var data models.UserData
	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	token_user_id := claims["user_id"].(string)
	is_site_admin := claims["site_admin"].(bool)

	// Get Userid from token
	token_u_id, tokenerr := uuid.Parse(token_user_id)
	if tokenerr != nil {
		data.Status = "Failure"
		data.Message = "id not valid"
		data.Data = nil
		return c.JSON(data)
	}

	// Convert the id parameter to a UUID for later use
	param_user_id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		data.Status = "Failure"
		data.Message = "id not valid"
		data.Data = nil
		return c.JSON(data)
	}

	type PasswordInput struct {
		Password string `json:"password"`
	}

	var pi PasswordInput

	helperservice, herr := helpers.NewHelperService(database.DB)
	if herr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}

	if is_site_admin {
		if err := c.BodyParser(&pi); err != nil {
			return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Review your input", "Data": nil})
		}
	} else {
		// Check ID in the url against the ID in the Claim
		if param_user_id != token_u_id {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
		}

		if err := c.BodyParser(&pi); err != nil {
			return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Review your input", "Data": nil})
		}

		if !helpers.ValidToken(user_token, param_user_id) {
			return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Invalid token id", "Data": nil})
		}

		if !helperservice.ValidUser(param_user_id, pi.Password) {
			return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Not valid user", "Data": nil})
		}

	}
	userservice, err := users.NewUserService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	err = userservice.DeleteUser(param_user_id)
	if err != nil {
		return fmt.Errorf("unable to delete user error: %s", err.Error())
	}
	return c.JSON(fiber.Map{"Status": "Success", "Message": "User successfully deleted", "Data": nil})
}
