# API Reference

Ramble provides a REST API for programmatic access to packs and jobs.

## Content Negotiation

The API uses content negotiation. Send `Accept: application/json` header to receive JSON responses.

## Global Endpoints

### List All Packs

```
GET /v1/packs
```

Returns all packs across all namespaces.

**Response:**

```json
{
  "packs": [
    {
      "name": "mysql",
      "description": "MySQL database pack",
      "namespace": "hashicorp",
      "latest_version": "v1.2.0"
    }
  ]
}
```

### Search Packs

```
GET /v1/packs/search?q=<query>
```

Search packs by name or description.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `q` | string | Search query |

**Example:**

```bash
curl "https://ramble.openwander.org/v1/packs/search?q=mysql"
```

### List All Jobs

```
GET /v1/jobs
```

Returns all jobs across all namespaces.

**Response:**

```json
{
  "jobs": [
    {
      "name": "postgres",
      "description": "PostgreSQL database job",
      "namespace": "myuser",
      "latest_version": "v1.0.0"
    }
  ]
}
```

### Search Jobs

```
GET /v1/jobs/search?q=<query>
```

Search jobs by name or description.

### List Registries

```
GET /v1/registries
```

Returns all available registry namespaces (users and organizations).

**Response:**

```json
{
  "registries": ["hashicorp", "myuser", "myorg"]
}
```

## Namespaced Endpoints

### List Packs by Namespace

```
GET /{namespace}
Accept: application/json
```

Returns packs for a specific user or organization.

**Example:**

```bash
curl -H "Accept: application/json" https://ramble.openwander.org/myuser
```

**Response:**

```json
{
  "packs": [
    {
      "name": "mysql",
      "description": "MySQL database pack"
    }
  ]
}
```

### Get Pack Details

```
GET /{namespace}/{pack}
Accept: application/json
```

Returns detailed information about a pack.

**Example:**

```bash
curl -H "Accept: application/json" https://ramble.openwander.org/myuser/mysql
```

**Response:**

```json
{
  "name": "mysql",
  "description": "MySQL database pack",
  "versions": [
    {
      "version": "v1.2.0",
      "url": "https://ramble.openwander.org/myuser/mysql/v/v1.2.0/raw"
    },
    {
      "version": "v1.1.0",
      "url": "https://ramble.openwander.org/myuser/mysql/v/v1.1.0/raw"
    }
  ]
}
```

### Get Raw Pack Content

```
GET /{namespace}/{pack}/raw
```

Returns the raw pack content (tarball or HCL).

### Get Raw Pack Version

```
GET /{namespace}/{pack}/v/{version}/raw
```

Returns the raw content for a specific version.

## Error Responses

Every error from a `/v1` path uses one shape:

```json
{
  "error": {
    "code": "not_found",
    "message": "pack not found"
  }
}
```

`code` is a stable snake_case identifier to branch on; `message` is for people
and may change. Unknown `/v1` paths return `404 not_found` in the same shape.

| Status | Code | Meaning |
|--------|------|---------|
| 401 | `unauthorized` | Missing, invalid or expired API token |
| 403 | `insufficient_scope` | The token lacks the required scope |
| 403 | `forbidden` | The request is not allowed |
| 403 | `email_unverified` | The account email is not verified |
| 404 | `not_found` | Namespace, pack, job or path does not exist |
| 405 | `method_not_allowed` | Method not supported on this path |
| 429 | `rate_limited` | Too many requests |
| 500 | `internal_error` | Unexpected server or database error |

**Breaking change:** earlier versions returned `{"error": "message"}` (a
string). Clients that read `error` as a string must read `error.message`
instead.

Raw content endpoints (`/{namespace}/{name}/raw`) and the HTML pages are not
part of `/v1`; they return plain-text errors.

## Swagger Documentation

Full API documentation is available at `/swagger/` on development instances.
