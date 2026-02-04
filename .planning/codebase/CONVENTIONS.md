# Coding Conventions

**Analysis Date:** 2026-02-04

## Naming Patterns

**Files:**
- Source files: lowercase with underscores for multi-word names (e.g., `database.go`, `server_test.go`)
- Test files: `*_test.go` suffix (standard Go convention)
- Package directories: lowercase, single-word (e.g., `handlers`, `database`, `services`)
- Multi-word packages: combined lowercase (e.g., `testcontainers`)

**Functions & Methods:**
- Public functions: PascalCase (e.g., `GetDocs()`, `NewClient()`, `InitSession()`)
- Private functions: camelCase (e.g., `getSSLMode()`, `validatePassword()`, `getKey()`)
- Handler functions: Verb+Noun pattern in PascalCase (e.g., `PostSignup()`, `PostLogin()`, `GetRequests()`)
- Middleware functions: PascalCase (e.g., `RequireAuth()`, `RequireAdmin()`, `RequireVerifiedEmail()`)

**Variables & Constants:**
- Public variables: PascalCase (e.g., `Store`, `Log`, `DB`)
- Private variables: camelCase (e.g., `dsn`, `pageSize`, `validStatuses`)
- Constants: PascalCase (e.g., `ErrNoEncryptionKey`, `ErrInvalidKey`, `ResourceTypeJob`)
- Enums/const groups: Named with Type suffix (e.g., `ResourceType`, constants: `ResourceTypeJob`, `ResourceTypePack`)

**Types & Structs:**
- Type names: PascalCase (e.g., `Config`, `RenderContext`, `User`, `PackRequest`)
- Struct fields: PascalCase for exported (e.g., `Username`, `Email`, `CreatedAt`)
- Struct tags: Use GORM and JSON tags for database and serialization (e.g., `gorm:"uniqueIndex;not null"`, `json:"name"`)

**Interfaces:**
- Interface names: PascalCase, often end in "er" (e.g., `Reader`, `Writer`)
- Example: Methods follow verb-first pattern

## Code Style

