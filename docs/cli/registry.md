# Registry Commands

Commands for managing configured registries.

## registry list

List all configured registries.

```bash
ramble registry list
```

**Example output:**

```
Configured registries:

  ramble (default)
    URL: https://ramble.openwander.org

  internal
    URL: https://packs.internal.example.com
    Namespace: myteam
```

## registry add

Add a new registry.

```bash
# Add a registry
ramble registry add myregistry https://packs.example.com

# Add with a default namespace
ramble registry add myregistry https://packs.example.com --namespace myteam

# Add and set as default
ramble registry add myregistry https://packs.example.com --default
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--namespace` | `-n` | Default namespace for this registry |
| `--default` | `-d` | Set as default registry |

## registry remove

Remove a configured registry.

```bash
ramble registry remove myregistry
```

This removes the registry from your configuration. If it was the default, you'll need to set a new default.

## registry default

Set the default registry.

```bash
ramble registry default myregistry
```

The default registry is used when no `--registry` flag is specified.

## Configuration File

Registry configuration is stored in `~/.config/ramble/config.json`:

```json
{
  "default_registry": "ramble",
  "registries": {
    "ramble": {
      "url": "https://ramble.openwander.org",
      "namespace": ""
    },
    "internal": {
      "url": "https://packs.internal.example.com",
      "namespace": "myteam"
    }
  }
}
```

## Using Registries

Once configured, use `--registry` flag with any command:

```bash
# List packs from specific registry
ramble pack list --registry internal

# Run pack from specific registry
ramble pack run mypack --registry internal --var name=test
```

Or set the default registry:

```bash
ramble registry default internal
ramble pack list  # Uses "internal" registry
```
