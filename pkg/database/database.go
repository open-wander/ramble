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
