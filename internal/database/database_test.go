package database

import (
	"testing"
)

func TestGetSSLMode_Default(t *testing.T) {
	// Clear any existing settings
	t.Setenv("DB_SSLMODE", "")
	t.Setenv("ENV", "development")

	mode := getSSLMode()
	if mode != "disable" {
		t.Errorf("getSSLMode() = %s, want disable in development", mode)
	}
}

func TestGetSSLMode_Production(t *testing.T) {
	t.Setenv("DB_SSLMODE", "")
	t.Setenv("ENV", "production")

	mode := getSSLMode()
	if mode != "require" {
		t.Errorf("getSSLMode() = %s, want require in production", mode)
	}
}

func TestGetSSLMode_ExplicitOverride(t *testing.T) {
	// Explicit DB_SSLMODE should override environment defaults
	testCases := []struct {
		sslmode  string
		env      string
		expected string
	}{
		{"disable", "production", "disable"},
		{"require", "development", "require"},
		{"verify-full", "production", "verify-full"},
		{"prefer", "development", "prefer"},
	}

	for _, tc := range testCases {
		t.Run(tc.sslmode+"_"+tc.env, func(t *testing.T) {
			t.Setenv("DB_SSLMODE", tc.sslmode)
			t.Setenv("ENV", tc.env)

			mode := getSSLMode()
			if mode != tc.expected {
				t.Errorf("getSSLMode() = %s, want %s", mode, tc.expected)
			}
		})
	}
}

func TestGetSSLMode_EmptyWithNoEnv(t *testing.T) {
	t.Setenv("DB_SSLMODE", "")
	t.Setenv("ENV", "")

	mode := getSSLMode()
	if mode != "disable" {
		t.Errorf("getSSLMode() = %s, want disable for empty ENV", mode)
	}
}
