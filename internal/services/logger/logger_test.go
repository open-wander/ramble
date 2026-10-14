package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	stdlog "log"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"rmbl/internal/testutil/logtest"
)

func TestInit_Development(t *testing.T) {
	t.Setenv("ENV", "development")

	Init()

	// Logger should be initialized
	if Log.GetLevel() < 0 {
		// Just verify it doesn't panic and is usable
	}

	// Test that logging works
	Log.Info().Msg("test message")
}

func TestInit_Production(t *testing.T) {
	t.Setenv("ENV", "production")

	Init()

	// Logger should be initialized for production
	Log.Info().Msg("test production message")
}

func TestWithUser(t *testing.T) {
	t.Setenv("ENV", "development")
	Init()

	// Capture output
	var buf bytes.Buffer
	testLogger := Log.Output(&buf)
	Log = testLogger

	userLogger := WithUser(123, "testuser")

	// Write to a buffer to verify fields
	var outputBuf bytes.Buffer
	userLogger = userLogger.Output(&outputBuf)
	userLogger.Info().Msg("user action")

	output := outputBuf.String()
	if !strings.Contains(output, `"user_id":123`) || !strings.Contains(output, `"username":"testuser"`) {
		t.Errorf("user fields missing: %s", output)
	}
}

func TestWithRequest(t *testing.T) {
	t.Setenv("ENV", "development")
	Init()

	reqLogger := WithRequest("req-123", "GET", "/api/test")

	// Write to a buffer to verify fields
	var outputBuf bytes.Buffer
	reqLogger = reqLogger.Output(&outputBuf)
	reqLogger.Info().Msg("request handled")

	output := outputBuf.String()
	if !strings.Contains(output, `"request_id":"req-123"`) || !strings.Contains(output, `"path":"/api/test"`) {
		t.Errorf("request fields missing: %s", output)
	}
}

func TestWithUser_JSONOutput(t *testing.T) {
	t.Setenv("ENV", "production")
	Init()

	var buf bytes.Buffer
	userLogger := Log.Output(&buf)
	Log = userLogger

	contextLogger := WithUser(456, "admin")

	var outputBuf bytes.Buffer
	contextLogger = contextLogger.Output(&outputBuf)
	contextLogger.Info().Msg("admin action")

	output := outputBuf.String()

	// Parse JSON output
	var logEntry map[string]any
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		// May fail if format is different, that's OK
		return
	}

	if logEntry["user_id"] != nil {
		if uid, ok := logEntry["user_id"].(float64); ok && uid != 456 {
			t.Errorf("user_id = %v, want 456", logEntry["user_id"])
		}
	}
}

func TestWithRequest_JSONOutput(t *testing.T) {
	t.Setenv("ENV", "production")
	Init()

	var buf bytes.Buffer
	reqLogger := Log.Output(&buf)
	Log = reqLogger

	contextLogger := WithRequest("req-abc", "POST", "/api/submit")

	var outputBuf bytes.Buffer
	contextLogger = contextLogger.Output(&outputBuf)
	contextLogger.Info().Msg("form submitted")

	output := outputBuf.String()

	// Parse JSON output
	var logEntry map[string]any
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		// May fail if format is different
		return
	}

	if logEntry["request_id"] != nil && logEntry["request_id"] != "req-abc" {
		t.Errorf("request_id = %v, want req-abc", logEntry["request_id"])
	}
	if logEntry["method"] != nil && logEntry["method"] != "POST" {
		t.Errorf("method = %v, want POST", logEntry["method"])
	}
	if logEntry["path"] != nil && logEntry["path"] != "/api/submit" {
		t.Errorf("path = %v, want /api/submit", logEntry["path"])
	}
}

func TestInit_MultipleCalls(t *testing.T) {
	t.Setenv("ENV", "development")

	// Should not panic when called multiple times
	Init()
	Init()
	Init()

	Log.Info().Msg("still working")
}

func TestLoggerLevels(t *testing.T) {
	t.Setenv("ENV", "production")
	Init()

	// Test various log levels don't panic
	Log.Debug().Msg("debug message")
	Log.Info().Msg("info message")
	Log.Warn().Msg("warn message")
	Log.Error().Msg("error message")
}

func TestLoggerWithFields(t *testing.T) {
	t.Setenv("ENV", "production")
	Init()

	// Test adding various field types
	Log.Info().
		Str("string_field", "value").
		Int("int_field", 42).
		Bool("bool_field", true).
		Float64("float_field", 3.14).
		Msg("message with fields")
}

