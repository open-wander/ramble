# Architecture

**Analysis Date:** 2026-02-04

## Pattern Overview

**Overall:** Layered MVC with CLI/Server dual entry points

Ramble follows a classic layered architecture with clear separation between CLI operations and web server functionality. Both entry points share a common business logic layer. The application serves dual purposes: a command-line tool for pack management and a web registry server for browsing and discovering Nomad packs/jobs.

**Key Characteristics:**
- **Dual mode operation**: CLI tool (`cmd/ramble/`) and web server both use shared internal packages
- **Handler-centric web layer**: HTTP handlers in `internal/handlers/` manage all request routing and logic
- **Database-first data model**: GORM models in `internal/models/` with PostgreSQL backend
- **Template rendering separation**: Custom render engine in `internal/render/` for Nomad pack template processing
- **OAuth + Session-based auth**: Support for GitHub/GitLab OAuth and local authentication
- **External API client**: Pack registry client in `internal/pack/` for interacting with Nomad registries

## Layers

**Entry Point Layer:**
- Purpose: Application entry points and command routing
- Location: `cmd/ramble/main.go`, `internal/cli/cmd/`
- Contains: CLI command definitions via Cobra, server startup logic
- Depends on: Internal packages (handlers, database, server)
- Used by: End users running CLI or starting web server

**Handler/Request Layer:**
- Purpose: HTTP request processing, response rendering, business logic
- Location: `internal/handlers/`
- Contains: HTTP handlers for web pages, API endpoints, webhooks, and file operations
- Depends on: Database models, services, render engine, external APIs (GitHub/GitLab)
- Used by: Fiber web framework via route definitions in server.go

**Service Layer:**
- Purpose: Cross-cutting concerns and external integrations
- Location: `internal/services/` (email, github, logger, version)
- Contains: Email sending, GitHub integration, logging utilities, version management
- Depends on: External APIs (GitHub API, email providers)
- Used by: Handlers and other services

**Data/Model Layer:**
- Purpose: Data structures and database operations
- Location: `internal/models/models.go`, `internal/database/`
- Contains: GORM model definitions (User, Organization, NomadResource, etc.), database connection management, migrations
- Depends on: PostgreSQL via GORM, environment configuration
- Used by: All handlers and services

**Utility/Support Layer:**
- Purpose: Specialized functionality for pack processing and rendering
- Location: `internal/pack/`, `internal/render/`, `internal/nomad/`, `internal/crypto/`
- Contains: Pack registry client, template rendering engine, Nomad job submission, cryptographic utilities
- Depends on: Models, external HTTP requests
- Used by: Handlers, CLI commands

## Data Flow

**Web Request Flow (example: browsing packs):**

1. HTTP request arrives at Fiber web server
2. Middleware chain executes: request ID, logger, HTTPS enforcement, CSRF, session, flash messages
3. Handler function executes (e.g., `handlers.GetPacks()`)
4. Handler queries database via GORM (`database.DB.Find()`)
5. Handler fetches supplementary data (tags, popular items) via database queries
6. Handler constructs context map with data
7. Handler calls `c.Render()` with template name and context
8. Template engine renders HTML view from `views/` directory with layout
9. Fiber sends response to client

**Webhook Flow (GitHub push):**

1. GitHub webhook POST to `/resource/:id/webhook`
2. `handlers.HandleWebhook()` extracts payload
3. Validates GitHub signature via `validateGitHubSignature()`
4. Updates resource version count and metadata in database
5. Records webhook delivery status (success/failure)
6. Returns 200 OK to GitHub

**Authentication Flow (OAuth):**

1. User visits `/login` → handler shows login form
2. User clicks "Sign in with GitHub" → handler calls `handlers.BeginAuth()`
3. Redirects to GitHub OAuth endpoint
4. GitHub redirects back to `/auth/:provider/callback`
5. `handlers.AuthCallback()` exchanges code for token
6. Creates/updates user in database
7. Stores user ID in session
8. Subsequent requests load user from session middleware

**State Management:**

- **Session State**: User authentication via Fiber session middleware, stored in cookies (encrypted)
- **Database State**: All persistent data (users, resources, versions, audit logs) in PostgreSQL
- **Request Context**: Flash messages, CSRF tokens, user info via `c.Locals()` map per request
- **Cache**: Pack list caching via CLI `pack cache` command (not in server)

## Key Abstractions

