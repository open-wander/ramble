package main

import (
	"rmbl/internal/cli/cmd"
)

// @title RMBL Nomad Registry API
// @version 1.0
// @description This is the API documentation for the RMBL Nomad Job & Pack Registry.
// @contact.name API Support
// @license.name MPL-2.0
// @host localhost:3000
// @BasePath /
func main() {
	cmd.Execute()
}
