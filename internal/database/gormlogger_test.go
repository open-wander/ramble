package database

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"rmbl/internal/models"
	"rmbl/internal/services/logger"
)

func bufLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestGormLoggerTrace(t *testing.T) {
	const threshold = 200 * time.Millisecond
	sql := "SELECT * FROM users WHERE id = $1"

	tests := []struct {
		name      string
		elapsed   time.Duration
		err       error
		wantMsg   string // empty means no line at all
		wantLevel string
	}{
		{name: "fast query is silent", elapsed: time.Millisecond},
		{name: "record not found is silent", elapsed: time.Millisecond, err: gorm.ErrRecordNotFound},
		// A miss is not an error, but a slow miss is still a slow query.
		{name: "slow record not found warns as slow", elapsed: time.Second, err: gorm.ErrRecordNotFound, wantMsg: "db.slow_query", wantLevel: "WARN"},
		{name: "wrapped record not found is silent", elapsed: time.Millisecond, err: errors.Join(errors.New("ctx"), gorm.ErrRecordNotFound)},
		{name: "slow query warns", elapsed: 350 * time.Millisecond, wantMsg: "db.slow_query", wantLevel: "WARN"},
		{name: "query error errors", elapsed: time.Millisecond, err: errors.New("relation does not exist"), wantMsg: "db.query_error", wantLevel: "ERROR"},
		{name: "slow query error is one error line", elapsed: time.Second, err: errors.New("timeout"), wantMsg: "db.query_error", wantLevel: "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, buf := bufLogger()
			gl := NewGormLogger(base, threshold)

			called := false
			gl.Trace(context.Background(), time.Now().Add(-tt.elapsed), func() (string, int64) {
				called = true
				return sql, 3
			}, tt.err)

			lines := decodeLines(t, buf)
			if tt.wantMsg == "" {
				if len(lines) != 0 {
					t.Fatalf("want no output, got %v", lines)
				}
				if called {
					t.Error("SQL callback evaluated for a query that is not logged")
				}
				return
			}
			if len(lines) != 1 {
				t.Fatalf("want 1 line, got %d: %v", len(lines), lines)
			}
			got := lines[0]
			if got["msg"] != tt.wantMsg {
				t.Errorf("msg = %v, want %s", got["msg"], tt.wantMsg)
			}
			if got["level"] != tt.wantLevel {
				t.Errorf("level = %v, want %s", got["level"], tt.wantLevel)
			}
			if got["sql"] != sql {
				t.Errorf("sql = %v, want %s", got["sql"], sql)
			}
			if got["rows"] != float64(3) {
				t.Errorf("rows = %v, want 3", got["rows"])
			}
			ms, ok := got["elapsed_ms"].(float64)
			if !ok || ms < float64(tt.elapsed.Milliseconds()) {
				t.Errorf("elapsed_ms = %v, want >= %d", got["elapsed_ms"], tt.elapsed.Milliseconds())
			}
		})
	}
}

func TestGormLoggerSilentMode(t *testing.T) {
	base, buf := bufLogger()
	gl := NewGormLogger(base, time.Nanosecond).LogMode(0) // gormlogger.Silent
	gl.Trace(context.Background(), time.Now().Add(-time.Second), func() (string, int64) { return "SELECT 1", 0 }, errors.New("boom"))
	if buf.Len() != 0 {
		t.Fatalf("silent mode logged: %s", buf.String())
	}
}

func TestGormLoggerUsesContextLogger(t *testing.T) {
	base, baseBuf := bufLogger()
	reqLog, reqBuf := bufLogger()
	ctx := logger.SWithContext(context.Background(), reqLog.With(slog.String("request_id", "req-42")))

	NewGormLogger(base, time.Millisecond).Trace(ctx, time.Now().Add(-time.Second), func() (string, int64) { return "SELECT 1", 1 }, nil)

	if baseBuf.Len() != 0 {
		t.Errorf("base logger used despite context logger: %s", baseBuf.String())
	}
	lines := decodeLines(t, reqBuf)
	if len(lines) != 1 || lines[0]["request_id"] != "req-42" {
		t.Fatalf("want slow query line with request_id, got %v", lines)
	}
}

// TestGormLoggerKeepsBindValuesOut runs a real GORM statement (dry run, no
// server) through the adapter and checks the logged SQL keeps placeholders
// instead of the bound values.
func TestGormLoggerKeepsBindValuesOut(t *testing.T) {
	base, buf := bufLogger()
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 user=x dbname=x sslmode=disable"}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		Logger:                 NewGormLogger(base, time.Nanosecond), // every statement counts as slow
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	const secret = "s3cret-reset-token-value"
	db.Model(&models.User{}).Where("reset_token = ?", secret).Update("name", "x")

	out := buf.String()
	if out == "" {
		t.Fatal("expected a db.slow_query line")
	}
	if strings.Contains(out, secret) {
		t.Fatalf("bound value leaked into log: %s", out)
	}
	if !strings.Contains(out, "$") {
		t.Errorf("expected placeholders in logged SQL: %s", out)
	}
}

