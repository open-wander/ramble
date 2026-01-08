# Pack Commands

Commands for discovering, rendering, and running Nomad packs.

## pack list

List packs from a registry.

```bash
# List all packs
ramble pack list

# List from specific namespace
ramble pack list --namespace myuser

# Search packs
ramble pack list --search mysql

# Use a different registry
ramble pack list --registry myregistry
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--namespace` | `-n` | Filter by namespace |
| `--search` | `-s` | Search query |
| `--registry` | `-r` | Registry to use |

## pack info

Get detailed information about a pack.

```bash
ramble pack info myuser/mysql

# Show specific version
ramble pack info myuser/mysql@v1.2.0
```

**Output includes:**
- Pack name and description
- Available versions
- Variable definitions
- README content

## pack render

Render pack templates without submitting to Nomad.

```bash
# Render with variables
ramble pack render myuser/mysql --var db_name=mydb --var port=3306

# Render specific version
ramble pack render myuser/mysql@v1.2.0 --var db_name=mydb

# Variables from file
ramble pack render myuser/mysql --var-file vars.hcl

# Output to file
ramble pack render myuser/mysql --var db_name=mydb --output job.nomad.hcl
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--var` | `-v` | Set variable (repeatable) |
| `--var-file` | `-f` | Load variables from HCL file |
| `--output` | `-o` | Write output to file |
| `--registry` | `-r` | Registry to use |

## pack run

Download, render, and submit a pack to Nomad.

```bash
# Run a pack
ramble pack run myuser/mysql --var db_name=mydb

# Dry run (render only, don't submit)
ramble pack run myuser/mysql --var db_name=mydb --dry-run

# Run specific version
ramble pack run myuser/mysql@v1.2.0 --var db_name=mydb

# Variables from file
ramble pack run myuser/mysql --var-file vars.hcl
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--var` | `-v` | Set variable (repeatable) |
| `--var-file` | `-f` | Load variables from HCL file |
| `--dry-run` | | Render only, don't submit to Nomad |
| `--registry` | `-r` | Registry to use |

## Variable Files

Variables can be loaded from HCL files:

```hcl
# vars.hcl
db_name    = "mydb"
port       = 3306
datacenters = ["dc1", "dc2"]
resources = {
  cpu    = 500
  memory = 256
}
```

Use with `--var-file vars.hcl`.

## Template Functions

Pack templates use `[[ ]]` delimiters and support these functions:

| Function | Description | Example |
|----------|-------------|---------|
| `var "name" .` | Get variable value | `[[ var "db_name" . ]]` |
| `meta "key" .` | Get pack metadata | `[[ meta "pack.name" . ]]` |
| `quote` | Wrap in quotes | `[[ var "name" . \| quote ]]` |
| `toStringList` | Convert to HCL list | `[[ var "dcs" . \| toStringList ]]` |
| `coalesce` | First non-empty value | `[[ coalesce (var "name" .) "default" ]]` |
| `toJSON` | Convert to JSON | `[[ var "config" . \| toJSON ]]` |
| `indent N` | Indent by N spaces | `[[ var "block" . \| indent 2 ]]` |
