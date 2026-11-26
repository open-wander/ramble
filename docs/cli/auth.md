# Authentication Commands

Commands for storing an API token so the CLI can call the authenticated parts of the `/v1` API. Create tokens on the web at `<registry>/settings/tokens`.

## login

Store a token for a registry.

```bash
# Default registry, prompts for the token without echoing it
ramble login

# A specific registry (by its configured name)
ramble login --registry internal

# Non-interactive: the first line of stdin is the token
ramble login < token.txt
echo "$TOKEN" | ramble login
```

The token is checked with `GET /v1/user` before it is stored. On success:

```
Logged in to ramble as alice
```

If the server rejects the token, its error message is printed and nothing is stored. Network errors and server errors also store nothing.

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--registry` | `-r` | Registry name (default: the default registry) |

### https only

A token is only sent to `https://` registries. Plain `http://` is accepted only for `localhost`, `127.0.0.0/8` and `::1`, for local development. Redirects are never followed when sending a token.

### Where the token is stored

The token is saved per registry in `~/.config/ramble/config.json` (or `$XDG_CONFIG_HOME/ramble/config.json`). The file is written with mode `0600`, including when an older, more permissive file already existed. `ramble registry list` shows a `LOGGED IN` column but never the token itself.

## logout

Remove the stored token for a registry.

```bash
ramble logout
ramble logout --registry internal
```

Logging out when no token is stored is not an error. Logout does not contact the server: the token stays valid until you revoke it at `<registry>/settings/tokens`.

## RAMBLE_TOKEN

When the `RAMBLE_TOKEN` environment variable is set and non-empty, it is used instead of any stored token, for every registry. Precedence, highest first:

1. `RAMBLE_TOKEN`
2. The token stored for the registry by `ramble login`
3. No token

`ramble login` itself always verifies and stores the token it reads from stdin; it prints a note when `RAMBLE_TOKEN` is set, since that variable will still win.
