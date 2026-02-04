# Testing Patterns

**Analysis Date:** 2026-02-04

## Test Framework

**Runner:**
- Go's built-in `testing` package (no external test runner)
- Version: Go 1.25

**Assertion Library:**
- `github.com/stretchr/testify` (v1.11.1)
- Commonly used: `assert` and `require` subpackages
- `assert.*` for non-fatal assertions
- `require.*` for fatal assertions (stops test on failure)

**Run Commands:**
```bash
go test ./...              # Run all tests
go test -v ./...           # Verbose output
go test -run TestName ./...  # Run specific test
go test -cover ./...       # Show coverage
go test -race ./...        # Run with race detector
```

**CI Configuration:**
- Located: `.github/workflows/ci.yml`
- Runs: `go test -v ./...` on every push to main and pull requests
- Uses Go 1.25.x with `check-latest: true`
- Includes: `go mod verify` to validate dependencies

## Test File Organization

**Location:**
- Co-located with source files: `*_test.go` in same directory as source
- Example: `internal/handlers/auth.go` paired with `internal/handlers/auth_test.go`

**Naming:**
- Test files: `{source}_test.go`
- Test functions: `Test{FunctionName}` (e.g., `TestValidatePassword`, `TestRequireAuth_Unauthenticated`)
- Sub-tests (table-driven): `t.Run(tt.name, func(t *testing.T) { ... })`

**Structure:**
```
internal/
├── handlers/
│   ├── auth.go
│   ├── auth_test.go
│   ├── admin_test.go
│   └── handlers_test.go  (TestMain setup)
├── database/
│   ├── database.go
│   ├── database_test.go
│   └── test_helper.go
└── ...
```

## Test Structure

**Suite Organization:**

From `internal/handlers/auth_test.go`:
```go
func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "Valid password",
			password: "SecurePass123!",
			wantErr:  false,
		},
		{
			name:     "Too short",
			password: "Short1!",
			wantErr:  true,
			errMsg:   "at least 12 characters",
		},
		// ... more test cases
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.password)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
```

**Patterns:**
- Table-driven tests: Using anonymous structs with test cases
- Test naming: `Test{Function}_{Scenario}` for clarity (e.g., `TestRequireAuth_Unauthenticated`, `TestRequireAdmin_IsAdmin`)
- Setup: Helper functions like `setupTestApp()`, `createTestUser()`, `createAdminUser()`
- Teardown: `cleanupTestData(t)` called with `defer` in individual tests
- Global setup: `TestMain` function in `handlers_test.go` for test database initialization

**TestMain pattern from `internal/handlers/handlers_test.go`:**
```go
func TestMain(m *testing.M) {
	// Setup Test DB (Container)
	_, cleanup := database.SetupTestDB()

	// Run tests
	code := m.Run()

	// Cleanup container
	cleanup()

	os.Exit(code)
}
```

## Mocking

**Framework:**
- No dedicated mocking library (net/http/httptest used for HTTP testing)
- Manual mocking via test helpers and middleware

**Patterns:**

HTTP Server Mocking (from `internal/pack/client_test.go`):
```go
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	assert.Equal(t, "/v1/packs", r.URL.Path)
	json.NewEncoder(w).Encode(map[string][]PackSummary{
		"packs": {
			{Name: "pack1", Description: "First pack"},
		},
	})
}))
defer server.Close()

client := NewClient(server.URL)
packs, err := client.ListAllPacks()
```

Middleware Mocking (from `internal/handlers/admin_test.go`):
```go
app.Use(func(c *fiber.Ctx) error {
	sess, _ := Store.Get(c)
	sess.Set("user_id", user.ID)
	sess.Save()
	c.Locals("UserID", user.ID)
	c.Locals("User", user)
	return c.Next()
})
```

Environment Variable Mocking (from `internal/database/database_test.go`):
```go
func TestGetSSLMode_Production(t *testing.T) {
	t.Setenv("DB_SSLMODE", "")
	t.Setenv("ENV", "production")

	mode := getSSLMode()
	if mode != "require" {
		t.Errorf("getSSLMode() = %s, want require in production", mode)
	}
}
```

