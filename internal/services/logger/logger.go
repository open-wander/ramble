package logger

import (
	"bytes"
	"context"
	"io"
	stdlog "log"
	"log/slog"
	"os"
)

// Log is the global logger. Until Init runs it discards, so callers never see
// a nil logger.
var Log = slog.New(slog.DiscardHandler)

// ctxKey keys the request-scoped *slog.Logger in a context.
type ctxKey struct{}

// Init initializes the structured logger. In production every line is one
// JSON object on stdout with time, level (upper case) and msg, in every
// environment.
func Init() { initWithWriter(os.Stdout) }

// InitWithWriter is Init with an injectable output, for tests in other packages.
func InitWithWriter(w io.Writer) { initWithWriter(w) }

// initWithWriter is Init with an injectable output, for tests.
func initWithWriter(w io.Writer) {
	Log = NewJSON(w)

	// Dependencies that call the slog package functions get the same handler.
	slog.SetDefault(Log)

	// Route any stdlib log output (ours or a dependency's) through slog so it
	// is JSON too. This must follow SetDefault: SetDefault points the stdlib
	// log at the handler with the raw text as msg, which would break the
	// dotted-key contract.
	stdlog.SetFlags(0)
	stdlog.SetOutput(stdlibWriter{})
}

// NewJSON returns the production slog logger writing to w. Debug stays
// enabled. No AddSource: it would add a key the output
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

// stdlibWriter adapts the stdlib log package to the global logger.
// The text is variable, so it goes in a field under a stable msg key.
type stdlibWriter struct{}

func (stdlibWriter) Write(p []byte) (int, error) {
	Log.Info("stdlib.log", slog.String("text", string(bytes.TrimRight(p, "\n"))))
	return len(p), nil
}

// WithContext returns ctx carrying l.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the logger attached to ctx (for example the
// request-scoped one carrying request_id), or Log.
func FromContext(ctx context.Context) *slog.Logger { return FromContextOr(ctx, Log) }

// FromContextOr returns the logger attached to ctx, or fallback when there
// is none.
func FromContextOr(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
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
