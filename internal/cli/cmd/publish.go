package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"rmbl/internal/cli/apiclient"
	"rmbl/internal/cli/config"
	"rmbl/internal/pack"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// stdinIsTerminal reports whether r is an interactive terminal. It is a
// variable so tests can stand in for a terminal.
var stdinIsTerminal = func(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd())) //#nosec G115 -- file descriptors fit in int
}

// refPart matches a namespace or a resource name. It rejects ".", ".." and
// anything with a slash or control character.
var refPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func init() {
	packCmd.AddCommand(newPublishCmd("pack"), newDeleteCmd("pack"))
	jobCmd.AddCommand(newPublishCmd("job"), newDeleteCmd("job"))
}

// apiClientFor builds the /v1 client for a registry, using the stored token or
// RAMBLE_TOKEN. It returns the resolved registry name.
func apiClientFor(registry string) (*apiclient.ClientWithResponses, string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, "", err
	}
	name, reg, err := cfg.ResolveRegistry(registry)
	if err != nil {
		return nil, "", err
	}
	token := cfg.TokenFor(name)
	if token == "" {
		return nil, "", fmt.Errorf("not logged in to %s: run \"ramble login --registry %s\" or set %s", name, name, config.EnvToken)
	}
	c, err := apiclient.New(reg.URL, token)
	if err != nil {
		return nil, "", err
	}
	return c, name, nil
}

// apiError turns a /v1 answer the command did not expect into an error. The
// server message is sanitised; the token never appears in it.
func apiError(status int, body []byte) error {
	msg := pack.ErrorMessageFromBody(body)
	switch {
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("rate limited by the registry (%d): try again later", status)
	case msg == "":
		if status == http.StatusUnauthorized {
			return fmt.Errorf("unexpected status code: %d (run \"ramble login\" or set %s)", status, config.EnvToken)
		}
		return fmt.Errorf("unexpected status code: %d", status)
	case status == http.StatusUnauthorized:
		return fmt.Errorf("server error (%d): %s (run \"ramble login\" or set %s)", status, msg, config.EnvToken)
	}
	return fmt.Errorf("server error (%d): %s", status, msg)
}

// parseRef splits "namespace/name".
func parseRef(arg string) (string, string, error) {
	ns, name, ok := strings.Cut(arg, "/")
	if !ok || !refPart.MatchString(ns) || !refPart.MatchString(name) {
		return "", "", fmt.Errorf("invalid argument %q: expected NAMESPACE/NAME", pack.Printable(arg))
	}
	return ns, name, nil
}

// cleanRepoPath turns --path into a repo-relative path, "" for the repo root.
func cleanRepoPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || p == "." {
		return "", nil
	}
	if path.IsAbs(p) || strings.Contains(p, "\\") || pack.Printable(p) != p {
		return "", fmt.Errorf("invalid --path %q: must be a relative path inside the repository", pack.Printable(p))
	}
	p = path.Clean(p)
	if p == "." {
		return "", nil
	}
	if p == ".." || strings.HasPrefix(p, "../") {
		return "", fmt.Errorf("invalid --path %q: must stay inside the repository", p)
	}
	return p, nil
}

// defaultName derives a resource name from the repo path or, for the repo
// root, from the repository URL. A job file's Nomad extension is dropped, so
// jobs/web.nomad.hcl publishes as "web".
func defaultName(repoPath, repoURL string) string {
	if repoPath != "" {
		base := path.Base(repoPath)
		for _, ext := range []string{".nomad.hcl", ".nomad", ".hcl"} {
			if trimmed := strings.TrimSuffix(base, ext); trimmed != base && trimmed != "" {
				return trimmed
			}
		}
		return base
	}
	u, err := url.Parse(repoURL)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(path.Base(strings.TrimSuffix(u.Path, "/")), ".git")
}

