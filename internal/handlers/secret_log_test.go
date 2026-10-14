package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	"rmbl/internal/services/resource"
	"rmbl/internal/testutil/logtest"
)

// captureAllLogs routes the global logger, the request logger and every SQL
// statement (slow threshold 0, so all of them are logged) into one buffer, so
// "absent from the buffer" means absent from all output.
func captureAllLogs(t *testing.T) (*bytes.Buffer, *slog.Logger) {
	t.Helper()
	var buf bytes.Buffer
	sbase := testSlog(&buf)

	origS := logger.S
	logger.S = sbase
	origDB := database.DB
	database.DB = origDB.Session(&gorm.Session{Logger: database.NewGormLogger(sbase, 0)})
	t.Cleanup(func() {
		logger.S = origS
		database.DB = origDB
	})
	return &buf, sbase
}

func requireMsg(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	m := logtest.Find(t, buf, msg)
	if m == nil {
		t.Fatalf("expected log line %q not found in: %s", msg, buf.String())
	}
	return m
}

func requireNoSecrets(t *testing.T, buf *bytes.Buffer, secrets ...string) {
	t.Helper()
	out := buf.String()
	for _, s := range secrets {
		assert.NotContains(t, out, s)
	}
}

type auditCreds struct {
	User          string
	WebhookSecret string `json:"webhook_secret"`
	Password      string
}

func TestRedactDetails(t *testing.T) {
	ptr := &auditCreds{User: "u", WebhookSecret: "S", Password: "S"}
	redactedCreds := map[string]any{"User": "u", "webhook_secret": "[REDACTED]", "Password": "[REDACTED]"}
	tests := []struct {
		name string
		in   map[string]any
		want map[string]any
	}{
		{"nil", nil, nil},
		{"flat", map[string]any{"password": "S", "reason": "r"},
			map[string]any{"password": "[REDACTED]", "reason": "r"}},
		{"mixed case and separators", map[string]any{"Password": "S", "X-Api-Key": "S", "webhook_secret": "S", "Reset_Token": "S", "csrf_token": "S", "Authorization": "S", "Cookie": "S", "session_id": "S", "hub_signature": "S", "passwd": "S", "apikey": "S", "note": "n"},
			map[string]any{"Password": "[REDACTED]", "X-Api-Key": "[REDACTED]", "webhook_secret": "[REDACTED]", "Reset_Token": "[REDACTED]", "csrf_token": "[REDACTED]", "Authorization": "[REDACTED]", "Cookie": "[REDACTED]", "session_id": "[REDACTED]", "hub_signature": "[REDACTED]", "passwd": "[REDACTED]", "apikey": "[REDACTED]", "note": "n"}},
		{"nested map", map[string]any{"outer": map[string]any{"token": "S", "keep": 1}},
			map[string]any{"outer": map[string]any{"token": "[REDACTED]", "keep": 1}}},
		{"nested fiber.Map", map[string]any{"outer": fiber.Map{"secret": "S", "reason": "r"}},
			map[string]any{"outer": map[string]any{"secret": "[REDACTED]", "reason": "r"}}},
		{"nested string map", map[string]any{"hdr": map[string]string{"Authorization": "S", "ua": "u"}},
			map[string]any{"hdr": map[string]string{"Authorization": "[REDACTED]", "ua": "u"}}},
		{"slice of maps", map[string]any{"items": []any{map[string]any{"api_key": "S", "n": 1}, "plain"}},
			map[string]any{"items": []any{map[string]any{"api_key": "[REDACTED]", "n": 1}, "plain"}}},
		{"typed slice of maps", map[string]any{"items": []map[string]any{{"token": "S"}}},
			map[string]any{"items": []map[string]any{{"token": "[REDACTED]"}}}},
		{"struct", map[string]any{"c": auditCreds{User: "u", WebhookSecret: "S", Password: "S"}},
			map[string]any{"c": redactedCreds}},
		{"pointer to struct", map[string]any{"c": ptr},
			map[string]any{"c": redactedCreds}},
		{"struct nested in map", map[string]any{"outer": map[string]any{"c": auditCreds{User: "u", WebhookSecret: "S", Password: "S"}}},
			map[string]any{"outer": map[string]any{"c": redactedCreds}}},
		{"slice of structs", map[string]any{"cs": []auditCreds{{User: "u", WebhookSecret: "S", Password: "S"}}},
			map[string]any{"cs": []any{redactedCreds}}},
		{"large int kept exact", map[string]any{"c": struct{ N uint64 }{1<<63 + 1}},
			map[string]any{"c": map[string]any{"N": json.Number("9223372036854775809")}}},
		{"unmarshalable value", map[string]any{"ch": make(chan int), "reason": "r"},
			map[string]any{"ch": "[UNSERIALIZABLE]", "reason": "r"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Snapshot the input to prove it is not modified; skipped when the
			// input itself cannot be marshalled (the chan row).
			var before []byte
			if tt.in != nil {
				if b, err := json.Marshal(tt.in); err == nil {
					before = b
				}
			}
			got := redactDetails(tt.in)
			assert.Equal(t, tt.want, got)
			if before != nil {
				after, _ := json.Marshal(tt.in)
				assert.JSONEq(t, string(before), string(after), "caller's map must not be modified")
			}
			assert.Equal(t, "S", ptr.WebhookSecret, "caller's struct must not be modified")
		})
	}
}

