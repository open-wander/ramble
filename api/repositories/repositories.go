package repositories

import (
	"errors"
	"fmt"
	"rmbl/models"
	"rmbl/pkg/apperr"
	"rmbl/pkg/database"
	h "rmbl/pkg/helpers"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func Paginate(c *fiber.Ctx) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		// sort, _ := strconv.Atoi(c.Query("sort", "ID"))
		// order, _ := strconv.Atoi(c.Query("order", "DESC"))
		offset, _ := strconv.Atoi(c.Query("offset", "0"))
		limit, _ := strconv.Atoi(c.Query("limit", "25"))
		return db.Offset(offset).Limit(limit)
	}
}

// GetUser by name
func getUserIDByUserName(username string) (id uint) {
	db := database.DB
	var user models.User
	db.Where("username LIKE ?", "%"+username+"%").Find(&user)
	return user.ID
}

// Get All Repositories limited to 25 results
// you can use ?limit=25&offset=0&order=desc to override the defaults
func GetAllRepositories(c *fiber.Ctx) error {

	db := database.DB
	var repositories []models.Repository
	var data models.Data
	var userRepos []models.RepositoryViewStruct

	order := c.Query("order", "true")
	// offset := c.Query("offset", "0")
	limit := c.Query("limit", "25")
	search := c.Query("search")

	dbquery := db.Debug().Model(&repositories).Joins("inner join users on users.id = repositories.user_id")
	dbquery.Count(&data.TotalData)
	dbquery.Order(order)
	// dbquery.Limit(h.Limit(limit))
	dbquery.Scopes(h.Search(search), Paginate(c))
	// dbquery.Offset(h.Offset(offset))
	dbquery.Select("repositories.id ,repositories.name, repositories.version, repositories.description, repositories.url, users.username").Scan(&userRepos)
	dbquery.Count(&data.FilteredData)

	// dbquery.Scan(&userRepos)
	filtereddata := &data.FilteredData
	pagelimit := h.Limit(limit)
	var pagesize int64 = int64(pagelimit)
	fmt.Println(pagesize)
	data.FilteredData = *filtereddata
	data.Data = userRepos

	return c.JSON(data)
}

// Get All Repositories limited to 25 results
// you can use ?limit=25&offset=0&order=desc to override the defaults

func GetUserRepositories(c *fiber.Ctx) error {
	fmt.Println("user Repos Hit")
	search := c.Query("search")
	order := c.Query("order", "DESC")
	username := c.Params("user")
	db := database.DB
	var repositories []models.Repository
	userRepos := []models.RepositoryViewStruct{}
	userid := getUserIDByUserName(username)

	if search != "" {
		db.Debug().Model(&repositories).Joins("inner join users on users.id = repositories.user_id").Order("name "+order).Scopes(Paginate(c)).Where(&models.Repository{UserID: userid}).Where("name LIKE ?", "%"+search+"%").
			Select("repositories.id ,repositories.name, repositories.version, repositories.description, repositories.url, users.username").Scan(&userRepos)
	} else {
		db.Debug().Model(&repositories).Joins("inner join users on users.id = repositories.user_id").Order("name " + order).Scopes(Paginate(c)).Where(&models.Repository{UserID: userid}).
			Select("repositories.id, repositories.name, repositories.version, repositories.description, repositories.url, users.username").Scan(&userRepos)
	}
	return c.JSON(userRepos)
}

func GetRepository(c *fiber.Ctx) error {
	user := c.Params("user")
	name := c.Params("name")
	db := database.DB
	var repositories []models.Repository
	userRepo := models.RepositoryViewStruct{}
	userid := getUserIDByUserName(user)

	// Get Useragent from request
	useragent := string(c.Context().UserAgent())

	err := db.Debug().Model(&repositories).Where(&models.Repository{UserID: userid}).Joins("inner join users on users.id = repositories.user_id").Where("name = ?", name).
		Select("repositories.id ,repositories.name, repositories.version, repositories.description, repositories.url, users.username").Scan(&userRepo).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}
	if strings.HasPrefix(useragent, "git") {
		p := "/" + c.Params("*") + "?" + string(c.Context().QueryArgs().QueryString())
		return c.Redirect(userRepo.URL+p, 302)
	} else {
		return c.JSON(userRepo)
	}
}

func NewRepository(c *fiber.Ctx) error {
	c.Accepts("application/json")
	username := c.Params("user")
	fmt.Println(username)
	db := database.DB
	repository := new(models.Repository)
	// TODO Create methods to determing the user name
	// for now we just create the user as part of the query

	repository.UserID = getUserIDByUserName(username)
	fmt.Println(repository.UserID)
	if err := c.BodyParser(repository); err != nil {
		// fmt.Println(repository)
		fmt.Println("Error")
		fmt.Println(err)
		// return apperr.BadRequest("Invalid params")
	}
	db.Create(&repository)
	fmt.Println(repository)
	return c.JSON(repository)
}

func UpdateRepository(c *fiber.Ctx) error {
	c.Accepts("application/json")
	user := c.Params("user")
	name := c.Params("name")
	db := database.DB
	userid := getUserIDByUserName(user)
	var repository models.Repository
	err := db.Debug().Where(&models.Repository{UserID: userid}).Where("name = ?", name).First(&repository).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}

	updatedRepository := new(models.Repository)

	if err := c.BodyParser(updatedRepository); err != nil {
		return apperr.BadRequest("Invalid params")
	}

	updatedRepository = &models.Repository{Name: updatedRepository.Name, UserID: updatedRepository.UserID, Version: updatedRepository.Version, Description: updatedRepository.Description, URL: updatedRepository.URL}
	updatedRepository.UserID = userid
	updatedRepository.Name = name
	if err = db.Debug().Where(&models.Repository{UserID: userid}).Model(&repository).Where("name = ?", name).Updates(updatedRepository).Error; err != nil {
		return apperr.Unexpected(err.Error())
	}

	return c.SendStatus(204)
}

func DeleteRepository(c *fiber.Ctx) error {
	user := c.Params("user")
	name := c.Params("name")
	db := database.DB
	userid := getUserIDByUserName(user)

	var repository models.Repository
	err := db.Debug().Where(&models.Repository{UserID: userid}).Where("name = ?", name).First(&repository).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No Repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}

	db.Delete(&repository)
	return c.SendStatus(204)
}
