# Technology Stack

**Analysis Date:** 2026-02-04

## Languages

**Primary:**
- Go 1.25.5 - Backend server and CLI application

## Runtime

**Environment:**
- Go runtime (built-in)

**Package Manager:**
- Go modules (`go.mod`, `go.sum`)
- Lockfile: Present

## Frameworks

**Core:**
- Fiber v2.52.10 - HTTP web framework
- Cobra v1.8.1 - CLI command framework

**Templates:**
- `gofiber/template/html/v2` v2.1.3 - HTML template rendering

**API Documentation:**
- Swagger v1.16.6 - OpenAPI specification generation
- `gofiber/swagger` v1.1.1 - Swagger UI integration

**Data:**
- GORM v1.31.1 - ORM for database abstraction
- `gorm.io/driver/postgres` v1.6.0 - PostgreSQL driver

**Authentication:**
- goth v1.82.0 - OAuth2/OIDC abstraction
- `shareed2k/goth_fiber` v0.3.3 - Goth integration for Fiber
- `golang.org/x/crypto` v0.46.0 - Cryptographic operations

**Logging:**
- zerolog v1.34.0 - Structured logging

**Configuration:**
- HCL v2.24.0 - HashiCorp Configuration Language parsing (for job/pack validation)
- YAML v3.0.1 - YAML parsing

## Key Dependencies

**Critical:**
- `github.com/gofiber/fiber/v2` v2.52.10 - Foundation of HTTP server
- `gorm.io/gorm` v1.31.1 - Database ORM ensuring data persistence
- `github.com/markbates/goth` v1.82.0 - OAuth/social login functionality

**Infrastructure:**
- `github.com/testcontainers/testcontainers-go` v0.40.0 - Docker-based test database containers
- `github.com/testcontainers/testcontainers-go/modules/postgres` v0.40.0 - PostgreSQL test containers
- `github.com/rs/zerolog` v1.34.0 - Structured logging for observability

**Build & Documentation:**
- `github.com/swaggo/swag` v1.16.6 - Swagger/OpenAPI doc generation
- `github.com/spf13/cobra` v1.8.1 - CLI framework

**Utilities:**
- `golang.org/x/oauth2` v0.34.0 - OAuth2 utilities
- `gopkg.in/yaml.v3` v3.0.1 - YAML unmarshaling
- `github.com/stretchr/testify` v1.11.1 - Testing assertions

## Configuration

**Environment:**
- Environment-based configuration via `os.Getenv()`
- Key configs:
  - `ENV` - "production" or development
  - `BASE_URL` - Public application URL
  - `DATABASE_URL` - PostgreSQL connection string
  - `SESSION_SECRET` - Session encryption key
  - `GITHUB_KEY`, `GITHUB_SECRET` - OAuth credentials
  - `GITLAB_KEY`, `GITLAB_SECRET` - OAuth credentials
  - `SMTP_*` - Email service credentials
  - `GITHUB_REQUESTS_TOKEN` - GitHub API token
  - `GITHUB_WEBHOOK_SECRET` - Webhook verification

**Build:**
- `Dockerfile` - Multi-stage Docker build (builder + runtime)
- `.air.toml` - Air live reload configuration for development
- Tailwind CSS v4.1.18 - CSS framework (CLI-based compilation)

## Platform Requirements

**Development:**
- Go 1.25.5
- PostgreSQL 15+ (local or Docker)
- Docker & Docker Compose (for containerized dev)
- Make (for build commands)

**Production:**
- PostgreSQL 15+ database
- Container runtime (Docker/Kubernetes)
- Reverse proxy with SSL/TLS (Traefik, Caddy, nginx)
- 1GB RAM minimum, 2GB+ recommended

---

*Stack analysis: 2026-02-04*
