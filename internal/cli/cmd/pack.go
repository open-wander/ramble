package cmd

import (
	"github.com/spf13/cobra"
)

var packCmd = &cobra.Command{
	Use:   "pack",
	Short: "Manage Nomad packs",
	Long: `Commands for listing, downloading, and running Nomad packs from the registry.

Examples:
  ramble pack list                    List all packs
  ramble pack list --namespace user1  List packs from a namespace
  ramble pack info user1/mypack       Get pack details
  ramble pack run user1/mypack        Run a pack`,
}

func init() {
	rootCmd.AddCommand(packCmd)
}
