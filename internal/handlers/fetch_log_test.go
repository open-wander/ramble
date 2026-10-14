package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	"rmbl/internal/services/resource"
)

// failFastRepoURL is neither github.com nor gitlab.com, so downloadFile returns
// "unsupported repository host or file not found" without touching the
// network, and isPermanentError ("not found") skips the backoff retries. An
// empty URL would make the fetch succeed instead.
const failFastRepoURL = "https://example.invalid/o/r"

// lockedBuffer is a goroutine-safe log sink: fetch goroutines write while the
// test polls.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// findLogLine returns the first line with the given msg whose sql field
// contains every one of sqlParts, or nil.
func (b *lockedBuffer) findLogLine(msg string, sqlParts ...string) map[string]any {
	for line := range strings.SplitSeq(b.String(), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil || m["msg"] != msg {
			continue
		}
		sql, _ := m["sql"].(string)
		ok := true
		for _, p := range sqlParts {
			ok = ok && strings.Contains(sql, p)
		}
		if ok {
			return m
		}
	}
	return nil
}

// eventually polls cond until it holds or a few seconds pass.
func eventually(cond func() bool) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// fetchLogHarness returns a request-scoped log sink (reqBuf) and a separate
// sink for the global loggers (baseBuf). database.DB and resource.Service log
// every query as db.slow_query (1ns threshold), so the SQL lines of a fetch
// show which logger they were bound to. Both globals are restored on cleanup.
func fetchLogHarness(t *testing.T) (reqBuf, baseBuf *lockedBuffer) {
	t.Helper()
	reqBuf, baseBuf = &lockedBuffer{}, &lockedBuffer{}
	baseLog := testSlog(baseBuf)

	origDB, origSvc, origS := database.DB, resource.Service, logger.Log
	logger.Log = baseLog
	database.DB = origDB.Session(&gorm.Session{Logger: database.NewGormLogger(baseLog, time.Nanosecond)})
	resource.Service = resource.NewResourceService(database.DB)
	t.Cleanup(func() {
		database.DB = origDB
		resource.Service = origSvc
		logger.Log = origS
	})
	return reqBuf, baseBuf
}

// requireFetchLinesCarryRequestID asserts that version_fetch.failed and the
// two resource_versions UPDATEs of a failed fetch (status fetching, then
// RecordFetchFailed) are on the request logger with the request's id, and that
// the fetch did not log through the global logger. It waits for the last of
// them, so the fetch goroutine is done when it returns.
func requireFetchLinesCarryRequestID(t *testing.T, reqBuf, baseBuf *lockedBuffer, rid string) {
	t.Helper()
	require.True(t, eventually(func() bool {
		return reqBuf.findLogLine("db.slow_query", `UPDATE "resource_versions"`, "fetch_completed_at") != nil
	}), "RecordFetchFailed UPDATE not logged on the request logger:\nreq: %s\nbase: %s", reqBuf.String(), baseBuf.String())

	for _, want := range []struct {
		msg string
		sql []string
	}{
		{"download.permanent_error", nil},
		{"version_fetch.failed", nil},
		{"db.slow_query", []string{`UPDATE "resource_versions"`, "fetch_started_at"}},
		{"db.slow_query", []string{`UPDATE "resource_versions"`, "fetch_completed_at"}},
	} {
		line := reqBuf.findLogLine(want.msg, want.sql...)
		require.NotNil(t, line, "%s %v missing on the request logger: %s", want.msg, want.sql, reqBuf.String())
		assert.Equal(t, rid, line["request_id"], "%s %v", want.msg, want.sql)
	}
	assert.Nil(t, baseBuf.findLogLine("version_fetch.failed"), "version_fetch.failed leaked to the global logger")
	assert.Nil(t, baseBuf.findLogLine("download.permanent_error"), "download.permanent_error leaked to the global logger")
}

