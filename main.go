package main

import (
	"log"
	"rmbl/api"
	"rmbl/models"
	appconfig "rmbl/pkg/config"
	"rmbl/pkg/database"
	"rmbl/pkg/server"
)

func init() {
	appconfig.Setup()
}

func main() {

	// config := appconfig.GetConfig()
	// config.Setup()

	// Server initialization
	app := server.Create()

	// Migrations
	database.DB.AutoMigrate(&models.User{})
	database.DB.AutoMigrate(&models.Repository{})

	// Api routes
	api.Setup(app)

	if err := server.Listen(app); err != nil {
		log.Panic(err)
	}
}
