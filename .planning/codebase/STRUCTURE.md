# Codebase Structure

**Analysis Date:** 2026-02-04

## Directory Layout

```
ramble/
├── cmd/                        # Executable entry points
│   ├── ramble/                 # Main CLI binary
│   │   └── main.go             # Calls cmd.Execute()
│   └── nomad-vars/             # Utility binary (Nomad integration)
│
├── internal/                   # Private packages (not importable by external code)
│   ├── cli/                    # CLI command definitions
│   │   ├── cmd/                # Cobra command implementations
│   │   │   ├── root.go         # Root command, subcommand registration
│   │   │   ├── server.go       # "ramble server" command
│   │   │   ├── pack*.go        # Pack-related commands (list, run, render, info)
│   │   │   ├── job*.go         # Job-related commands (list, run, info, validate)
│   │   │   ├── registry.go     # Registry browsing commands
│   │   │   ├── cache.go        # Cache management command
│   │   │   └── migrate.go      # Database migration command
│   │   ├── config/             # CLI configuration handling
│   │   └── update/             # Update checking logic
│   │
│   ├── handlers/               # HTTP request handlers (main business logic)
│   │   ├── web.go              # Page handlers (Home, Search, GetPacks)
│   │   ├── resource.go         # Resource CRUD operations, webhook handling
│   │   ├── auth.go             # Authentication (login, signup, OAuth)
│   │   ├── admin.go            # Admin panel handlers
│   │   ├── nomad_pack.go       # Pack-specific operations
│   │   ├── pages.go            # Misc page handlers (docs, about)
│   │   ├── org.go              # Organization management
│   │   ├── requests.go         # Community pack/job requests
│   │   ├── github_sync.go      # GitHub integration for requests
│   │   ├── seo.go              # SEO data generation
│   │   ├── sitemap.go          # Sitemap generation
│   │   ├── audit.go            # Audit logging
│   │   ├── utils.go            # Shared utilities (GitHub/GitLab API calls, HCL parsing)
│   │   ├── helpers.go          # Template helper functions
│   │   ├── web_extra.go        # Extra web utilities
│   │   └── dev.go              # Development-only handlers
│   │
│   ├── server/                 # Web server setup and routing
│   │   └── server.go           # Fiber app configuration, middleware setup, route registration
│   │
│   ├── database/               # Database connection and migrations
│   │   ├── database.go         # PostgreSQL connection, AutoMigrate setup
│   │   ├── seed.go             # Initial data seeding
│   │   └── test_helper.go      # Test database utilities
│   │
│   ├── models/                 # Data models
│   │   └── models.go           # GORM models: User, Organization, NomadResource, etc.
│   │
│   ├── services/               # Cross-cutting services
│   │   ├── email/              # Email sending service
│   │   ├── github/             # GitHub API integration
│   │   ├── logger/             # Structured logging
│   │   └── version/            # Version management
│   │
│   ├── pack/                   # Pack registry client
│   │   ├── client.go           # HTTP client for Ramble registry API
│   │   └── client_test.go      # Client tests
│   │
│   ├── render/                 # Template rendering engine
│   │   ├── engine.go           # Nomad pack template render engine
│   │   └── funcs.go            # Template functions (meta, var, etc.)
│   │
│   ├── nomad/                  # Nomad job operations
│   │   └── submit.go           # Job submission logic
│   │
│   └── crypto/                 # Cryptographic utilities
│       └── [crypto helpers]    # Password hashing, secrets, etc.
│
├── views/                      # HTML templates (Fiber template engine)
│   ├── layouts/                # Layout templates (main.html)
│   ├── partials/               # Reusable template fragments
│   ├── index.html              # Home page
│   ├── login.html              # Login page
│   ├── signup.html             # Sign up page
│   ├── resource_detail.html    # Pack/job detail view
│   ├── search.html             # Search results
│   ├── admin/                  # Admin dashboard templates
│   ├── profile.html            # User profile
│   └── [other pages]           # Various pages
│
├── public/                     # Static assets (CSS, JS, images)
│   ├── css/                    # Stylesheets (Tailwind)
│   ├── js/                     # JavaScript (HTMX, minimal JS)
│   └── img/                    # Images, icons
│
├── api-docs/                   # Swagger/OpenAPI documentation
│   └── swagger.json            # Generated API spec
│
├── docs/                       # Markdown documentation
│   └── [various *.md files]    # User guides, docs
│
├── examples/                   # Example packs/jobs
│   └── [example files]
│
├── .github/                    # GitHub configuration
│   └── workflows/              # CI/CD workflows
│
├── go.mod                      # Go module definition
├── go.sum                      # Go module checksums
├── Makefile                    # Build and test targets
├── Dockerfile                  # Container image definition
├── docker-compose.curr.yml     # Local dev environment
├── VERSION                     # Current version number
├── CLAUDE.md                   # Project-specific Claude instructions
├── README.md                   # Project overview
├── SELF-HOSTING.md             # Self-hosting guide
└── tailwind.config.js          # Tailwind CSS configuration
```

