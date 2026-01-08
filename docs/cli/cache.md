# Cache Commands

Commands for managing the local pack cache.

## Overview

Ramble caches downloaded packs locally to avoid re-downloading them. The cache is organized by:

```
~/.cache/ramble/packs/{registry}/{namespace}/{pack}/{version}/
```

## cache list

List cached packs.

```bash
ramble cache list
```

**Example output:**

```
Cached packs:

  ramble/hashicorp/consul/v1.0.0
    Size: 12.3 KB
    Cached: 2024-01-15 10:30:22

  ramble/myuser/mysql/v2.1.0
    Size: 8.7 KB
    Cached: 2024-01-14 15:45:10

Total: 21.0 KB in 2 packs
```

## cache clear

Clear the cache.

```bash
# Clear entire cache
ramble cache clear

# Clear specific pack
ramble cache clear myuser/mysql

# Clear specific version
ramble cache clear myuser/mysql@v1.0.0
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--all` | `-a` | Clear all cached packs |
| `--registry` | `-r` | Clear only from specific registry |

## cache path

Show the cache directory path.

```bash
ramble cache path
```

**Output:**

```
/Users/yourname/.cache/ramble/packs
```

## Cache Behavior

- **Automatic caching**: Packs are cached when first downloaded
- **Version-specific**: Each version is cached separately
- **No expiration**: Cached packs don't expire automatically
- **Manual clearing**: Use `cache clear` to free space

## Using Cached Packs

When you run a pack, Ramble:

1. Checks if the version is cached
2. If cached, uses the local copy
3. If not cached, downloads and caches it
4. Renders templates from the cached files

To force a fresh download:

```bash
# Clear the specific pack first
ramble cache clear myuser/mysql@v1.0.0

# Then run
ramble pack run myuser/mysql@v1.0.0 --var db_name=test
```