// TestGormConfigUsesSlogAdapter guards the production wiring: Connect must
// open the database with the slog adapter, not GORM's default logger, and
// must read logger.S when called, not at package init.
func TestGormConfigUsesSlogAdapter(t *testing.T) {
	// Swaps a global: no t.Parallel.
	orig := logger.S
	t.Cleanup(func() { logger.S = orig })
	var buf bytes.Buffer
	logger.S = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := gormConfig()
	gl, ok := cfg.Logger.(*gormLogger)
	if !ok {
		t.Fatalf("gormConfig().Logger is %T, want *gormLogger", cfg.Logger)
	}
	if gl.slowThreshold != SlowQueryThreshold {
		t.Errorf("slowThreshold = %v, want %v", gl.slowThreshold, SlowQueryThreshold)
	}
	if gl.level != gormlogger.Warn {
		t.Errorf("level = %v, want Warn", gl.level)
	}
	if gl.base != logger.S {
		t.Error("adapter base is not the global logger.S")
	}

	ctx := context.Background()
	query := func() (string, int64) { return "SELECT 1 WHERE id = $1", 0 }

	cfg.Logger.Trace(ctx, time.Now().Add(-time.Millisecond), query, nil)
	if buf.Len() != 0 {
		t.Fatalf("fast successful query was logged: %s", buf.String())
	}

	cfg.Logger.Trace(ctx, time.Now(), query, errors.New("boom"))
	lines := decodeLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	l := lines[0]
	if l["msg"] != "db.query_error" || l["level"] != "ERROR" || l["error"] != "boom" {
		t.Errorf("unexpected line: %v", l)
	}
	if sql, _ := l["sql"].(string); !strings.Contains(sql, "$1") {
		t.Errorf("sql = %q, want bound placeholder $1", sql)
	}
}

func TestGormLoggerMessageArgsNotSerialised(t *testing.T) {
	type unserialisable struct {
		F      func()
		Secret string
	}
	tests := []struct {
		name     string
		msg      string
		args     []any
		wantText func(string) bool
	}{
		{
			name: "struct and error args",
			msg:  "failed %v got %v",
			args: []any{unserialisable{Secret: "STRUCTSENTINEL"}, errors.New("boom")},
			wantText: func(s string) bool {
				return strings.Contains(s, "boom") && strings.Contains(s, "failed")
			},
		},
		{
			name:     "no args keeps literal percent",
			msg:      "100% literal %v",
			wantText: func(s string) bool { return s == "100% literal %v" },
		},
	}
	levels := []struct {
		call  func(gormlogger.Interface, string, ...any)
		msg   string
		level string
	}{
		{func(g gormlogger.Interface, m string, a ...any) { g.Info(context.Background(), m, a...) }, "db.info", "INFO"},
		{func(g gormlogger.Interface, m string, a ...any) { g.Warn(context.Background(), m, a...) }, "db.warn", "WARN"},
		{func(g gormlogger.Interface, m string, a ...any) { g.Error(context.Background(), m, a...) }, "db.error", "ERROR"},
	}
	for _, tt := range tests {
		for _, lv := range levels {
			t.Run(tt.name+"/"+lv.level, func(t *testing.T) {
				l, buf := bufLogger()
				g := NewGormLogger(l, SlowQueryThreshold).LogMode(gormlogger.Info)
				lv.call(g, tt.msg, tt.args...)

				raw := buf.String()
				if strings.Contains(raw, "marshaling error") || strings.Contains(raw, "STRUCTSENTINEL") {
					t.Fatalf("raw output leaks or fails to marshal: %s", raw)
				}
				lines := decodeLines(t, buf)
				if len(lines) != 1 {
					t.Fatalf("want 1 line, got %d", len(lines))
				}
				m := lines[0]
				if _, ok := m["args"]; ok {
					t.Errorf("args key present: %v", m)
				}
				if m["msg"] != lv.msg || m["level"] != lv.level {
					t.Errorf("got msg=%v level=%v, want %s/%s", m["msg"], m["level"], lv.msg, lv.level)
				}
				if text, _ := m["text"].(string); !tt.wantText(text) {
					t.Errorf("unexpected text %q", text)
				}
			})
		}
	}
}

func TestGormOpenConnectFailureKeepsPasswordOut(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	l, buf := bufLogger()
	dsn := fmt.Sprintf("host=127.0.0.1 port=%d user=ramble password=PWSENTINEL dbname=ramble sslmode=disable connect_timeout=2", port)
	_, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: NewGormLogger(l, SlowQueryThreshold)})
	if err == nil {
		t.Fatal("expected connect failure")
	}

	raw := buf.String()
	if strings.Contains(raw, "PWSENTINEL") || strings.Contains(raw, "marshaling error") {
		t.Fatalf("output leaks password or fails to marshal: %s", raw)
	}
	lines := decodeLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d: %s", len(lines), raw)
	}
	m := lines[0]
	text, _ := m["text"].(string)
	if m["msg"] != "db.error" || m["level"] != "ERROR" ||
		!strings.Contains(text, "failed to initialize database") || !strings.Contains(text, "failed to connect") {
		t.Errorf("unexpected line: %v", m)
	}
}

type ptrErr struct{}

func (*ptrErr) Error() string { return "ptr err" }

func TestSafeArg(t *testing.T) {
	type secretCfg struct{ Password string }
	var typedNil *ptrErr
	tests := []struct {
		name string
		in   any
		want any
	}{
		{"nil", nil, "<nil>"},
		{"typed nil error", error(typedNil), "<nil>"},
		{"error", errors.New("boom"), "boom"},
		{"string", "s", "s"},
		{"int", 3, 3},
		{"named scalar keeps value", 2 * time.Second, 2 * time.Second},
		{"struct becomes type name", secretCfg{Password: "SENTINEL_PW"}, "<database.secretCfg>"},
		{"pointer to struct becomes type name", &secretCfg{Password: "SENTINEL_PW"}, "<*database.secretCfg>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeArg(tt.in); got != tt.want {
				t.Errorf("safeArg(%T) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
	if got := formatText("slow by %v", []any{2 * time.Second}); got != "slow by 2s" {
		t.Errorf("formatText = %q, want %q", got, "slow by 2s")
	}
}
