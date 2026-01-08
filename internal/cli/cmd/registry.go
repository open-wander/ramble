package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"rmbl/internal/cli/config"
	"rmbl/internal/pack"

	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage pack registries",
	Long: `Commands for managing saved registry configurations.

Registries can be:
  - The full Ramble registry (all namespaces)
  - A specific user/org namespace within Ramble
  - A self-hosted Ramble instance

Examples:
  ramble registry add ramble https://ramble.openwander.org
  ramble registry add myteam https://ramble.openwander.org --namespace myteam
  ramble registry list
  ramble registry default myteam`,
}

var registryAddCmd = &cobra.Command{
	Use:   "add <name> <url>",
	Short: "Add a registry",
	Long: `Add a new registry configuration.

Examples:
  # Add the main Ramble registry
  ramble registry add ramble https://ramble.openwander.org

  # Add a specific namespace within Ramble
  ramble registry add myteam https://ramble.openwander.org --namespace myteam

  # Add a self-hosted instance
  ramble registry add internal https://ramble.internal.company.com`,
	Args: cobra.ExactArgs(2),
	RunE: runRegistryAdd,
}

var registryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured registries",
	RunE:  runRegistryList,
}

var registryRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a registry",
	Args:  cobra.ExactArgs(1),
	RunE:  runRegistryRemove,
}

var registryDefaultCmd = &cobra.Command{
	Use:   "default [name]",
	Short: "Get or set the default registry",
	Long: `Get or set the default registry.

Without arguments, shows the current default.
With a name argument, sets that registry as the default.

Examples:
  ramble registry default           # Show current default
  ramble registry default myteam    # Set myteam as default`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRegistryDefault,
}

var registryBrowseCmd = &cobra.Command{
	Use:   "browse",
	Short: "Browse available namespaces on the registry",
	Long: `List or search namespaces (users/organizations) that have published packs.

Examples:
  ramble registry browse                  List all namespaces
  ramble registry browse --search wander  Search for namespaces`,
	RunE: runRegistryBrowse,
}

var (
	registryNamespace    string
	registryBrowseSearch string
)

func init() {
	rootCmd.AddCommand(registryCmd)
	registryCmd.AddCommand(registryAddCmd)
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryRemoveCmd)
	registryCmd.AddCommand(registryDefaultCmd)
	registryCmd.AddCommand(registryBrowseCmd)

	registryAddCmd.Flags().StringVarP(&registryNamespace, "namespace", "n", "", "Limit registry to a specific namespace")
	registryBrowseCmd.Flags().StringVarP(&registryBrowseSearch, "search", "s", "", "Search namespaces by name")
}

func runRegistryAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	url := args[1]

	// Normalize URL
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	url = strings.TrimSuffix(url, "/")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	cfg.AddRegistry(name, url, registryNamespace)

	if err := cfg.Save(); err != nil {
		return err
	}

	if registryNamespace != "" {
		fmt.Printf("Added registry '%s': %s (namespace: %s)\n", name, url, registryNamespace)
	} else {
		fmt.Printf("Added registry '%s': %s\n", name, url)
	}

	// Set as default if it's the first registry
	if len(cfg.Registries) == 1 || cfg.DefaultRegistry == "" {
		cfg.DefaultRegistry = name
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Printf("Set '%s' as default registry\n", name)
	}

	return nil
}

func runRegistryList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if len(cfg.Registries) == 0 {
		fmt.Println("No registries configured")
		fmt.Println("\nAdd one with: ramble registry add <name> <url>")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tURL\tNAMESPACE\tDEFAULT")
	for name, reg := range cfg.Registries {
		isDefault := ""
		if name == cfg.DefaultRegistry {
			isDefault = "*"
		}
		namespace := reg.Namespace
		if namespace == "" {
			namespace = "(all)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, reg.URL, namespace, isDefault)
	}
	return w.Flush()
}

func runRegistryRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if err := cfg.RemoveRegistry(name); err != nil {
		return err
	}

	if err := cfg.Save(); err != nil {
		return err
	}

	fmt.Printf("Removed registry '%s'\n", name)
	return nil
}

func runRegistryDefault(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		// Show current default
		if cfg.DefaultRegistry == "" {
			fmt.Println("No default registry set")
		} else {
			reg, _ := cfg.GetRegistry(cfg.DefaultRegistry)
			if reg != nil {
				fmt.Printf("%s (%s)\n", cfg.DefaultRegistry, reg.URL)
			} else {
				fmt.Println(cfg.DefaultRegistry)
			}
		}
		return nil
	}

	// Set new default
	name := args[0]
	if err := cfg.SetDefault(name); err != nil {
		return err
	}

	if err := cfg.Save(); err != nil {
		return err
	}

	fmt.Printf("Default registry set to '%s'\n", name)
	return nil
}

func runRegistryBrowse(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	registryURL := cfg.GetDefaultURL()
	client := pack.NewClient(registryURL)

	var namespaces []string
	if registryBrowseSearch != "" {
		namespaces, err = client.SearchRegistries(registryBrowseSearch)
	} else {
		namespaces, err = client.ListRegistries()
	}

	if err != nil {
		return fmt.Errorf("failed to browse registries: %w", err)
	}

	if len(namespaces) == 0 {
		if registryBrowseSearch != "" {
			fmt.Printf("No namespaces found matching '%s'\n", registryBrowseSearch)
		} else {
			fmt.Println("No namespaces found")
		}
		return nil
	}

	fmt.Println("NAMESPACE")
	for _, ns := range namespaces {
		fmt.Println(ns)
	}

	return nil
}
