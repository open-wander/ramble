package database

import (
	"fmt"
	"log"
	"time"

	appconfig "rmbl/pkg/config"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

type DefaultModel struct {
	ID        uuid.UUID  `gorm:"type:uuid;index:,type:btree;primaryKey;default:uuid_generate_v4()" json:"id"`
	CreatedAt time.Time  `json:"-"`
	UpdatedAt time.Time  `json:"-"`
	DeletedAt *time.Time `json:"-"`
}

// SetupDatabase initializes the database connection using the configuration values from appconfig.
// It establishes a connection to the database using the provided credentials and opens the connection.
// The connection configuration can be customized based on the GormLogger value in appconfig.
// If the GormLogger is set to "Error", the logger will log only error messages.
// If the GormLogger is set to "Info", the logger will log both error and info messages.
// If the GormLogger is not set to "Error" or "Info", the logger will be silent and not log any messages.
// If there is an error during the connection setup, it will be logged and the program will exit.
// After a successful connection, a message will be printed to indicate that the connection is open.
func SetupDatabase() {
	appconfig := appconfig.GetConfig()
	username := appconfig.Database.Username
	password := appconfig.Database.Password
	dbName := appconfig.Database.Dbname
	dbHost := appconfig.Database.Host
	dbPort := appconfig.Database.Port

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
