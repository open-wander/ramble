package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootVersionFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"long flag", []string{"--version"}},
		{"short flag", []string{"-v"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			rootCmd.SetOut(&out)
			rootCmd.SetErr(&out)
			rootCmd.SetArgs(tt.args)
			t.Cleanup(func() { rootCmd.SetArgs(nil) })

			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("Execute(%v) returned error: %v", tt.args, err)
			}
			want := "ramble " + Version + "\n"
			if got := out.String(); got != want {
				t.Errorf("Execute(%v) output = %q, want %q", tt.args, got, want)
			}
		})
	}
}

func TestRootVersionFlagRunsNoCommand(t *testing.T) {
	// --version must short-circuit before PersistentPreRun, otherwise it
	// kicks off the background update check and hits the network.
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"--version"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	updateCheck = nil

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if updateCheck != nil {
		t.Error("--version triggered the update check; it should exit before PersistentPreRun")
	}
	if strings.Contains(out.String(), "Usage:") {
		t.Errorf("--version printed usage: %q", out.String())
	}
}
