package main

import (
	"github.com/nsreg/rmbl/pkg/config"
	"github.com/nsreg/rmbl/pkg/database"
	"github.com/nsreg/rmbl/pkg/router"
)

func init() {
	config.Setup()
	database.Setup()
}

func main() {
	config := config.GetConfig()

	r := router.Setup()
	// log.Printf("server port is %s", config.Server.RMBLServerPort)
	r.Run("0.0.0.0:" + config.Server.RMBLServerPort)
}
