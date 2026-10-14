package logger

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/rs/zerolog"

	"rmbl/internal/testutil/logtest"
)

func newLoggedApp(t *testing.T) (*fiber.App, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	base := zerolog.New(&buf).With().Timestamp().Logger()

	sbase := slog.New(slog.NewJSONHandler(&buf, nil))

	app := fiber.New()
	app.Use(requestid.New())
	app.Use(RequestLogger(&base, sbase))
	app.Get("/ok", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/boom", func(c *fiber.Ctx) error { return fiber.NewError(fiber.StatusInternalServerError, "boom") })
	app.Get("/bad", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusBadRequest) })
	app.Get("/inner", func(c *fiber.Ctx) error {
		FromContext(c.UserContext()).Info().Msg("handler.event")
		return c.SendStatus(fiber.StatusNoContent)
	})
	app.Get("/sinner", func(c *fiber.Ctx) error {
		SFromContext(c.UserContext()).Info("handler.slog_event")
		return c.SendStatus(fiber.StatusNoContent)
	})
	return app, &buf
}

func TestRequestLogger(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantPath   string
		wantStatus int
		wantLevel  string
	}{
		{"ok", "/ok", "/ok", 200, "info"},
		{"query string stripped", "/ok?token=s3cret&email=a%40b.c", "/ok", 200, "info"},
		{"client error stays info", "/bad", "/bad", 400, "info"},
		{"unknown route 404 stays info", "/wp-login.php?x=1", "/wp-login.php", 404, "info"},
		{"returned error logs error with handled status", "/boom", "/boom", 500, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, buf := newLoggedApp(t)
			req := httptest.NewRequest("GET", tt.target, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("response status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			if strings.Contains(buf.String(), "s3cret") || strings.Contains(buf.String(), "?") {
				t.Fatalf("query string reached the log: %s", buf.String())
			}

			lines := logtest.Lines(t, buf)
			if len(lines) != 1 {
				t.Fatalf("want 1 line, got %d: %v", len(lines), lines)
			}
			got := lines[0]
			for _, k := range []string{"time", "level", zerolog.MessageFieldName, "request_id", "method", "path", "status", "duration_ms", "client_ip"} {
				if _, ok := got[k]; !ok {
					t.Errorf("missing field %q in %v", k, got)
				}
			}
			if got[zerolog.MessageFieldName] != "http.request" {
				t.Errorf("msg = %v, want http.request", got[zerolog.MessageFieldName])
			}
			if got["level"] != tt.wantLevel {
				t.Errorf("level = %v, want %s", got["level"], tt.wantLevel)
			}
			if got["method"] != "GET" || got["path"] != tt.wantPath {
				t.Errorf("method/path = %v %v, want GET %s", got["method"], got["path"], tt.wantPath)
			}
			if got["status"] != float64(tt.wantStatus) {
				t.Errorf("status = %v, want %d", got["status"], tt.wantStatus)
			}
			if rid := resp.Header.Get(fiber.HeaderXRequestID); rid == "" || got["request_id"] != rid {
				t.Errorf("request_id = %v, want response header %q", got["request_id"], rid)
			}
		})
	}
}

func TestRequestLoggerAttachesRequestIDToHandlerLogs(t *testing.T) {
	app, buf := newLoggedApp(t)
	resp, err := app.Test(httptest.NewRequest("GET", "/inner", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	rid := resp.Header.Get(fiber.HeaderXRequestID)

	lines := logtest.Lines(t, buf)
	if len(lines) != 2 {
		t.Fatalf("want handler line + access line, got %v", lines)
	}
	if lines[0][zerolog.MessageFieldName] != "handler.event" {
		t.Fatalf("first line = %v, want handler.event", lines[0])
	}
	for _, l := range lines {
		if l["request_id"] != rid {
			t.Errorf("line %v: request_id = %v, want %q", l[zerolog.MessageFieldName], l["request_id"], rid)
		}
	}
}

func TestRequestLoggerAttachesSlogLoggerWithRequestID(t *testing.T) {
	app, buf := newLoggedApp(t)
	resp, err := app.Test(httptest.NewRequest("GET", "/sinner", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	rid := resp.Header.Get(fiber.HeaderXRequestID)

	// The zerolog line's message key depends on whether Init ran, the slog
	// line's is always "msg", so look each up by its own key.
	var slogLine, accessLine map[string]any
	lines := logtest.Lines(t, buf)
	for _, l := range lines {
		if l["msg"] == "handler.slog_event" {
			slogLine = l
		}
		if l[zerolog.MessageFieldName] == "http.request" {
			accessLine = l
		}
	}
	if len(lines) != 2 || slogLine == nil || accessLine == nil {
		t.Fatalf("want slog handler line + access line, got %v", lines)
	}
	if rid == "" || slogLine["request_id"] != rid || accessLine["request_id"] != rid {
		t.Errorf("request_id slog=%v access=%v, want %q", slogLine["request_id"], accessLine["request_id"], rid)
	}
	if slogLine["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", slogLine["level"])
	}
}

func TestSFromContextOr(t *testing.T) {
	fb := slog.New(slog.DiscardHandler)
	attached := slog.New(slog.DiscardHandler)
	var nilCtx context.Context

	tests := []struct {
		name string
		ctx  context.Context
		want *slog.Logger
	}{
		{"no logger attached", context.Background(), fb},
		{"nil context", nilCtx, fb},
		{"attached logger", SWithContext(context.Background(), attached), attached},
		{"attached nil logger", SWithContext(context.Background(), nil), fb},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SFromContextOr(tt.ctx, fb); got != tt.want {
				t.Errorf("got %p, want %p", got, tt.want)
			}
		})
	}
	if got := SFromContext(context.Background()); got != S {
		t.Error("SFromContext without attached logger should return S")
	}
}

func TestLevelForStatus(t *testing.T) {
	tests := []struct {
		status int
		want   zerolog.Level
	}{
		{200, zerolog.InfoLevel},
		{301, zerolog.InfoLevel},
		{404, zerolog.InfoLevel},
		{429, zerolog.InfoLevel},
		{499, zerolog.InfoLevel},
		{500, zerolog.ErrorLevel},
		{503, zerolog.ErrorLevel},
	}
	for _, tt := range tests {
		if got := levelForStatus(tt.status); got != tt.want {
			t.Errorf("levelForStatus(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}
}
