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
	listRegistry  string
	listNamespace string
	listJSON      bool
	listSearch    string
)

var packListCmd = &cobra.Command{
	Use:   "list",
	Short: "List packs from the registry",
	Long: `List all available Nomad packs from the registry.

Examples:
  ramble pack list                          List all packs
  ramble pack list --namespace user1        List packs from a specific namespace
  ramble pack list --search mysql           Search for packs
  ramble pack list --json                   Output as JSON`,
	RunE: runPackList,
}

func init() {
	packCmd.AddCommand(packListCmd)

	packListCmd.Flags().StringVarP(&listRegistry, "registry", "r", "", "Registry URL (uses default if not specified)")
	packListCmd.Flags().StringVarP(&listNamespace, "namespace", "n", "", "Filter by namespace (user or org)")
	packListCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
	packListCmd.Flags().StringVarP(&listSearch, "search", "s", "", "Search packs by name or description")
}

func runPackList(cmd *cobra.Command, args []string) error {
	registryURL := listRegistry
	if registryURL == "" {
		cfg, _ := config.Load()
		registryURL = cfg.GetDefaultURL()
	}

	client := pack.NewClient(registryURL)

	var packs []pack.PackSummary
	var err error

	if listSearch != "" {
		packs, err = client.SearchPacks(listSearch)
	} else if listNamespace != "" {
		packs, err = client.ListPacks(listNamespace)
	} else {
		packs, err = client.ListAllPacks()
	}

	if err != nil {
		return fmt.Errorf("failed to list packs: %w", err)
	}

	if listJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(packs)
	}

	if len(packs) == 0 {
		fmt.Println("No packs found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION")
	for _, p := range packs {
		desc := p.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		fullName := p.Name
		if p.Namespace != "" {
			fullName = p.Namespace + "/" + p.Name
		}
		fmt.Fprintf(w, "%s\t%s\n", fullName, desc)
	}
	return w.Flush()
}