## Directory Purposes

**`cmd/`:**
- Purpose: Executable entry points
- Contains: `main()` functions for CLI binary
- Key files: `cmd/ramble/main.go` (calls `cmd.Execute()` from Cobra root)

**`internal/handlers/`:**
- Purpose: HTTP request handlers implementing all web server functionality
- Contains: One file per feature area (auth, resources, admin, etc.) plus utils and tests
- Key files: `resource.go` (756 lines, largest), `web.go`, `auth.go`, `admin.go`

**`internal/cli/cmd/`:**
- Purpose: CLI command definitions using Cobra framework
- Contains: One file per command (server, pack list, job list, etc.)
- Key files: `root.go` (command setup), `server.go` (web server startup)

**`internal/models/`:**
- Purpose: GORM data models
- Contains: All struct definitions for database tables
- Key files: `models.go` (single file with all models)

**`internal/database/`:**
- Purpose: Database connection and lifecycle
- Contains: Connection setup, migrations, test helpers
- Key files: `database.go` (GORM setup), `seed.go` (initial data)

**`internal/server/`:**
- Purpose: Web server setup and routing
- Contains: Fiber configuration, middleware, route definitions
- Key files: `server.go` (entire server configuration)

**`internal/services/`:**
- Purpose: Cross-cutting concerns and external integrations
- Contains: Email, GitHub API, logging, version checking
- Key files: `email/`, `github/`, `logger/`, `version/`

**`internal/pack/`:**
- Purpose: Registry API client
- Contains: HTTP client for querying Ramble or other registries
- Key files: `client.go` (API client), `client_test.go`

**`internal/render/`:**
- Purpose: Nomad pack template rendering
- Contains: Custom template engine for [[variable]] substitution
- Key files: `engine.go` (template execution), `funcs.go` (template functions)

**`views/`:**
- Purpose: HTML templates rendered server-side
- Contains: Layout, pages, partials for web UI
- Key files: `layouts/main.html` (base layout), `index.html`, `resource_detail.html`

**`public/`:**
- Purpose: Static web assets
- Contains: Tailwind CSS, minimal JavaScript (HTMX), images
- Key files: Tailwind CSS output, HTMX library

## Key File Locations

**Entry Points:**
- `cmd/ramble/main.go`: CLI entry point - calls `cmd.Execute()`
- `internal/cli/cmd/root.go`: Cobra root command with subcommand registration
- `internal/cli/cmd/server.go`: Server startup command
- `internal/server/server.go`: Web server (Fiber) initialization

**Configuration:**
- `go.mod`: Go module manifest
- `docker-compose.curr.yml`: Local development setup with PostgreSQL
- `Dockerfile`: Container image for production
- `CLAUDE.md`: Project-specific guidelines

**Core Logic:**
- `internal/handlers/`: All HTTP request handling (largest package with 19,500 lines total)
- `internal/models/models.go`: Data model definitions
- `internal/database/database.go`: Database connection and AutoMigrate

**Testing:**
- Test files co-located with implementation: `*_test.go` in same package
- `internal/handlers/*_test.go`: HTTP handler tests
- `internal/database/test_helper.go`: Test database utilities