func newPublishCmd(kind string) *cobra.Command {
	var repo, repoPath, namespace, name, version, registry string
	cmd := &cobra.Command{
		Use:   "publish",
		Short: fmt.Sprintf("Publish a %s version to the registry", kind),
		Long: fmt.Sprintf(`Publish a version of a %[1]s that lives in a Git repository.

If NAMESPACE/NAME does not exist yet it is created and the webhook URL and
secret are printed. The secret is shown only once. Otherwise the version is
queued for the existing %[1]s; the registry fetches it in the background.

--name defaults to the last element of --path without a .nomad.hcl, .nomad or
.hcl extension, or to the repository name when --path is empty. --namespace defaults to the organisation of an organisation
token, otherwise to the username of the token owner.

Needs a token with write scope: run "ramble login" or set RAMBLE_TOKEN.

Examples:
  ramble %[1]s publish --repo https://github.com/alice/packs --path packs/mysql --version v1.0.0
  ramble %[1]s publish --repo https://github.com/acme/stack --namespace acme --name web --version v2.1.0`, kind),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPublish(cmd, kind, repo, repoPath, namespace, name, version, registry)
		},
	}
	f := cmd.Flags()
	f.StringVar(&repo, "repo", "", "Git repository URL (required)")
	f.StringVar(&repoPath, "path", "", "Path of the "+kind+" inside the repository (default: the repository root)")
	f.StringVar(&version, "version", "", "Version to publish, e.g. v1.0.0 (required)")
	f.StringVar(&namespace, "namespace", "", "Namespace (default: from the token)")
	f.StringVar(&name, "name", "", "Name (default: last element of --path or of the repository URL)")
	f.StringVarP(&registry, "registry", "r", "", "Registry name (default: the default registry)")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("version")
	return cmd
}

func runPublish(cmd *cobra.Command, kind, repo, repoPath, namespace, name, version, registry string) error {
	repoPath, err := cleanRepoPath(repoPath)
	if err != nil {
		return err
	}
	if name == "" {
		name = defaultName(repoPath, repo)
		if !refPart.MatchString(name) {
			return errors.New("cannot derive a name from --path or --repo: pass --name")
		}
	}
	if !refPart.MatchString(name) {
		return fmt.Errorf("invalid --name %q", pack.Printable(name))
	}
	if namespace != "" && !refPart.MatchString(namespace) {
		return fmt.Errorf("invalid --namespace %q", pack.Printable(namespace))
	}
	if strings.TrimSpace(version) == "" {
		return errors.New("--version must not be empty")
	}

	client, _, err := apiClientFor(registry)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	if namespace == "" {
		resp, err := client.GetCurrentUserWithResponse(ctx)
		if err != nil {
			return err
		}
		if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
			return apiError(resp.StatusCode(), resp.Body)
		}
		namespace = resp.JSON200.Username
		if resp.JSON200.TokenOrg != nil && *resp.JSON200.TokenOrg != "" {
			namespace = *resp.JSON200.TokenOrg
		}
		if !refPart.MatchString(namespace) {
			return errors.New("the registry returned an invalid namespace: pass --namespace")
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Using namespace %s\n", namespace)
	}

	lookup, err := client.GetResourceByNameWithResponse(ctx, namespace, name)
	if err != nil {
		return err
	}
	switch lookup.StatusCode() {
	case http.StatusNotFound:
		return createResource(cmd, client, kind, namespace, name, repo, repoPath, version)
	case http.StatusOK:
		if lookup.JSON200 == nil {
			return apiError(lookup.StatusCode(), lookup.Body)
		}
		r := lookup.JSON200
		if err := checkKind(kind, string(r.Type), "publish", namespace, name); err != nil {
			return err
		}
		vr, err := client.CreateResourceVersionWithResponse(ctx, r.Id, apiclient.CreateResourceVersionJSONRequestBody{Version: version})
		if err != nil {
			return err
		}
		if vr.StatusCode() != http.StatusAccepted {
			return apiError(vr.StatusCode(), vr.Body)
		}
		fmt.Fprintf(out, "Queued version %s of %s %s/%s (the registry fetches it in the background)\n",
			pack.Printable(version), kind, namespace, name)
		return nil
	default:
		return apiError(lookup.StatusCode(), lookup.Body)
	}
}