// The goroutine's lookup of the fetched version (finishWebhookFetch, shared by
// both webhook paths) is bound to the request context too.
func TestHandleWebhook_FetchLogsCarryRequestID(t *testing.T) {
	tests := []struct {
		name  string
		event string
		body  string
		// versions to pre-create; the handler refreshes the latest one when
		// the request is not a new-tag event.
		versions []string
	}{
		{name: "latest refresh", versions: []string{"v1.0.0"}},
		{name: "new tag", event: "create", body: `{"ref":"v9.9.9","ref_type":"tag"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer cleanupTestData(t)

			user := createTestUser(t, "fetchlogwh-"+strings.ReplaceAll(tt.name, " ", "-"))
			const secret = "fetch-log-secret"
			res := models.NomadResource{
				Name:          "fetch-log-wh",
				Type:          models.ResourceTypePack,
				UserID:        user.ID,
				WebhookSecret: secret,
				RepositoryURL: failFastRepoURL,
			}
			require.NoError(t, database.DB.Create(&res).Error)
			for _, v := range tt.versions {
				require.NoError(t, database.DB.Create(&models.ResourceVersion{ResourceID: res.ID, Version: v}).Error)
			}

			reqBuf, baseBuf := fetchLogHarness(t)
			app := newRequestLogApp(testSlog(reqBuf))
			app.Post("/webhook/:id", HandleWebhook)

			ts := fmt.Sprintf("%d", time.Now().Unix())
			req := httptest.NewRequest("POST", "/webhook/"+toString(res.ID), strings.NewReader(tt.body))
			req.Header.Set("X-Request-ID", "rid-85-wh")
			req.Header.Set("X-Webhook-Timestamp", ts)
			req.Header.Set("X-Hub-Signature-256", computeExpectedSignature([]byte(tt.body), ts, secret))
			if tt.event != "" {
				req.Header.Set("X-GitHub-Event", tt.event)
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// webhook.fetch_failed is the goroutine's last line, written after its
			// DB update: waiting for it means the goroutine is done before cleanup.
			require.True(t, eventually(func() bool {
				return reqBuf.findLogLine("webhook.fetch_failed") != nil
			}), "goroutine did not finish: %s", reqBuf.String())

			requireFetchLinesCarryRequestID(t, reqBuf, baseBuf, "rid-85-wh")

			// The goroutine's own follow-up write (not the in-request Save, which
			// filters on "id" rather than id) is bound to the same context.
			line := reqBuf.findLogLine("db.slow_query", `UPDATE "nomad_resources"`, "last_webhook_error", "WHERE id = ")
			require.NotNil(t, line, "webhook goroutine UPDATE not on the request logger: %s", reqBuf.String())
			assert.Equal(t, "rid-85-wh", line["request_id"])

			requireSQLOnRequestLogger(t, reqBuf, baseBuf, "rid-85-wh",
				`SELECT * FROM "resource_versions"`, "(resource_id = ", "AND version = ", "LIMIT")
		})
	}
}

func TestRequestTriggeredFetch_LogsCarryRequestID(t *testing.T) {
	tests := []struct {
		name string
		// request registers its route on app and returns the request to send.
		request func(t *testing.T, app *fiber.App, user models.User) *http.Request
	}{
		{"PostNewResource", func(t *testing.T, app *fiber.App, user models.User) *http.Request {
			app.Post("/new", PostNewResource)
			form := "name=fetch-log-new&type=pack&version=v1.0.0&description=d&repository_url=" + failFastRepoURL
			req := httptest.NewRequest("POST", "/new", strings.NewReader(form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return req
		}},
		{"PostNewVersion", func(t *testing.T, app *fiber.App, user models.User) *http.Request {
			res := models.NomadResource{Name: "fetch-log-ver", Type: models.ResourceTypePack, UserID: user.ID, RepositoryURL: failFastRepoURL}
			require.NoError(t, database.DB.Create(&res).Error)
			app.Post("/res/:id/versions", PostNewVersion)
			req := httptest.NewRequest("POST", "/res/"+toString(res.ID)+"/versions", strings.NewReader("version=v2.0.0"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return req
		}},
		{"PostRetryFetch", func(t *testing.T, app *fiber.App, user models.User) *http.Request {
			res := models.NomadResource{Name: "fetch-log-retry", Type: models.ResourceTypePack, UserID: user.ID, RepositoryURL: failFastRepoURL}
			require.NoError(t, database.DB.Create(&res).Error)
			require.NoError(t, database.DB.Create(&models.ResourceVersion{
				ResourceID: res.ID, Version: "v1.0.0", FetchStatus: models.FetchStatusFailed,
			}).Error)
			app.Post("/res/:id/retry", PostRetryFetch)
			return httptest.NewRequest("POST", "/res/"+toString(res.ID)+"/retry", nil)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer cleanupTestData(t)
			user := createTestUser(t, "fetchlogcrud-"+tt.name)
			reqBuf, baseBuf := fetchLogHarness(t)

			InitSession()
			app := newRequestLogApp(testSlog(reqBuf))
			app.Use(func(c *fiber.Ctx) error {
				sess, _ := Store.Get(c)
				sess.Set("user_id", user.ID)
				_ = sess.Save()
				c.Locals("UserID", user.ID)
				c.Locals("User", user)
				return c.Next()
			})

			req := tt.request(t, app, user)
			req.Header.Set("X-Request-ID", "rid-85-crud")
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			requireFetchLinesCarryRequestID(t, reqBuf, baseBuf, "rid-85-crud")
		})
	}
}

// A fetch with no request logger in its context (a true background job) keeps
// logging through the global logger.
func TestFetchVersionContent_NoContextLoggerUsesGlobal(t *testing.T) {
	defer cleanupTestData(t)
	user := createTestUser(t, "fetchlogglobal")
	res := models.NomadResource{Name: "fetch-log-global", Type: models.ResourceTypePack, UserID: user.ID, RepositoryURL: failFastRepoURL}
	require.NoError(t, database.DB.Create(&res).Error)
	require.NoError(t, database.DB.Create(&models.ResourceVersion{ResourceID: res.ID, Version: "v1.0.0"}).Error)

	var buf lockedBuffer
	orig := logger.Log
	logger.Log = testSlog(&buf)
	t.Cleanup(func() { logger.Log = orig })

	fetchVersionContent(context.Background(), res.ID, "v1.0.0", failFastRepoURL, string(res.Type), res.Name, "")

	line := buf.findLogLine("version_fetch.failed")
	require.NotNil(t, line, "version_fetch.failed not on the global logger: %s", buf.String())
	assert.NotContains(t, line, "request_id")
}

// reqLogCtx builds a context carrying a request logger on w with request_id
// rid, as logger.RequestLogger does.
func reqLogCtx(w *lockedBuffer, rid string) context.Context {
	return logger.WithContext(context.Background(), testSlog(w).With(slog.String("request_id", rid)))
}

// requireSQLOnRequestLogger asserts that the db.slow_query line for the
// statement matching sqlParts is on the request logger with rid, and that no
// such statement went through the global logger (which is where it lands when
// WithContext(ctx) is dropped).
func requireSQLOnRequestLogger(t *testing.T, reqBuf, baseBuf *lockedBuffer, rid string, sqlParts ...string) {
	t.Helper()
	line := reqBuf.findLogLine("db.slow_query", sqlParts...)
	require.NotNil(t, line, "%v not on the request logger:\nreq: %s\nbase: %s", sqlParts, reqBuf.String(), baseBuf.String())
	assert.Equal(t, rid, line["request_id"], "%v", sqlParts)
	assert.Nil(t, baseBuf.findLogLine("db.slow_query", sqlParts...), "%v leaked to the global logger", sqlParts)
}

// The panic path persists the failure through the request context, so the
// RecordFetchFailed UPDATE carries the request_id.
func TestFetchVersionContent_PanicRecordCarriesRequestID(t *testing.T) {
	defer cleanupTestData(t)
	user := createTestUser(t, "fetchlogpanic")
	res := models.NomadResource{Name: "fetch-log-panic", Type: models.ResourceTypePack, UserID: user.ID, RepositoryURL: failFastRepoURL}
	require.NoError(t, database.DB.Create(&res).Error)
	require.NoError(t, database.DB.Create(&models.ResourceVersion{ResourceID: res.ID, Version: "v1.0.0"}).Error)

	reqBuf, baseBuf := fetchLogHarness(t)
	fetchVersionHook = func() { panic("boom") }
	t.Cleanup(func() { fetchVersionHook = nil })

	ctx := reqLogCtx(reqBuf, "rid-810-panic")
	fetchVersionContent(ctx, res.ID, "v1.0.0", failFastRepoURL, string(res.Type), res.Name, "")

	line := reqBuf.findLogLine("version_fetch.panic")
	require.NotNil(t, line, "version_fetch.panic not on the request logger: %s", reqBuf.String())
	assert.Equal(t, "rid-810-panic", line["request_id"])
	requireSQLOnRequestLogger(t, reqBuf, baseBuf, "rid-810-panic", `UPDATE "resource_versions"`, "fetch_completed_at")

	var v models.ResourceVersion
	require.NoError(t, database.DB.Where("resource_id = ? AND version = ?", res.ID, "v1.0.0").First(&v).Error)
	assert.Equal(t, models.FetchStatusFailed, v.FetchStatus)
}

// The success path's final content UPDATE carries the request_id. An empty
// repository URL makes every download succeed with no network.
func TestFetchVersionContent_SuccessUpdateCarriesRequestID(t *testing.T) {
	defer cleanupTestData(t)
	user := createTestUser(t, "fetchlogok")
	res := models.NomadResource{Name: "fetch-log-ok", Type: models.ResourceTypePack, UserID: user.ID}
	require.NoError(t, database.DB.Create(&res).Error)
	require.NoError(t, database.DB.Create(&models.ResourceVersion{ResourceID: res.ID, Version: "v1.0.0"}).Error)

	reqBuf, baseBuf := fetchLogHarness(t)
	ctx := reqLogCtx(reqBuf, "rid-810-ok")
	fetchVersionContent(ctx, res.ID, "v1.0.0", "", string(res.Type), res.Name, "")

	line := reqBuf.findLogLine("version_fetch.completed")
	require.NotNil(t, line, "version_fetch.completed not on the request logger: %s", reqBuf.String())
	assert.Equal(t, "rid-810-ok", line["request_id"])
	requireSQLOnRequestLogger(t, reqBuf, baseBuf, "rid-810-ok", `UPDATE "resource_versions"`, `"readme"`, "fetch_completed_at")

	var v models.ResourceVersion
	require.NoError(t, database.DB.Where("resource_id = ? AND version = ?", res.ID, "v1.0.0").First(&v).Error)
	assert.Equal(t, models.FetchStatusCompleted, v.FetchStatus)
}

// duration is milliseconds as a JSON float, not
// the integer nanoseconds slog.Duration would write. A nanosecond count is
// orders of magnitude above the elapsed wall time in milliseconds.
func TestFetchVersionContent_DurationIsMilliseconds(t *testing.T) {
	tests := []struct {
		name    string
		resType models.ResourceType
		msg     string
	}{
		{"completed", models.ResourceType("other"), "version_fetch.completed"},
		{"failed", models.ResourceTypePack, "version_fetch.failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer cleanupTestData(t)
			user := createTestUser(t, "fetchdur-"+tt.name)
			res := models.NomadResource{Name: "fetch-dur-" + tt.name, Type: tt.resType, UserID: user.ID, RepositoryURL: failFastRepoURL}
			require.NoError(t, database.DB.Create(&res).Error)
			require.NoError(t, database.DB.Create(&models.ResourceVersion{ResourceID: res.ID, Version: "v1.0.0"}).Error)

			var buf lockedBuffer
			start := time.Now()
			fetchVersionContent(reqLogCtx(&buf, "rid-dur"), res.ID, "v1.0.0", failFastRepoURL, string(tt.resType), res.Name, "")
			elapsedMs := float64(time.Since(start)) / float64(time.Millisecond)

			line := buf.findLogLine(tt.msg)
			require.NotNil(t, line, "%s not logged: %s", tt.msg, buf.String())
			d, ok := line["duration"].(float64)
			require.True(t, ok, "duration is not a JSON number: %v", line["duration"])
			assert.Greater(t, d, 0.0)
			assert.LessOrEqual(t, d, elapsedMs, "duration is not in milliseconds")
		})
	}
}
