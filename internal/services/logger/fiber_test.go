package logger

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/rs/zerolog"
)

func newLoggedApp(t *testing.T) (*fiber.App, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	base := zerolog.New(&buf).With().Timestamp().Logger()

	app := fiber.New()
	app.Use(requestid.New())
	app.Use(RequestLogger(&base))
	app.Get("/ok", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/boom", func(c *fiber.Ctx) error { return fiber.NewError(fiber.StatusInternalServerError, "boom") })
	app.Get("/bad", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusBadRequest) })
	app.Get("/inner", func(c *fiber.Ctx) error {
		FromContext(c.UserContext()).Info().Msg("handler.event")
		return c.SendStatus(fiber.StatusNoContent)
	})
	return app, &buf
}

func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
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

			lines := logLines(t, buf)
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

	lines := logLines(t, buf)
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
