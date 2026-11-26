package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"rmbl/internal/cli/config"
	"rmbl/internal/handlers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRegistry serves the /v1 shapes the publish and delete commands use.
type fakeRegistry struct {
	*httptest.Server
	mu        sync.Mutex
	existing  *handlers.ResourceResponse // nil: lookup answers 404
	lookupErr int                        // non-zero: lookup answers this status
	versionSt int                        // status for POST versions (default 202)
	versionMg string
	username  string
	tokenOrg  *string
	requests  []string
	bodies    map[string]string
	auths     []string
}

func newFakeRegistry(t *testing.T) *fakeRegistry {
	t.Helper()
	f := &fakeRegistry{username: "alice", bodies: map[string]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func apiErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, handlers.APIErrorResponse{Error: handlers.APIErrorBody{Code: code, Message: msg}})
}

func (f *fakeRegistry) serve(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	key := r.Method + " " + r.URL.Path
	f.mu.Lock()
	f.requests = append(f.requests, key)
	f.bodies[key] = string(b)
	f.auths = append(f.auths, r.Header.Get("Authorization"))
	f.mu.Unlock()

	switch {
	case key == "GET /v1/user":
		writeJSON(w, 200, handlers.CurrentUserResponse{Username: f.username, TokenScope: "write", TokenOrg: f.tokenOrg, TokenExpiresAt: time.Now().Add(time.Hour)})
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/namespaces/"):
		if f.lookupErr != 0 {
			apiErr(w, f.lookupErr, "x", "lookup failed")
			return
		}
		if f.existing == nil {
			apiErr(w, 404, "not_found", "resource not found")
			return
		}
		writeJSON(w, 200, f.existing)
	case key == "POST /v1/resources":
		var in struct{ Type, Name, Namespace string }
		_ = json.Unmarshal(b, &in)
		res := handlers.ResourceResponse{ID: 7, Name: in.Name, Type: in.Type, Namespace: in.Namespace, LatestVersion: "v1.0.0", WebhookURL: f.URL + "/resource/7/webhook", Tags: []string{}}
		writeJSON(w, 201, handlers.CreateResourceResponse{ResourceResponse: res, WebhookSecret: "whsec_onlyonce"})
	case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/versions"):
		st := f.versionSt
		if st == 0 {
			st = 202
		}
		if st != 202 {
			apiErr(w, st, "conflict", f.versionMg)
			return
		}
		writeJSON(w, 202, handlers.ResourceVersionResponse{Version: "v2.0.0", FetchStatus: "pending"})
	case r.Method == "DELETE":
		w.WriteHeader(204)
	default:
		w.WriteHeader(404)
	}
}

func (f *fakeRegistry) called(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.requests, key)
}

func existingRes(typ string) *handlers.ResourceResponse {
	return &handlers.ResourceResponse{ID: 7, Name: "mysql", Type: typ, Namespace: "alice", Tags: []string{}}
}

func setupAuth(t *testing.T, url string) {
	t.Helper()
	setupRegistry(t, url)
	t.Setenv(config.EnvToken, secretToken)
}

func noLeak(t *testing.T, out, errOut string, err error) {
	t.Helper()
	assert.NotContains(t, out+errOut+errString(err), secretToken)
}

func TestPublishCreate(t *testing.T) {
	f := newFakeRegistry(t)
	setupAuth(t, f.URL)

	out, errOut, err := runCLI(t, "", "pack", "publish", "--repo", "https://github.com/alice/packs", "--path", "./packs/mysql/", "--version", "v1.0.0", "--namespace", "alice")
	require.NoError(t, err)
	assert.Contains(t, out, "Created pack alice/mysql version v1.0.0")
	assert.Contains(t, out, "Webhook URL: "+f.URL+"/resource/7/webhook")
	assert.Contains(t, out, "Webhook secret: whsec_onlyonce")
	assert.Contains(t, out, "shown only once")
	assert.NotContains(t, errOut, "whsec_onlyonce")
	noLeak(t, out, errOut, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(f.bodies["POST /v1/resources"]), &body))
	assert.Equal(t, map[string]any{
		"type": "pack", "name": "mysql", "namespace": "alice",
		"repository_url": "https://github.com/alice/packs", "file_path": "packs/mysql", "version": "v1.0.0",
	}, body)
	for _, a := range f.auths {
		assert.Equal(t, "Bearer "+secretToken, a)
	}
}

