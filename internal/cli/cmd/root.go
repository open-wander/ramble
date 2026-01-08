package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:   "ramble",
	Short: "Ramble - Nomad Job & Pack Registry",
	Long: `Ramble is a registry for HashiCorp Nomad job files and Nomad Packs.

This CLI allows you to:
  - Run the Ramble web server
  - List, download, and run packs from the registry
  - Run and validate Nomad job files

Examples:
  ramble server              Start the web server
  ramble pack list           List available packs
  ramble pack run mysql      Run a pack with default variables
  ramble job run app.nomad   Run a job file`,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "V", false, "Enable verbose output")
}
