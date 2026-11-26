# API Reference

Ramble provides a REST API for programmatic access to packs and jobs.

## OpenAPI Specification

The `/v1` API is described by an OpenAPI 3.0 spec at `api-docs/openapi.yaml` in the repository. It is generated from the handler annotations (`make openapi`), checked for drift in CI, and attached to every GitHub release as `openapi.yaml`. Use it with `oapi-codegen` or any other OpenAPI generator. The interactive docs at `/api-docs` (Swagger UI) still serve the Swagger 2.0 document, which also covers the HTML routes.

## Content Negotiation

The API uses content negotiation. Send `Accept: application/json` header to receive JSON responses.

## Authentication

Public read endpoints need no credentials. To call the API as a user, create an API token at `/settings/tokens` and send it as a Bearer token:

```bash
curl -H "Authorization: Bearer <your-token>" https://ramble.openwander.org/v1/user
```

- Scope: a `read` token can call read endpoints; a `write` token can also call write endpoints.
- Organisation: a token can optionally be restricted to one organisation, and then only acts inside it.
- Expiry: every token expires; the expiry is chosen at creation.
- The token value is shown once when it is created. Store it safely; only a hash is kept.

A missing, revoked or expired token returns `401` in the shape described under [Error Responses](#error-responses).

### Current User

`GET /v1/user` identifies the token's user and describes the token. It is meant for login checks and credential validation in tools such as the CLI and the Terraform provider.

```json
{
  "username": "alice",
  "token_scope": "read",
  "token_org": "acme",
  "token_expires_at": "2027-01-15T10:30:00Z"
}
```

`token_scope` is `read` or `write`. `token_org` is the organisation name when the token is restricted to one, otherwise `null`. No other user fields are returned.

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