**Formatting:**
- No explicit linter config file (relies on Go's standard `gofmt`)
- Code follows idiomatic Go style
- Tab indentation (2 spaces in some inline functions for brevity but inconsistent)
- Line length: No strict limit enforced, typically around 80-100 characters
- Imports organized: stdlib imports first, then external packages, then internal packages

**Linting:**
- No `.golangci.yml` found; project does not use automated linting
- Manual code review expected
- CI tests run: `go mod verify`, `go test -v ./...`, swagger generation

**Error Handling:**
- Errors returned as last return value (e.g., `func Foo() (string, error)`)
- Custom error variables defined at package level using `errors.New()` (e.g., `ErrNoEncryptionKey`, `ErrInvalidKey`)
- Error checking pattern: `if err != nil { ... }`
- Wrapping errors with context: Uses `fmt.Errorf()` with `%w` verb (e.g., `fmt.Errorf("failed to read templates directory: %w", err)`)
- Fatal errors in initialization: Uses `log.Fatal()` for unrecoverable startup issues
- HTTP error responses: Uses Fiber's response methods (e.g., `c.SendStatus(403)`, `c.Redirect()`)

## Import Organization

**Order:**
1. Standard library imports (stdlib)
2. External third-party packages (github.com, gopkg.in, gorm.io, etc.)
3. Internal packages (rmbl/internal/...)

**Path Aliases:**
- Internal module: `rmbl` (from go.mod)
- No alias usage observed
- Full paths used: `rmbl/internal/database`, `rmbl/internal/models`, `rmbl/internal/handlers`

**Example import block from `internal/handlers/auth.go`:**
```go
import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"rmbl/internal/crypto"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/email"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/markbates/goth"
	...
)
```

## Error Handling

**Patterns:**
- Early return pattern for validation errors: Check parameters at function start, return early on error
- Nil checks before type assertions: `if userLoc == nil { return ... }` before `user := userLoc.(models.User)`
- Map-based validation: Using maps to define allowed values (e.g., `validStatuses := map[string]bool{"all": true, "open": true, ...}`)
- Panic usage: Reserved for truly unrecoverable situations (e.g., `panic("failed to generate webhook secret: " + err.Error())`)
- Logging with context: Uses structured logger with context (user ID, request ID)

**Example pattern from `internal/handlers/requests.go`:**
```go
// Validate status parameter
status := c.Query("status", "open")
validStatuses := map[string]bool{
	"all": true, "open": true, "in_progress": true,
	"completed": true, "closed": true,
}
if !validStatuses[status] {
	status = "open"
}
```

## Logging

**Framework:** `github.com/rs/zerolog`

**Global logger instance:** `var Log zerolog.Logger` in `internal/services/logger/logger.go`

**Initialization pattern:**
- Called via `logger.Init()` at server startup
- Development mode: Pretty console output with timestamps and caller info
- Production mode: JSON structured output

**Contextual logging patterns:**
```go
// With user context
logger.WithUser(userID uint, username string) zerolog.Logger

// With request context
logger.WithRequest(requestID string, method string, path string) zerolog.Logger
```

**Usage in handlers:**
- Typically logged via middleware that adds request ID
- Fiber middleware adds request ID: `app.Use(requestid.New())`
- Fiber logger middleware: `app.Use(fiberlogger.New(...))`

## Comments

**When to Comment:**
- Comments for non-obvious logic (e.g., "Only send over HTTPS in production")
- Comments explaining why, not what (the code shows what)
- Short comments for helper functions (e.g., `// validatePassword checks if a password meets security requirements`)
- Section comments for related function groups (none observed, but implied pattern)

**GoDoc/JSDoc Pattern:**
- GoDoc comments for exported functions: `// FunctionName description`
- Example from `internal/handlers/pages.go`:
```go
// GetDocs godoc
// @Summary Get documentation page
// @Description Renders the documentation page with sidebar navigation.
// @Tags pages
// @Produce html
// @Success 200 {string} string "HTML content"
// @Router /docs [get]
func GetDocs(c *fiber.Ctx) error {
```

- Swagger annotations used for API documentation (using swaggo)
- Private functions have shorter comments or none if self-explanatory

## Function Design

**Size:** Functions are typically 20-80 lines
- Handlers tend to be longer (60-100 lines) with query param validation and database queries
- Helper functions are concise (10-30 lines)
- Example: `GetRequests()` is ~80 lines with filtering logic; `validatePassword()` is ~20 lines

**Parameters:**
- Fiber handlers: Single parameter `c *fiber.Ctx`
- Database operations: Receive `*gorm.DB` instance
- Middleware: `(c *fiber.Ctx) error`
- Constructor functions: Named `New[Type]` (e.g., `NewClient()`, `NewEngine()`)

**Return Values:**
- Handlers return `error` (Fiber convention)
- Utility functions return `(T, error)` where T is the result type
- Constructors return `*Type`
- No multiple error returns; errors are last value

**Example handler from `internal/handlers/auth.go`:**
```go
func BeginAuth(c *fiber.Ctx) error {
	return goth_fiber.BeginAuthHandler(c)
}

func AuthCallback(c *fiber.Ctx) error {
	gothUser, err := goth_fiber.CompleteUserAuth(c)
	if err != nil {
		SetFlash(c, "error", "Authentication failed: "+err.Error())
		return c.Redirect("/login")
	}
	// ... more logic
}
```

## Module Design

**Exports:**
- Package-level variables exported only when needed (e.g., `var DB *gorm.DB`)
- Most database operations go through the global `DB` instance
- No explicit export patterns enforced; relies on PascalCase visibility

**Barrel Files:**
- No barrel/index files observed
- Each file handles one concern (e.g., `auth.go`, `admin.go`, `resource.go`)
- Imports done explicitly from full paths

**Package Structure:**
- Each `internal/` directory is a focused package with a single responsibility
- No sub-packages within packages (flat structure)
- Related functions grouped in same file when they share state (e.g., auth functions share `Store`)

---

*Convention analysis: 2026-02-04*
