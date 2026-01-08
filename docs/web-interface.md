# Ramble Documentation

Ramble is a registry for HashiCorp Nomad job files and Nomad Packs. This guide covers everything you need to publish and consume resources.

## Getting Started

### Creating an Account

1. Click **Sign Up** in the navigation bar
2. Enter your username, email, and password
3. Verify your email if email verification is enabled

You can also sign in with GitHub or GitLab if OAuth is configured on the instance.

### Your Profile

After signing in, access your profile to:
- View your published resources
- Update your account settings
- Generate API tokens (coming soon)

## Publishing Resources

### Adding a Resource

1. Click **New Resource** from your profile or the navigation
2. Fill in the details:
   - **Name**: A unique identifier (lowercase, hyphens allowed)
   - **Type**: Job or Pack
   - **Repository URL**: Git repository containing your resource
   - **Path** (optional): Subdirectory if not in repo root
3. Click **Create**

Ramble will fetch your repository and index the initial version.

### Resource Requirements

To successfully list your Nomad Jobs and Packs, ensure your repositories follow these structures.

#### Nomad Packs

A Nomad Pack is a templated job specification. Required files:

**`metadata.hcl`** (Required)
```hcl
pack {
  name        = "my_pack"
  description = "A description of my pack"
  version     = "1.0.0"
}
```

**`README.md`** (Required) - Documentation displayed on the registry

**`templates/`** (Required) - Directory containing Nomad job templates (e.g., `my_job.nomad.tpl`)

**`variables.hcl`** (Optional) - Input variables for your pack

**`outputs.tpl`** (Optional) - Output messages after deployment

Directory structure:
```
my_pack/
  ├── metadata.hcl
  ├── README.md
  ├── variables.hcl
  └── templates/
      └── my_job.nomad.tpl
```

#### Nomad Jobs

A Nomad Job is a standalone job specification.

**Job File** (Required) - A valid Nomad job file ending in `.nomad` or `.nomad.hcl`

**`README.md`** (Recommended) - Documentation for your job

## Versioning & Releases

Ramble uses Git tags to manage versions.

### Creating a Version

Push a git tag to your repository:
```bash
git tag v1.0.0
git push origin v1.0.0
```

The registry will create a version entry and fetch the README and content for that tag.

### Automatic Updates with Webhooks

To automatically update when you push new tags:

1. Go to your resource's settings page
2. Copy the **Webhook URL** and **Secret**
3. Add a webhook in your Git provider:
   - **GitHub**: Settings > Webhooks > Add webhook
   - **GitLab**: Settings > Webhooks
4. Set content type to `application/json`
5. Select "Tag push events" (or equivalent)

When you push a new tag, Ramble will automatically create the new version.

### Manual Version Updates

You can also add versions manually from the resource detail page if you prefer not to use webhooks.

## Organizations

Organizations let you group resources and collaborate with others.

### Creating an Organization

1. Click **New Organization** from the navigation
2. Enter a name and optional description
3. Click **Create**

### Managing Members

From the organization settings:
- **Owners** can manage settings and members
- **Members** can publish resources under the organization

### Publishing Under an Organization

When creating a new resource, select the organization as the owner instead of your personal account.

## Using the Ramble CLI

The Ramble CLI lets you discover, render, and run packs directly from the registry.

### Installation

See the [Getting Started](getting-started.md) guide for installation instructions.

### Listing Packs

```bash
# List all packs
ramble pack list

# List packs from a specific user
ramble pack list --namespace myuser

# Search for packs
ramble pack list --search mysql
```

### Running a Pack

```bash
# Run a pack (renders and submits to Nomad)
ramble pack run myuser/mysql --var db_name=mydb

# Dry run (render only, don't submit)
ramble pack run myuser/mysql --var db_name=mydb --dry-run

# Run a specific version
ramble pack run myuser/mysql@v1.0.0 --var db_name=mydb
```

### Managing Registries

```bash
# List configured registries
ramble registry list

# Add a custom registry
ramble registry add myregistry https://my-registry.example.com

# Set default registry
ramble registry default myregistry
```

### Viewing Registries

See all available registries on the [Registries](/registries) page.

## Using with nomad-pack (Alternative)

Ramble is also compatible with the `nomad-pack` CLI tool:

```bash
# Add Ramble as a registry
nomad-pack registry add ramble https://ramble.openwander.org

# List packs
nomad-pack registry list --registry=ramble

# Run a pack
nomad-pack run <pack-name> --registry=ramble
```

## API

Ramble provides a REST API for programmatic access. The API uses content negotiation - send `Accept: application/json` header to receive JSON responses.

### Namespaced Endpoints (Content-Negotiated)

| Endpoint | Accept Header | Response |
|----------|---------------|----------|
| `GET /{user}` | `text/html` | User profile page |
| `GET /{user}` | `application/json` | Pack list (JSON) |
| `GET /{user}/{pack}` | `text/html` | Resource detail page |
| `GET /{user}/{pack}` | `application/json` | Pack metadata (JSON) |

### Global Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /v1/packs` | List all packs across all namespaces |
| `GET /v1/packs/search?q=query` | Search packs |
| `GET /v1/registries` | List all registry namespaces |

### Example

```bash
# List packs for a user (JSON)
curl -H "Accept: application/json" https://ramble.openwander.org/myuser

# Get pack details (JSON)
curl -H "Accept: application/json" https://ramble.openwander.org/myuser/my-pack

# Search all packs
curl https://ramble.openwander.org/v1/packs/search?q=traefik
```

Full API documentation is available at `/swagger/` on instances with Swagger enabled (development mode).

## Tips

- **Use semantic versioning** for your tags (e.g., `v1.0.0`, `v1.1.0`)
- **Keep your README updated** - it's the first thing users see
- **Add variables.hcl** for packs to make them configurable
- **Test locally** with `nomad-pack` before publishing
