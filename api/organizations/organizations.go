package organizations

import (
	"fmt"
	"rmbl/models"
	"rmbl/pkg/database"
	h "rmbl/pkg/helpers"
	"strconv"

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
	is_site_admin := claims["site_admin_user"].(bool)

	if !is_site_admin {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"status": "error", "message": "Unauthorized", "Data": nil})
	}
	db := database.DB
	var organizations []models.Organization
	var data models.OrgData

	order := c.Query("order", "true")
	search := c.Query("search")
	dbquery := db.Model(&organizations).Preload("Repositories")
	dbquery.Order(order)
	dbquery.Scopes(h.Search(search))
	dbquery.Count(&data.TotalRecords)
	dbquery.Scopes(Paginate(c)).Find(&organizations)
	data.Data = organizations
	data.Status = "Success"
	data.Message = "Records found"
	fmt.Println("Organizations Endpoint")
	return c.JSON(data)
}
