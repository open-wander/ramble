package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Log is the global logger instance
var Log zerolog.Logger

// Init initializes the structured logger
func Init() {
	zerolog.TimeFieldFormat = time.RFC3339

	if os.Getenv("ENV") != "production" {
		// Pretty console output in development
		Log = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}).With().Timestamp().Caller().Logger()
	} else {
		// JSON output in production
		Log = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	// Set as default logger
	log.Logger = Log
}

// WithUser creates a logger with user context
func WithUser(userID uint, username string) zerolog.Logger {
	return Log.With().Uint("user_id", userID).Str("username", username).Logger()
}

// WithRequest creates a logger with request context
func WithRequest(requestID string, method string, path string) zerolog.Logger {
	return Log.With().
		Str("request_id", requestID).
		Str("method", method).
		Str("path", path).
		Logger()
}
