# External Integrations

**Analysis Date:** 2026-02-04

## APIs & External Services

**OAuth2 / Social Authentication:**
- GitHub OAuth - User authentication via GitHub
  - SDK/Client: `github.com/markbates/goth` v1.82.0
  - Env vars: `GITHUB_KEY`, `GITHUB_SECRET`
  - Scopes: `public_repo`, `user:email`
  - Implementation: `internal/handlers/auth.go` - InitSession() configures goth providers

- GitLab OAuth - User authentication via GitLab
  - SDK/Client: `github.com/markbates/goth` v1.82.0
  - Env vars: `GITLAB_KEY`, `GITLAB_SECRET`
  - Scopes: `read_api`, `read_user`
  - Implementation: `internal/handlers/auth.go`

**GitHub API Integration:**
- Issues API - Create and manage GitHub issues for pack/job requests
  - Base URL: `https://api.github.com`
  - Auth: Bearer token (`GITHUB_REQUESTS_TOKEN`)
  - Endpoints:
    - POST `/repos/{owner}/{repo}/issues` - Create issue
    - GET `/repos/{owner}/{repo}/issues` - List issues
    - PATCH `/repos/{owner}/{repo}/issues/{issue_number}` - Update issue
  - Client implementation: `internal/services/github/github.go`
  - Handler: `internal/handlers/github_sync.go` - Bidirectional sync with issues

**Version Check Service:**
- GitHub Releases API - Check for latest Ramble version updates
  - Base URL: `https://api.github.com`
  - Endpoint: GET `/repos/open-wander/ramble/releases/latest`
  - Auth: Unauthenticated (public)
  - Client: `internal/services/version/version.go`
  - Used in: Server middleware for showing latest version

## Data Storage

