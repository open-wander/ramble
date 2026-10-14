package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
)

// objErr is an error that would marshal as a JSON object, not a string.
type objErr struct{ Code int }

func (e objErr) Error() string { return "obj failure" }

func (e objErr) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]int{"code": e.Code})
}

func TestErr(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		zero    bool
		wantMsg string
	}{
		{"nil is the zero attr", nil, true, ""},
		{"plain error", errors.New("boom"), false, "boom"},
		{"json.Marshaler error", objErr{Code: 7}, false, "obj failure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Err(tt.err)
			if tt.zero {
				if !a.Equal(slog.Attr{}) {
					t.Fatalf("Err(nil) = %v, want zero Attr", a)
				}
				// A handler drops the zero Attr, so no empty error key is written.
				var buf bytes.Buffer
				slog.New(slog.NewJSONHandler(&buf, nil)).Info("x.y", a)
				var m map[string]any
				if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
					t.Fatal(err)
				}
				if _, ok := m["error"]; ok {
					t.Errorf("nil error wrote an error key: %v", m)
				}
				return
			}
			if a.Key != "error" || a.Value.Kind() != slog.KindString || a.Value.String() != tt.wantMsg {
				t.Errorf("Err = key %q kind %v value %q, want error/String/%q", a.Key, a.Value.Kind(), a.Value.String(), tt.wantMsg)
			}
		})
	}
}
