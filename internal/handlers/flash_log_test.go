package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
)

// failingStorage lets sessions load (nothing stored) but fails every save.
type failingStorage struct{}

func (failingStorage) Get(string) ([]byte, error)              { return nil, nil }
func (failingStorage) Set(string, []byte, time.Duration) error { return errors.New("boom") }
func (failingStorage) Delete(string) error                     { return nil }
func (failingStorage) Reset() error                            { return nil }
func (failingStorage) Close() error                            { return nil }

// Not parallel: swaps the package-global Store.
func TestSetFlash_SaveFailureLogsWithRequestID(t *testing.T) {
	orig := Store
	Store = session.New(session.Config{Storage: failingStorage{}})
	t.Cleanup(func() { Store = orig })

	var buf bytes.Buffer
	app := newRequestLogApp(testSlog(&buf))
	app.Get("/flash", func(c *fiber.Ctx) error {
		SetFlash(c, "success", "hi")
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/flash", nil)
	req.Header.Set("X-Request-ID", "rid-flash-1")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	var found bool
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("non-JSON log line %q: %v", line, err)
		}
		if m["msg"] != "session.save_failed" && m["message"] != "session.save_failed" {
			continue
		}
		found = true
		if m["request_id"] != "rid-flash-1" {
			t.Errorf("request_id = %v", m["request_id"])
		}
		if m["op"] != "set_flash" {
			t.Errorf("op = %v", m["op"])
		}
		if m["error"] != "boom" {
			t.Errorf("error = %v", m["error"])
		}
	}
	if !found {
		t.Fatalf("no session.save_failed line in %q", buf.String())
	}
}
