# Getting Started

Ramble can be used as both a registry server and a CLI tool.

## Installation

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/open-wander/ramble/releases).

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

**macOS**: The binary is not signed with an Apple Developer certificate. If you see "cannot be opened because it is from an unidentified developer", run:
```bash
xattr -d com.apple.quarantine /usr/local/bin/ramble
```

Alternatively, right-click the binary in Finder, select "Open", and confirm.

**Windows**: The binary is not signed with a Windows code signing certificate. If SmartScreen shows "Windows protected your PC":
1. Click "More info"
2. Click "Run anyway"

### Using Docker

```bash
docker pull ghcr.io/open-wander/ramble:latest
```

## CLI Quick Start

### List Available Packs

```bash
# List all packs from the default registry
ramble pack list

# List packs from a specific namespace
ramble pack list --namespace myuser
```

### Get Pack Information

```bash
ramble pack info myuser/mysql
```

### Run a Pack

```bash
# Render and submit to Nomad
ramble pack run myuser/mysql --var db_name=mydb

# Dry run (render only, don't submit)
ramble pack run myuser/mysql --var db_name=mydb --dry-run
```

### Run a Job File

```bash
# Submit a local job file
ramble job run myjob.nomad.hcl

# Validate without submitting
ramble job validate myjob.nomad.hcl
```

## Server Quick Start

### Prerequisites

- PostgreSQL database
- Environment variables configured

### Running the Server

```bash
# Set required environment variables
export DATABASE_URL="host=localhost user=postgres password=postgres dbname=ramble port=5432 sslmode=disable"
export SESSION_SECRET="your-secret-key"

# Start the server
ramble server

# With custom port
ramble server --port 8080
```

### Using Docker

```bash
docker run -p 3000:3000 \
  -e DATABASE_URL="your-connection-string" \
  -e SESSION_SECRET="your-secret" \
  ghcr.io/open-wander/ramble:latest
```

## Next Steps

- [Web Interface Guide](web-interface.md) - Learn how to use the registry website
- [CLI Commands](cli/overview.md) - Complete CLI reference
- [API Reference](api.md) - REST API documentation
