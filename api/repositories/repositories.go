package repositories

import (
	"errors"
	"fmt"
	paginate "rmbl/api/util"
	"rmbl/models"
	"rmbl/pkg/apperr"
	"rmbl/pkg/database"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// TODO create a filtered list of repositories as this will get bigger as time goes on

func GetAllRepositories(c *fiber.Ctx) error {
	search := c.Query("search")
	order := c.Query("order", "DESC")
	db := database.DB
	var repositories []models.Repository
	if search != "" {
		db.Order("name "+order).Scopes(paginate.Paginate(c)).Where("name LIKE ?", "%"+search+"%").Find(&repositories)
	} else {
		db.Order("name " + order).Scopes(paginate.Paginate(c)).Find(&repositories)
	}
	return c.JSON(repositories)
}

// TODO create a filtered list of repositories as this will get bigger as time goes on

func GetUserRepositories(c *fiber.Ctx) error {
	fmt.Println("user Repos Hit")
	search := c.Query("search")
	order := c.Query("order", "DESC")
	user := c.Params("user")
	db := database.DB
	var repositories []models.Repository
	if search != "" {
		db.Order("name "+order).Scopes(paginate.Paginate(c)).Where(&models.Repository{User: user}).Where("name LIKE ?", "%"+search+"%").Find(&repositories)
	} else {
		db.Order("name " + order).Scopes(paginate.Paginate(c)).Where(&models.Repository{User: user}).Find(&repositories)
	}
	return c.JSON(repositories)
}

func GetRepository(c *fiber.Ctx) error {
	user := c.Params("user")
	name := c.Params("name")
	db := database.DB
	var repository models.Repository
	err := db.Where("user = ? AND name = ?", user, name).Find(&repository).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}

	return c.JSON(repository)
}

func NewRepository(c *fiber.Ctx) error {
	c.Accepts("application/json")
	user := c.Params("user")
	db := database.DB
	repository := new(models.Repository)
	// TODO Create methods to determing the user name
	// for now we just create the user as part of the query

	repository.User = user
	if err := c.BodyParser(repository); err != nil {
		fmt.Println(repository)
		return apperr.BadRequest("Invalid params")
	}
	db.Create(&repository)
	return c.JSON(repository)
}

func UpdateRepository(c *fiber.Ctx) error {
	c.Accepts("application/json")
	user := c.Params("user")
	name := c.Params("name")
	db := database.DB
	var repository models.Repository
	err := db.Where("user = ? AND name = ?", user, name).First(&repository).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}

	updatedRepository := new(models.Repository)

	if err := c.BodyParser(updatedRepository); err != nil {
		return apperr.BadRequest("Invalid params")
	}

	updatedRepository = &models.Repository{Name: updatedRepository.Name, User: updatedRepository.User, Version: updatedRepository.Version, Description: updatedRepository.Description, URL: updatedRepository.URL}
	updatedRepository.User = user
	updatedRepository.Name = name
	if err = db.Model(&repository).Where("user = ? AND name = ?", user, name).Updates(updatedRepository).Error; err != nil {
		return apperr.Unexpected(err.Error())
	}

	return c.SendStatus(204)
}

func DeleteRepository(c *fiber.Ctx) error {
	user := c.Params("user")
	name := c.Params("name")
	db := database.DB

	var repository models.Repository
	err := db.Where("user = ? AND name = ?", user, name).First(&repository).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No Repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}

	db.Delete(&repository)
	return c.SendStatus(204)
}
