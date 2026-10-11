package database

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"rmbl/internal/models"
	"rmbl/internal/services/logger"
)

func bufLogger() (*zerolog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	l := zerolog.New(&buf).With().Timestamp().Logger()
	return &l, &buf
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
		{name: "slow record not found warns as slow", elapsed: time.Second, err: gorm.ErrRecordNotFound, wantMsg: "db.slow_query", wantLevel: "warn"},
		{name: "wrapped record not found is silent", elapsed: time.Millisecond, err: errors.Join(errors.New("ctx"), gorm.ErrRecordNotFound)},
		{name: "slow query warns", elapsed: 350 * time.Millisecond, wantMsg: "db.slow_query", wantLevel: "warn"},
		{name: "query error errors", elapsed: time.Millisecond, err: errors.New("relation does not exist"), wantMsg: "db.query_error", wantLevel: "error"},
		{name: "slow query error is one error line", elapsed: time.Second, err: errors.New("timeout"), wantMsg: "db.query_error", wantLevel: "error"},
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
			if got[zerolog.MessageFieldName] != tt.wantMsg {
				t.Errorf("msg = %v, want %s", got[zerolog.MessageFieldName], tt.wantMsg)
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
	withID := reqLog.With().Str("request_id", "req-42").Logger()
	ctx := withID.WithContext(context.Background())

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

// TestGormConfigUsesZerologAdapter guards the production wiring: Connect must
// open the database with the zerolog adapter, not GORM's default logger.
func TestGormConfigUsesZerologAdapter(t *testing.T) {
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
	if gl.base != &logger.Log {
		t.Error("adapter base is not the global logger.Log")
	}

	// Swaps a global: no t.Parallel.
	orig := logger.Log
	t.Cleanup(func() { logger.Log = orig })
	var buf bytes.Buffer
	logger.Log = zerolog.New(&buf)

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
	if l["message"] != "db.query_error" || l["level"] != "error" || l["error"] != "boom" {
		t.Errorf("unexpected line: %v", l)
	}
	if sql, _ := l["sql"].(string); !strings.Contains(sql, "$1") {
		t.Errorf("sql = %q, want bound placeholder $1", sql)
	}
}