**What to Mock:**
- External HTTP APIs: Use `httptest.NewServer()` with mock handlers
- Database: Use real testcontainers for integration tests
- Session data: Inject via middleware using `c.Locals()` and `sess.Set()`
- Environment variables: Use `t.Setenv()` to override for test isolation

**What NOT to Mock:**
- Real database operations: Use `testcontainers` with actual PostgreSQL
- GORM operations: Test against real database schema
- Fiber request/response cycle: Use real Fiber app with `app.Test()`

## Fixtures and Factories

**Test Data Patterns:**

Factory functions (from `internal/handlers/admin_test.go`):
```go
func createAdminUser(t *testing.T, username string) models.User {
	user := models.User{
		Username:      username,
		Email:         username + "@test.com",
		Name:          "Admin " + username,
		EmailVerified: true,
		IsAdmin:       true,
	}
	err := database.DB.Create(&user).Error
	assert.NoError(t, err)
	return user
}

func createTestUser(t *testing.T, username string) models.User {
	// Similar pattern
}
```

**Location:**
- Factory functions: Inline in test files next to tests that use them
- Test database setup: `internal/database/test_helper.go`
- Test data cleanup: `cleanupTestData(t)` function in handler tests

**TestContainers Setup (from `internal/database/test_helper.go`):**
```go
func SetupTestDB() (*gorm.DB, func()) {
	ctx := context.Background()
	dbName := "rmbl_test"
	dbUser := "postgres"
	dbPassword := "password"

	pgContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		log.Fatalf("failed to start postgres container: %s", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	// ... setup GORM DB

	cleanup := func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			log.Printf("failed to terminate container: %s", err)
		}
	}
	return db, cleanup
}
```

## Coverage

**Requirements:** No explicit coverage targets enforced

**View Coverage:**
```bash
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

**Coverage observations:**
- 26 test files found across the codebase
- Tests exist for core packages: `handlers/`, `database/`, `pack/`, `crypto/`, `render/`, `services/`
- Some packages fully tested: `auth_test.go`, `admin_test.go`, `database_test.go`
- CLI commands appear to have minimal test coverage

## Test Types

**Unit Tests:**
- Scope: Individual functions in isolation
- Approach: Table-driven tests with multiple input scenarios
- Example: `TestValidatePassword()` tests password validation logic with 8 different cases
- Uses: `assert` for simple checks, `assert.Error()` / `assert.NoError()` for error cases
- Database: Mocked via in-memory or isolated transactions when needed

**Integration Tests:**
- Scope: Multiple components working together with real database
- Approach: TestMain sets up PostgreSQL testcontainer for all handler tests
- Example: `TestSignupAndLogin()` tests signup and login handlers against real DB
- Uses: Real `database.DB` instance, fixtures created with factory functions
- Database isolation: Tests run against fresh container; cleanup via defer in individual tests

**E2E Tests:**
- Framework: Not used
- No end-to-end test suite found
- Assumed to be manual or handled via external tools

## Common Patterns

**Async Testing:**

Not observed in codebase. Fiber handlers are synchronous. If async needed:
```go
// Hypothetical pattern based on Go testing conventions:
func TestAsync(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		// async operation
		done <- err
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for async operation")
	}
}
```

**Error Testing:**

From `internal/handlers/auth_test.go`:
```go
func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "No uppercase",
			password: "securepass123!",
			wantErr:  true,
			errMsg:   "uppercase letter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.password)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
```

**HTTP Handler Testing:**

From `internal/handlers/handlers_test.go`:
```go
func TestHome(t *testing.T) {
	app := setupTestApp()
	app.Get("/", Home)

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
```

## Test Execution & CI

**CI Pipeline (`.github/workflows/ci.yml`):**
1. Checkout code
2. Setup Go 1.25.x
3. Verify dependencies: `go mod verify`
4. Run tests: `go test -v ./...`
5. Check Swagger docs: Generate and verify no changes needed
6. Build Docker images (only on version tags)
7. Create GitHub release (only on version tags)

**Pre-commit Requirements:**
- All tests must pass before merge
- Swagger documentation must be up-to-date

**Test Isolation:**
- TestMain-based setup ensures fresh database per test run
- `defer cleanupTestData(t)` cleans up test-created data
- `t.Setenv()` provides test-scoped environment variables

---

*Testing analysis: 2026-02-04*
