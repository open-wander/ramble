package cmd

import (
	"fmt"
	"runtime"

	"rmbl/internal/cli/update"

	"github.com/spf13/cobra"
)

// Version information - set via ldflags at build time
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("ramble %s\n", Version)
		fmt.Printf("  commit:  %s\n", Commit)
		fmt.Printf("  built:   %s\n", BuildDate)
		fmt.Printf("  go:      %s\n", runtime.Version())
		fmt.Printf("  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)

		// Check for updates
		if info := update.CheckForUpdate(Version); info != nil {
			fmt.Print(info.FormatUpdateMessage())
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
