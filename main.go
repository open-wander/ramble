package main

import (
	"fmt"
	"log"
	"rmbl/api"
	"rmbl/models"
	appconfig "rmbl/pkg/config"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"
	"rmbl/pkg/server"
)

func init() {
	appconfig.Setup()
}

func main() {

	// Server initialization
	app := server.Create()

	// Migrations
	database.DB.AutoMigrate(&models.Organization{})
	database.DB.AutoMigrate(&models.Repository{})
	database.DB.AutoMigrate(&models.User{})

	// Api routes
	api.Setup(app)

	// Setup System User
	systemPassword := helpers.CreateSystemUser(appconfig.Config.Server.AdminEmailAddress)
	if systemPassword != "" {
		fmt.Println("")
		fmt.Println("!!!!!IMPORTANT!!!!!")
		fmt.Println("")
		fmt.Println("This is the System User Password")
		fmt.Println("Save it as it will not appear again")
		fmt.Println("")
		fmt.Println("System user password: ** " + systemPassword + " **")
		fmt.Println("")
		fmt.Println("!!!!!IMPORTANT!!!!!")
		fmt.Println("")
	}

	if err := server.Listen(app); err != nil {
		log.Panic(err)
	}
}
