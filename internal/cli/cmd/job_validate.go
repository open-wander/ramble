package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"rmbl/internal/nomad"

	"github.com/spf13/cobra"
)

var (
	jobValidateNomadAddr string
)

var jobValidateCmd = &cobra.Command{
	Use:   "validate <jobfile>",
	Short: "Validate a Nomad job file",
	Long: `Validate a Nomad job file without submitting it.

Examples:
  ramble job validate ./app.nomad.hcl`,
	Args: cobra.ExactArgs(1),
	RunE: runJobValidate,
}

func init() {
	jobCmd.AddCommand(jobValidateCmd)

	jobValidateCmd.Flags().StringVar(&jobValidateNomadAddr, "nomad-addr", "", "Nomad address (overrides NOMAD_ADDR)")
}

func runJobValidate(cmd *cobra.Command, args []string) error {
	jobFile := args[0]

	// Clean path to normalize
	cleanPath := filepath.Clean(jobFile)

	// G304: Path is explicitly provided by user via CLI argument.
	// This is intentional functionality - users choose which job files to validate.
	content, err := os.ReadFile(cleanPath) //#nosec G304 -- user-provided CLI path, intentional functionality
	if err != nil {
		return fmt.Errorf("failed to read job file: %w", err)
	}

	// Check nomad is available
	if err := nomad.CheckNomadAvailable(); err != nil {
		return err
	}

	// Validate
	if err := nomad.ValidateJob(string(content), jobValidateNomadAddr); err != nil {
		return err
	}

	fmt.Println("Job is valid!")
	return nil
}