**Databases:**
- PostgreSQL 15+
  - Connection: `DATABASE_URL` env var or individual components (`DB_HOST`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_PORT`)
  - Client: `github.com/jackc/pgx/v5` (via GORM)
  - ORM: `gorm.io/gorm` v1.31.1 with `gorm.io/driver/postgres`
  - Connection file: `internal/database/database.go`
  - SSL mode configuration: `DB_SSLMODE` (default: "require" in production, "disable" in dev)

**File Storage:**
- Local filesystem only
  - Templates: `./views` directory
  - Static files: `./public` directory
  - Documentation: `./docs` directory

**Caching:**
- None currently implemented
- Session state: Fiber session store (cookie-based, in-memory)

## Authentication & Identity

**Auth Provider:**
- OAuth2 (GitHub and GitLab) + Local email/password
  - Implementation: `internal/handlers/auth.go`
  - Flow:
    - OAuth users: Automatic account creation/linking via email
    - Local users: Email/password with bcrypt hashing
    - Session store: `github.com/gofiber/fiber/v2/middleware/session`
    - Session config: 24-hour expiration, max 7-day absolute lifetime

**Token Management:**
- Password reset tokens - Time-limited for password reset flow
  - Stored in: `User.ResetToken`, `User.ResetTokenExpires`
  - Generated in: `internal/handlers/auth.go`

- Email verification tokens - Time-limited for email verification
  - Stored in: `User.VerificationToken`, `User.VerificationTokenExpires`

- OAuth access tokens - Encrypted storage for API access
  - Stored in: `User.AccessToken` (encrypted)
  - Encryption: `internal/crypto/tokens.go` - AES encryption
  - Env var: `SESSION_SECRET` (used as encryption key)

## Monitoring & Observability

**Error Tracking:**
- None detected
- Ad-hoc error logging via standard `log` and zerolog

**Logs:**
- Structured logging via `github.com/rs/zerolog` v1.34.0
  - Location: `internal/services/logger/logger.go`
  - Development: Pretty console output with timestamps and caller info
  - Production: JSON output to stdout
  - Fiber request logging: Per-request with latency, status, method, path
  - Request ID tracking: Fiber `requestid` middleware for distributed tracing

**Health Checks:**
- GET `/` - Health endpoint (confirms application is running)
- Application readiness indicated by successful database connection

## CI/CD & Deployment

**Container Registry:**
- GitHub Container Registry (ghcr.io)
- Image: `ghcr.io/open-wander/ramble:latest`

**Hosting:**
- Docker containers (Alpine Linux base image)
- Deployment: Self-hosted via Docker/Docker Compose or Kubernetes
- Port: 3000 (HTTP)
- Reverse proxy requirement: Yes (for SSL/TLS termination)

**Build Process:**
- Multi-stage Docker build (see `Dockerfile`):
  1. Builder stage: Go 1.25 Bookworm image
  2. Swagger docs generation: `swag init`
  3. CSS build: Tailwind CLI compilation
  4. Go binary compilation: `CGO_ENABLED=0` for static binary
  5. Final stage: Alpine Linux with runtime dependencies

**CI Pipeline:**
- Not detected (GitHub Actions likely used based on ghcr.io registry)

## Environment Configuration

**Required env vars (production):**
- `DATABASE_URL` - PostgreSQL connection string
- `BASE_URL` - Public application URL (e.g., `https://ramble.example.com`)
- `ENV` - Set to `"production"`
- `SESSION_SECRET` - Random string for session encryption (32+ bytes recommended)

**Optional - Initial setup:**
- `AUTO_SEED` - Set to `"true"` to create default admin user (only on first run)
- `INITIAL_USER_USERNAME` - Admin username (if `AUTO_SEED=true`)
- `INITIAL_USER_EMAIL` - Admin email (if `AUTO_SEED=true`)
- `INITIAL_USER_PASSWORD` - Admin password (if `AUTO_SEED=true`)

**Optional - OAuth:**
- `GITHUB_KEY` - GitHub OAuth App Client ID
- `GITHUB_SECRET` - GitHub OAuth App Client Secret
- `GITLAB_KEY` - GitLab OAuth App ID
- `GITLAB_SECRET` - GitLab OAuth App Secret
- `BASE_URL` - Required for OAuth callback URL construction

**Optional - GitHub Integration (pack/job requests):**
- `GITHUB_REQUESTS_TOKEN` - GitHub Personal Access Token (PAT) with `repo` scope
- `GITHUB_WEBHOOK_SECRET` - Random shared secret for webhook signature validation (created by user)
- Database setting: `github_requests_repo` (stored in `SiteSetting` table) - Format: `owner/repo`

**Optional - Email (password reset):**
- `SMTP_HOST` - SMTP server hostname
- `SMTP_PORT` - SMTP port (typically 587)
- `SMTP_USER` - SMTP username
- `SMTP_PASSWORD` - SMTP password
- `FROM_ADDRESS` - Sender email address

**Optional - Development:**
- `DB_HOST`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_PORT` - Individual database components (alternative to `DATABASE_URL`)
- `DB_SSLMODE` - PostgreSQL SSL mode override
- `GITHUB_WEBHOOK_SECRET` (optional in dev for webhook testing)

**Secrets location:**
- Docker Compose: `.env` file (see `docker-compose.yml`)
- Kubernetes: Secrets resources
- Traditional: System environment variables or `.env` files (not committed)

## Webhooks & Callbacks

**Incoming - GitHub:**
- Endpoint: `POST /webhooks/github/issues`
- Payload: GitHub Issue webhook events
- Signature validation: HMAC-SHA256 using `GITHUB_WEBHOOK_SECRET`
- Events accepted: `issues` (opened, edited, closed, reopened)
- Implementation: `internal/handlers/github_sync.go` - SyncFromGitHub()
- Purpose: Sync GitHub issue status changes back to Ramble pack/job requests

**Outgoing - GitHub:**
- Service: GitHub API for creating and updating issues
- Triggered by: Pack/job request creation or status changes
- API calls: POST `/repos/{owner}/{repo}/issues`, PATCH updates
- Implementation: `internal/handlers/github_sync.go` - SyncToGitHub()
- Purpose: Create GitHub issues for pack/job requests and keep them in sync

**Outgoing - OAuth:**
- Redirect callbacks after GitHub/GitLab authentication
- Callback URLs: `{BASE_URL}/auth/{provider}/callback`
- Redirect destinations:
  - GitHub: `{BASE_URL}/auth/github/callback`
  - GitLab: `{BASE_URL}/auth/gitlab/callback`
- Implementation: `internal/handlers/auth.go` - AuthCallback()

## Database Models & Relationships

**Core Models** (see `internal/models/models.go`):
- `User` - User accounts with OAuth linking
- `Organization` - Team/org grouping
- `Membership` - User-Organization relationships
- `NomadResource` - Job/Pack registry items
- `ResourceVersion` - Version history of resources
- `Tag` - Categorization
- `PackRequest` - Community requests with GitHub sync
- `SiteSetting` - Configuration storage
- `AuditLog` - Security audit trail

**Key relationships:**
- Users can belong to multiple Organizations (via Memberships)
- Users can own/contribute to NomadResources
- PackRequests sync bidirectionally with GitHub Issues
- ResourceVersions track historical content and variables

---

*Integration audit: 2026-02-04*
