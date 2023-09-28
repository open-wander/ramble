package repositories

import (
	"errors"
	"strconv"
	"strings"

	"rmbl/models"
	"rmbl/pkg/apperr"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"
	appvalid "rmbl/pkg/validator"

	jwt "github.com/form3tech-oss/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Paginate returns a function which can be used to paginate results.
// Deprecated: please use repositories.paginate instead.
func Paginate(c *fiber.Ctx) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		offset, _ := strconv.Atoi(c.Query("offset", "0"))
		limit, _ := strconv.Atoi(c.Query("limit", "25"))
		return db.Offset(offset).Limit(limit)
	}
}

// paginate returns a function which can be used to paginate results.
func paginate(offset int, limit int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
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

// GetAllRepositories returns all the repositories in the database.
func GetAllRepositories(order bool, search string, limit int, offset int) models.RepoData {
	// TODO: We may want to validate the incoming parameters, and in the future
	// consider using the Options pattern to ensure sane defaults.

	db := database.DB
	search = strings.ToLower(search)

	var repositories []models.RepositoryViewStruct
	var data models.RepoData

	dbQuery := db.Model(&models.Repository{}).Joins("inner join organizations on organizations.id = repositories.organization_id")
	dbQuery.Order(order)
	dbQuery.Scopes(helpers.Search(search))
	dbQuery.Select("repositories.id, repositories.name, repositories.version, repositories.description, repositories.url, organizations.org_name")
	dbQuery.Count(&data.TotalRecords)
	dbQuery.Scopes(paginate(offset, limit))
	dbQuery.Scan(&repositories)

	data.Status = "Success"
	data.Message = "Records found"
	data.Data = repositories

	// Override the message if there were no records.
	if dbQuery.RowsAffected == 0 {
		data.Message = "No Records found"
	}

	return data
}

// Get All Org Repositories limited to 25 results
// you can use ?limit=25&offset=0&order=desc to override the defaults

func GetOrgRepositories(c *fiber.Ctx) error {
	orgname := strings.ToLower(c.Params("org"))
	orgid := getOrganizationIDByUserName(orgname)
	db := database.DB
	var repositories []models.RepositoryViewStruct
	var data models.RepoData

	search := strings.ToLower(c.Query("search"))
	dbquery := db.Model(&models.Repository{}).Joins("inner join organizations on organizations.id = repositories.organization_id")
	dbquery.Where("organization_id = ?", orgid)
	dbquery.Order("repositories.name DESC")
	dbquery.Scopes(helpers.Search(search))
	dbquery.Select("repositories.id, repositories.name, repositories.version, repositories.description, repositories.url, organizations.org_name")
	dbquery.Count(&data.TotalRecords)
	dbquery.Scopes(Paginate(c))
	dbquery.Scan(&repositories)
	if dbquery.RowsAffected == 0 {
		data.Status = "Success"
		data.Message = "No Records found"
		data.Data = repositories
		return c.Status(200).JSON(data)
	}
	data.Data = repositories
	data.Status = "Success"
	data.Message = "Records found"
	return c.Status(200).JSON(data)
}

// Get an individual repository detail
func GetRepository(c *fiber.Ctx) error {
	orgname := strings.ToLower(c.Params("org"))
	reponame := strings.ToLower(c.Params("reponame"))
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
		return c.Status(404).JSON(fiber.Map{"Status": "Error", "Message": "No repository found with that name", "Data": nil})
	}
	if strings.HasPrefix(useragent, "git") {
		p := "/" + c.Params("*") + "?" + string(c.Context().QueryArgs().QueryString())
		return c.Redirect(repositories.URL+p, 302)
	} else {
		return c.JSON(fiber.Map{"Status": "Success", "Message": "Repository Found", "Data": repositories})
	}
}

// Create a new repository
func NewRepository(c *fiber.Ctx) error {
	c.Accepts("application/json")
	// Valid Header
	if !helpers.ValidRequestHeader(c) {
		return apperr.UnsupportedMediaType("Not Valid Content Type, Expect application/json")
	}
	var id uuid.UUID
	orgname := strings.ToLower(c.Params("org"))

	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	username := claims["username"].(string)
	// userorg_id := claims["userorg_id"].(uuid.UUID)
	is_siteadmin := claims["site_admin"].(bool)
	db := database.DB
	// orgid := getOrganizationIDByUserName(orgname)
	id = helpers.GetUserIDByUserName(username)
	var organization models.Organization
	repository := new(models.Repository)

	// check JSON input
	if err := c.BodyParser(repository); err != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Review your repository input", "Data": err})
	}

	validate := appvalid.NewValidator()
	if validateerr := validate.Struct(repository); validateerr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"Status":  "validation-error",
			"Message": appvalid.ValidationErrors(validateerr),
		})
	}

	// Check for a valid token
	if !helpers.ValidToken(user_token, id) {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Invalid token id", "Data": nil})
	}
	// find the org in the organization table
	db.Where("org_name = ?", orgname).Find(&organization)
	if username != orgname {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}
	if username == orgname || is_siteadmin {
		db.Model(&organization).Where("org_name = ?", orgname).Association("Repositories").Append(repository)
	}
	return c.JSON(repository)
}

// Update a repository
func UpdateRepository(c *fiber.Ctx) error {
	c.Accepts("application/json")
	// Valid Header
	if !helpers.ValidRequestHeader(c) {
		return apperr.UnsupportedMediaType("Not Valid Content Type, Expect application/json")
	}
	var id uuid.UUID
	orgname := c.Params("org")
	reponame := c.Params("reponame")

	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	username := claims["username"].(string)
	is_siteadmin := claims["site_admin"].(bool)
	userorg_id, _ := uuid.Parse(claims["userorg_id"].(string))
	id = helpers.GetUserIDByUserName(username)

	db := database.DB
	orgid := getOrganizationIDByUserName(orgname)
	repository := new(models.Repository)
	updatedRepository := new(models.Repository)

	if err := c.BodyParser(updatedRepository); err != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Review your repository input", "Data": err})
	}

	validate := appvalid.NewValidator()
	if validateerr := validate.Struct(updatedRepository); validateerr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"Status":  "validation-error",
			"Message": appvalid.ValidationErrors(validateerr),
		})
	}

	if !helpers.ValidToken(user_token, id) {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Invalid token id", "Data": nil})
	}
	if userorg_id != orgid || username != orgname {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}
	err := db.Where(&models.Repository{OrganizationID: orgid}).Where("name = ?", reponame).First(&repository).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}

	if userorg_id == orgid || username == orgname || is_siteadmin {
		if err = db.Where(&models.Repository{OrganizationID: orgid}).Model(&repository).Where("name = ?", reponame).Updates(updatedRepository).Error; err != nil {
			return apperr.Unexpected(err.Error())
		}
	}
	// return c.SendStatus(204)
	return c.JSON(updatedRepository)
}

func DeleteRepository(c *fiber.Ctx) error {
	orgname := c.Params("org")
	reponame := c.Params("reponame")
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	username := claims["username"].(string)
	is_siteadmin := claims["site_admin"].(bool)
	userorg_id, _ := uuid.Parse(claims["userorg_id"].(string))

	db := database.DB
	orgid := getOrganizationIDByUserName(orgname)

	var repository models.Repository
	if userorg_id != orgid || username != orgname {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}
	err := db.Where(&models.Repository{OrganizationID: orgid}).Where("name = ?", reponame).First(&repository).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No Repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}
	if userorg_id == orgid || username == orgname || is_siteadmin {
		db.Delete(&repository)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"Status": "Success", "Message": reponame + " Deleted", "Data": nil})
}
