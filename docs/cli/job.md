# Job Commands

Commands for listing, running, and validating Nomad job files.

## job list

List jobs from a registry.

```bash
# List all jobs
ramble job list

# List from specific namespace
ramble job list --namespace myuser

# Search jobs
ramble job list --search postgres

# Use a different registry
ramble job list --registry myregistry
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--namespace` | `-n` | Filter by namespace |
| `--search` | `-s` | Search query |
| `--registry` | `-r` | Registry to use |

## job info

Get detailed information about a job.

```bash
ramble job info myuser/postgres

# Show specific version
ramble job info myuser/postgres@v1.0.0
```

**Output includes:**
- Job name and description
- Available versions
- README content

## job run

Submit a Nomad job file.

```bash
# Run a local job file
ramble job run myjob.nomad.hcl

# Run from registry
ramble job run myuser/postgres

# Run specific version
ramble job run myuser/postgres@v1.0.0

# Dry run (validate only)
ramble job run myjob.nomad.hcl --dry-run
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--dry-run` | | Validate only, don't submit to Nomad |
| `--registry` | `-r` | Registry to use (for registry jobs) |

## job validate

Validate a Nomad job file without submitting.

```bash
# Validate local file
ramble job validate myjob.nomad.hcl

# Validate from registry
ramble job validate myuser/postgres
```

This command parses the job file and runs `nomad job validate` to check for errors.

## Job File Format

Ramble works with standard Nomad job files:

```hcl
job "example" {
  datacenters = ["dc1"]
  type = "service"

  group "web" {
    count = 3

    task "server" {
      driver = "docker"

      config {
        image = "nginx:latest"
        ports = ["http"]
      }

      resources {
        cpu    = 100
        memory = 128
      }
    }

    network {
      port "http" {
        to = 80
      }
    }
  }
}
```

Jobs can use `.nomad` or `.nomad.hcl` file extensions.
