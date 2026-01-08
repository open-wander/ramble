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
	infoRegistry string
	infoJSON     bool
)

var packInfoCmd = &cobra.Command{
	Use:   "info <namespace/pack>",
	Short: "Get detailed information about a pack",
	Long: `Display detailed information about a specific pack including version history.

The pack can be specified as:
  - namespace/packname (e.g., user1/mysql)
  - Just packname if --namespace is provided

Examples:
  ramble pack info user1/mysql
  ramble pack info mysql --namespace user1
  ramble pack info user1/mysql --json`,
	Args: cobra.ExactArgs(1),
	RunE: runPackInfo,
}

func init() {
	packCmd.AddCommand(packInfoCmd)

	packInfoCmd.Flags().StringVarP(&infoRegistry, "registry", "r", "", "Registry URL (uses default if not specified)")
	packInfoCmd.Flags().BoolVar(&infoJSON, "json", false, "Output as JSON")
}

func runPackInfo(cmd *cobra.Command, args []string) error {
	packRef := args[0]
	namespace, name := parsePackRef(packRef)

	if namespace == "" {
		return fmt.Errorf("namespace required: use namespace/packname format")
	}

	registryURL := infoRegistry
	if registryURL == "" {
		cfg, _ := config.Load()
		registryURL = cfg.GetDefaultURL()
	}

	client := pack.NewClient(registryURL)

	detail, err := client.GetPack(namespace, name)
	if err != nil {
		return fmt.Errorf("failed to get pack info: %w", err)
	}

	if infoJSON {
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

// parsePackRef parses "namespace/name" or just "name" into components
func parsePackRef(ref string) (namespace, name string) {
	// Handle version suffix first (namespace/name@version)
	ref = strings.Split(ref, "@")[0]

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}
