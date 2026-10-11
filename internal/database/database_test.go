package database

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"rmbl/internal/models"
	"rmbl/internal/services/logger"

	"github.com/rs/zerolog"
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

func TestUserModelHasTokenSecurityFields(t *testing.T) {
	// Verify User struct has required fields for token security
	userType := reflect.TypeOf(models.User{})

	// Check ResetTokenUsedAt field
	usedAtField, found := userType.FieldByName("ResetTokenUsedAt")
	if !found {
		t.Error("User model missing ResetTokenUsedAt field")
	}
	if usedAtField.Type.String() != "*time.Time" {
		t.Errorf("ResetTokenUsedAt should be *time.Time, got %s", usedAtField.Type.String())
	}

	// Check PasswordChangedAt field
	changedAtField, found := userType.FieldByName("PasswordChangedAt")
	if !found {
		t.Error("User model missing PasswordChangedAt field")
	}
	if changedAtField.Type.String() != "time.Time" {
		t.Errorf("PasswordChangedAt should be time.Time, got %s", changedAtField.Type.String())
	}
}

// swapFailSeams captures log output and exit codes, restoring globals after.
func swapFailSeams(t *testing.T) (*bytes.Buffer, *[]int) {
	t.Helper()
	var buf bytes.Buffer
	var codes []int
	oldLog, oldField, oldExit, oldDB := logger.Log, zerolog.MessageFieldName, exit, DB
	zerolog.MessageFieldName = "msg"
	logger.Log = zerolog.New(&buf)
	exit = func(code int) { codes = append(codes, code) }
	t.Cleanup(func() {
		logger.Log, zerolog.MessageFieldName, exit, DB = oldLog, oldField, oldExit, oldDB
	})
	return &buf, &codes
}

func TestConnectConnectFailed(t *testing.T) {
	t.Setenv("ENV", "development")
	// Nothing listens on port 1, so Open fails fast.
	t.Setenv("DATABASE_URL", "host=127.0.0.1 port=1 user=x dbname=x sslmode=disable connect_timeout=2")
	buf, codes := swapFailSeams(t)
	DB = nil

	Connect()

	if len(*codes) != 1 || (*codes)[0] != 1 {
		t.Fatalf("exit calls = %v, want exactly [1]", *codes)
	}
	// gorm's own logger may emit a line first; the failure line is the last.
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["level"] != "error" || rec["msg"] != "db.connect_failed" {
		t.Errorf("record = %v", rec)
	}
	if s, _ := rec["error"].(string); s == "" {
		t.Errorf("error field missing: %v", rec)
	}
}

func TestConnectCredentialsMissing(t *testing.T) {
	t.Setenv("ENV", "production")
	for _, k := range []string{"DATABASE_URL", "DB_HOST", "DB_USER", "DB_NAME"} {
		t.Setenv(k, "")
	}
	buf, codes := swapFailSeams(t)
	DB = nil

	Connect()

	if len(*codes) != 1 || (*codes)[0] != 1 {
		t.Fatalf("exit calls = %v, want exactly [1]", *codes)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &rec); err != nil {
		t.Fatalf("want exactly one JSON line: %v: %q", err, buf.String())
	}
	if rec["level"] != "error" || rec["msg"] != "db.credentials_missing" {
		t.Errorf("record = %v", rec)
	}
	if _, has := rec["error"]; has {
		t.Errorf("unexpected error field: %v", rec)
	}
	if DB != nil {
		t.Error("DB was assigned after credentials_missing")
	}
}
