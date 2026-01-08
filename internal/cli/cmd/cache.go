package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"rmbl/internal/pack"

	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the local pack cache",
	Long: `Commands for managing locally cached packs.

Examples:
  ramble cache list   List cached packs
  ramble cache clear  Remove all cached packs
  ramble cache path   Show cache directory path`,
}

var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cached packs",
	RunE: func(cmd *cobra.Command, args []string) error {
		cache := pack.NewCache()
		packs, err := cache.List()
		if err != nil {
			return fmt.Errorf("failed to list cache: %w", err)
		}

		if len(packs) == 0 {
			fmt.Println("No packs cached")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "REGISTRY\tNAMESPACE\tNAME\tVERSION")
		for _, p := range packs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Registry, p.Namespace, p.Name, p.Version)
		}
		return w.Flush()
	},
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove all cached packs",
	RunE: func(cmd *cobra.Command, args []string) error {
		cache := pack.NewCache()
		if err := cache.Clear(); err != nil {
			return fmt.Errorf("failed to clear cache: %w", err)
		}
		fmt.Println("Cache cleared")
		return nil
	},
}

var cachePathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show cache directory path",
	Run: func(cmd *cobra.Command, args []string) {
		cache := pack.NewCache()
		fmt.Println(cache.Dir)
	},
}

func init() {
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cachePathCmd)
}
