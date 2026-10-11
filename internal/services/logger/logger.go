package logger

import (
	"bytes"
	"context"
	"io"
	stdlog "log"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Log is the global logger instance
var Log zerolog.Logger

func defaultLevelMarshal(l zerolog.Level) string { return l.String() }

func upperLevelMarshal(l zerolog.Level) string { return strings.ToUpper(l.String()) }

// Init initializes the structured logger. In production every line is one
// JSON object on stdout with time, level (upper case) and msg; elsewhere a
// console writer is used.
func Init() { initWithWriter(os.Stdout) }

// initWithWriter is Init with an injectable output, for tests.
func initWithWriter(w io.Writer) {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.MessageFieldName = "msg"

	if os.Getenv("ENV") != "production" {
		// Pretty console output in development
		zerolog.LevelFieldMarshalFunc = defaultLevelMarshal
		Log = zerolog.New(zerolog.ConsoleWriter{
			Out:        w,
			TimeFormat: time.RFC3339,
		}).With().Timestamp().Caller().Logger()
	} else {
		// JSON output in production
		zerolog.LevelFieldMarshalFunc = upperLevelMarshal
		Log = zerolog.New(w).With().Timestamp().Logger()
	}

	// Set as default logger
	log.Logger = Log

	// Route any stdlib log output (ours or a dependency's) through zerolog so
	// it is JSON in production too.
	stdlog.SetFlags(0)
	stdlog.SetOutput(stdlibWriter{})
}

// stdlibWriter adapts the stdlib log package to the global zerolog logger.
// The text is variable, so it goes in a field under a stable msg key.
type stdlibWriter struct{}

func (stdlibWriter) Write(p []byte) (int, error) {
	Log.Info().Str("text", string(bytes.TrimRight(p, "\n"))).Msg("stdlib.log")
	return len(p), nil
}

// FromContext returns the logger attached to ctx (for example the
// request-scoped logger carrying request_id), or the global logger.
func FromContext(ctx context.Context) *zerolog.Logger {
	return FromContextOr(ctx, &Log)
}

// FromContextOr returns the logger attached to ctx, or fallback when there
// is none.
func FromContextOr(ctx context.Context, fallback *zerolog.Logger) *zerolog.Logger {
	if ctx != nil {
		if l := zerolog.Ctx(ctx); l.GetLevel() != zerolog.Disabled {
			return l
		}
	}
	return fallback
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
