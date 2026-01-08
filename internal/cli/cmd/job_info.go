package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"rmbl/internal/cli/config"
	"rmbl/internal/pack"

	"github.com/spf13/cobra"
)

var (
	jobInfoRegistry string
	jobInfoJSON     bool
)

var jobInfoCmd = &cobra.Command{
	Use:   "info <namespace/job>",
	Short: "Get detailed information about a job",
	Long: `Display detailed information about a specific job including version history.

Examples:
  ramble job info user1/my-job
  ramble job info user1/my-job --json`,
	Args: cobra.ExactArgs(1),
	RunE: runJobInfo,
}

func init() {
	jobCmd.AddCommand(jobInfoCmd)

	jobInfoCmd.Flags().StringVarP(&jobInfoRegistry, "registry", "r", "", "Registry URL (uses default if not specified)")
	jobInfoCmd.Flags().BoolVar(&jobInfoJSON, "json", false, "Output as JSON")
}

func runJobInfo(cmd *cobra.Command, args []string) error {
	jobRef := args[0]
	namespace, name := parseJobRef(jobRef)

	if namespace == "" {
		return fmt.Errorf("namespace required: use namespace/jobname format")
	}

	registryURL := jobInfoRegistry
	if registryURL == "" {
		cfg, _ := config.Load()
		registryURL = cfg.GetDefaultURL()
	}

	client := pack.NewClient(registryURL)

	detail, err := client.GetJob(namespace, name)
	if err != nil {
		return fmt.Errorf("failed to get job info: %w", err)
	}

	if jobInfoJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(detail)
	}

	fmt.Printf("Name:        %s\n", detail.Name)
	fmt.Printf("Description: %s\n", detail.Description)
	fmt.Printf("Versions:    %d\n", len(detail.Versions))
	fmt.Println()

	if len(detail.Versions) > 0 {
		fmt.Println("Version History:")
		for _, v := range detail.Versions {
			fmt.Printf("  - %s\n", v.Version)
		}
	}

	return nil
}

// parseJobRef parses "namespace/name" into components
func parseJobRef(ref string) (namespace, name string) {
	// Handle version suffix
	ref = strings.Split(ref, "@")[0]

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}
