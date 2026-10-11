package server

import (
	"bytes"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
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

	lines := logLines(t, &buf)
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
