package cmd

import (
	"rmbl/internal/server"
	"rmbl/internal/services/logger"

	"github.com/spf13/cobra"
)

// runServer is a seam so tests can avoid starting a real server.
var runServer = server.Run

// initLogger is a seam so tests can redirect the logger output.
var initLogger = logger.Init

// serverFlagError logs a flag parse error as JSON. Flag errors never reach
// RunE, so the logger is not initialised yet.
func serverFlagError(_ *cobra.Command, err error) error {
	initLogger()
	logger.Log.Error("server.flag_error", logger.Err(err))
	return loggedError{err}
}

// loggedError marks an error already written to the JSON log, so Execute
// does not print it again in plain text.
type loggedError struct{ err error }

func (e loggedError) Error() string { return e.err.Error() }
func (e loggedError) Unwrap() error { return e.err }

// runServerCmd runs the server and logs a failure once as JSON. The log call
// must happen after run returns: run initialises the logger.
func runServerCmd(run func(server.Config) error, cfg server.Config) error {
	if err := run(cfg); err != nil {
		logger.Log.Error("server.exit_error", logger.Err(err))
		return loggedError{err}
	}
	return nil
}

var (
	serverPort string
	serverSeed bool
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the Ramble web server",
	// The server logs its own failures as JSON; cobra's plain-text error and
	// usage output would break that.
	SilenceErrors: true,
	SilenceUsage:  true,
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
		return runServerCmd(runServer, server.Config{
			Port:    serverPort,
			Seed:    serverSeed,
			Version: Version,
		})
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.SetFlagErrorFunc(serverFlagError)

	serverCmd.Flags().StringVarP(&serverPort, "port", "p", "3000", "Port to listen on")
	serverCmd.Flags().BoolVar(&serverSeed, "seed", false, "Seed initial user from environment variables")
}
