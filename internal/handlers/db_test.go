package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"rmbl/internal/database"
	"rmbl/internal/services/logger"
)

// newRequestLogApp returns a fiber app wired like server.Run: requestid, then
// the real logger.RequestLogger built from base, so handler tests exercise the
// production middleware rather than a copy.
func newRequestLogApp(base zerolog.Logger, sbase *slog.Logger) *fiber.App {
	app := fiber.New()
	app.Use(requestid.New())
	app.Use(logger.RequestLogger(&base, sbase))
	return app
}

// testSlog returns a JSON slog logger on w at DEBUG, like production gating.
// A writer shared with goroutines must be goroutine-safe (lockedBuffer).
func testSlog(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// TestReqDB_QueryErrorCarriesRequestID proves a query run through reqDB logs
// via the request-scoped logger (and so carries request_id), not the global
// GORM base logger.
func TestReqDB_QueryErrorCarriesRequestID(t *testing.T) {
	var baseBuf, reqBuf bytes.Buffer
	baseLog := testSlog(&baseBuf)
	reqBase := zerolog.New(&reqBuf)

	orig := database.DB
	database.DB = orig.Session(&gorm.Session{Logger: database.NewGormLogger(baseLog, database.SlowQueryThreshold)})
	t.Cleanup(func() { database.DB = orig })

	app := newRequestLogApp(reqBase, testSlog(&reqBuf))
	app.Get("/q", func(c *fiber.Ctx) error {
		var x []map[string]any
		_ = reqDB(c).Raw("SELECT * FROM no_such_table_63").Scan(&x).Error
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/q", nil)
	req.Header.Set("X-Request-ID", "rid-63-test")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var found bool
	for line := range bytes.SplitSeq(bytes.TrimSpace(reqBuf.Bytes()), []byte("\n")) {
		var m map[string]any
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		if m[zerolog.MessageFieldName] == "db.query_error" {
			found = true
			assert.Equal(t, "rid-63-test", m["request_id"])
		}
	}
	assert.True(t, found, "db.query_error not logged via request logger: %s", reqBuf.String())
	assert.Empty(t, baseBuf.String(), "query leaked to the global GORM logger")
}

// TestRequestChain_QueryErrorSharesRequestIDWithAccessLine runs the production
// chain (requestid, logger.RequestLogger, reqDB, GORM logger) with one base
// logger and one buffer, as in prod, and proves the db.query_error line and
// the http.request line of a request carry the same generated request_id.
func TestRequestChain_QueryErrorSharesRequestIDWithAccessLine(t *testing.T) {
	var buf bytes.Buffer
	base := zerolog.New(&buf)
	sbase := testSlog(&buf)

	orig := database.DB
	database.DB = orig.Session(&gorm.Session{Logger: database.NewGormLogger(sbase, database.SlowQueryThreshold)})
	t.Cleanup(func() { database.DB = orig })

	app := newRequestLogApp(base, sbase)
	app.Get("/chain", func(c *fiber.Ctx) error {
		var x []map[string]any
		_ = reqDB(c).Raw("SELECT * FROM no_such_table_81").Scan(&x).Error
		return c.SendString("ok")
	})

	pair := func() (access, dbErr map[string]any) {
		buf.Reset()
		// No X-Request-ID header: requestid must generate the id.
		resp, err := app.Test(httptest.NewRequest("GET", "/chain", nil))
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)

		var nAccess, nErr int
		for line := range bytes.SplitSeq(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
			var m map[string]any
			require.NoError(t, json.Unmarshal(line, &m), "non-JSON line: %s", line)
			switch m[zerolog.MessageFieldName] {
			case "http.request":
				nAccess++
				access = m
			case "db.query_error":
				nErr++
				dbErr = m
			}
		}
		require.Equal(t, 1, nAccess, "http.request lines: %s", buf.String())
		require.Equal(t, 1, nErr, "db.query_error lines: %s", buf.String())
		return access, dbErr
	}

	access1, err1 := pair()
	rid1, ok := access1["request_id"].(string)
	require.True(t, ok, "http.request has no request_id: %v", access1)
	assert.NotEmpty(t, rid1)
	assert.Equal(t, rid1, err1["request_id"], "db.query_error request_id differs from http.request")
	assert.Equal(t, float64(200), access1["status"])
	assert.Equal(t, "/chain", access1["path"])

	access2, err2 := pair()
	rid2, _ := access2["request_id"].(string)
	assert.NotEmpty(t, rid2)
	assert.Equal(t, rid2, err2["request_id"])
	assert.NotEqual(t, rid1, rid2, "request_id reused across requests")
}
