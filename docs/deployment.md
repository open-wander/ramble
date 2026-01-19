# Deployment Guide

This guide covers automated deployment of Ramble to production.

## Overview

Ramble uses GitHub Actions for automated deployment. When you trigger a release:

1. VERSION file is automatically bumped (v0.3.7 -> v0.3.8)
2. A git tag is created and pushed
3. Docker image is built for amd64 and arm64
4. Image is pushed to GitHub Container Registry
5. Server is updated via SSH (docker compose)
6. Health check verifies the deployment
7. GoReleaser creates a GitHub release with CLI binaries

## Server Structure

Production runs on a single server with:

```
/opt/wander/
  docker-compose.yml    # Full stack config
  .env                  # Environment variables
  website/              # Static website files
  backup/               # Backup service Dockerfile

/mnt/data/
  postgres/             # Database files
  traefik/              # SSL certs, access logs
  backups/              # Daily SQL backups
  goaccess/             # Analytics reports
  geoip/                # GeoIP database
  htpasswd/             # Basic auth credentials
```

## GitHub Secrets Required

Add these secrets to your repository (Settings > Secrets and variables > Actions):

| Secret | Description | Example |
|--------|-------------|---------|
| `SSH_PRIVATE_KEY` | Private SSH key for server access | Contents of ~/.ssh/id_ed25519 |
| `SSH_HOST` | Production server hostname or IP | server.example.com |
| `SSH_USER` | SSH username | deploy |
| `SSH_PORT` | SSH port | 22 |

## Triggering a Release

### Option 1: GitHub Actions UI

1. Go to Actions > "Release and Deploy"
2. Click "Run workflow"
3. Select version bump type:
   - **patch**: 0.3.7 -> 0.3.8 (bug fixes)
   - **minor**: 0.3.7 -> 0.4.0 (new features)
   - **major**: 0.3.7 -> 1.0.0 (breaking changes)
4. Choose deployment targets
5. Click "Run workflow"

### Option 2: Command Line

Using Make:

```bash
# Interactive - prompts for version type
make release

# Direct commands
make release-patch   # v0.3.7 -> v0.3.8
make release-minor   # v0.3.7 -> v0.4.0
make release-major   # v0.3.7 -> v1.0.0
```

Using GitHub CLI:

```bash
gh workflow run release.yml \
  -f bump_type=patch \
  -f deploy_compose=true \
  -f deploy_nomad=false
```

## What Happens During Deployment

1. **Version Bump**: Updates VERSION file, commits, creates git tag
2. **Docker Build**: Builds multi-arch image (amd64 + arm64)
3. **Push to GHCR**: Pushes `ghcr.io/open-wander/ramble:0.3.8` and `:latest`
4. **SSH Deploy**:
   - Updates docker-compose.yml with new version
   - Pulls new image
   - Restarts only the ramble container (traefik, db stay running)
5. **Health Check**: Verifies site responds at https://ramble.openwander.org
6. **GitHub Release**: Creates release with CLI binaries via GoReleaser

## Monitoring Deployment

Watch the workflow progress:
```bash
gh run watch
```

Check container status on server:
```bash
ssh user@server "cd /opt/wander && docker compose ps"
ssh user@server "docker compose -f /opt/wander/docker-compose.yml logs -f ramble"
```

## Rollback

To rollback to a previous version:

```bash
# SSH to server
ssh user@server

# Change to wander directory
cd /opt/wander

# Update to specific version
sed -i 's|ghcr.io/open-wander/ramble:[0-9.]*|ghcr.io/open-wander/ramble:0.3.6|' docker-compose.yml

# Pull and restart
docker compose pull ramble
docker compose up -d ramble

# Verify
docker compose ps
curl -I https://ramble.openwander.org
```

## Manual Deployment

If you need to deploy without GitHub Actions:

```bash
# Build and push locally
make docker-build
make docker-push

# Or on server, just pull latest
ssh user@server "cd /opt/wander && docker compose pull ramble && docker compose up -d ramble"
```

## Troubleshooting

### Deployment failed - SSH connection

Check that:
- SSH_PRIVATE_KEY secret is set correctly (include full key with headers)
- SSH_HOST and SSH_USER are correct
- Server allows connections from GitHub Actions IPs
- SSH key is authorized on the server

### Container won't start

Check logs:
```bash
ssh user@server "docker compose -f /opt/wander/docker-compose.yml logs ramble"
```

### Health check fails but container is running

The container may need more time to start. Check if it's healthy:
```bash
ssh user@server "docker inspect ramble --format='{{.State.Health.Status}}'"
```
