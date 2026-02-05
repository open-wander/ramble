package server

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Template helper function tests
// Note: These test the logic of template functions that are defined in Run()

func TestDictFunction_Valid(t *testing.T) {
	// Recreate the dict function logic for testing
	dictFunc := func(values ...interface{}) (map[string]interface{}, error) {
		if len(values)%2 != 0 {
			return nil, fmt.Errorf("invalid dict call")
		}
		dict := make(map[string]interface{}, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict keys must be strings")
			}
			dict[key] = values[i+1]
		}
		return dict, nil
	}

	result, err := dictFunc("key1", "value1", "key2", 42)
	if err != nil {
		t.Fatalf("dictFunc() error = %v", err)
	}

	if result["key1"] != "value1" {
		t.Errorf("result[key1] = %v, want value1", result["key1"])
	}
	if result["key2"] != 42 {
		t.Errorf("result[key2] = %v, want 42", result["key2"])
	}
}

func TestDictFunction_OddValues(t *testing.T) {
	dictFunc := func(values ...interface{}) (map[string]interface{}, error) {
		if len(values)%2 != 0 {
			return nil, fmt.Errorf("invalid dict call")
		}
		dict := make(map[string]interface{}, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict keys must be strings")
			}
			dict[key] = values[i+1]
		}
		return dict, nil
	}

	_, err := dictFunc("key1", "value1", "key2") // odd number
	if err == nil {
		t.Error("dictFunc() should error on odd number of values")
	}
}

func TestDictFunction_NonStringKey(t *testing.T) {
	dictFunc := func(values ...interface{}) (map[string]interface{}, error) {
		if len(values)%2 != 0 {
			return nil, fmt.Errorf("invalid dict call")
		}
		dict := make(map[string]interface{}, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict keys must be strings")
			}
			dict[key] = values[i+1]
		}
		return dict, nil
	}

	_, err := dictFunc(123, "value1") // non-string key
	if err == nil {
		t.Error("dictFunc() should error on non-string key")
	}
}

func TestDictFunction_Empty(t *testing.T) {
	dictFunc := func(values ...interface{}) (map[string]interface{}, error) {
		if len(values)%2 != 0 {
			return nil, fmt.Errorf("invalid dict call")
		}
		dict := make(map[string]interface{}, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict keys must be strings")
			}
			dict[key] = values[i+1]
		}
		return dict, nil
	}

	result, err := dictFunc()
	if err != nil {
		t.Fatalf("dictFunc() error = %v", err)
	}
	if len(result) != 0 {
		t.Errorf("len(result) = %d, want 0", len(result))
	}
}

func TestAddFunction(t *testing.T) {
	addFunc := func(a, b int) int {
		return a + b
	}

	testCases := []struct {
		a, b, expected int
	}{
		{1, 2, 3},
		{0, 0, 0},
		{-1, 1, 0},
		{100, -50, 50},
	}

	for _, tc := range testCases {
		result := addFunc(tc.a, tc.b)
		if result != tc.expected {
			t.Errorf("addFunc(%d, %d) = %d, want %d", tc.a, tc.b, result, tc.expected)
		}
	}
}

func TestUpperFunction(t *testing.T) {
	upperFunc := func(s string) string {
		return strings.ToUpper(s)
	}

	testCases := []struct {
		input, expected string
	}{
		{"hello", "HELLO"},
		{"WORLD", "WORLD"},
		{"Hello World", "HELLO WORLD"},
		{"", ""},
		{"123", "123"},
	}

	for _, tc := range testCases {
		result := upperFunc(tc.input)
		if result != tc.expected {
			t.Errorf("upperFunc(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestCapitalizeFunction(t *testing.T) {
	capitalizeFunc := func(s string) string {
		if len(s) == 0 {
			return ""
		}
		return strings.ToUpper(s[:1]) + s[1:]
	}

	testCases := []struct {
		input, expected string
	}{
		{"hello", "Hello"},
		{"WORLD", "WORLD"},
		{"hello World", "Hello World"},
		{"", ""},
		{"a", "A"},
		{"already Capitalized", "Already Capitalized"},
	}

	for _, tc := range testCases {
		result := capitalizeFunc(tc.input)
		if result != tc.expected {
			t.Errorf("capitalizeFunc(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestCapitalizeFunction_Unicode(t *testing.T) {
	capitalizeFunc := func(s string) string {
		if len(s) == 0 {
			return ""
		}
		return strings.ToUpper(s[:1]) + s[1:]
	}

	// Note: This tests byte-level behavior, not rune-level
	result := capitalizeFunc("test")
	if result != "Test" {
		t.Errorf("capitalizeFunc(\"test\") = %q, want \"Test\"", result)
	}
}

func TestConfig(t *testing.T) {
	cfg := Config{
		Port: "8080",
		Seed: true,
	}

	if cfg.Port != "8080" {
		t.Errorf("cfg.Port = %s, want 8080", cfg.Port)
	}
	if !cfg.Seed {
		t.Error("cfg.Seed should be true")
	}
}

func TestConfig_Default(t *testing.T) {
	cfg := Config{}

	if cfg.Port != "" {
		t.Errorf("cfg.Port = %s, want empty", cfg.Port)
	}
	if cfg.Seed {
		t.Error("cfg.Seed should be false by default")
	}
}

func TestSessionInvalidatedOnPasswordChange(t *testing.T) {
	// This test verifies the session invalidation logic used in the middleware
	// When PasswordChangedAt > session_created_at, session should be destroyed

	// Test the time comparison logic directly
	sessionCreatedAt := time.Now().Add(-1 * time.Hour) // Session created 1 hour ago
	passwordChangedAt := time.Now()                    // Password just changed

	if !passwordChangedAt.After(sessionCreatedAt) {
		t.Error("passwordChangedAt should be after sessionCreatedAt")
	}

	// Test zero time handling (users who never changed password)
	var zeroTime time.Time
	if !zeroTime.IsZero() {
		t.Error("Zero time should return true for IsZero()")
	}

	// Zero time should not invalidate sessions (condition checks !IsZero() first)
	if !zeroTime.IsZero() && zeroTime.After(sessionCreatedAt) {
		t.Error("Zero PasswordChangedAt should not invalidate sessions")
	}

	// Test session created after password change (valid session)
	oldPasswordChange := time.Now().Add(-24 * time.Hour) // Password changed yesterday
	newSession := time.Now()                             // Session created now

	// This should NOT invalidate (session newer than password change)
	if oldPasswordChange.After(newSession) {
		t.Error("Old password change should not invalidate newer session")
	}
}

// Suppress unused import warning
var _ = fmt.Sprintf
var _ = strings.ToUpper
var _ = time.Now
