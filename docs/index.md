# Ramble Documentation

Ramble is a registry for HashiCorp Nomad job files and Nomad Packs, with a built-in CLI for discovering, rendering, and running packs.

## Quick Links

- [Getting Started](getting-started.md) - Installation and first steps
- [Web Interface](web-interface.md) - Using the registry website
- [CLI Overview](cli/overview.md) - Command-line tool usage
- [API Reference](api.md) - REST API documentation

## Features

### Registry Server
- **Modern UI** - Responsive design with live search
- **Job & Pack Registry** - Discover and share Nomad specifications
- **Versioning** - Git tag-based version management
- **Webhooks** - Automatic updates on new releases
- **Organizations** - Team collaboration features

### CLI Tool
- **Pack Discovery** - Search and browse packs from registries
- **Template Rendering** - Render pack templates with variables
- **Direct Submission** - Run packs and jobs on Nomad clusters
- **Multi-Registry** - Manage multiple registries
- **Caching** - Local pack caching for offline use

## CLI Commands

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