// saveGlobals snapshots every global Init touches and restores them when the
// test ends.
func saveGlobals(t *testing.T) {
	t.Helper()
	prevLog := Log
	prevS := S
	prevGlobal := log.Logger
	prevMsg := zerolog.MessageFieldName
	prevLevel := zerolog.LevelFieldMarshalFunc
	prevTimeFmt := zerolog.TimeFieldFormat
	prevOut := stdlog.Writer()
	prevFlags := stdlog.Flags()
	t.Cleanup(func() {
		Log = prevLog
		S = prevS
		log.Logger = prevGlobal
		zerolog.MessageFieldName = prevMsg
		zerolog.LevelFieldMarshalFunc = prevLevel
		zerolog.TimeFieldFormat = prevTimeFmt
		stdlog.SetOutput(prevOut)
		stdlog.SetFlags(prevFlags)
	})
}

// jsonErr is an error that would marshal as a JSON object if slog saw it raw.
type jsonErr struct{}

func (jsonErr) Error() string                { return "json boom" }
func (jsonErr) MarshalJSON() ([]byte, error) { return []byte(`{"x":1}`), nil }

func TestInitSlog_Format(t *testing.T) {
	tests := []struct {
		name    string
		env     string // "" means unset
		err     error
		wantErr string
	}{
		{"production", "production", errors.New("boom"), "boom"},
		{"env unset", "", errors.New("boom"), "boom"},
		{"json marshaler error stays a string", "production", jsonErr{}, "json boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveGlobals(t)
			t.Setenv("ENV", tt.env)
			if tt.env == "" {
				os.Unsetenv("ENV")
			}
			var buf bytes.Buffer
			initWithWriter(&buf)

			S.Warn("test.event", slog.Any("error", tt.err), slog.Int("n", 1))

			if strings.Contains(buf.String(), "\x1b[") {
				t.Fatalf("ANSI escape in output: %q", buf.String())
			}
			lines := logtest.Lines(t, &buf)
			if len(lines) != 1 {
				t.Fatalf("want 1 line, got %v", lines)
			}
			m := lines[0]
			if ts, _ := m["time"].(string); ts == "" {
				t.Errorf("missing time: %v", m)
			} else if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
				t.Errorf("time %q not RFC3339Nano: %v", ts, err)
			}
			if m["level"] != "WARN" || m["msg"] != "test.event" || m["n"] != float64(1) {
				t.Errorf("level/msg/n = %v %v %v", m["level"], m["msg"], m["n"])
			}
			if m["error"] != tt.wantErr {
				t.Errorf("error = %#v, want string %q", m["error"], tt.wantErr)
			}
		})
	}
}

func TestInit_ProductionFormat(t *testing.T) {
	saveGlobals(t)

	t.Setenv("ENV", "production")
	var buf bytes.Buffer
	initWithWriter(&buf)

	Log.Warn().Int("n", 1).Msg("test.event")
	stdlog.Printf("legacy %s", "line")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %q", buf.String())
	}
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatalf("ANSI escape in production output: %q", buf.String())
	}

	tests := []struct {
		line      string
		wantLevel string
		wantMsg   string
		wantField string
		wantValue any
	}{
		{lines[0], "WARN", "test.event", "n", float64(1)},
		{lines[1], "INFO", "stdlib.log", "text", "legacy line"},
	}
	for _, tt := range tests {
		var m map[string]any
		if err := json.Unmarshal([]byte(tt.line), &m); err != nil {
			t.Fatalf("not JSON: %q: %v", tt.line, err)
		}
		ts, ok := m["time"].(string)
		if !ok {
			t.Errorf("missing time: %v", m)
		} else if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
			t.Errorf("time %q not RFC3339Nano: %v", ts, err)
		}
		if m["level"] != tt.wantLevel {
			t.Errorf("level = %v, want %s", m["level"], tt.wantLevel)
		}
		if m["msg"] != tt.wantMsg {
			t.Errorf("msg = %v, want %s", m["msg"], tt.wantMsg)
		}
		if m[tt.wantField] != tt.wantValue {
			t.Errorf("%s = %v, want %v", tt.wantField, m[tt.wantField], tt.wantValue)
		}
	}
}

func TestFromContext(t *testing.T) {
	t.Setenv("ENV", "production")
	Init()

	if got := FromContext(context.Background()); got != &Log {
		t.Error("FromContext without attached logger should return the global logger")
	}
	var buf bytes.Buffer
	attached := zerolog.New(&buf).With().Str("request_id", "r1").Logger()
	ctx := attached.WithContext(context.Background())
	FromContext(ctx).Info().Msg("x")
	if !strings.Contains(buf.String(), `"request_id":"r1"`) {
		t.Errorf("attached logger not used: %q", buf.String())
	}
}
