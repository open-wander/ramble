package controller

import (
	"fmt"
	"log"

	"github.com/nsreg/rmbl/model"
	"github.com/nsreg/rmbl/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
)

var db *gorm.DB
var err error

// Repository struct alias
type Repository = model.Repository

// CreateRep struct alias
type CreateRep = model.CreateRep

// Data is mainly generated for filtering and pagination
type Data struct {
	TotalData    int64
	FilteredData int64
	Data         []Repository
}

// GetRepo func
func GetRepo(c *gin.Context) {
	fmt.Println("Endpoint Hit: returnSingleRepo")
	db = database.GetDB()
	repoName := c.Params.ByName("name")
	fmt.Println("RepoName")
	fmt.Println(repoName)
	fmt.Println("Context Params")
	fmt.Println(c.Params)
	var repos Repository
	if err := db.Preload("RelVersions").Where("name = ? ", repoName).First(&repos).Error; err != nil {
		log.Println(err)
		c.AbortWithStatus(404)
		return
	}

	db.Model(&repos)
	c.JSON(200, repos)
}

// GetRepos Func
func GetRepos(c *gin.Context) {
	fmt.Println("Endpoint Hit: GetAllRepos")
	db = database.GetDB()
	var repositories []Repository
	var data Data

	// Define and get sorting field
	sort := c.DefaultQuery("Sort", "ID")

	// Define and get sorting order field
	order := c.DefaultQuery("Order", "DESC")

	// Define and get offset for pagination
	offset := c.DefaultQuery("Offset", "0")

	// Define and get limit for pagination
	limit := c.DefaultQuery("Limit", "25")

	// Get search keyword for Search Scope
	search := c.DefaultQuery("Search", "")

	table := "repositories"
	query := db.Preload("RelVersions").Select(table + ".*")
	query = query.Offset(Offset(offset))
	query = query.Limit(Limit(limit))
	query = query.Order(SortOrder(table, sort, order))
	query = query.Scopes(Search(search))

	if err := query.Find(&repositories).Error; err != nil {
		log.Println(err)
		c.AbortWithStatus(404)
		return
	}
	// Count filtered table
	// We are resetting offset to 0 to return total number.
	// This is a fix for Gorm offset issue
	query = query.Offset(0)
	query.Table(table).Count(&data.FilteredData)

	// Count total table
	db.Table(table).Count(&data.TotalData)

	// Set Data result
	data.Data = repositories

	c.JSON(200, data)
	// c.JSON(200, repositories)
}