**NomadResource:**
- Purpose: Represents a Nomad job or pack in the registry
- Examples: `internal/models/models.go` lines 65-90
- Pattern: GORM model with relationships to User, Organization, ResourceVersion, Tags
- Supports dual resource types (job/pack) via ResourceType enum
- Tracks webhook deliveries and download/star counts

**Handler Functions:**
- Purpose: HTTP request processors following handler middleware pattern
- Examples: `handlers.GetPacks()`, `handlers.PostNewResource()`, `handlers.HandleWebhook()`
- Pattern: Accept `*fiber.Ctx`, return error; handlers compose HTTP logic with database operations
- Middleware pattern: Early-exit handlers (`RequireAuth`, `RequireVerifiedEmail`) for authorization checks

**RenderContext/Engine:**
- Purpose: Render Nomad pack templates with variable substitution
- Location: `internal/render/engine.go`
- Pattern: Stateful context with template functions, renders `[[variable]]` syntax templates
- Used by: CLI pack render/run commands and web handlers for pack info display

**PackClient:**
- Purpose: HTTP client for Ramble registry API
- Location: `internal/pack/client.go`
- Pattern: Wraps HTTP requests with structured response types (PackSummary, PackDetail, etc.)
- Used by: CLI list/search commands for querying remote registries

## Entry Points

**CLI Main Entry:**
- Location: `cmd/ramble/main.go`
- Triggers: User runs `ramble` command
- Responsibilities: Calls `cmd.Execute()` which routes to Cobra subcommands

**Cobra Root Command:**
- Location: `internal/cli/cmd/root.go`
- Triggers: All CLI operations
- Responsibilities: Sets up global flags (verbose), starts update check in background, manages help text

**Server Command:**
- Location: `internal/cli/cmd/server.go`
- Triggers: `ramble server` CLI invocation
- Responsibilities: Parses port/seed flags, calls `server.Run()`

**Server.Run():**
- Location: `internal/server/server.go`
- Triggers: Server startup via CLI
- Responsibilities:
  - Initializes logger and database connection
  - Configures Fiber app with middleware
  - Registers all HTTP routes
  - Starts listening on configured port

**HTTP Routes:**
- Location: `internal/server/server.go` lines 224-357
- Pattern: All routes registered with handlers in single function
- Categories: Public pages (/, /search, /packs), auth (login, signup, oauth), resource management, admin, API (/v1/*)

## Error Handling

**Strategy:** Middleware-first with graceful degradation

**Patterns:**

1. **Handler Returns Error**: Handlers return `error` which Fiber converts to HTTP response (typically 500 if no custom handling)

2. **Validation Errors**: Many handlers validate input and use SetFlash() to display user-friendly messages, then redirect

3. **Authentication Errors**: Early-exit middleware (RequireAuth) redirect to /login rather than returning 4xx

4. **Database Errors**: Handlers check `gorm.DB.Error` and handle missing records with redirects or JSON errors

5. **Request Body Parsing**: Handlers use `c.BindJSON()` with struct tags; errors are caught and returned

6. **Template Errors**: Render timeout (5 seconds) prevents hanging pack template renders via context cancellation

## Cross-Cutting Concerns

**Logging:**
- Framework: Standard Go logger in handlers, structured logger in `internal/services/logger/`
- Approach: Fiber middleware logs all requests with request ID; errors logged in handlers with context

**Validation:**
- Approach: Manual validation in handlers (password strength, URL format, HCL parsing)
- HCL parsing: `internal/handlers/utils.go` uses hashicorp/hcl library for pack metadata and variables

**Authentication:**
- Multiple strategies:
  - Local auth: bcrypt password hashing stored in `models.User.PasswordHash`
  - OAuth: GitHub/GitLab via goth library
  - Session: Fiber session store with 24-hour expiration + 7-day absolute limit
  - Authorization: Role-based (admin, org owner, regular member) via middleware checks

**CSRF Protection:**
- Middleware: `csrf.New()` validates tokens from headers (X-CSRF-Token) or form fields (_csrf)
- Single-use: Disabled (token reuse allowed)
- Same-site: Lax cookie (permits OAuth redirects)

**Rate Limiting:**
- Applied to: Search endpoints (60/min), voting (30/min), auth attempts (5/15min)
- Strategy: IP-based limiter via Fiber middleware
- Returns 429 with user message when exceeded

---

*Architecture analysis: 2026-02-04*
