package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"rmbl/internal/cli/config"
	"rmbl/internal/pack"

	"github.com/spf13/cobra"
)

var (
	jobListRegistry string
	jobListJSON     bool
	jobListSearch   string
)

var jobListCmd = &cobra.Command{
	Use:   "list",
	Short: "List jobs from the registry",
	Long: `List all available Nomad job files from the registry.

Examples:
  ramble job list                    List all jobs
  ramble job list --search postgres  Search for jobs
  ramble job list --json             Output as JSON`,
	RunE: runJobList,
}

func init() {
	jobCmd.AddCommand(jobListCmd)

	jobListCmd.Flags().StringVarP(&jobListRegistry, "registry", "r", "", "Registry URL (uses default if not specified)")
	jobListCmd.Flags().BoolVar(&jobListJSON, "json", false, "Output as JSON")
	jobListCmd.Flags().StringVarP(&jobListSearch, "search", "s", "", "Search jobs by name or description")
}

func runJobList(cmd *cobra.Command, args []string) error {
	registryURL := jobListRegistry
	if registryURL == "" {
		cfg, _ := config.Load()
		registryURL = cfg.GetDefaultURL()
	}

	client := pack.NewClient(registryURL)

	var jobs []pack.JobSummary
	var err error

	if jobListSearch != "" {
		jobs, err = client.SearchJobs(jobListSearch)
	} else {
		jobs, err = client.ListAllJobs()
	}

	if err != nil {
		return fmt.Errorf("failed to list jobs: %w", err)
	}

	if jobListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jobs)
	}

	if len(jobs) == 0 {
		fmt.Println("No jobs found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION")
	for _, j := range jobs {
		desc := j.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\n", j.Name, desc)
	}
	return w.Flush()
}