func TestPublishCreateJobDefaults(t *testing.T) {
	tests := []struct {
		name     string
		tokenOrg *string
		wantNS   string
	}{
		{"username", nil, "alice"},
		{"token org wins", new("acme"), "acme"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeRegistry(t)
			f.tokenOrg = tt.tokenOrg
			setupAuth(t, f.URL)

			out, errOut, err := runCLI(t, "", "job", "publish", "--repo", "https://github.com/alice/web.git", "--version", "v1")
			require.NoError(t, err)
			assert.Contains(t, errOut, "Using namespace "+tt.wantNS)
			assert.Contains(t, out, "Created job "+tt.wantNS+"/web version")
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(f.bodies["POST /v1/resources"]), &body))
			assert.Equal(t, "job", body["type"])
			assert.Equal(t, "web", body["name"])
			assert.NotContains(t, body, "file_path")
			assert.True(t, f.called("GET /v1/user"))
			noLeak(t, out, errOut, err)
		})
	}
}

func TestPublishExistingAddsVersion(t *testing.T) {
	f := newFakeRegistry(t)
	f.existing = existingRes("pack")
	setupAuth(t, f.URL)

	out, errOut, err := runCLI(t, "", "pack", "publish", "--repo", "https://x/y", "--path", "mysql", "--version", "v2.0.0", "--namespace", "alice")
	require.NoError(t, err)
	assert.Contains(t, out, "Queued version v2.0.0 of pack alice/mysql")
	assert.JSONEq(t, `{"version":"v2.0.0"}`, f.bodies["POST /v1/resources/7/versions"])
	assert.False(t, f.called("POST /v1/resources"))
	assert.False(t, f.called("GET /v1/user"))
	noLeak(t, out, errOut, err)
}

func TestPublishTypeMismatch(t *testing.T) {
	f := newFakeRegistry(t)
	f.existing = existingRes("job")
	setupAuth(t, f.URL)

	out, errOut, err := runCLI(t, "", "pack", "publish", "--repo", "https://x/y", "--path", "mysql", "--version", "v2", "--namespace", "alice")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alice/mysql is a job, not a pack")
	assert.Contains(t, err.Error(), "ramble job publish")
	assert.False(t, f.called("POST /v1/resources/7/versions"))
	noLeak(t, out, errOut, err)
}

func TestPublishDuplicateVersion(t *testing.T) {
	f := newFakeRegistry(t)
	f.existing = existingRes("pack")
	f.versionSt = 409
	f.versionMg = "version v2 already exists\x1b[31m"
	setupAuth(t, f.URL)

	out, errOut, err := runCLI(t, "", "pack", "publish", "--repo", "https://x/y", "--path", "mysql", "--version", "v2", "--namespace", "alice")
	require.Error(t, err)
	assert.Equal(t, "server error (409): version v2 already exists[31m", err.Error())
	assert.NotContains(t, err.Error(), "\x1b")
	noLeak(t, out, errOut, err)
}

func TestPublishErrorStatuses(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{401, `run "ramble login"`},
		{429, "rate limited"},
		{403, "server error (403)"},
		{500, "server error (500)"},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			f := newFakeRegistry(t)
			f.lookupErr = tt.status
			setupAuth(t, f.URL)
			out, errOut, err := runCLI(t, "", "pack", "publish", "--repo", "https://x/y", "--path", "mysql", "--version", "v1", "--namespace", "alice")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			noLeak(t, out, errOut, err)
		})
	}
}

func TestPublishValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing repo", []string{"--version", "v1"}, "repo"},
		{"missing version", []string{"--repo", "https://x/y"}, "version"},
		{"absolute path", []string{"--repo", "https://x/y", "--version", "v1", "--path", "/etc"}, "invalid --path"},
		{"dotdot path", []string{"--repo", "https://x/y", "--version", "v1", "--path", "../x"}, "inside the repository"},
		{"bad name", []string{"--repo", "https://x/y", "--version", "v1", "--name", "a/b"}, "invalid --name"},
		{"bad namespace", []string{"--repo", "https://x/y", "--version", "v1", "--name", "n", "--namespace", ".."}, "invalid --namespace"},
		{"no derivable name", []string{"--repo", "https://x", "--version", "v1"}, "pass --name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeRegistry(t)
			setupAuth(t, f.URL)
			_, _, err := runCLI(t, "", append([]string{"pack", "publish"}, tt.args...)...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Empty(t, f.requests)
		})
	}
}