func TestAuditLog_RedactsSensitiveDetails(t *testing.T) {
	const s1, s2, s3, s4, s5, s6 = "SENTINEL_AUDIT_ONE", "SENTINEL_AUDIT_TWO", "SENTINEL_AUDIT_THREE", "SENTINEL_AUDIT_FOUR", "SENTINEL_AUDIT_FIVE", "SENTINEL_AUDIT_SIX"
	newDetails := func() map[string]any {
		return map[string]any{
			"reason":         "kept-reason",
			"webhook_secret": s1,
			"nested":         fiber.Map{"X-Api-Key": s2, "ok": "kept-nested"},
			"list":           []any{map[string]any{"reset_token": s3}},
			"struct":         auditCreds{User: "kept-struct", WebhookSecret: s4},
			"ptr":            &auditCreds{User: "kept-ptr", Password: s5},
			"deep":           map[string]any{"c": auditCreds{WebhookSecret: s6}},
		}
	}

	check := func(t *testing.T, buf *bytes.Buffer, action string) {
		t.Helper()
		line := requireMsg(t, buf, "audit.recorded")
		assert.Equal(t, action, line["action"])
		details, _ := json.Marshal(line["details"])
		assert.Contains(t, string(details), "kept-reason")
		assert.Contains(t, string(details), "kept-nested")
		assert.Contains(t, string(details), "[REDACTED]")

		var audit models.AuditLog
		require.NoError(t, database.DB.Where("action = ?", action).First(&audit).Error)
		assert.Contains(t, audit.Details, "kept-reason")
		assert.Contains(t, audit.Details, "[REDACTED]")
		for _, s := range []string{s1, s2, s3, s4, s5, s6} {
			assert.NotContains(t, audit.Details, s, "stored Details")
		}
		requireNoSecrets(t, buf, s1, s2, s3, s4, s5, s6)
	}

	t.Run("AuditLog", func(t *testing.T) {
		buf, sbase := captureAllLogs(t)
		const action = "secret.audit_with_ctx"
		t.Cleanup(func() { database.DB.Exec("DELETE FROM audit_logs WHERE action = ?", action) })
		app := newRequestLogApp(sbase)
		app.Get("/a", func(c *fiber.Ctx) error {
			AuditLog(c, action, "t", 1, "n", newDetails())
			return c.SendStatus(200)
		})
		resp, err := app.Test(httptest.NewRequest("GET", "/a", nil))
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		check(t, buf, action)
	})

	t.Run("AuditLogNoContext", func(t *testing.T) {
		buf, _ := captureAllLogs(t)
		const action = "secret.audit_no_ctx"
		t.Cleanup(func() { database.DB.Exec("DELETE FROM audit_logs WHERE action = ?", action) })
		AuditLogNoContext(action, 0, "job", "t", 1, "n", newDetails())
		check(t, buf, action)
	})
}

