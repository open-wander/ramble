package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// eventKeyRe is the dotted event-key pattern, copied from
// internal/services/logger/msgkey_test.go (not importable from a _test file).
var eventKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)

// syncBuffer is a bytes.Buffer safe for concurrent writes by exec and reads by the test.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type e2eRequest struct {
	name      string
	requestID string
	status    int
}

// TestE2EProductionLogging builds the real binary, runs it in production mode
// against a real Postgres and checks that everything it writes is structured
// JSON and leaks no secrets.
func TestE2EProductionLogging(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end test in -short mode")
	}
	// Skip cleanly without Docker locally, but never let CI pass silently.
	if os.Getenv("CI") != "" {
		provider, err := testcontainers.ProviderDocker.GetProvider()
		if err == nil {
			err = provider.Health(context.Background())
		}
		if err != nil {
			t.Fatalf("docker is required in CI: %v", err)
		}
	} else {
		testcontainers.SkipIfProviderIsNotHealthy(t)
	}

	ctx := context.Background()

	// Postgres.
	const dbName, dbUser, dbPass = "rmbl_e2e", "postgres", "password"
	pg, err := postgres.Run(ctx, "postgres:15-alpine",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPass),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	if pg != nil {
		t.Cleanup(func() { _ = pg.Terminate(context.Background()) })
	}
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	dbHost, err := pg.Host(ctx)
	if err != nil {
		t.Fatalf("postgres host: %v", err)
	}
	dbPort, err := pg.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("postgres port: %v", err)
	}

	// Build the real binary.
	bin := filepath.Join(t.TempDir(), "ramble")
	buildCtx, cancelBuild := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	// Free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	portStr := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()

	// The secrets double as leak sentinels: they must never appear in output.
	sentinel := "e2esentinel" + randHex(t)
	resSecret := "e2eressecret" + randHex(t)
	ghSecret := "e2eghsecret" + randHex(t)

	var stdout, stderr syncBuffer
	// Explicit env: an inherited DATABASE_URL or SENTRY_DSN would change behaviour.
	cmd := exec.Command(bin, "server", "--port", portStr)
	cmd.Dir = repoRoot // ./views and ./public are relative
	cmd.Env = []string{
		"ENV=production",
		"DB_HOST=" + dbHost,
		"DB_PORT=" + dbPort.Port(),
		"DB_USER=" + dbUser,
		"DB_PASSWORD=" + dbPass,
		"DB_NAME=" + dbName,
		"DB_SSLMODE=disable", // production defaults to require; the container has no TLS
		"BASE_URL=https://example.test",
		"TRUSTED_PROXIES=127.0.0.1/32",
		"GITHUB_WEBHOOK_SECRET=" + ghSecret,
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.TempDir(),
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		select {
		case <-exited:
		default:
			_ = cmd.Process.Kill()
			<-exited
		}
	})
	dump := func() string {
		return "stdout:\n" + stdout.String() + "\nstderr:\n" + stderr.String()
	}

	// Readiness: server.started is logged before Listen binds, so also dial the port.
	// A TCP dial, not HTTP: a probe request would add an http.request line.
	deadline := time.Now().Add(60 * time.Second)
	for ready := false; !ready; {
		select {
		case <-exited:
			t.Fatalf("server exited before becoming ready\n%s", dump())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not ready within 60s\n%s", dump())
		}
		if strings.Contains(stdout.String(), `"msg":"server.started"`) {
			if c, err := net.DialTimeout("tcp", "127.0.0.1:"+portStr, time.Second); err == nil {
				_ = c.Close()
				ready = true
				continue
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Seed a resource with a known webhook secret (migrations have run by now).
	resID := seedResource(t, net.JoinHostPort(dbHost, dbPort.Port()), dbUser, dbPass, dbName, resSecret)

	// Requests.
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	base := "http://127.0.0.1:" + portStr
	var reqs []e2eRequest
	do := func(name, method, path string, form url.Values, hdr map[string]string) int {
		t.Helper()
		req, err := http.NewRequest(method, base+path, strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Forwarded-Proto", "https")
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, dump())
		}
		_ = resp.Body.Close()
		reqs = append(reqs, e2eRequest{name, resp.Header.Get("X-Request-ID"), resp.StatusCode})
		return resp.StatusCode
	}

	doRaw := func(name, path string, body []byte, hdr map[string]string) int {
		t.Helper()
		req, err := http.NewRequest("POST", base+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Forwarded-Proto", "https")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, dump())
		}
		_ = resp.Body.Close()
		reqs = append(reqs, e2eRequest{name, resp.Header.Get("X-Request-ID"), resp.StatusCode})
		return resp.StatusCode
	}

	if s := do("home", "GET", "/", nil, nil); s != 200 {
		t.Errorf("GET / = %d, want 200\n%s", s, dump())
	}
	if s := do("not-found", "GET", "/no-such-page", nil, nil); s != 404 {
		t.Errorf("GET /no-such-page = %d, want 404\n%s", s, dump())
	}
	if s := do("query", "GET", "/?token="+sentinel+"&q="+sentinel, nil, nil); s != 200 {
		t.Errorf("GET /?token= = %d, want 200\n%s", s, dump())
	}
	// A normal form route without a CSRF token must still be rejected by CSRF (exactly 403).
	if s := do("login", "POST", "/login", url.Values{"username": {"x"}, "password": {sentinel}}, nil); s != 403 {
		t.Errorf("POST /login = %d, want 403", s)
	}

	// Resource webhook, HMAC + timestamp, no event headers (no remote fetch).
	resPath := "/resource/" + strconv.FormatUint(resID, 10) + "/webhook"
	body := []byte(`{}`)
	signedHdr := func(secret string) map[string]string {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		return map[string]string{
			"X-Webhook-Timestamp": ts,
			"X-Hub-Signature-256": hmacSig(secret, ts+"."+string(body)),
			"Content-Type":        "application/json",
		}
	}
	if s := doRaw("webhook-signed", resPath, body, signedHdr(resSecret)); s < 200 || s > 299 {
		t.Errorf("signed resource webhook = %d, want 2xx\n%s", s, dump())
	}
	if s := doRaw("webhook-bad-signature", resPath, body, signedHdr("wrong-secret")); s != 401 {
		t.Errorf("badly signed resource webhook = %d, want 401 from the handler\n%s", s, dump())
	}

	// GitHub issue webhook in the signature-only form GitHub sends.
	if s := doRaw("github-signed", "/webhooks/github/issues", body, map[string]string{
		"X-Hub-Signature-256": hmacSig(ghSecret, string(body)),
		"X-GitHub-Event":      "ping",
		"Content-Type":        "application/json",
	}); s != 200 {
		t.Errorf("signed github webhook = %d, want 200\n%s", s, dump())
	}
	if s := do("github-bad-signature", "POST", "/webhooks/github/issues", url.Values{"payload": {"{}"}},
		map[string]string{"X-Hub-Signature-256": "sha256=deadbeef"}); s != 401 {
		t.Errorf("badly signed github webhook = %d, want 401 from the handler\n%s", s, dump())
	}

	// The reset route is a logged-in form action and must keep CSRF.
	if s := do("webhook-reset", "POST", resPath+"/reset", nil, nil); s != 403 {
		t.Errorf("POST %s/reset = %d, want 403", resPath, s)
	}

	// Stop: SIGTERM, then kill. The exit code is not asserted (no graceful handler).
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-exited
	}

	out, errOut := stdout.String(), stderr.String()

	if strings.TrimSpace(errOut) != "" {
		t.Errorf("stderr must be empty, got:\n%s", errOut)
	}
	for name, s := range map[string]string{"stdout": out, "stderr": errOut} {
		if strings.Contains(s, "\x1b[") {
			t.Errorf("%s contains ANSI escapes:\n%s", name, s)
		}
		for _, secret := range []string{sentinel, resSecret, ghSecret} {
			if strings.Contains(s, secret) {
				t.Errorf("%s leaks a secret:\n%s", name, s)
			}
		}
	}

	seen := map[string]bool{}
	httpLines := map[string]int{} // request_id -> count
	httpTotal := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Errorf("stdout line is not a single JSON object (%v): %q", err, line)
			continue
		}
		if ts, _ := m["time"].(string); ts == "" {
			t.Errorf("missing time: %q", line)
		}
		switch m["level"] {
		case "DEBUG", "INFO", "WARN", "ERROR":
		default:
			t.Errorf("level not DEBUG|INFO|WARN|ERROR: %q", line)
		}
		msg, _ := m["msg"].(string)
		if !eventKeyRe.MatchString(msg) {
			t.Errorf("msg is not a dotted event key: %q", line)
		}
		seen[msg] = true
		if msg == "http.request" {
			httpTotal++
			id, _ := m["request_id"].(string)
			if id == "" {
				t.Errorf("http.request without request_id: %q", line)
			}
			httpLines[id]++
		}
	}

	// Rejections must come from the handlers (log keys), not from CSRF.
	for _, want := range []string{"webhook.validated", "webhook.validation_failed", "webhook.signature_invalid"} {
		if !seen[want] {
			t.Errorf("handler line %q missing from stdout:\n%s", want, out)
		}
	}
	for _, want := range []string{"db.connected", "db.migrate_completed", "server.started"} {
		if !seen[want] {
			t.Errorf("startup line %q missing from stdout:\n%s", want, out)
		}
	}
	if httpTotal != len(reqs) {
		t.Errorf("http.request lines = %d, want %d (one per request)\n%s", httpTotal, len(reqs), out)
	}
	for _, r := range reqs {
		if r.requestID == "" {
			t.Errorf("%s: response has no X-Request-ID", r.name)
			continue
		}
		if n := httpLines[r.requestID]; n != 1 {
			t.Errorf("%s (status %d): %d http.request lines for request_id %q, want 1\n%s", r.name, r.status, n, r.requestID, out)
		}
	}
}

func randHex(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func hmacSig(secret, payload string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// seedResource inserts a user and a resource with the given webhook secret into
// the migrated schema and returns the resource id.
func seedResource(t *testing.T, hostPort, user, pass, name, secret string) uint64 {
	t.Helper()
	dsn := "postgres://" + user + ":" + pass + "@" + hostPort + "/" + name + "?sslmode=disable"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer db.Close()
	var userID uint64
	if err := db.QueryRow(`INSERT INTO users (created_at, updated_at, username, email) VALUES (now(), now(), 'e2eowner', 'e2eowner@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var id uint64
	if err := db.QueryRow(`INSERT INTO nomad_resources (created_at, updated_at, name, type, webhook_secret, user_id) VALUES (now(), now(), 'e2e-res', 'pack', $1, $2) RETURNING id`, secret, userID).Scan(&id); err != nil {
		t.Fatalf("seed resource: %v", err)
	}
	return id
}
