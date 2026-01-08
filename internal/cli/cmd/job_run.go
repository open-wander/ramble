package cmd

import (
	"fmt"
	"os"
	"strings"

	"rmbl/internal/cli/config"
	"rmbl/internal/nomad"
	"rmbl/internal/pack"

	"github.com/spf13/cobra"
)

var (
	jobRunNomadAddr  string
	jobRunRegistry   string
	jobRunDryRun     bool
)

var jobRunCmd = &cobra.Command{
	Use:   "run <jobfile>",
	Short: "Run a Nomad job file",
	Long: `Submit a Nomad job file to the cluster.

The job can be specified as:
  - Local file path (e.g., ./app.nomad.hcl)
  - Registry reference (e.g., namespace/jobname)

Examples:
  ramble job run ./app.nomad.hcl
  ramble job run user1/my-job
  ramble job run ./app.nomad.hcl --nomad-addr http://localhost:4646`,
	Args: cobra.ExactArgs(1),
	RunE: runJobRun,
}

func init() {
	jobCmd.AddCommand(jobRunCmd)

	jobRunCmd.Flags().StringVar(&jobRunNomadAddr, "nomad-addr", "", "Nomad address (overrides NOMAD_ADDR)")
	jobRunCmd.Flags().StringVarP(&jobRunRegistry, "registry", "r", "", "Registry URL (uses default if not specified)")
	jobRunCmd.Flags().BoolVar(&jobRunDryRun, "dry-run", false, "Print job content without submitting")
}

func runJobRun(cmd *cobra.Command, args []string) error {
	jobRef := args[0]

	var jobContent string

	// Check if it's a local file
	if _, err := os.Stat(jobRef); err == nil {
		// Local file
		content, err := os.ReadFile(jobRef)
		if err != nil {
			return fmt.Errorf("failed to read job file: %w", err)
		}
		jobContent = string(content)
	} else if strings.Contains(jobRef, "/") && !strings.HasPrefix(jobRef, "/") {
		// Registry reference: namespace/jobname
		parts := strings.SplitN(jobRef, "/", 2)
		namespace := parts[0]
		name := parts[1]

		// Handle version suffix
		version := ""
		if idx := strings.Index(name, "@"); idx != -1 {
			version = name[idx+1:]
			name = name[:idx]
		}

		registryURL := jobRunRegistry
		if registryURL == "" {
			cfg, _ := config.Load()
			registryURL = cfg.GetDefaultURL()
		}

		client := pack.NewClient(registryURL)
		content, err := client.GetRawContent(namespace, name, version)
		if err != nil {
			return fmt.Errorf("failed to fetch job from registry: %w", err)
		}
		jobContent = content
	} else {
		return fmt.Errorf("job file not found: %s", jobRef)
	}

	if jobRunDryRun {
		fmt.Println(jobContent)
		return nil
	}

	// Check nomad is available
	if err := nomad.CheckNomadAvailable(); err != nil {
		return err
	}

	// Submit to Nomad
	fmt.Println("Submitting job to Nomad...")
	return nomad.SubmitJob(jobContent, jobRunNomadAddr)
}