func TestFailedLogin_NeverLogsPassword(t *testing.T) {
	defer cleanupTestData(t)
	const pw = "SENTINEL_LOGIN_PW"
	createTestUser(t, "secretlogin")

	for _, email := range []string{"nobody@test.com", "secretlogin@test.com"} {
		t.Run(email, func(t *testing.T) {
			buf, sbase := captureAllLogs(t)
			app := newRequestLogApp(sbase)
			app.Post("/login", PostLogin)

			req := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"email": {email}, "password": {pw}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, 401, resp.StatusCode)

			line := requireMsg(t, buf, "audit.recorded")
			assert.Equal(t, "auth.login_failed", line["action"])
			requireMsg(t, buf, "http.request")
			requireNoSecrets(t, buf, pw)
		})
	}
}

func TestForgotPassword_NeverLogsResetLink(t *testing.T) {
	defer cleanupTestData(t)
	createTestUser(t, "secretforgot")
	t.Setenv("BASE_URL", "https://SENTINELHOST.example")

	buf, sbase := captureAllLogs(t)
	app := newRequestLogApp(sbase)
	app.Post("/forgot-password", PostForgotPassword)

	req := httptest.NewRequest("POST", "/forgot-password", strings.NewReader(url.Values{"email": {"secretforgot@test.com"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err := app.Test(req)
	require.NoError(t, err)

	// SMTP is unconfigured in tests, so the send fails after the link is built.
	requireMsg(t, buf, "email.reset_send_failed")
	requireMsg(t, buf, "http.request")
	requireNoSecrets(t, buf, "SENTINELHOST", "/reset-password?token=")
}

func TestResetPassword_NeverLogsToken(t *testing.T) {
	const tokPost, tokGet = "SENTINEL_RESET_POST", "SENTINEL_RESET_GET"

	t.Run("POST invalid token", func(t *testing.T) {
		buf, sbase := captureAllLogs(t)
		app := newRequestLogApp(sbase)
		app.Post("/reset-password", PostResetPassword)

		form := url.Values{"token": {tokPost}, "password": {"Str0ng!Passw0rd#2026"}}
		req := httptest.NewRequest("POST", "/reset-password", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)

		line := requireMsg(t, buf, "audit.recorded")
		assert.Equal(t, "auth.password_reset_failed", line["action"])
		requireNoSecrets(t, buf, tokPost, "Str0ng!Passw0rd#2026")
	})

	t.Run("GET query token reaches http.request as path only", func(t *testing.T) {
		buf, sbase := captureAllLogs(t)
		app := newRequestLogApp(sbase)
		app.Get("/reset-password", GetResetPassword)

		resp, err := app.Test(httptest.NewRequest("GET", "/reset-password?token="+tokGet, nil))
		require.NoError(t, err)
		assert.Equal(t, 302, resp.StatusCode)

		line := requireMsg(t, buf, "http.request")
		assert.Equal(t, "/reset-password", line["path"])
		requireNoSecrets(t, buf, tokGet)
	})
}

func TestWebhooks_NeverLogSecretsOrSignatures(t *testing.T) {
	const (
		resSecret = "SENTINEL_WH_RES_SECRET"
		qSecret   = "SENTINEL_WH_QUERY_SECRET"
		sig       = "SENTINEL_WH_SIGNATURE"
		ghSecret  = "SENTINEL_WH_GH_SECRET"
		ghSig     = "SENTINEL_WH_GH_SIGNATURE"
	)

	t.Run("HandleWebhook", func(t *testing.T) {
		defer cleanupTestData(t)
		user := createTestUser(t, "secretwebhook")
		res := models.NomadResource{Name: "secret-wh-pack", Type: models.ResourceTypePack, UserID: user.ID, WebhookSecret: resSecret}
		require.NoError(t, database.DB.Create(&res).Error)

		buf, sbase := captureAllLogs(t)
		app := newRequestLogApp(sbase)
		app.Post("/webhook/:id", HandleWebhook)

		req := httptest.NewRequest("POST", "/webhook/"+toString(res.ID)+"?secret="+qSecret, nil)
		req.Header.Set("X-Hub-Signature-256", "sha256="+sig)
		req.Header.Set("X-Webhook-Timestamp", "1")
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 401, resp.StatusCode)

		requireMsg(t, buf, "webhook.validation_failed")
		requireNoSecrets(t, buf, resSecret, qSecret, sig)
	})

	t.Run("HandleGitHubIssueWebhook", func(t *testing.T) {
		t.Setenv("GITHUB_WEBHOOK_SECRET", ghSecret)
		buf, sbase := captureAllLogs(t)
		app := newRequestLogApp(sbase)
		app.Post("/webhooks/github/issues", HandleGitHubIssueWebhook)

		for _, ts := range []string{"", "1"} {
			req := httptest.NewRequest("POST", "/webhooks/github/issues", nil)
			req.Header.Set("X-Hub-Signature-256", "sha256="+ghSig)
			if ts != "" {
				req.Header.Set("X-Webhook-Timestamp", ts)
			}
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, 401, resp.StatusCode)
		}

		requireMsg(t, buf, "webhook.signature_invalid")
		requireMsg(t, buf, "webhook.validation_failed")
		requireNoSecrets(t, buf, ghSecret, ghSig)
	})
}

func TestStripURLUserinfo(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"no userinfo", "https://github.com/o/r", "https://github.com/o/r"},
		{"user and password", "https://user:SECRET@example.invalid/o/r", "https://example.invalid/o/r"},
		{"token in username", "https://ghp_SECRET@example.invalid/o/r", "https://example.invalid/o/r"},
		{"unparseable", "http://[::1", "[unparseable url]"},
		{"unparseable with secret", "https://user:SECRET@host:notaport/x", "[unparseable url]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripURLUserinfo(tt.in)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "SECRET")
		})
	}
}

