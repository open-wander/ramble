package users

import (
	"rmbl/models"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"
	"strconv"

	jwt "github.com/form3tech-oss/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func Paginate(c *fiber.Ctx) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		offset, _ := strconv.Atoi(c.Query("offset", "0"))
		limit, _ := strconv.Atoi(c.Query("limit", "25"))
		return db.Offset(offset).Limit(limit)
	}
}

// GetAllUsers get all user
// TODO create a filtered list of repositories as this will get bigger as time goes on

func GetAllUsers(c *fiber.Ctx) error {
	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	is_site_admin := claims["site_admin"].(bool)

	if !is_site_admin {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}
	db := database.DB
	var users []models.User
	var data models.UserData

	order := c.Query("order", "true")
	search := c.Query("search")
	dbquery := db.Model(&users).Preload("Organization")
	dbquery.Order(order)
	dbquery.Scopes(helpers.UserSearch(search))
	dbquery.Count(&data.TotalRecords)
	dbquery.Scopes(Paginate(c))
	dbquery.Find(&users)
	if dbquery.RowsAffected == 0 {
		data.Status = "Failure"
		data.Message = "No Records found"
		data.Data = users
		return c.JSON(data)
	}
	data.Status = "Success"
	data.Message = "Records found"
	data.Data = users
	return c.JSON(data)
}

// TODO create a function for getting all users related to an ORG

// GetUser returns a user
// if you add the query parameter ?repositories=true it will return the repositories as well.
func GetUser(c *fiber.Ctx) error {
	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	is_site_admin := claims["site_admin"].(bool)
	username := claims["username"].(string)
	user_id := helpers.GetUserIDByUserName(username)
	var id uuid.UUID
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Invalid ID", "Data": nil})
	}

	if user_id != id || !is_site_admin {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}

	repos := c.Query("repositories", "false")

	db := database.DB
	var user models.User
	dbquery := db.Model(&user).Preload("Organization")
	if repos == "true" {
		dbquery.Preload("Organization.Repositories")
	}
	dbquery.Find(&user, id)
	if user.Username == "" {
		return c.Status(404).JSON(fiber.Map{"Status": "Error", "Message": "No user found with ID", "Data": nil})
	}
	return c.JSON(fiber.Map{"Data": user, "Message": "User found", "Status": "Success"})
}

// UpdateUser update user
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
	if err := c.BodyParser(&uui); err != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Review your input", "data": err})
	}

	if !helpers.ValidToken(user_token, id) {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Invalid token id", "data": nil})
	}

	db := database.DB
	var user models.User
	db.First(&user, id)
	if !helpers.ValidUser(id, uui.CurrentPassword) || !is_site_admin {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Invalid Credentials", "Data": err})
	}
	if uui.NewPassword != "" {
		hash, err := helpers.HashPassword(uui.NewPassword)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Couldn't hash password", "Data": err})
		}
		user.Password = hash
	} else if uui.EmailAddress != "" {
		user.Email = uui.EmailAddress
	}
	db.Save(&user)

	return c.JSON(fiber.Map{"Status": "Success", "Message": "User successfully updated", "data": user})
}

// DeleteUser delete user
func DeleteUser(c *fiber.Ctx) error {
	var data models.UserData
	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	token_user_id := claims["user_id"].(string)
	is_site_admin := claims["site_admin"].(bool)

	// Get Userid from token
	user_id, tokenerr := uuid.Parse(token_user_id)
	if tokenerr != nil {
		data.Status = "Failure"
		data.Message = "id not valid"
		data.Data = nil
		return c.JSON(data)
	}

	// Convert the id parameter to a UUID for later use
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		data.Status = "Failure"
		data.Message = "id not valid"
		data.Data = nil
		return c.JSON(data)
	}

	// Check ID in the url against the ID in the Claim
	if id != user_id || !is_site_admin {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}
	type PasswordInput struct {
		Password string `json:"password"`
	}
	var pi PasswordInput
	if err := c.BodyParser(&pi); err != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Review your input", "data": err})
	}

	if !helpers.ValidToken(user_token, id) || !is_site_admin {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Invalid token id", "data": nil})

	}

	if !helpers.ValidUser(id, pi.Password) || is_site_admin {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Not valid user", "data": nil})

	}

	db := database.DB
	var user models.User

	db.Preload("Organization").First(&user, id)

	// When Users are Deleted their repositories are deleted at the same time.

	db.Select("Organization").Delete(&user)
	return c.JSON(fiber.Map{"Status": "Success", "Message": "User successfully deleted", "data": nil})
}
