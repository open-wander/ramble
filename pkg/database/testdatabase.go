package database

import (
	"fmt"
	"log"

	appconfig "rmbl/pkg/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func SetupTestDatabase() {
	appconfig := appconfig.GetConfig()
	username := appconfig.Database.Username
	password := appconfig.Database.Password
	dbName := "rmbltestdb"
	dbHost := appconfig.Database.Host
	dbPort := "5433"
	var err error
	var config gorm.Config
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable password=%s", dbHost, dbPort, username, dbName, password)

	if appconfig.Database.GormLogger == "Error" {
		config = gorm.Config{
			Logger: logger.Default.LogMode(logger.Error),
		}
	} else if appconfig.Database.GormLogger == "Info" {
		config = gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
		}
	} else {
		config = gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		}
	}

	DB, err = gorm.Open(postgres.Open(dsn), &config)

	if err != nil {
		log.Fatal(err)
		panic("Failed to connect database")
	}

	fmt.Println("Connection Opened to Database")
}
