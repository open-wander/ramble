package cmd

import (
	"rmbl/internal/server"

	"github.com/spf13/cobra"
)

var (
	serverPort string
	serverSeed bool
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the Ramble web server",
	Long: `Start the Ramble web server to serve the pack registry.

The server provides:
  - Web UI for browsing packs and jobs
  - REST API for pack discovery (/v1/packs)
  - Webhook endpoints for automatic version updates

Environment variables:
  DATABASE_URL    PostgreSQL connection string
  SESSION_SECRET  Secret for session encryption
  ENV             Set to "production" for production mode`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return server.Run(server.Config{
			Port: serverPort,
			Seed: serverSeed,
		})
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().StringVarP(&serverPort, "port", "p", "3000", "Port to listen on")
	serverCmd.Flags().BoolVar(&serverSeed, "seed", false, "Seed initial user from environment variables")
}
