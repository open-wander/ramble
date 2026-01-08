package cmd

import (
	"fmt"
	"os"
	"strings"

	"rmbl/internal/cli/config"
	"rmbl/internal/nomad"
	"rmbl/internal/pack"
	"rmbl/internal/render"

	"github.com/spf13/cobra"
)

var (
	runRegistry string
	runVars     []string
	runVarFile  string
	runDryRun   bool
	runOutput   string
	runNomadAddr string
)

var packRunCmd = &cobra.Command{
	Use:   "run <pack>",
	Short: "Download and run a pack from the registry",
	Long: `Download a pack from the Ramble registry, render its templates, and submit to Nomad.

The pack can be specified as:
  - namespace/packname[@version] (from registry)
  - Local path (if starts with ./ or /)

Examples:
  ramble pack run user1/mysql
  ramble pack run user1/mysql@v1.0.0
  ramble pack run user1/mysql --var count=3 --var db_name=mydb
  ramble pack run ./my-local-pack
  ramble pack run user1/mysql --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runPackRun,
}

func init() {
	packCmd.AddCommand(packRunCmd)

	packRunCmd.Flags().StringVarP(&runRegistry, "registry", "r", "", "Registry URL (uses default if not specified)")
	packRunCmd.Flags().StringArrayVarP(&runVars, "var", "v", nil, "Variable override (key=value)")
	packRunCmd.Flags().StringVar(&runVarFile, "var-file", "", "Variable file (JSON)")
	packRunCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "Render and print without submitting to Nomad")
	packRunCmd.Flags().StringVarP(&runOutput, "output", "o", "", "Write rendered job to file instead of submitting")
	packRunCmd.Flags().StringVar(&runNomadAddr, "nomad-addr", "", "Nomad address (overrides NOMAD_ADDR)")
}

func runPackRun(cmd *cobra.Command, args []string) error {
	packRef := args[0]

	// Check if it's a local path
	isLocal := strings.HasPrefix(packRef, "./") || strings.HasPrefix(packRef, "/")

	var packPath string
	var metadata *packMetadata
	var err error

	if isLocal {
		// Local pack
		packPath = packRef
		if _, err := os.Stat(packPath); os.IsNotExist(err) {
			return fmt.Errorf("pack path does not exist: %s", packPath)
		}
	} else {
		// Remote pack from registry
		namespace, name, version := parsePackReference(packRef)
		if namespace == "" {
			return fmt.Errorf("namespace required: use namespace/packname format")
		}

		registryURL := runRegistry
		if registryURL == "" {
			cfg, _ := config.Load()
			registryURL = cfg.GetDefaultURL()
		}

		cache := pack.NewCache()
		client := pack.NewClient(registryURL)

		// Check if version specified
		if version == "" {
			// Get latest version from registry
			detail, err := client.GetPack(namespace, name)
			if err != nil {
				return fmt.Errorf("failed to get pack info: %w", err)
			}
			if len(detail.Versions) == 0 {
				return fmt.Errorf("no versions available for pack: %s/%s", namespace, name)
			}
			version = detail.Versions[0].Version
			fmt.Printf("Using latest version: %s\n", version)
		}

		// Check cache
		if cache.IsCached(registryURL, namespace, name, version) {
			packPath, _ = cache.Load(registryURL, namespace, name, version)
			fmt.Printf("Using cached pack: %s\n", packPath)
		} else {
			// Get pack detail for download URL
			detail, err := client.GetPack(namespace, name)
			if err != nil {
				return fmt.Errorf("failed to get pack info: %w", err)
			}

			// Find version
			var downloadURL string
			for _, v := range detail.Versions {
				if v.Version == version {
					downloadURL = v.URL
					break
				}
			}
			if downloadURL == "" {
				return fmt.Errorf("version not found: %s", version)
			}

			fmt.Printf("Downloading pack %s/%s@%s...\n", namespace, name, version)
			packPath, err = cache.Store(registryURL, namespace, name, version, downloadURL)
			if err != nil {
				return fmt.Errorf("failed to download pack: %w", err)
			}
			fmt.Printf("Cached to: %s\n", packPath)
		}
	}

	// Load pack metadata
	metadata, err = loadPackMetadata(packPath)
	if err != nil {
		metadata = &packMetadata{}
	}

	// Load variables defaults from variables.hcl
	variables := make(map[string]any)
	varsPath := packPath + "/variables.hcl"
	if _, err := os.Stat(varsPath); err == nil {
		content, err := os.ReadFile(varsPath)
		if err == nil {
			if defs, err := parseVariablesSimple(string(content)); err == nil {
				for k, v := range defs {
					variables[k] = v
				}
			}
		}
	}

	// Load var file if specified
	if runVarFile != "" {
		fileVars, err := pack.ParseVarFile(runVarFile)
		if err != nil {
			return fmt.Errorf("failed to load var file: %w", err)
		}
		for k, v := range fileVars {
			variables[k] = v
		}
	}

	// Parse CLI var flags
	for _, v := range runVars {
		key, val, err := pack.ParseVarFlag(v)
		if err != nil {
			return err
		}
		variables[key] = val
	}

	// Create render engine
	engine := render.NewEngine()
	engine.SetPackMetadata(metadata.Name, metadata.Description, metadata.Version)
	engine.SetVariables(variables)

	// Render the pack
	result, err := engine.RenderPack(packPath)
	if err != nil {
		return fmt.Errorf("failed to render pack: %w", err)
	}

	// Handle output modes
	if runOutput != "" {
		if err := os.WriteFile(runOutput, []byte(result), 0644); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
		fmt.Printf("Rendered job written to: %s\n", runOutput)
		return nil
	}

	if runDryRun {
		fmt.Println(result)
		return nil
	}

	// Check nomad is available
	if err := nomad.CheckNomadAvailable(); err != nil {
		return err
	}

	// Submit to Nomad
	fmt.Println("Submitting job to Nomad...")
	return nomad.SubmitJob(result, runNomadAddr)
}

// parsePackReference parses "namespace/name@version" into components
func parsePackReference(ref string) (namespace, name, version string) {
	// Handle version suffix
	if idx := strings.Index(ref, "@"); idx != -1 {
		version = ref[idx+1:]
		ref = ref[:idx]
	}

	// Handle namespace
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 {
		namespace = parts[0]
		name = parts[1]
	} else {
		name = parts[0]
	}

	return
}