**Views:**
- `views/layouts/main.html`: Base template included in all pages
- `views/index.html`: Home page
- `views/resource_detail.html`: Pack/job detail pages
- `views/admin/`: Admin dashboard templates

## Naming Conventions

**Files:**
- Handler files: Feature-based naming (`auth.go`, `resource.go`, `admin.go`)
- Test files: Match implementation with `_test.go` suffix (`auth_test.go`)
- CLI commands: Hyphenated feature names (`pack_list.go`, `job_run.go`)
- Models: Single file `models.go` containing all GORM structs

**Directories:**
- Package names: Lowercase, single word or short phrases (`handlers`, `models`, `services`)
- Feature grouping: Nested by feature (`services/email/`, `cli/cmd/`)
- No single-letter directories

**Packages:**
- Internal packages: Located under `internal/` (Go convention for private packages)
- Public packages: Exported types start with capital letter (Go convention)

**Functions:**
- HTTP handlers: Capitalized action verbs (Get/Post/Delete + feature) e.g., `GetLogin()`, `PostNewResource()`
- Middleware: Named patterns like `RequireAuth()`, `RequireVerifiedEmail()`
- Utility: Lowercase helpers like `escapeLikeString()`, `downloadFile()`

**Variables:**
- Struct fields: Capitalized for exported, follow CamelCase
- Local variables: camelCase (Go convention)
- Constants: SCREAMING_SNAKE_CASE (not heavily used)

## Where to Add New Code

**New Feature (e.g., new pack endpoint):**
- Primary code: Add handler function to `internal/handlers/nomad_pack.go` or create new `internal/handlers/feature.go`
- Routes: Register route in `internal/server/server.go` route section (lines 224-357)
- Tests: Create `internal/handlers/feature_test.go` with table-driven tests
- Database: If new data needed, add model to `internal/models/models.go` and add to AutoMigrate in `database.go`
- Views: Create HTML template in `views/` directory

**New Service (e.g., Slack notifications):**
- Implementation: Create `internal/services/slack/slack.go`
- Interface: Define functions in same file or separate interface file
- Tests: Create `internal/services/slack/slack_test.go`
- Usage: Import and call from handlers where needed

**New CLI Command (e.g., `ramble config set`):**
- Implementation: Create `internal/cli/cmd/config.go` with Cobra command definition
- Registration: Add to subcommands in `internal/cli/cmd/root.go` `init()`
- Tests: Create `internal/cli/cmd/config_test.go`
- Shared logic: Extract to `internal/cli/config/` package if reusable

**New Page/Template:**
- Handler: Create function in appropriate `internal/handlers/*.go` file
- Route: Register in `internal/server/server.go`
- View: Create HTML in `views/page_name.html`
- Layout: Use `c.Render("page", data, "layouts/main")` pattern

**Utilities/Helpers:**
- Shared across handlers: Add to `internal/handlers/utils.go`
- Domain-specific: Create new package under `internal/` (e.g., `internal/validation/`)
- Crypto: Add to `internal/crypto/` package
- Rendering: Add template functions to `internal/render/funcs.go`

## Special Directories

**`bin/`:**
- Purpose: Local build outputs
- Generated: Yes (created by Makefile build)
- Committed: No (.gitignored)

**`dist/`:**
- Purpose: Release binaries built by GoReleaser
- Generated: Yes (created by CI/CD)
- Committed: No (.gitignored)

**`api-docs/`:**
- Purpose: Swagger/OpenAPI documentation
- Generated: Yes (generated from Go doc comments with swag tool)
- Committed: Yes (checked in for documentation)

**`.planning/codebase/`:**
- Purpose: GSD codebase analysis documents
- Generated: Yes (created by GSD mapping commands)
- Committed: No (GSD planning directory)

**`public/`:**
- Purpose: Static web assets served directly
- Generated: Partially (Tailwind CSS binary committed, output CSS compiled)
- Committed: Yes (assets needed for web server)

---

*Structure analysis: 2026-02-04*
