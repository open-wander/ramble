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

	"rmbl/internal/testutil/logtest"
)

// saveGlobals snapshots every global Init touches and restores them when the
// test ends. slog.SetDefault runs first: with a non-default handler it
// rewrites the stdlib log output, which is then restored explicitly.
func saveGlobals(t *testing.T) {
	t.Helper()
	prevLog := Log
	prevDefault := slog.Default()
	prevOut := stdlog.Writer()
	prevFlags := stdlog.Flags()
	t.Cleanup(func() {
		Log = prevLog
		slog.SetDefault(prevDefault)
		stdlog.SetOutput(prevOut)
		stdlog.SetFlags(prevFlags)
	})
}

// initBuf runs initWithWriter into a fresh buffer with globals restored after.
func initBuf(t *testing.T) *bytes.Buffer {
	t.Helper()
	saveGlobals(t)
	var buf bytes.Buffer
	initWithWriter(&buf)
	return &buf
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

			Log.Warn("test.event", slog.Any("error", tt.err), slog.Int("n", 1))

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

	var buf bytes.Buffer
	initWithWriter(&buf)

	Log.Warn("test.event", slog.Int("n", 1))
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

func TestInit_StdlibLogIsJSON(t *testing.T) {
	buf := initBuf(t)

	stdlog.Printf("legacy %s", "line")

	lines := logtest.Lines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %q", buf.String())
	}
	m := lines[0]
	if m["level"] != "INFO" || m["msg"] != "stdlib.log" || m["text"] != "legacy line" {
		t.Errorf("level/msg/text = %v %v %v", m["level"], m["msg"], m["text"])
	}
}

func TestInit_SlogDefault(t *testing.T) {
	buf := initBuf(t)

	slog.Info("x.y")

	lines := logtest.Lines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %q", buf.String())
	}
	if lines[0]["level"] != "INFO" || lines[0]["msg"] != "x.y" {
		t.Errorf("level/msg = %v %v", lines[0]["level"], lines[0]["msg"])
	}
}

func TestInit_Idempotent(t *testing.T) {
	saveGlobals(t)
	var buf bytes.Buffer
	initWithWriter(&buf)
	initWithWriter(&buf)

	Log.Info("a.b")

	if lines := logtest.Lines(t, &buf); len(lines) != 1 {
		t.Fatalf("want 1 line, got %q", buf.String())
	}
}

func TestInit_Levels(t *testing.T) {
	buf := initBuf(t)

	Log.Debug("a.debug")
	Log.Info("a.info")
	Log.Warn("a.warn")
	Log.Error("a.error")

	lines := logtest.Lines(t, buf)
	want := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	if len(lines) != len(want) {
		t.Fatalf("want %d lines, got %q", len(want), buf.String())
	}
	for i, w := range want {
		if lines[i]["level"] != w {
			t.Errorf("line %d level = %v, want %s", i, lines[i]["level"], w)
		}
	}
}

func TestLogWithFields(t *testing.T) {
	buf := initBuf(t)

	Log.With("k", "v").Info("a.b")

	lines := logtest.Lines(t, buf)
	if len(lines) != 1 || lines[0]["k"] != "v" {
		t.Fatalf("want one line carrying k=v, got %q", buf.String())
	}
}

func TestFromContext(t *testing.T) {
	buf := initBuf(t)

	if got := FromContext(context.Background()); got != Log {
		t.Error("FromContext without attached logger should return the global logger")
	}
	attached := Log.With("request_id", "r1")
	FromContext(WithContext(context.Background(), attached)).Info("x.y")
	lines := logtest.Lines(t, buf)
	if len(lines) != 1 || lines[0]["request_id"] != "r1" {
		t.Errorf("attached logger not used: %q", buf.String())
	}
	fb := slog.New(slog.DiscardHandler)
	var nilCtx context.Context
	if got := FromContextOr(nilCtx, fb); got != fb {
		t.Error("FromContextOr with nil ctx should return the fallback")
	}
}
