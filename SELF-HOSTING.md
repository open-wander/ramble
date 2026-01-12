# Self-Hosting Ramble

Ramble is designed to be self-hosted. This guide covers the requirements and general approach for running your own instance.

## Requirements

- **Docker** and **Docker Compose** (recommended)
- **PostgreSQL 15+** database
- A domain with SSL (recommended for production)
- ~1GB RAM minimum, 2GB+ recommended

## Quick Start (Development)

The included `docker-compose.yml` is suitable for local development:

```bash
# Clone the repo
git clone https://github.com/open-wander/ramble.git
cd ramble

# Start services
docker compose up -d

# Access at http://localhost:3001
```

Default credentials (development only):
- Username: `admin`
- Email: `admin@example.com`
- Password: `password123`

## Production Deployment

For production, you'll need:

1. **Reverse proxy with SSL** (Traefik, Caddy, nginx)
2. **PostgreSQL** with persistent storage
3. **Backup strategy** for your database

### Using the Docker Image

```bash
docker pull ghcr.io/open-wander/ramble:latest
```

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `BASE_URL` | Yes | Public URL (e.g., `https://ramble.example.com`) |
| `ENV` | Yes | Set to `production` for production deployments |
| `SESSION_SECRET` | Yes | Random string for session encryption |

#### Database URL Format

```
host=<host> user=<user> password=<password> dbname=<dbname> port=5432 sslmode=disable
```

#### Initial Admin User

On first run, set these to create an admin user:

| Variable | Description |
|----------|-------------|
| `AUTO_SEED` | Set to `true` for first run |
| `INITIAL_USER_USERNAME` | Admin username |
| `INITIAL_USER_EMAIL` | Admin email |
| `INITIAL_USER_PASSWORD` | Admin password |

**Important**: Set `AUTO_SEED=false` after the first successful start.

#### OAuth (Optional)

| Variable | Description |
|----------|-------------|
| `GITHUB_KEY` | GitHub OAuth App Client ID |
| `GITHUB_SECRET` | GitHub OAuth App Client Secret |
| `GITLAB_KEY` | GitLab OAuth App ID |
| `GITLAB_SECRET` | GitLab OAuth App Secret |

#### Community Requests / GitHub Integration (Optional)

For the pack/job request feature with GitHub issue sync:

| Variable | Description |
|----------|-------------|
| `GITHUB_REQUESTS_TOKEN` | GitHub Personal Access Token with `repo` scope for creating/updating issues |
| `GITHUB_WEBHOOK_SECRET` | Random secret string for validating GitHub webhook signatures |

**Setup Steps:**

1. **Create a GitHub PAT:**
   - Go to GitHub Settings > Developer settings > Personal access tokens > Tokens (classic)
   - Generate new token with `repo` scope (or `public_repo` for public repos only)
   - Set as `GITHUB_REQUESTS_TOKEN`

2. **Configure the requests repository:**
   - In Ramble admin panel (/admin/settings), set `github_requests_repo` to `owner/repo`
   - This is where issues will be created for pack/job requests

3. **Set up the webhook (for automatic sync):**
   - Generate a random secret: `openssl rand -hex 32`
   - Set as `GITHUB_WEBHOOK_SECRET` in your environment
   - In GitHub repo: Settings > Webhooks > Add webhook
     - Payload URL: `https://your-domain.com/webhooks/github/issues`
     - Content type: `application/json`
     - Secret: same value as `GITHUB_WEBHOOK_SECRET`
     - Events: Select "Issues" only

**Note:** `GITHUB_REQUESTS_TOKEN` is a GitHub API token for authentication. `GITHUB_WEBHOOK_SECRET` is a shared secret you create yourself (any random string) - it's not a GitHub token.

#### Email (Optional)

For password reset functionality:

| Variable | Description |
|----------|-------------|
| `SMTP_HOST` | SMTP server hostname |
| `SMTP_PORT` | SMTP port (usually 587) |
| `SMTP_USER` | SMTP username |
| `SMTP_PASSWORD` | SMTP password |
| `FROM_ADDRESS` | From email address |

### Example Production Compose

```yaml
services:
  ramble:
    image: ghcr.io/open-wander/ramble:latest
    restart: unless-stopped
    depends_on:
      - db
    environment:
      DATABASE_URL: "host=db user=ramble password=changeme dbname=ramble port=5432 sslmode=disable"
      BASE_URL: "https://ramble.example.com"
      ENV: "production"
      SESSION_SECRET: "generate-a-random-32-byte-hex-string"
      AUTO_SEED: "false"
    ports:
      - "3000:3000"

  db:
    image: postgres:15-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: ramble
      POSTGRES_PASSWORD: changeme
      POSTGRES_DB: ramble
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

### SSL/TLS

Ramble expects to run behind a reverse proxy that handles SSL termination. Popular options:

- **Traefik** - Automatic Let's Encrypt, Docker-native
- **Caddy** - Simple config, automatic HTTPS
- **nginx** - Traditional, widely documented

When `ENV=production`, Ramble enforces secure cookies, so HTTPS is required.

### Database Backups

**Critical**: Always implement a backup strategy before going to production.

Simple approach using `pg_dump`:

```bash
# Backup
pg_dump -h localhost -U ramble -d ramble | gzip > backup_$(date +%Y%m%d).sql.gz

# Restore
gunzip -c backup_20240115.sql.gz | psql -h localhost -U ramble -d ramble
```

Consider:
- Daily automated backups
- Offsite storage (S3, Backblaze B2, etc.)
- Regular restore testing

## Building from Source

```bash
# Install dependencies
make bootstrap

# Build
make build

# Run
./bin/ramble
```

Or build the Docker image:

```bash
docker build -t ramble:local .
```

## Health Check

Ramble exposes a health endpoint:

```bash
curl http://localhost:3000/
```

## Security Considerations

1. **Use strong passwords** for database and admin accounts
2. **Enable HTTPS** in production (required for secure cookies)
3. **Restrict database access** - don't expose PostgreSQL publicly
4. **Keep updated** - pull new images regularly
5. **Backup regularly** - test your restore procedure

## Getting Help

- GitHub Issues: https://github.com/open-wander/ramble/issues
- Documentation: https://ramble.openwander.org (when available)
