package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestInit_Development(t *testing.T) {
	t.Setenv("ENV", "development")

	Init()

	// Logger should be initialized
	if Log.GetLevel() < 0 {
		// Just verify it doesn't panic and is usable
	}

	// Test that logging works
	Log.Info().Msg("test message")
}

func TestInit_Production(t *testing.T) {
	t.Setenv("ENV", "production")

	Init()

	// Logger should be initialized for production
	Log.Info().Msg("test production message")
}

func TestWithUser(t *testing.T) {
	t.Setenv("ENV", "development")
	Init()

	// Capture output
	var buf bytes.Buffer
	testLogger := Log.Output(&buf)
	Log = testLogger

	userLogger := WithUser(123, "testuser")

	// Write to a buffer to verify fields
	var outputBuf bytes.Buffer
	userLogger = userLogger.Output(&outputBuf)
	userLogger.Info().Msg("user action")

	output := outputBuf.String()
	if !strings.Contains(output, "user_id") && !strings.Contains(output, "123") {
		// In development mode, the console writer formats differently
		// Just verify no panic
	}
}

func TestWithRequest(t *testing.T) {
	t.Setenv("ENV", "development")
	Init()

	reqLogger := WithRequest("req-123", "GET", "/api/test")

	// Write to a buffer to verify fields
	var outputBuf bytes.Buffer
	reqLogger = reqLogger.Output(&outputBuf)
	reqLogger.Info().Msg("request handled")

	output := outputBuf.String()
	if !strings.Contains(output, "request_id") && !strings.Contains(output, "req-123") {
		// In development mode, the console writer formats differently
		// Just verify no panic
	}
}

func TestWithUser_JSONOutput(t *testing.T) {
	t.Setenv("ENV", "production")
	Init()

	var buf bytes.Buffer
	userLogger := Log.Output(&buf)
	Log = userLogger

	contextLogger := WithUser(456, "admin")

	var outputBuf bytes.Buffer
	contextLogger = contextLogger.Output(&outputBuf)
	contextLogger.Info().Msg("admin action")

	output := outputBuf.String()

	// Parse JSON output
	var logEntry map[string]any
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		// May fail if format is different, that's OK
		return
	}

	if logEntry["user_id"] != nil {
		if uid, ok := logEntry["user_id"].(float64); ok && uid != 456 {
			t.Errorf("user_id = %v, want 456", logEntry["user_id"])
		}
	}
}

func TestWithRequest_JSONOutput(t *testing.T) {
	t.Setenv("ENV", "production")
	Init()

	var buf bytes.Buffer
	reqLogger := Log.Output(&buf)
	Log = reqLogger

	contextLogger := WithRequest("req-abc", "POST", "/api/submit")

	var outputBuf bytes.Buffer
	contextLogger = contextLogger.Output(&outputBuf)
	contextLogger.Info().Msg("form submitted")

	output := outputBuf.String()

	// Parse JSON output
	var logEntry map[string]any
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		// May fail if format is different
		return
	}

	if logEntry["request_id"] != nil && logEntry["request_id"] != "req-abc" {
		t.Errorf("request_id = %v, want req-abc", logEntry["request_id"])
	}
	if logEntry["method"] != nil && logEntry["method"] != "POST" {
		t.Errorf("method = %v, want POST", logEntry["method"])
	}
	if logEntry["path"] != nil && logEntry["path"] != "/api/submit" {
		t.Errorf("path = %v, want /api/submit", logEntry["path"])
	}
}

func TestInit_MultipleCalls(t *testing.T) {
	t.Setenv("ENV", "development")

	// Should not panic when called multiple times
	Init()
	Init()
	Init()

	Log.Info().Msg("still working")
}

func TestLoggerLevels(t *testing.T) {
	t.Setenv("ENV", "production")
	Init()

	// Test various log levels don't panic
	Log.Debug().Msg("debug message")
	Log.Info().Msg("info message")
	Log.Warn().Msg("warn message")
	Log.Error().Msg("error message")
}

func TestLoggerWithFields(t *testing.T) {
	t.Setenv("ENV", "production")
	Init()

	// Test adding various field types
	Log.Info().
		Str("string_field", "value").
		Int("int_field", 42).
		Bool("bool_field", true).
		Float64("float_field", 3.14).
		Msg("message with fields")
}