func TestNoTokenIsAClearError(t *testing.T) {
	f := newFakeRegistry(t)
	setupRegistry(t, f.URL)
	_, _, err := runCLI(t, "", "pack", "publish", "--repo", "https://x/y", "--version", "v1", "--namespace", "alice", "--name", "n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ramble login")
	assert.Contains(t, err.Error(), config.EnvToken)
	assert.Empty(t, f.requests)
}

func TestPublishPlainHTTPRefused(t *testing.T) {
	// The registry host does not exist: the refusal must come before any
	// connection attempt, so the error is the refusal, not a dial failure.
	setupAuth(t, "http://registry.example.invalid")

	out, errOut, err := runCLI(t, "", "pack", "publish", "--repo", "https://x/y", "--version", "v1", "--namespace", "alice", "--name", "n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to send a token")
	noLeak(t, out, errOut, err)

	_, _, err = runCLI(t, "", "pack", "delete", "alice/n", "--yes")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to send a token")
}

func TestRedirectNotFollowed(t *testing.T) {
	var bHits atomic.Int32
	var bAuth atomic.Value
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bHits.Add(1)
		bAuth.Store(r.Header.Get("Authorization"))
	}))
	defer b.Close()
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, b.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer a.Close()
	setupAuth(t, a.URL)

	out, errOut, err := runCLI(t, "", "pack", "publish", "--repo", "https://x/y", "--version", "v1", "--namespace", "alice", "--name", "n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "307")
	assert.Zero(t, bHits.Load())
	assert.Nil(t, bAuth.Load())
	noLeak(t, out, errOut, err)
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		stdin     string
		terminal  bool
		wantErr   string
		wantOut   string
		wantErrIO string
		wantCall  bool
	}{
		{"yes flag", []string{"--yes"}, "", false, "", "Deleted pack alice/mysql", "", true},
		{"y answer", nil, "y\n", true, "", "Deleted pack alice/mysql", "Delete pack alice/mysql? [y/N]", true},
		{"YES answer", nil, " YES \n", true, "", "Deleted pack alice/mysql", "", true},
		{"n answer", nil, "n\n", true, "", "", "Aborted", false},
		{"empty answer", nil, "\n", true, "", "", "Aborted", false},
		{"other answer", nil, "maybe\n", true, "", "", "Aborted", false},
		{"non-interactive refused", nil, "y\n", false, "pass --yes", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeRegistry(t)
			f.existing = existingRes("pack")
			setupAuth(t, f.URL)
			prev := stdinIsTerminal
			stdinIsTerminal = func(io.Reader) bool { return tt.terminal }
			t.Cleanup(func() { stdinIsTerminal = prev })

			out, errOut, err := runCLI(t, tt.stdin, append([]string{"pack", "delete", "alice/mysql"}, tt.args...)...)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Contains(t, out, tt.wantOut)
			assert.Contains(t, errOut, tt.wantErrIO)
			assert.Equal(t, tt.wantCall, f.called("DELETE /v1/resources/7"))
			noLeak(t, out, errOut, err)
		})
	}
}

func TestDeleteTypeMismatchAndMissing(t *testing.T) {
	f := newFakeRegistry(t)
	f.existing = existingRes("job")
	setupAuth(t, f.URL)
	_, _, err := runCLI(t, "", "pack", "delete", "alice/mysql", "--yes")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a job, not a pack")
	assert.False(t, f.called("DELETE /v1/resources/7"))

	f.existing = nil
	_, _, err = runCLI(t, "", "job", "delete", "alice/mysql", "--yes")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "job alice/mysql not found")
}

func TestDeleteBadRef(t *testing.T) {
	f := newFakeRegistry(t)
	setupAuth(t, f.URL)
	for _, arg := range []string{"mysql", "a/b/c", "/b", "a/", "../x", "a/..", "a/b\x1b"} {
		t.Run(arg, func(t *testing.T) {
			_, _, err := runCLI(t, "", "pack", "delete", arg, "--yes")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "NAMESPACE/NAME")
		})
	}
	assert.Empty(t, f.requests)
}

func TestDefaultName(t *testing.T) {
	tests := []struct{ path, repo, want string }{
		{"jobs/web.nomad.hcl", "", "web"},
		{"jobs/web.nomad", "", "web"},
		{"jobs/web.hcl", "", "web"},
		{"packs/mysql", "", "mysql"},
		{"", "https://github.com/alice/web.git", "web"},
		{"", "https://github.com/alice/web/", "web"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, defaultName(tt.path, tt.repo), "%q %q", tt.path, tt.repo)
	}
}