func TestDownloadAndFetch_NeverLogRepoURLUserinfo(t *testing.T) {
	// example.invalid is not a GitHub or GitLab host, so no network call is made
	// and the "not found" error is permanent (no backoff sleep).
	for _, repo := range []string{
		"https://user:SENTINEL_URL_PW@example.invalid/o/r",
		"https://SENTINEL_URL_TOKEN@example.invalid/o/r",
	} {
		t.Run(repo, func(t *testing.T) {
			buf, _ := captureAllLogs(t)
			_, err := downloadFileWithRetry(context.Background(), repo, "metadata.hcl")
			require.Error(t, err)

			line := requireMsg(t, buf, "download.permanent_error")
			assert.Equal(t, "https://example.invalid/o/r", line["repo_url"])
			requireMsg(t, buf, "download.failed")
			requireNoSecrets(t, buf, "SENTINEL_URL_PW", "SENTINEL_URL_TOKEN")
		})
	}

	t.Run("fetchVersionContent failure and stored fetch_error", func(t *testing.T) {
		defer cleanupTestData(t)
		user := createTestUser(t, "secretfetch")
		res := models.NomadResource{Name: "secret-fetch-pack", Type: models.ResourceTypePack, UserID: user.ID}
		require.NoError(t, database.DB.Create(&res).Error)
		require.NoError(t, database.DB.Create(&models.ResourceVersion{ResourceID: res.ID, Version: "v1"}).Error)

		origSvc := resource.Service
		resource.Service = resource.NewResourceService(database.DB)
		t.Cleanup(func() { resource.Service = origSvc })

		buf, _ := captureAllLogs(t)
		fetchVersionContent(context.Background(), res.ID, "v1",
			"https://user:SENTINEL_FETCH_PW@example.invalid/o/r", string(models.ResourceTypePack), res.Name, "")

		requireMsg(t, buf, "version_fetch.failed")
		requireNoSecrets(t, buf, "SENTINEL_FETCH_PW")

		var v models.ResourceVersion
		require.NoError(t, database.DB.Where("resource_id = ? AND version = ?", res.ID, "v1").First(&v).Error)
		assert.Equal(t, models.FetchStatusFailed, v.FetchStatus)
		assert.NotContains(t, v.FetchError, "SENTINEL_FETCH_PW")
	})
}
