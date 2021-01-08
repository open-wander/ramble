package database

import (
	"fmt"

	"github.com/nsreg/rmbl/model"
	"github.com/nsreg/rmbl/pkg/config"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
)

var (
	DB    *gorm.DB
	err   error
	DBErr error
)

type Database struct {
	*gorm.DB
}

// Setup opens a database and saves the reference to `Database` struct.
func Setup() {
	var db = DB

	config := config.GetConfig()

	database := config.Database.Dbname
	username := config.Database.Username
	password := config.Database.Password
	host := config.Database.Host
	port := config.Database.Port

	db, err = gorm.Open("postgres", "host="+host+" port="+port+" user="+username+" dbname="+database+"  sslmode=disable password="+password)
	if err != nil {
		DBErr = err
		fmt.Println("db err: ", err)
	}

	// Change this to true if you want to see SQL queries
	db.LogMode(false)

	// Auto migrate project models
	db.AutoMigrate(&model.Repository{}, &model.User{}, &model.RelVersion{})
	db.DB().SetMaxIdleConns(20)
	db.DB().SetMaxOpenConns(200)
	DB = db
}

// GetDB helps you to get a connection
func GetDB() *gorm.DB {
	return DB
}

// GetDBErr helps you to get a connection
func GetDBErr() error {
	return DBErr
}
