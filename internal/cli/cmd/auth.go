package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"rmbl/internal/cli/config"
	"rmbl/internal/pack"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var authRegistry string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Store an API token for a registry",
	Long: `Read an API token and store it for a registry.

On a terminal the token is read without echo. Otherwise one line is read from
stdin. The token is verified against the registry before it is stored, in the
CLI config file (mode 0600). Create tokens at <registry>/settings/tokens.

The RAMBLE_TOKEN environment variable, when set, overrides the stored token.

Examples:
  ramble login
  ramble login --registry internal
  echo "$TOKEN" | ramble login`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runLogin,
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove the stored API token for a registry",
	Long: `Remove the stored API token for a registry.

This does not contact the server: the token stays valid until you revoke it at
<registry>/settings/tokens.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runLogout,
}

func init() {
	rootCmd.AddCommand(loginCmd, logoutCmd)
	loginCmd.Flags().StringVarP(&authRegistry, "registry", "r", "", "Registry name (default: the default registry)")
	logoutCmd.Flags().StringVarP(&authRegistry, "registry", "r", "", "Registry name (default: the default registry)")
}

func runLogin(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	name, reg, err := cfg.ResolveRegistry(authRegistry)
	if err != nil {
		return err
	}
	if err := pack.TokenURLAllowed(reg.URL); err != nil {
		return err
	}

	token, err := readToken(cmd.InOrStdin(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	user, err := pack.NewClient(reg.URL).CurrentUser(ctx, token)
	if err != nil {
		if apiErr, ok := errors.AsType[*pack.APIError](err); ok && apiErr.StatusCode == 401 && apiErr.Message != "" {
			return errors.New(apiErr.Message)
		}
		return fmt.Errorf("login failed: %w", err)
	}

	if err := cfg.SetToken(name, token); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	if os.Getenv(config.EnvToken) != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "Note: %s is set and overrides the stored token\n", config.EnvToken)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Logged in to %s as %s\n", name, user.Username)
	return nil
}

func runLogout(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	name, reg, err := cfg.ResolveRegistry(authRegistry)
	if err != nil {
		return err
	}
	had, err := cfg.ClearToken(name)
	if err != nil {
		return err
	}
	if !had {
		fmt.Fprintf(cmd.OutOrStdout(), "Not logged in to %s\n", name)
		return nil
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Logged out of %s. The token is still valid; revoke it at %s/settings/tokens.\n", name, reg.URL)
	return nil
}

// readToken reads a token from a terminal without echo, or one line from a
// pipe. The prompt goes to prompt (stderr) so stdout stays clean.
func readToken(in io.Reader, prompt io.Writer) (string, error) {
	var raw string
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) { //#nosec G115 -- file descriptors fit in int
		fmt.Fprint(prompt, "Token: ")
		b, err := term.ReadPassword(int(f.Fd())) //#nosec G115 -- file descriptors fit in int
		fmt.Fprintln(prompt)
		if err != nil {
			return "", fmt.Errorf("failed to read token: %w", err)
		}
		raw = string(b)
	} else {
		line, err := bufio.NewReader(io.LimitReader(in, 4096)).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("failed to read token: %w", err)
		}
		raw = line
	}
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", errors.New("no token provided")
	}
	return token, nil
}
