package main

import (
	"rmbl/internal/cli/cmd"
)

// @title RMBL Nomad Registry API
// @version 1.0
// @description This is the API documentation for the RMBL Nomad Job & Pack Registry.
// @contact.name API Support
// @license.name MPL-2.0
// @host ramble.openwander.org
// @BasePath /
// @securityDefinitions.apikey bearerAuth
// @in header
// @name Authorization
// @description Ramble API token, sent as the Authorization header: Bearer <token>
func main() {
	cmd.Execute()
}
