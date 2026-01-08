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

All errors follow this format:

```json
{
  "error": "Error message here"
}
```

**HTTP Status Codes:**

| Code | Description |
|------|-------------|
| 200 | Success |
| 400 | Bad Request |
| 404 | Not Found |
| 500 | Internal Server Error |

## Swagger Documentation

Full API documentation is available at `/swagger/` on development instances.
