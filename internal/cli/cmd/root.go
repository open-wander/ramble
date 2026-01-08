package cmd

import (
	"fmt"
	"os"

	"rmbl/internal/cli/update"

	"github.com/spf13/cobra"
)

var (
	verbose     bool
	updateCheck <-chan *update.ReleaseInfo
)

var rootCmd = &cobra.Command{
	Use:   "ramble",
	Short: "Ramble - Nomad Job & Pack Registry",
	Long: `Ramble is a registry for HashiCorp Nomad job files and Nomad Packs.

This CLI allows you to:
  - Run the Ramble web server
  - List, search, and run packs from the registry
  - List, search, and run Nomad job files
  - Browse and search registry namespaces

Examples:
  ramble server                       Start the web server
  ramble pack list                    List available packs
  ramble pack list --search mysql     Search for packs
  ramble job list --search postgres   Search for jobs
  ramble registry browse --search go  Search namespaces
  ramble pack run user/mysql          Run a pack`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Start update check in background (skip for version command, it does its own check)
		if cmd.Name() != "version" && cmd.Name() != "help" {
			updateCheck = update.CheckForUpdateAsync(Version)
		}
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// Check if update is available (non-blocking)
		if updateCheck != nil {
			select {
			case info := <-updateCheck:
				if info != nil {
					fmt.Fprint(os.Stderr, info.FormatUpdateMessage())
				}
			default:
				// Check not complete yet, skip
			}
		}
	},
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
