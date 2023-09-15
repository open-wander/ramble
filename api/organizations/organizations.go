package organizations

import (
	"strconv"

	"rmbl/models"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"

	jwt "github.com/form3tech-oss/jwt-go"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func Paginate(c *fiber.Ctx) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		offset, _ := strconv.Atoi(c.Query("offset", "0"))
		limit, _ := strconv.Atoi(c.Query("limit", "25"))
		return db.Offset(offset).Limit(limit)
	}
}

// Get All Organizations limited to 25 results
// you can use ?limit=25&offset=0&order=desc to override the defaults
func GetAllOrgs(c *fiber.Ctx) error {
	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	is_site_admin := claims["site_admin"].(bool)
	repos := c.Query("repositories", "false")
	if !is_site_admin {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}
	db := database.DB
	var organizations []models.Organization
	var data models.OrgData

	order := c.Query("order", "true")
	search := c.Query("search")
	dbquery := db.Model(&organizations)
	if repos == "true" {
		dbquery.Preload("Repositories")
	}
	dbquery.Order(order)
	dbquery.Scopes(helpers.Search(search))
	dbquery.Count(&data.TotalRecords)
	dbquery.Scopes(Paginate(c))
	dbquery.Find(&organizations)
	data.Data = organizations
	data.Status = "Success"
	data.Message = "Records found"
	return c.JSON(data)
}
