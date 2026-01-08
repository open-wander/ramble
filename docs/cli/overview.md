# CLI Overview

The Ramble CLI lets you discover, render, and run Nomad packs directly from the command line.

## Installation

See [Getting Started](../getting-started.md) for installation instructions.

## Configuration

Ramble stores its configuration in `~/.config/ramble/config.json`.

### Default Registry

The default Ramble registry is pre-configured:

```json
{
  "default_registry": "ramble",
  "registries": {
    "ramble": {
      "url": "https://ramble.openwander.org"
    }
  }
}
```

### Adding Registries

```bash
# Add a new registry
ramble registry add myregistry https://my-registry.example.com

# Add with a default namespace
ramble registry add myregistry https://my-registry.example.com --namespace myteam

# Set as default
ramble registry default myregistry
```

## Environment Variables

The CLI respects standard Nomad environment variables:

| Variable | Description |
|----------|-------------|
| `NOMAD_ADDR` | Nomad server address (default: `http://127.0.0.1:4646`) |
| `NOMAD_TOKEN` | Nomad ACL token |
| `NOMAD_CACERT` | Path to CA certificate |
| `NOMAD_CLIENT_CERT` | Path to client certificate |
| `NOMAD_CLIENT_KEY` | Path to client key |

## Global Flags

| Flag | Description |
|------|-------------|
| `--registry, -r` | Registry to use (overrides default) |
| `--help, -h` | Show help for any command |

## Command Categories

- [Pack Commands](pack.md) - Discover, render, and run packs
- [Job Commands](job.md) - Run and validate job files
- [Registry Commands](registry.md) - Manage registries
- [Cache Commands](cache.md) - Manage local cache
