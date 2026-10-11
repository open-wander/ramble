// Package logtest parses the JSON log lines zerolog writes in tests. It must
// not import anything under rmbl, so any test package can use it.
package logtest

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// Lines decodes every line in buf; a non-JSON line fails the test. An empty
// buffer yields nil.
func Lines(t testing.TB, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	s := strings.TrimSpace(buf.String())
	if s == "" {
		return nil
	}
	var out []map[string]any
	for line := range strings.SplitSeq(s, "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

// Find returns the first line whose message equals msg, or nil.
func Find(t testing.TB, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	for _, m := range Lines(t, buf) {
		if m[zerolog.MessageFieldName] == msg {
			return m
		}
	}
	return nil
}