func createResource(cmd *cobra.Command, client *apiclient.ClientWithResponses, kind, namespace, name, repo, repoPath, version string) error {
	body := apiclient.CreateResourceJSONRequestBody{
		Type:          apiclient.HandlersCreateResourceRequestType(kind),
		Name:          name,
		Namespace:     namespace,
		RepositoryUrl: repo,
		Version:       version,
	}
	if repoPath != "" {
		body.FilePath = &repoPath
	}
	resp, err := client.CreateResourceWithResponse(cmd.Context(), body)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusCreated || resp.JSON201 == nil {
		return apiError(resp.StatusCode(), resp.Body)
	}
	r := resp.JSON201
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Created %s %s/%s version %s\n", kind, pack.Printable(r.Namespace), pack.Printable(r.Name), pack.Printable(r.LatestVersion))
	fmt.Fprintf(out, "Webhook URL: %s\n", pack.Printable(r.WebhookUrl))
	fmt.Fprintf(out, "Webhook secret: %s\n", pack.Printable(r.WebhookSecret))
	fmt.Fprintln(out, "This secret is shown only once; store it now. Rotate it in the web UI if lost.")
	return nil
}

func checkKind(want, got, verb, namespace, name string) error {
	if got == want {
		return nil
	}
	got = pack.Printable(got)
	return fmt.Errorf("%s/%s is a %s, not a %s; use \"ramble %s %s\"", namespace, name, got, want, got, verb)
}

func newDeleteCmd(kind string) *cobra.Command {
	var yes bool
	var registry string
	cmd := &cobra.Command{
		Use:   "delete NAMESPACE/NAME",
		Short: fmt.Sprintf("Delete a %s from the registry", kind),
		Long: fmt.Sprintf(`Delete a %[1]s and all its versions from the registry.

Asks for confirmation on the terminal. Without a terminal, pass --yes.
Needs a token with write scope: run "ramble login" or set RAMBLE_TOKEN.

Examples:
  ramble %[1]s delete alice/mysql
  ramble %[1]s delete alice/mysql --yes`, kind),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd, kind, args[0], yes, registry)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Do not ask for confirmation")
	cmd.Flags().StringVarP(&registry, "registry", "r", "", "Registry name (default: the default registry)")
	return cmd
}

func runDelete(cmd *cobra.Command, kind, arg string, yes bool, registry string) error {
	ns, name, err := parseRef(arg)
	if err != nil {
		return err
	}
	if !yes && !stdinIsTerminal(cmd.InOrStdin()) {
		return errors.New("refusing to delete without confirmation: stdin is not a terminal; pass --yes")
	}
	client, _, err := apiClientFor(registry)
	if err != nil {
		return err
	}
	ctx := cmd.Context()

	lookup, err := client.GetResourceByNameWithResponse(ctx, ns, name)
	if err != nil {
		return err
	}
	if lookup.StatusCode() == http.StatusNotFound {
		return fmt.Errorf("%s %s/%s not found", kind, ns, name)
	}
	if lookup.StatusCode() != http.StatusOK || lookup.JSON200 == nil {
		return apiError(lookup.StatusCode(), lookup.Body)
	}
	r := lookup.JSON200
	if err := checkKind(kind, string(r.Type), "delete", ns, name); err != nil {
		return err
	}

	if !yes {
		fmt.Fprintf(cmd.ErrOrStderr(), "Delete %s %s/%s? [y/N] ", kind, ns, name)
		line, err := bufio.NewReader(io.LimitReader(cmd.InOrStdin(), 4096)).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("failed to read answer: %w", err)
		}
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fmt.Fprintln(cmd.ErrOrStderr(), "Aborted")
			return nil
		}
	}

	resp, err := client.DeleteResourceWithResponse(ctx, r.Id)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted %s %s/%s\n", kind, ns, name)
	return nil
}
