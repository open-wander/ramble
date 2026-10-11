package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"rmbl/internal/database"
)

// TestReqDB_QueryErrorCarriesRequestID proves a query run through reqDB logs
// via the request-scoped logger (and so carries request_id), not the global
// GORM base logger.
func TestReqDB_QueryErrorCarriesRequestID(t *testing.T) {
	var baseBuf, reqBuf bytes.Buffer
	baseLog := zerolog.New(&baseBuf)
	reqBase := zerolog.New(&reqBuf)

	orig := database.DB
	database.DB = orig.Session(&gorm.Session{Logger: database.NewGormLogger(&baseLog, database.SlowQueryThreshold)})
	t.Cleanup(func() { database.DB = orig })

	app := fiber.New()
	app.Use(requestid.New())
	// Mirrors server.requestLogger: attach a request-scoped logger to the user context.
	app.Use(func(c *fiber.Ctx) error {
		rid, _ := c.Locals(requestid.ConfigDefault.ContextKey).(string)
		l := reqBase.With().Str("request_id", rid).Logger()
		c.SetUserContext(l.WithContext(c.UserContext()))
		return c.Next()
	})
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
	for _, line := range bytes.Split(bytes.TrimSpace(reqBuf.Bytes()), []byte("\n")) {
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
