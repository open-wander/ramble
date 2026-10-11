package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"rmbl/internal/server"
	"rmbl/internal/services/logger"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// captureLog swaps logger.Log for a JSON logger writing to the returned buffer.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	oldLog, oldField := logger.Log, zerolog.MessageFieldName
	zerolog.MessageFieldName = "msg"
	logger.Log = zerolog.New(&buf)
	t.Cleanup(func() {
		logger.Log = oldLog
		zerolog.MessageFieldName = oldField
	})
	return &buf
}

func TestWantsUpdateCheck(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{"server", serverCmd, false},
		{"version", &cobra.Command{Use: "version"}, false},
		{"help", &cobra.Command{Use: "help"}, false},
		{"pack list", packListCmd, true},
		{"job list", &cobra.Command{Use: "list"}, true},
		{"root", rootCmd, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wantsUpdateCheck(tt.cmd); got != tt.want {
				t.Errorf("wantsUpdateCheck(%s) = %v, want %v", tt.cmd.Name(), got, tt.want)
			}
		})
	}
}

func TestServerCmdSilenced(t *testing.T) {
	if !serverCmd.SilenceErrors {
		t.Error("serverCmd.SilenceErrors = false, want true")
	}
	if !serverCmd.SilenceUsage {
		t.Error("serverCmd.SilenceUsage = false, want true")
	}
	if rootCmd.SilenceErrors || rootCmd.SilenceUsage {
		t.Error("rootCmd must keep human-readable errors and usage")
	}
}

func TestRunServerCmd(t *testing.T) {
	boom := errors.New("listen tcp :3000: address already in use")

	t.Run("error is logged once and returned", func(t *testing.T) {
		buf := captureLog(t)
		err := runServerCmd(func(server.Config) error { return boom }, server.Config{})
		if err == nil || !errors.Is(err, boom) {
			t.Fatalf("err = %v, want wrapping %v", err, boom)
		}
		var le loggedError
		if !errors.As(err, &le) {
			t.Fatalf("err %T is not loggedError", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("got %d log lines, want 1: %q", len(lines), buf.String())
		}
		var rec map[string]any
		if e := json.Unmarshal([]byte(lines[0]), &rec); e != nil {
			t.Fatalf("log line is not JSON: %v", e)
		}
		if lvl, _ := rec["level"].(string); !strings.EqualFold(lvl, "error") {
			t.Errorf("level = %v", rec["level"])
		}
		if rec["msg"] != "server.exit_error" {
			t.Errorf("msg = %v", rec["msg"])
		}
		if rec["error"] != boom.Error() {
			t.Errorf("error = %v", rec["error"])
		}
	})

	t.Run("success logs nothing", func(t *testing.T) {
		buf := captureLog(t)
		if err := runServerCmd(func(server.Config) error { return nil }, server.Config{}); err != nil {
			t.Fatalf("err = %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("unexpected log output: %q", buf.String())
		}
	})
}

func TestReportExecError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"already logged", loggedError{errors.New("x")}, ""},
		{"plain", errors.New("unknown flag: --bogus"), "unknown flag: --bogus\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w bytes.Buffer
			reportExecError(tt.err, &w)
			if w.String() != tt.want {
				t.Errorf("output = %q, want %q", w.String(), tt.want)
			}
		})
	}
}

func TestServerCommandEndToEnd(t *testing.T) {
	buf := captureLog(t)
	boom := errors.New("listen failed")
	oldRun := runServer
	runServer = func(server.Config) error { return boom }
	var out bytes.Buffer
	updateCheck = nil
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"server"})
	t.Cleanup(func() {
		runServer = oldRun
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		updateCheck = nil
	})

	_, err := rootCmd.ExecuteC()
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if out.Len() != 0 {
		t.Errorf("cobra wrote output: %q", out.String())
	}
	if updateCheck != nil {
		t.Error("update check started for server")
	}
	if !strings.Contains(buf.String(), `"msg":"server.exit_error"`) {
		t.Errorf("missing log line: %q", buf.String())
	}
}
