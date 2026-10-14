package logger

import (
	"bytes"
	"context"
	"io"
	stdlog "log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Log is the global logger instance
var Log zerolog.Logger

// S is the slog logger that replaces Log in phase 11 (renamed Log in 11.5).
// Until Init runs it discards, so callers never see a nil logger.
var S = slog.New(slog.DiscardHandler)

// sloggerKey keys the request-scoped *slog.Logger in a context.
type sloggerKey struct{}

func upperLevelMarshal(l zerolog.Level) string { return strings.ToUpper(l.String()) }

// Init initializes the structured logger. In production every line is one
// JSON object on stdout with time, level (upper case) and msg, in every
// environment.
func Init() { initWithWriter(os.Stdout) }

// InitWithWriter is Init with an injectable output, for tests in other packages.
func InitWithWriter(w io.Writer) { initWithWriter(w) }

// initWithWriter is Init with an injectable output, for tests.
func initWithWriter(w io.Writer) {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.MessageFieldName = "msg"

	zerolog.LevelFieldMarshalFunc = upperLevelMarshal
	Log = zerolog.New(w).With().Timestamp().Logger()

	S = NewJSON(w)

	// Set as default logger
	log.Logger = Log

	// Route any stdlib log output (ours or a dependency's) through zerolog so
	// it is JSON in production too.
	stdlog.SetFlags(0)
	stdlog.SetOutput(stdlibWriter{})
}

// NewJSON returns the production slog logger writing to w. Debug stays
// enabled, as it was for zerolog. No AddSource: it would add a key the output
// contract does not have. Tests that need exact output build their logger
// with it.
func NewJSON(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: replaceAttr,
	}))
}

// replaceAttr keeps errors strings even for error types that implement
// json.Marshaler, which the JSON handler would otherwise write as objects.
func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindAny {
		if err, ok := a.Value.Any().(error); ok {
			return slog.String(a.Key, err.Error())
		}
	}
	return a
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

// SWithContext returns ctx carrying l (renamed WithContext in 11.5).
func SWithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, sloggerKey{}, l)
}

// SFromContext returns the slog logger attached to ctx (for example the
// request-scoped one carrying request_id), or S (renamed FromContext in 11.5).
func SFromContext(ctx context.Context) *slog.Logger { return SFromContextOr(ctx, S) }

// SFromContextOr returns the slog logger attached to ctx, or fallback when
// there is none (renamed FromContextOr in 11.5).
func SFromContextOr(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(sloggerKey{}).(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return fallback
}

// Err returns the standard error attribute: key "error" with the message as a
// string, whatever the handler options. A nil error returns the zero Attr,
// which handlers drop.
func Err(err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}
	return slog.String("error", err.Error())
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
