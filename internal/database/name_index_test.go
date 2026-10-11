package database

import (
	"encoding/json"
	"strings"
	"testing"

	"rmbl/internal/models"

	postgresDriver "gorm.io/driver/postgres"
)

func TestCaseInsensitiveNameIndexes(t *testing.T) {
	origDB := DB
	t.Cleanup(func() { DB = origDB })
	db, cleanup := SetupTestDB()
	t.Cleanup(cleanup)

	// Twice: the startup migration runs on every deploy.
	for range 2 {
		if err := ensureCaseInsensitiveNameIndexes(db); err != nil {
			t.Fatalf("ensureCaseInsensitiveNameIndexes: %v", err)
		}
	}

	if err := db.Create(&models.User{Username: "Alice", Email: "alice1@test.com"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{Username: "alice", Email: "alice2@test.com"}).Error; err == nil {
		t.Error("a username differing only in case was accepted")
	}
	if err := db.Create(&models.User{Username: "bob", Email: "bob@test.com"}).Error; err != nil {
		t.Errorf("a distinct username was rejected: %v", err)
	}

	if err := db.Create(&models.Organization{Name: "Acme"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Organization{Name: "ACME"}).Error; err == nil {
		t.Error("an organisation name differing only in case was accepted")
	}
}

// An existing duplicate must not stop the app: Connect logs the failure and
// carries on, because the handlers already reject case-variant names.
func TestConnectNameIndexFailureIsNotFatal(t *testing.T) {
	origDB := DB
	t.Cleanup(func() { DB = origDB })
	db, cleanup := SetupTestDB()
	t.Cleanup(cleanup)
	dsn := db.Dialector.(*postgresDriver.Dialector).DSN
	for _, u := range []models.User{{Username: "Dup", Email: "dup1@test.com"}, {Username: "dup", Email: "dup2@test.com"}} {
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.Close()
	}

	t.Setenv("ENV", "development")
	t.Setenv("DATABASE_URL", dsn)
	buf, codes := swapFailSeams(t)

	Connect()

	if opened := DB; opened != nil {
		t.Cleanup(func() {
			if sqlDB, err := opened.DB(); err == nil {
				sqlDB.Close()
			}
		})
	}

	if len(*codes) != 0 {
		t.Fatalf("exit calls = %v, want none", *codes)
	}
	var failed bool
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("non-JSON line %q: %v", line, err)
		}
		if rec["msg"] == "db.name_index_failed" && rec["level"] == "error" {
			failed = true
		}
	}
	if !failed {
		t.Errorf("no ERROR db.name_index_failed line in:\n%s", buf.String())
	}
}
