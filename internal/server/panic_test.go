package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sentry "github.com/getsentry/sentry-go"
	"github.com/gofiber/fiber/v2"

	"rmbl/internal/services/logger"
	"rmbl/internal/testutil/logtest"
)

const panicSecret = "boom-secret-xyz"

func panicApp(t *testing.T, withSentry bool) (*fiber.App, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	l := logger.NewJSON(&buf)
	app := fiber.New()
	useOuterMiddleware(app, l, withSentry)
	useRequestLogging(app, l)
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

	lines := logtest.Lines(t, buf)
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2: %s", len(lines), buf.String())
	}
	var panics, reqs int
	for _, m := range lines {
		if m["request_id"] != rid {
			t.Errorf("%v request_id = %v, want %q", m["msg"], m["request_id"], rid)
		}
		switch m["msg"] {
		case "http.panic":
			panics++
			if m["level"] != "ERROR" {
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
	lines = logtest.Lines(t, buf)
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

// useSentryCapture binds a client built from the production sentryOptions
// whose transport records events, so the real BeforeSend runs.
func useSentryCapture(t *testing.T) *captureTransport {
	t.Helper()
	t.Setenv("SENTRY_DSN", "")
	tr := &captureTransport{}
	opts := sentryOptions("", "test", "v")
	opts.Transport = tr
	client, err := sentry.NewClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	prev := sentry.CurrentHub().Client()
	sentry.CurrentHub().BindClient(client)
	t.Cleanup(func() { sentry.CurrentHub().BindClient(prev) })
	return tr
}

type captureTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (c *captureTransport) Flush(time.Duration) bool              { return true }
func (c *captureTransport) FlushWithContext(context.Context) bool { return true }
func (c *captureTransport) Configure(sentry.ClientOptions)        {}
func (c *captureTransport) Close()                                {}
func (c *captureTransport) SendEvent(e *sentry.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

const sentinel = "SENTINEL"

func TestOuterBackstop(t *testing.T) {
	for _, withSentry := range []bool{false, true} {
		name := "without sentry"
		if withSentry {
			name = "with sentry"
		}
		t.Run(name, func(t *testing.T) {
			var tr *captureTransport
			if withSentry {
				tr = useSentryCapture(t)
			}
			var buf bytes.Buffer
			l := logger.NewJSON(&buf)
			app := fiber.New()
			useOuterMiddleware(app, l, withSentry)
			app.Use(func(c *fiber.Ctx) error {
				if c.Path() == "/boom" {
					panic(panicSecret)
				}
				return c.Next()
			})
			useRequestLogging(app, l)
			app.Get("/ok", func(c *fiber.Ctx) error { return c.SendString("ok") })

			resp, err := app.Test(httptest.NewRequest("GET", "/boom", nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != 500 {
				t.Errorf("status = %d, want 500", resp.StatusCode)
			}
			if strings.Contains(string(body), panicSecret) {
				t.Errorf("body leaks panic value: %q", body)
			}
			lines := logtest.Lines(t, &buf)
			if len(lines) != 1 {
				t.Fatalf("got %d log lines, want 1: %s", len(lines), buf.String())
			}
			m := lines[0]
			if m["msg"] != "http.panic" || m["outer"] != true || m["level"] != "ERROR" || m["panic"] != panicSecret {
				t.Errorf("unexpected line: %v", m)
			}
			if s, _ := m["stack"].(string); !strings.Contains(s, "panic") {
				t.Errorf("stack missing: %q", s)
			}
			if tr != nil {
				tr.mu.Lock()
				n := len(tr.events)
				tr.mu.Unlock()
				if n != 1 {
					t.Errorf("captured %d sentry events, want 1", n)
				}
			}

			// The app keeps serving after an outer panic.
			resp, err = app.Test(httptest.NewRequest("GET", "/ok", nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			if body, _ := io.ReadAll(resp.Body); resp.StatusCode != 200 || string(body) != "ok" {
				t.Errorf("follow-up /ok = %d %q, want 200 \"ok\"", resp.StatusCode, body)
			}
		})
	}
}

func TestScrubSentryEvent(t *testing.T) {
	if got := scrubSentryEvent(&sentry.Event{}, nil); got == nil {
		t.Fatal("nil Request must pass through")
	}
	e := &sentry.Event{Request: &sentry.Request{
		URL:         "http://x/boom?a=" + sentinel + "#" + sentinel,
		QueryString: "q=" + sentinel,
		Cookies:     "theme=" + sentinel,
		Data:        "body=" + sentinel,
		Headers: map[string]string{
			"authorization":       sentinel,
			"COOKIE":              sentinel,
			"X-Hub-Signature-256": sentinel,
			"x-webhook-timestamp": sentinel,
			"User-Agent":          "ua",
		},
	}}
	out := scrubSentryEvent(e, nil)
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), sentinel) {
		t.Errorf("sentinel survived: %s", b)
	}
	frag := scrubSentryEvent(&sentry.Event{Request: &sentry.Request{URL: "http://x/p#" + sentinel}}, nil)
	if strings.Contains(frag.Request.URL, sentinel) {
		t.Errorf("fragment survived: %q", frag.Request.URL)
	}
	if out.Request.Headers["User-Agent"] != "ua" || out.Request.URL != "http://x/boom" {
		t.Errorf("harmless fields damaged: %+v", out.Request)
	}
}

func TestSentryEventsCarryNoRequestSecrets(t *testing.T) {
	tr := useSentryCapture(t)
	var buf bytes.Buffer
	l := logger.NewJSON(&buf)
	app := fiber.New()
	useOuterMiddleware(app, l, true)
	useRequestLogging(app, l)
	app.Post("/boom", func(c *fiber.Ctx) error { panic(panicSecret) })
	req := httptest.NewRequest("POST", "/boom?q="+sentinel, strings.NewReader("f="+sentinel))
	req.Header.Set("Cookie", "theme="+sentinel)
	req.Header.Set("X-Hub-Signature-256", sentinel)
	req.Header.Set("X-Webhook-Timestamp", sentinel)
	if _, err := app.Test(req, -1); err != nil {
		t.Fatal(err)
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.events) != 1 {
		t.Fatalf("captured %d events, want 1", len(tr.events))
	}
	b, _ := json.Marshal(tr.events[0])
	if strings.Contains(string(b), sentinel) {
		t.Errorf("sentinel in event: %s", b)
	}
}
