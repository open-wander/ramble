package repositories

import (
	"errors"
	"fmt"
	"rmbl/models"
	"rmbl/pkg/apperr"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"
	"strconv"
	"strings"

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

// GetOrgID by name
func getOrganizationIDByUserName(username string) (id uuid.UUID) {
	db := database.DB
	var org models.Organization
	db.Where("org_name = ?", username).Find(&org)
	return uuid.UUID(org.ID)
}

// Get All Repositories limited to 25 results
// you can use ?limit=25&offset=0&order=desc to override the defaults

func GetAllRepositories(c *fiber.Ctx) error {

	db := database.DB
	var repositories []models.RepositoryViewStruct
	var data models.RepoData

	order := c.Query("order", "true")
	search := c.Query("search")
	dbquery := db.Model(&models.Repository{}).Joins("inner join organizations on organizations.id = repositories.organization_id")
	dbquery.Order(order)
	dbquery.Scopes(helpers.Search(search))
	dbquery.Select("repositories.id, repositories.name, repositories.version, repositories.description, repositories.url, organizations.org_name")
	dbquery.Count(&data.TotalRecords)
	dbquery.Scopes(Paginate(c))
	dbquery.Scan(&repositories)
	if dbquery.RowsAffected == 0 {
		data.Status = "Failure"
		data.Message = "No Records found"
		data.Data = repositories
		return c.JSON(data)
	}
	data.Data = repositories
	data.Status = "Success"
	data.Message = "Records found"
	return c.JSON(data)
}

// Get All Org Repositories limited to 25 results
// you can use ?limit=25&offset=0&order=desc to override the defaults

func GetOrgRepositories(c *fiber.Ctx) error {
	orgname := c.Params("org")
	orgid := getOrganizationIDByUserName(orgname)
	db := database.DB
	var repositories []models.RepositoryViewStruct
	var data models.RepoData

	order := c.Query("order", "true")
	search := c.Query("search")
	dbquery := db.Model(&models.Repository{}).Joins("inner join organizations on organizations.id = repositories.organization_id")
	dbquery.Where("organization_id = ?", orgid)
	dbquery.Order(order)
	dbquery.Scopes(helpers.Search(search))
	dbquery.Select("repositories.id, repositories.name, repositories.version, repositories.description, repositories.url, organizations.org_name")
	dbquery.Count(&data.TotalRecords)
	dbquery.Scopes(Paginate(c))
	dbquery.Scan(&repositories)
	if dbquery.RowsAffected == 0 {
		data.Status = "Failure"
		data.Message = "No Records found"
		data.Data = repositories
		return c.JSON(data)
	}
	data.Data = repositories
	data.Status = "Success"
	data.Message = "Records found"
	return c.JSON(data)
}

//Get an individual repository detail

func GetRepository(c *fiber.Ctx) error {
	orgname := c.Params("org")
	reponame := c.Params("reponame")
	orgid := getOrganizationIDByUserName(orgname)
	// Get Useragent from request
	useragent := string(c.Context().UserAgent())
	db := database.DB
	repositories := &models.RepositoryViewStruct{}
	dbquery := db.Model(&models.Repository{}).Joins("inner join organizations on organizations.id = repositories.organization_id")
	dbquery.Where("organization_id = ? and name = ?", orgid, reponame)
	dbquery.Select("repositories.id, repositories.name, repositories.version, repositories.description, repositories.url, organizations.org_name")
	dbquery.Scan(&repositories)
	if dbquery.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"Status": "error", "Message": "No repository found with that name", "Data": nil})
	}
	if strings.HasPrefix(useragent, "git") {
		p := "/" + c.Params("*") + "?" + string(c.Context().QueryArgs().QueryString())
		return c.Redirect(repositories.URL+p, 302)
	} else {
		return c.JSON(fiber.Map{"Status": "success", "Message": "Repository Found", "Data": repositories})
	}
}

//Create a new repository
func NewRepository(c *fiber.Ctx) error {
	c.Accepts("application/json")
	orgname := strings.ToLower(c.Params("org"))

	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	username := claims["username"].(string)

	db := database.DB

	var organization models.Organization
	repository := new(models.Repository)
	db.Where("org_name = ?", orgname).Find(&organization)
	if err := c.BodyParser(repository); err != nil {
		return c.Status(500).JSON(fiber.Map{"status": "error", "message": "Review your repository input", "Data": err})
	}
	if username != orgname {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"status": "error", "message": "Unauthorized", "Data": nil})
	}
	db.Model(&organization).Where("org_name = ?", orgname).Association("Repositories").Append(repository)
	return c.JSON(repository)
}

//Update a repository
func UpdateRepository(c *fiber.Ctx) error {
	c.Accepts("application/json")
	orgname := c.Params("org")
	reponame := c.Params("reponame")

	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	username := claims["username"].(string)
	userorg_id := claims["userorg_id"].(uuid.UUID)

	db := database.DB
	orgid := getOrganizationIDByUserName(orgname)
	var repository models.Repository
	if userorg_id != orgid || username != orgname {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"status": "error", "message": "Unauthorized", "Data": nil})
	}
	err := db.Where(&models.Repository{OrganizationID: orgid}).Where("name = ?", reponame).First(&repository).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}

	updatedRepository := new(models.Repository)

	if err := c.BodyParser(updatedRepository); err != nil {
		return apperr.BadRequest("Invalid params")
	}

	updatedRepository = &models.Repository{Name: updatedRepository.Name, OrganizationID: updatedRepository.OrganizationID, Version: updatedRepository.Version, Description: updatedRepository.Description, URL: updatedRepository.URL}
	updatedRepository.OrganizationID = orgid
	updatedRepository.Name = reponame
	if err = db.Where(&models.Repository{OrganizationID: orgid}).Model(&repository).Where("name = ?", reponame).Updates(updatedRepository).Error; err != nil {
		return apperr.Unexpected(err.Error())
	}

	return c.SendStatus(204)
}

func DeleteRepository(c *fiber.Ctx) error {
	orgname := c.Params("org")
	reponame := c.Params("reponame")
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	username := claims["username"].(string)
	userorg_id, _ := uuid.Parse(claims["userorg_id"].(string))

	fmt.Println(username)
	fmt.Println(userorg_id)

	db := database.DB
	orgid := getOrganizationIDByUserName(orgname)

	var repository models.Repository
	if userorg_id != orgid || username != orgname {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"status": "error", "message": "Unauthorized", "Data": nil})
	}
	err := db.Where(&models.Repository{OrganizationID: orgid}).Where("name = ?", reponame).First(&repository).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No Repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}

	db.Delete(&repository)
	return c.SendStatus(204)
}
