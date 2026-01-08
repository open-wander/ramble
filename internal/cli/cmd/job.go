package cmd

import (
	"github.com/spf13/cobra"
)

var jobCmd = &cobra.Command{
	Use:   "job",
	Short: "Manage Nomad jobs",
	Long: `Commands for running and validating Nomad job files.

Examples:
  ramble job run app.nomad.hcl       Run a job file
  ramble job validate app.nomad.hcl  Validate a job file`,
}

func init() {
	rootCmd.AddCommand(jobCmd)
}
