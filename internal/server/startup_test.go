package server

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"rmbl/internal/services/logger"
	"rmbl/internal/testutil/logtest"
)

func TestFiberConfigDisablesStartupMessage(t *testing.T) {
	cfg := fiberConfig(nil)
	if !cfg.DisableStartupMessage {
		t.Error("fiberConfig must set DisableStartupMessage")
	}
	if !fiber.New(cfg).Config().DisableStartupMessage {
		t.Error("DisableStartupMessage did not survive fiber.New")
	}
	// The extraction must not drop the proxy settings: c.IP() keys the rate limiters.
	if !cfg.EnableTrustedProxyCheck {
		t.Error("EnableTrustedProxyCheck lost")
	}
	if !cfg.EnableIPValidation {
		t.Error("EnableIPValidation lost")
	}
	if cfg.ProxyHeader != fiber.HeaderXForwardedFor {
		t.Errorf("ProxyHeader = %q", cfg.ProxyHeader)
	}
}

func TestLogStarted(t *testing.T) {
	var buf bytes.Buffer
	l := zerolog.New(&buf).With().Timestamp().Logger()
	logStarted(&l, ":3000", "v1.2.3")

	lines := logtest.Lines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	line := lines[0]
	want := map[string]string{
		zerolog.MessageFieldName: "server.started",
		zerolog.LevelFieldName:   "info",
		"addr":                   ":3000",
		"version":                "v1.2.3",
	}
	for k, v := range want {
		if line[k] != v {
			t.Errorf("%s = %v, want %q", k, line[k], v)
		}
	}
	if _, ok := line[zerolog.TimestampFieldName]; !ok {
		t.Error("time field missing")
	}
}

// requestid must run before RequestLogger, and both must be registered: the
// access line and a handler line share the id echoed in X-Request-ID.
func TestUseRequestLogging(t *testing.T) {
	var buf bytes.Buffer
	l := zerolog.New(&buf)

	app := fiber.New()
	useRequestLogging(app, &l)
	app.Get("/x", func(c *fiber.Ctx) error {
		logger.FromContext(c.UserContext()).Info().Msg("handler.ran")
		return c.SendStatus(fiber.StatusOK)
	})

	// No inbound X-Request-ID: requestid would echo it and weaken the check.
	resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
	if err != nil {
		t.Fatal(err)
	}
	hdr := resp.Header.Get(fiber.HeaderXRequestID)
	if hdr == "" {
		t.Fatal("X-Request-ID response header is empty")
	}

	lines := logtest.Lines(t, &buf)
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2: %s", len(lines), buf.String())
	}
	count := map[string]int{}
	for _, m := range lines {
		msg, _ := m[zerolog.MessageFieldName].(string)
		count[msg]++
		if id, _ := m["request_id"].(string); id == "" || id != hdr {
			t.Errorf("%s: request_id = %v, want %q", msg, m["request_id"], hdr)
		}
	}
	if count["http.request"] != 1 || count["handler.ran"] != 1 {
		t.Errorf("line counts = %v, want one http.request and one handler.ran", count)
	}
}
