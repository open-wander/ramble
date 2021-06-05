package users

import (
	"rmbl/models"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"
	h "rmbl/pkg/helpers"
	"strconv"

	"github.com/dgrijalva/jwt-go"
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
	db := database.DB
	var users []models.User
	var data models.UserData

	order := c.Query("order", "true")
	search := c.Query("search")
	dbquery := db.Model(&users).Preload("Organization")
	dbquery.Order(order)
	dbquery.Scopes(h.UserSearch(search))
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
	var id uuid.UUID
	var data models.UserData
	repos := c.Query("repositories", "false")
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		data.Status = "Failure"
		data.Message = "id not valid"
		data.Data = nil
		return c.JSON(data)
	}
	db := database.DB
	var user models.User
	dbquery := db.Model(&user).Preload("Organization")
	if repos == "true" {
		dbquery.Preload("Organization.Repositories")
	}
	dbquery.Find(&user, id)
	if user.Username == "" {
		return c.Status(404).JSON(fiber.Map{"Status": "error", "Message": "No user found with ID", "Data": nil})
	}
	return c.JSON(fiber.Map{"Data": user, "Message": "User found", "Status": "success"})
}

// UpdateUser update user
func UpdateUser(c *fiber.Ctx) error {
	c.Accepts("application/json")
	type UpdateUserInput struct {
		Names string `json:"names"`
	}
	var uui UpdateUserInput
	if err := c.BodyParser(&uui); err != nil {
		return c.Status(500).JSON(fiber.Map{"status": "error", "message": "Review your input", "data": err})
	}
	var id uuid.UUID
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	token := c.Locals("user").(*jwt.Token)

	if !helpers.ValidToken(token, id) {
		return c.Status(500).JSON(fiber.Map{"status": "error", "message": "Invalid token id", "data": nil})
	}

	db := database.DB
	var user models.User

	db.First(&user, id)
	db.Save(&user)

	return c.JSON(fiber.Map{"status": "success", "message": "User successfully updated", "data": user})
}

// DeleteUser delete user
func DeleteUser(c *fiber.Ctx) error {
	type PasswordInput struct {
		Password string `json:"password"`
	}
	var pi PasswordInput
	if err := c.BodyParser(&pi); err != nil {
		return c.Status(500).JSON(fiber.Map{"status": "error", "message": "Review your input", "data": err})
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	// token := c.Locals("user").(*jwt.Token)

	// if !helpers.ValidToken(token, id) {
	// 	return c.Status(500).JSON(fiber.Map{"status": "error", "message": "Invalid token id", "data": nil})

	// }

	if !helpers.ValidUser(id, pi.Password) {
		return c.Status(500).JSON(fiber.Map{"status": "error", "message": "Not valid user", "data": nil})

	}

	db := database.DB
	var user models.User

	db.Preload("Organization").First(&user, id)

	// When Users are Deleted their repositories are deleted at the same time.

	db.Select("Organization").Delete(&user)
	return c.JSON(fiber.Map{"status": "success", "message": "User successfully deleted", "data": nil})
}
