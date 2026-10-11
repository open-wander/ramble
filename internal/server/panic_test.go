package server

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sentry "github.com/getsentry/sentry-go"
	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

const panicSecret = "boom-secret-xyz"

func panicApp(t *testing.T, withSentry bool) (*fiber.App, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	l := zerolog.New(&buf).With().Timestamp().Logger()
	app := fiber.New()
	if withSentry {
		app.Use(sentryfiber.New(sentryfiber.Options{Repanic: false}))
	}
	useRequestLogging(app, &l)
	app.Get("/boom", func(c *fiber.Ctx) error { panic(panicSecret) })
	app.Get("/ok", func(c *fiber.Ctx) error { return c.SendString("ok") })
	return app, &buf
}

// assertPanicRecovered checks the 500, the two log lines and that the app keeps serving.
func assertPanicRecovered(t *testing.T, app *fiber.App, buf *bytes.Buffer) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", "/boom", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 500 {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if strings.Contains(string(body), panicSecret) {
		t.Errorf("response body leaks the panic value: %q", body)
	}
	rid := resp.Header.Get(fiber.HeaderXRequestID)
	if rid == "" {
		t.Fatal("no X-Request-ID header")
	}

	lines := logLines(t, buf)
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2: %s", len(lines), buf.String())
	}
	var panics, reqs int
	for _, m := range lines {
		if m["request_id"] != rid {
			t.Errorf("%v request_id = %v, want %q", m[zerolog.MessageFieldName], m["request_id"], rid)
		}
		switch m[zerolog.MessageFieldName] {
		case "http.panic":
			panics++
			if m["level"] != "error" {
				t.Errorf("http.panic level = %v", m["level"])
			}
			if m["panic"] != panicSecret {
				t.Errorf("panic = %v", m["panic"])
			}
			if s, _ := m["stack"].(string); !strings.Contains(s, "panic") {
				t.Errorf("stack missing or empty: %q", s)
			}
		case "http.request":
			reqs++
			if m["status"] != float64(500) {
				t.Errorf("http.request status = %v, want 500", m["status"])
			}
		}
	}
	if panics != 1 || reqs != 1 {
		t.Errorf("http.panic=%d http.request=%d, want 1 each", panics, reqs)
	}

	buf.Reset()
	resp, err = app.Test(httptest.NewRequest("GET", "/ok", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("follow-up status = %d, want 200", resp.StatusCode)
	}
	lines = logLines(t, buf)
	if len(lines) != 1 || lines[0]["status"] != float64(200) {
		t.Errorf("follow-up lines = %v", lines)
	}
}

func TestUseRequestLoggingRecoversPanic(t *testing.T) {
	app, buf := panicApp(t, false)
	assertPanicRecovered(t, app, buf)
}

func TestRequestLoggingPanicReportedToSentry(t *testing.T) {
	// NewClient falls back to the env DSN; empty gives a noop transport (no network).
	t.Setenv("SENTRY_DSN", "")
	var mu sync.Mutex
	var events []*sentry.Event
	client, err := sentry.NewClient(sentry.ClientOptions{
		BeforeSend: func(e *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, e)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	prev := sentry.CurrentHub().Client()
	sentry.CurrentHub().BindClient(client)
	t.Cleanup(func() { sentry.CurrentHub().BindClient(prev) })

	app, buf := panicApp(t, true)
	assertPanicRecovered(t, app, buf)

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 {
		t.Errorf("captured %d sentry events, want exactly 1", len(events))
	}
}
