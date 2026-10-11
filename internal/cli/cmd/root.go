package cmd

import (
	"errors"
	"fmt"
	"io"
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
		if wantsUpdateCheck(cmd) {
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

// wantsUpdateCheck reports whether cmd should start the background update
// check. version does its own check, help is not worth a network call, and
// server must write only JSON (no update notice, no network call at startup).
func wantsUpdateCheck(cmd *cobra.Command) bool {
	if cmd == versionCmd || cmd == serverCmd {
		return false
	}
	// cobra creates the help command lazily, so there is no variable to
	// compare; match the root's direct child. (rootCmd itself would be an
	// initialization cycle.)
	return !(cmd.Name() == "help" && cmd.Parent() != nil && !cmd.Parent().HasParent())
}

// reportExecError prints err in plain text unless it was already logged as
// structured JSON (see runServerCmd).
func reportExecError(err error, w io.Writer) {
	if _, ok := errors.AsType[loggedError](err); ok {
		return
	}
	fmt.Fprintln(w, err)
}

// Execute runs the root command
func Execute() {
	if _, err := rootCmd.ExecuteC(); err != nil {
		reportExecError(err, os.Stderr)
		os.Exit(1)
	}
}

func init() {
	// Setting Version makes cobra add --version/-v; the template keeps the
	// first line consistent with the fuller "ramble version" subcommand.
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("ramble {{.Version}}\n")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "V", false, "Enable verbose output")
}
