package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"rmbl/internal/cli/config"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secretToken = "rmbl_s3cr3t_token_value"

// runCLI executes rootCmd and returns stdout, stderr and the error.
func runCLI(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	rootCmd.SetIn(strings.NewReader(stdin))
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errb)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetIn(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		authRegistry = ""
		updateCheck = nil
		resetFlags(rootCmd)
	})
	err := rootCmd.Execute()
	return out.String(), errb.String(), err
}

// resetFlags returns every flag of cmd and its subcommands to its default, as
// cobra keeps flag values between Execute calls on the shared command tree.
func resetFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	for _, c := range cmd.Commands() {
		resetFlags(c)
	}
}

func setupRegistry(t *testing.T, url string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvToken, "")
	cfg, err := config.Load()
	require.NoError(t, err)
	cfg.AddRegistry("test", url, "")
	cfg.DefaultRegistry = "test"
	require.NoError(t, cfg.Save())
}

func storedToken(t *testing.T, name string) string {
	t.Helper()
	cfg, err := config.Load()
	require.NoError(t, err)
	return cfg.Registries[name].Token
}

func userServer(calls *atomic.Int32, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch {
		case status == 200 && r.Header.Get("Authorization") == "Bearer "+secretToken:
			_, _ = w.Write([]byte(`{"username":"alice","token_scope":"read","token_org":null}`))
		case status == 500:
			w.WriteHeader(500)
		default:
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"invalid or expired token"}}`))
		}
	}))
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		stdin      string
		wantErr    string
		wantOut    string
		wantStored string
	}{
		{"success", 200, secretToken + "\n", "", "Logged in to test as alice\n", secretToken},
		{"no trailing newline", 200, secretToken, "", "Logged in to test as alice\n", secretToken},
		{"whitespace trimmed", 200, "  " + secretToken + " \r\n", "", "Logged in to test as alice\n", secretToken},
		{"401 prints API message", 401, secretToken + "\n", "invalid or expired token", "", ""},
		{"500 stores nothing", 500, secretToken + "\n", "login failed", "", ""},
		{"empty input", 200, "\n", "no token provided", "", ""},
		{"eof", 200, "", "no token provided", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := userServer(&calls, tt.status)
			defer srv.Close()
			setupRegistry(t, srv.URL)

			out, errOut, err := runCLI(t, tt.stdin, "login")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, out)
			assert.Equal(t, tt.wantStored, storedToken(t, "test"))
			assert.NotContains(t, out+errOut+errString(err), secretToken)
		})
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestLoginNetworkError(t *testing.T) {
	var calls atomic.Int32
	srv := userServer(&calls, 200)
	url := srv.URL
	srv.Close()
	setupRegistry(t, url)

	out, errOut, err := runCLI(t, secretToken+"\n", "login")
	require.Error(t, err)
	assert.Empty(t, storedToken(t, "test"))
	assert.NotContains(t, out+errOut+err.Error(), secretToken)
}

func TestLoginRegistryFlag(t *testing.T) {
	var calls atomic.Int32
	srv := userServer(&calls, 200)
	defer srv.Close()
	setupRegistry(t, "https://unused.example")
	cfg, err := config.Load()
	require.NoError(t, err)
	cfg.AddRegistry("other", srv.URL, "")
	require.NoError(t, cfg.Save())

	out, _, err := runCLI(t, secretToken+"\n", "login", "--registry", "other")
	require.NoError(t, err)
	assert.Equal(t, "Logged in to other as alice\n", out)
	assert.Equal(t, secretToken, storedToken(t, "other"))
	assert.Empty(t, storedToken(t, "test"))

	_, _, err = runCLI(t, secretToken+"\n", "login", "-r", "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry not found: missing")
}

func TestLoginRefusesPlainHTTP(t *testing.T) {
	setupRegistry(t, "http://example.com")
	out, errOut, err := runCLI(t, secretToken+"\n", "login")
	require.Error(t, err)
	assert.Empty(t, storedToken(t, "test"))
	assert.NotContains(t, out+errOut+err.Error(), secretToken)

	// The URL is refused before any token is read: with nothing on stdin the
	// error is the refusal, not "no token provided".
	_, _, err = runCLI(t, "", "login")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to send a token")
}

func TestLogout(t *testing.T) {
	var calls atomic.Int32
	srv := userServer(&calls, 200)
	defer srv.Close()
	setupRegistry(t, srv.URL)

	_, _, err := runCLI(t, secretToken+"\n", "login")
	require.NoError(t, err)
	calls.Store(0)

	out, errOut, err := runCLI(t, "", "logout")
	require.NoError(t, err)
	assert.Equal(t, "Logged out of test. The token is still valid; revoke it at "+srv.URL+"/settings/tokens.\n", out)
	assert.Empty(t, storedToken(t, "test"))
	assert.NotContains(t, out+errOut, secretToken)
	assert.Zero(t, calls.Load(), "logout must not call the server")

	out, _, err = runCLI(t, "", "logout")
	require.NoError(t, err)
	assert.Equal(t, "Not logged in to test\n", out)
}

func TestRegistryListHidesToken(t *testing.T) {
	var calls atomic.Int32
	srv := userServer(&calls, 200)
	defer srv.Close()
	setupRegistry(t, srv.URL)
	_, _, err := runCLI(t, secretToken+"\n", "login")
	require.NoError(t, err)

	out, errOut, err := runCLI(t, "", "registry", "list")
	require.NoError(t, err)
	assert.NotContains(t, out+errOut, secretToken)
	assert.Contains(t, out, "LOGGED IN")
	assert.Contains(t, out, "yes")
}

func TestLoginConfigMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes only")
	}
	var calls atomic.Int32
	srv := userServer(&calls, 200)
	defer srv.Close()
	setupRegistry(t, srv.URL)
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "ramble", "config.json")
	require.NoError(t, os.Chmod(path, 0o644)) //nolint:gosec

	_, _, err := runCLI(t, secretToken+"\n", "login")
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestLoginNotesEnvOverride(t *testing.T) {
	var calls atomic.Int32
	srv := userServer(&calls, 200)
	defer srv.Close()
	setupRegistry(t, srv.URL)
	t.Setenv(config.EnvToken, "envtoken")

	_, errOut, err := runCLI(t, secretToken+"\n", "login")
	require.NoError(t, err)
	assert.Contains(t, errOut, "RAMBLE_TOKEN")
	assert.NotContains(t, errOut, secretToken)
}
