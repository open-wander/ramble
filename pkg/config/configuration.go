package appconfig

import (
	"fmt"
	"log"

	"github.com/spf13/viper"
)

// Config Variable
var Config *Configuration

// Configuration Struct
type Configuration struct {
	Server   ServerConfiguration
	Database DatabaseConfiguration
}

// Setup initializes the configuration by reading the config file and unmarshaling it into the Configuration struct.
// It searches for the config file in multiple paths and sets the global Config variable.
func Setup() {
	var configuration *Configuration

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.rmbl")
	viper.AddConfigPath("/etc/rmbl/")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file, %s", err)
	}

	err := viper.Unmarshal(&configuration)
	if err != nil {
		log.Fatalf("Unable to decode into struct, %v", err)
	}
	fmt.Println("Using config file:", viper.ConfigFileUsed())
	Config = configuration
}

// GetConfig helps you to get configuration data
func GetConfig() *Configuration {
	return Config
}
