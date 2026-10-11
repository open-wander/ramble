package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	stdlog "log"
	"strings"
	"testing"

	"rmbl/internal/server"
	"rmbl/internal/services/logger"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	rootCmd.InitDefaultHelpCmd()
	helpCmd, _, _ := rootCmd.Find([]string{"help"})
	pack := &cobra.Command{Use: "pack"}
	pack.AddCommand(&cobra.Command{Use: "server"})
	tests := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{"server", serverCmd, false},
		{"version", versionCmd, false},
		{"help", helpCmd, false},
		{"nested server", pack.Commands()[0], true},
		{"unparented help", &cobra.Command{Use: "help"}, true},
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

// restoreLoggerGlobals snapshots every global logger.Init touches and
// restores it in Cleanup.
func restoreLoggerGlobals(t *testing.T) {
	t.Helper()
	prevLog := logger.Log
	prevGlobal := log.Logger
	prevMsg := zerolog.MessageFieldName
	prevLevel := zerolog.LevelFieldMarshalFunc
	prevTimeFmt := zerolog.TimeFieldFormat
	prevOut := stdlog.Writer()
	prevFlags := stdlog.Flags()
	t.Cleanup(func() {
		logger.Log = prevLog
		log.Logger = prevGlobal
		zerolog.MessageFieldName = prevMsg
		zerolog.LevelFieldMarshalFunc = prevLevel
		zerolog.TimeFieldFormat = prevTimeFmt
		stdlog.SetOutput(prevOut)
		stdlog.SetFlags(prevFlags)
	})
}

// driveRoot runs rootCmd with args, capturing cobra's output, and restores
// the shared command state afterwards.
func driveRoot(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	var out bytes.Buffer
	updateCheck = nil
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		updateCheck = nil
	})
	_, err := rootCmd.ExecuteC()
	return &out, err
}

func TestServerCommandBadFlag(t *testing.T) {
	t.Setenv("ENV", "production")
	restoreLoggerGlobals(t)

	var buf bytes.Buffer
	oldInit, oldRun := initLogger, runServer
	// The real production Init path, pointed at a buffer instead of stdout.
	initLogger = func() { logger.InitWithWriter(&buf) }
	runServer = func(server.Config) error {
		t.Error("server ran despite a bad flag")
		return nil
	}
	t.Cleanup(func() { initLogger, runServer = oldInit, oldRun })

	out, err := driveRoot(t, "server", "--bogus")

	var le loggedError
	if err == nil || !errors.As(err, &le) {
		t.Fatalf("err = %v (%T), want loggedError", err, err)
	}
	var rep bytes.Buffer
	reportExecError(err, &rep)
	if rep.Len() != 0 {
		t.Errorf("Execute would reprint the error: %q", rep.String())
	}
	if out.Len() != 0 {
		t.Errorf("cobra wrote output: %q", out.String())
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 || lines[0] == "" {
		t.Fatalf("got %d log lines, want 1: %q", len(lines), buf.String())
	}
	var rec map[string]any
	if e := json.Unmarshal([]byte(lines[0]), &rec); e != nil {
		t.Fatalf("log line is not JSON: %v: %q", e, lines[0])
	}
	if rec["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", rec["level"])
	}
	if rec["msg"] != "server.flag_error" {
		t.Errorf("msg = %v", rec["msg"])
	}
	if s, _ := rec["error"].(string); !strings.Contains(s, "unknown flag: --bogus") {
		t.Errorf("error = %v", rec["error"])
	}
}

func TestOtherCommandBadFlagIsPlainText(t *testing.T) {
	buf := captureLog(t)
	out, err := driveRoot(t, "pack", "list", "--bogus")

	var le loggedError
	if err == nil || errors.As(err, &le) {
		t.Fatalf("err = %v (%T), want a plain error", err, err)
	}
	for _, want := range []string{"Error: unknown flag: --bogus", "Usage:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("cobra output missing %q: %q", want, out.String())
		}
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected JSON log output: %q", buf.String())
	}
	var rep bytes.Buffer
	reportExecError(err, &rep)
	if !strings.Contains(rep.String(), "unknown flag: --bogus") {
		t.Errorf("reportExecError wrote %q", rep.String())
	}
}
