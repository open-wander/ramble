package repositories

import (
	"errors"
	"fmt"
	"rmbl/models"
	"rmbl/pkg/apperr"
	"rmbl/pkg/database"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// TODO create a filtered list of repositories as this will get bigger as time goes on

func GetAllRepositories(c *fiber.Ctx) error {
	db := database.DB
	var repositories []models.Repository
	db.Find(&repositories)
	return c.JSON(repositories)
}

// TODO create a filtered list of repositories as this will get bigger as time goes on

func GetOrgRepositories(c *fiber.Ctx) error {
	fmt.Println("org Repos Hit")
	org := c.Params("org")
	db := database.DB
	var repositories []models.Repository
	db.Where(&models.Repository{Org: org}).Find(&repositories)
	return c.JSON(repositories)
}

func GetRepository(c *fiber.Ctx) error {
	org := c.Params("org")
	name := c.Params("name")
	db := database.DB
	var repository models.Repository
	err := db.Where("org = ? AND name = ?", org, name).Find(&repository).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}

	return c.JSON(repository)
}

func NewRepository(c *fiber.Ctx) error {
	org := c.Params("org")
	db := database.DB
	repository := new(models.Repository)
	// TODO Create methods to determing the org name
	// for now we just create the org as part of the query

	repository.Org = org
	if err := c.BodyParser(repository); err != nil {
		return apperr.BadRequest("Invalid params")
	}
	db.Create(&repository)
	return c.JSON(repository)
}

func UpdateRepository(c *fiber.Ctx) error {
	org := c.Params("org")
	name := c.Params("name")
	db := database.DB
	var repository models.Repository
	err := db.Where("org = ? AND name = ?", org, name).First(&repository).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}

	updatedRepository := new(models.Repository)

	if err := c.BodyParser(updatedRepository); err != nil {
		return apperr.BadRequest("Invalid params")
	}

	updatedRepository = &models.Repository{Name: updatedRepository.Name, Org: updatedRepository.Org, Version: updatedRepository.Version, Description: updatedRepository.Description, URL: updatedRepository.URL}
	updatedRepository.Org = org
	updatedRepository.Name = name
	if err = db.Model(&repository).Where("org = ? AND name = ?", org, name).Updates(updatedRepository).Error; err != nil {
		return apperr.Unexpected(err.Error())
	}

	return c.SendStatus(204)
}

func DeleteRepository(c *fiber.Ctx) error {
	org := c.Params("org")
	name := c.Params("name")
	db := database.DB

	var repository models.Repository
	err := db.Where("org = ? AND name = ?", org, name).First(&repository).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.EntityNotFound("No Repository found")
	} else if err != nil {
		return apperr.Unexpected(err.Error())
	}

	db.Delete(&repository)
	return c.SendStatus(204)
}
