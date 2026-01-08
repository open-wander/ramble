# Ramble - Nomad Job & Pack Registry

[![CI](https://github.com/open-wander/ramble/actions/workflows/ci.yml/badge.svg)](https://github.com/open-wander/ramble/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/open-wander/ramble)](https://github.com/open-wander/ramble/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/open-wander/ramble)](https://go.dev/)
[![License](https://img.shields.io/github/license/open-wander/ramble)](LICENSE)
[![Docker](https://img.shields.io/badge/docker-ghcr.io-blue)](https://github.com/open-wander/ramble/pkgs/container/ramble)

A modernized registry for HashiCorp Nomad job files and Nomad Packs, with a built-in CLI for discovering, rendering, and running packs.

## Features

### Registry Server
- **Modern UI:** Responsive design using Tailwind CSS
- **Interactive Search:** Live search-as-you-type powered by HTMX
- **Fast Navigation:** SPA-like experience with server-side rendering
- **Job/Pack Registry:** Easily discover and share Nomad specifications
- **Authentication:** User accounts and OAuth support (GitHub, GitLab)
- **Organizations:** Team collaboration features
- **Webhooks:** Automatic version updates on git tag push

### CLI Tool
- **Pack Discovery:** Search and browse packs from registries
- **Template Rendering:** Render pack templates with variables
- **Direct Submission:** Run packs and jobs on Nomad clusters
- **Multi-Registry:** Manage multiple registries
- **Caching:** Local pack caching for offline use

## Tech Stack

- **Backend:** Go 1.23+, Fiber v2
- **Database:** PostgreSQL (GORM)
- **Frontend:** HTMX, Hyperscript, Tailwind CSS
- **CLI:** Cobra

## Installation

### Download Binary

```bash
# macOS (Apple Silicon)
curl -L https://github.com/open-wander/ramble/releases/latest/download/ramble_Darwin_arm64.tar.gz | tar xz
sudo mv ramble /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/open-wander/ramble/releases/latest/download/ramble_Darwin_x86_64.tar.gz | tar xz
sudo mv ramble /usr/local/bin/

# Linux (x86_64)
curl -L https://github.com/open-wander/ramble/releases/latest/download/ramble_Linux_x86_64.tar.gz | tar xz
sudo mv ramble /usr/local/bin/

# Linux (ARM64)
curl -L https://github.com/open-wander/ramble/releases/latest/download/ramble_Linux_arm64.tar.gz | tar xz
sudo mv ramble /usr/local/bin/

# Windows (x86_64) - download and extract the zip
# https://github.com/open-wander/ramble/releases/latest/download/ramble_Windows_x86_64.zip
```

### Security Notes

**macOS**: If you see "cannot be opened because it is from an unidentified developer", run:
```bash
xattr -d com.apple.quarantine /usr/local/bin/ramble
```

**Windows**: If SmartScreen shows "Windows protected your PC", click "More info" then "Run anyway".

### Using Docker

```bash
docker pull ghcr.io/open-wander/ramble:latest
```

## CLI Quick Start

```bash
# List packs from the default registry
ramble pack list

# Get pack information
ramble pack info myuser/mysql

# Run a pack (renders and submits to Nomad)
ramble pack run myuser/mysql --var db_name=mydb

# Dry run (render only)
ramble pack run myuser/mysql --var db_name=mydb --dry-run

# Run a local job file
ramble job run myjob.nomad.hcl
```

### CLI Commands

```
ramble
├── server              # Start the web server
├── pack
│   ├── list            # List packs from registry
│   ├── info <pack>     # Get pack details
│   ├── run <pack>      # Download, render, and submit to Nomad
│   └── render <pack>   # Render templates without submitting
├── job
│   ├── list            # List jobs from registry
│   ├── info <job>      # Get job details
│   ├── run <file>      # Submit a raw .nomad.hcl file
│   └── validate <file> # Validate a job file
├── registry
│   ├── list            # List configured registries
│   ├── add <name>      # Add a new registry
│   ├── remove <name>   # Remove a registry
│   └── default <name>  # Set default registry
├── cache
│   ├── list            # Show cached packs
│   ├── clear           # Clear cache
│   └── path            # Show cache directory
└── version             # Show version information
```

## Server Quick Start

### Prerequisites

- Go 1.23+
- PostgreSQL

### Local Development

1. **Clone the repository:**
   ```bash
   git clone https://github.com/open-wander/ramble.git
   cd ramble
   ```

2. **Set up the database:**
   ```bash
   export DATABASE_URL="host=localhost user=postgres password=postgres dbname=rmbl port=5432 sslmode=disable"
   ```

3. **Run the server:**
   ```bash
   make run
   # or
   ramble server
   ```
   The server starts on `http://localhost:3000`.

### Using Docker

```bash
docker run -p 3000:3000 \
  -e DATABASE_URL="your-connection-string" \
  -e SESSION_SECRET="your-secret" \
  ghcr.io/open-wander/ramble:latest
```

## Documentation

Full documentation is available at:
- [Getting Started](docs/getting-started.md)
- [Web Interface](docs/web-interface.md)
- [CLI Commands](docs/cli/overview.md)
- [API Reference](docs/api.md)

## Deployment

For production setup and self-hosting instructions, see the [Self-Hosting Guide](SELF-HOSTING.md).

## License

[MPL-2.0](LICENSE)
