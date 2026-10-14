package database

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"rmbl/internal/models"
	"rmbl/internal/services/logger"

	postgresDriver "gorm.io/driver/postgres"
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
	userType := reflect.TypeFor[models.User]()

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
	oldS, oldExit, oldDB := logger.Log, exit, DB
	logger.Log = logger.NewJSON(&buf)
	exit = func(code int) { codes = append(codes, code) }
	t.Cleanup(func() {
		logger.Log, exit, DB = oldS, oldExit, oldDB
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
	if rec["level"] != "ERROR" || rec["msg"] != "db.connect_failed" {
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
	if rec["level"] != "ERROR" || rec["msg"] != "db.credentials_missing" {
		t.Errorf("record = %v", rec)
	}
	if _, has := rec["error"]; has {
		t.Errorf("unexpected error field: %v", rec)
	}
	if DB != nil {
		t.Error("DB was assigned after credentials_missing")
	}
}

// TestConnectMigrateFailed makes AutoMigrate fail against a real Postgres: a
// view named users stops the users table being created (gorm only treats a
// BASE TABLE as existing), so CREATE TABLE fails with "already exists".
func TestConnectMigrateFailed(t *testing.T) {
	// SetupTestDB assigns the global DB; restore the caller's handle last
	// (cleanups run in reverse), not the closed test handle.
	origDB := DB
	t.Cleanup(func() { DB = origDB })
	db, cleanup := SetupTestDB()
	t.Cleanup(cleanup)
	dsn := db.Dialector.(*postgresDriver.Dialector).DSN
	if err := db.Exec(`DROP TABLE users CASCADE`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE VIEW users AS SELECT 1 AS id`).Error; err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.Close()
	}

	t.Setenv("ENV", "development")
	t.Setenv("DATABASE_URL", dsn)
	buf, codes := swapFailSeams(t)

	Connect()

	// Close the connection Connect opened before the container goes (cleanup is LIFO).
	if opened := DB; opened != nil {
		t.Cleanup(func() {
			if sqlDB, err := opened.DB(); err == nil {
				sqlDB.Close()
			}
		})
	}

	if len(*codes) != 1 || (*codes)[0] != 1 {
		t.Fatalf("exit calls = %v, want exactly [1]", *codes)
	}
	// gorm's logger emits db.query_error first, so look for the migrate line.
	var found, queryErr bool
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("non-JSON line %q: %v", line, err)
		}
		if rec["msg"] == "db.migrate_completed" {
			t.Errorf("migrate_completed logged after a failed migration: %v", rec)
		}
		if rec["msg"] == "db.query_error" {
			// slog-origin line: upper-case level, error as a string.
			queryErr = true
			if rec["level"] != "ERROR" {
				t.Errorf("db.query_error level = %v, want ERROR", rec["level"])
			}
			if s, _ := rec["error"].(string); s == "" {
				t.Errorf("db.query_error error missing or not a string: %v", rec)
			}
		}
		if rec["msg"] != "db.migrate_failed" {
			continue
		}
		found = true
		if rec["level"] != "ERROR" {
			t.Errorf("level = %v, want ERROR", rec["level"])
		}
		if s, _ := rec["error"].(string); s == "" {
			t.Errorf("error field missing: %v", rec)
		} else {
			t.Logf("migrate error: %s", s)
		}
	}
	if !found {
		t.Errorf("no db.migrate_failed line in %q", buf.String())
	}
	if !queryErr {
		t.Errorf("no db.query_error line in %q", buf.String())
	}
}
