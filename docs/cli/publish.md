# Publish and Delete

Publish packs and jobs to a registry from the command line, and delete them again. These commands call the authenticated `/v1` API, so they need an API token: run `ramble login` (see [Authentication](/docs/cli/auth)) or set `RAMBLE_TOKEN`. Publishing and deleting need a token with **write** scope. Tokens are only ever sent over https, or plain http to `localhost`, and are never sent to a different host after a redirect.

The `pack` and `job` commands are identical apart from the resource type.

## publish

```bash
ramble pack publish --repo https://github.com/alice/packs --path packs/mysql --version v1.0.0
ramble job publish  --repo https://github.com/alice/jobs  --path web.nomad.hcl --version v2.1.0
```

| Flag | Description |
|------|-------------|
| `--repo` | Git repository URL (required) |
| `--version` | Version to publish, for example `v1.0.0` (required) |
| `--path` | Path of the pack or job inside the repository (default: the repository root) |
| `--namespace` | Namespace to publish under (default: see below) |
| `--name` | Name (default: see below) |
| `--registry, -r` | Registry name (default: the default registry) |

The command looks up `NAMESPACE/NAME` first:

- **Not found:** the resource is created with its first version. The command prints its webhook URL and its webhook secret.
- **Found:** the version is queued, and the registry fetches it in the background. If the existing resource is a job and you ran `ramble pack publish` (or the reverse), the command fails and tells you which command to use.
- **Version already exists:** the registry's message is printed and the command exits non-zero.

### Defaults

`--name` defaults to the last element of `--path`, without a `.nomad.hcl`, `.nomad` or `.hcl` extension (so `web.nomad.hcl` publishes as `web`). With no `--path`, or `--path .`, it is the last element of the repository URL with any `.git` suffix removed. Names and namespaces may contain letters, digits, `.`, `_` and `-`; pass `--name` when the default does not fit.

`--namespace` defaults to the token's organisation when the token is restricted to one, and to the username of the token owner otherwise. The command prints `Using namespace X` on stderr when it applies the default. Pass `--namespace` to publish under an organisation you belong to with a personal token.

### The webhook secret

The webhook secret is returned by the registry only when the resource is created. The command prints it once, followed by a note, and it cannot be shown again. Store it, then configure it in your Git host's webhook. If you lose it, rotate it in the web interface.

## delete

```bash
ramble pack delete alice/mysql
ramble job delete alice/web --yes
```

| Flag | Description |
|------|-------------|
| `--yes, -y` | Do not ask for confirmation |
| `--registry, -r` | Registry name (default: the default registry) |

The argument must be `NAMESPACE/NAME`. The command looks the resource up, checks that its type matches the command, and asks `Delete pack NAMESPACE/NAME? [y/N]`. Only `y` or `yes` proceeds. Deleting removes the resource and all its versions.

When stdin is not a terminal the command refuses to run without `--yes`, even if an answer is piped in.

## Use in CI

Create a token with write scope at `<registry>/settings/tokens` (restrict it to an organisation where possible) and store it as a CI secret named `RAMBLE_TOKEN`. `RAMBLE_TOKEN` applies to whichever registry you select, so set it only in jobs that talk to a registry you trust.

```bash
export RAMBLE_TOKEN="$RAMBLE_API_TOKEN"   # from your CI secret store
ramble pack publish \
  --repo "https://github.com/$GITHUB_REPOSITORY" \
  --path packs/mysql \
  --version "$GIT_TAG"
```

Warning: when the resource does not exist yet, the first publish prints the webhook secret, and CI logs are often readable by many people. Create the resource once from your own terminal, or rotate the webhook secret after the first CI run.

## Errors

The registry's error message is printed with its HTTP status, for example `server error (409): version v1.0.0 already exists`. A 401 suggests `ramble login`, and a 429 means the registry is rate limiting you: try again later.
