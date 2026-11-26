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

### Create a Resource

`POST /v1/resources` creates a pack or job. It needs a `write` token and a verified email address, and counts against the write rate limit of 120 requests per minute per token.

```bash
curl -X POST https://ramble.openwander.org/v1/resources \
  -H "Authorization: Bearer <your-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "mysql",
    "type": "pack",
    "namespace": "alice",
    "description": "MySQL on Nomad",
    "repository_url": "https://github.com/alice/packs",
    "file_path": "packs/mysql",
    "version": "v1.0.0",
    "license": "MIT",
    "tags": ["database"]
  }'
```

`name`, `type` (`pack` or `job`), `namespace`, `repository_url` and `version` are required. `namespace` is your own username or the name of an organisation you belong to (case-insensitive). Unknown fields and malformed JSON are rejected with `400 bad_request`. If `license` is empty it is detected from the repository's `LICENSE` file.

The response is `201` with the new resource:

```json
{
  "id": 42,
  "name": "mysql",
  "type": "pack",
  "namespace": "alice",
  "description": "MySQL on Nomad",
  "repository_url": "https://github.com/alice/packs",
  "file_path": "packs/mysql",
  "license": "MIT",
  "tags": ["database"],
  "webhook_url": "https://ramble.openwander.org/resource/42/webhook",
  "webhook_secret": "<secret>",
  "created_at": "2027-01-15T10:30:00Z",
  "updated_at": "2027-01-15T10:30:00Z",
  "latest_version": "v1.0.0"
}
```

`webhook_secret` is returned only in this response. Store it safely; it is not shown again. The content of the version is fetched in the background after the response.

`tags` must be lowercase, 2-20 characters of `a-z`, `0-9` and `-`, with no duplicates. The API rejects anything else with `422 invalid_tags` instead of normalising it (the web form still normalises). The response lists the stored tags sorted by name.

| Status | Code | Cause |
|--------|------|-------|
| 400 | `bad_request` | Malformed JSON, unknown field or trailing data |
| 401 | `unauthorized` | Missing, invalid or expired token |
| 403 | `insufficient_scope` | The token is a `read` token |
| 403 | `email_unverified` | The account email is not verified |
| 403 | `forbidden` | The token is restricted to another organisation, or you are not a member of the organisation |
| 404 | `not_found` | `namespace` is not your username or an existing organisation (another user's name looks the same) |
| 409 | `conflict` | A resource with this name already exists in the namespace |
| 422 | `invalid_name` | Name is not 1-64 characters of letters, digits, dot, dash or underscore |
| 422 | `invalid_type` | `type` is not `pack` or `job` |
| 422 | `invalid_repository_url` | The URL is not an https URL on an allowed git host |
| 422 | `invalid_version` | `version` is empty or longer than 64 characters |
| 422 | `invalid_tags` | A tag is not lowercase 2-20 characters of `a-z`, `0-9` and `-`, or appears twice |
| 429 | `rate_limited` | Too many requests |

### Get a Resource

Both read endpoints need a `read` token (a `write` token also works) and return `200` with the same body as the create response, without `webhook_secret`.

```bash
# by ID
curl https://ramble.openwander.org/v1/resources/42 \
  -H "Authorization: Bearer <your-token>"

# by namespace and name
curl https://ramble.openwander.org/v1/namespaces/alice/resources/mysql \
  -H "Authorization: Bearer <your-token>"
```

`GET /v1/resources/{id}` looks a resource up by its numeric ID. `GET /v1/namespaces/{namespace}/resources/{name}` looks it up by namespace (a username or an organisation name) and name. Both are matched case-insensitively; a username takes precedence over an organisation of the same name. The `namespace/name` form is the import ID of the Terraform provider.

`tags` are sorted by name and `latest_version` is the newest version. A deleted resource is `404`, like one that never existed.

| Status | Code | Cause |
|--------|------|-------|
| 401 | `unauthorized` | Missing, invalid or expired token |
| 403 | `forbidden` | The token is restricted to an organisation and the resource belongs to another organisation or to a personal namespace |
| 404 | `not_found` | The ID, namespace or name does not exist, or the resource was deleted |

### Update a Resource

`PATCH /v1/resources/{id}` changes a resource. It needs a `write` token and counts against the write rate limit. Any member of the owning organisation may edit; a personal resource can only be edited by its owner.

```bash
curl -X PATCH https://ramble.openwander.org/v1/resources/42 \
  -H "Authorization: Bearer <your-token>" \
  -H "Content-Type: application/json" \
  -d '{"description": "MySQL 8 on Nomad", "tags": ["database", "sql"]}'
```

All fields are optional: `description`, `repository_url`, `file_path`, `license` and `tags`. An omitted field, or one set to `null`, is left unchanged. An empty string clears `description` and `file_path`, an empty `license` becomes `Unknown`, and an empty `tags` list (`[]`) removes all tags. `{}` is accepted and changes nothing.

`name`, `type` and `namespace` cannot be changed; sending any of them is rejected with `422 immutable_field` and nothing is written. Unknown fields and malformed JSON are `400 bad_request`. `repository_url` must be an https URL on an allowed git host, otherwise `422 invalid_repository_url`.

The response is `200` with the updated resource, in the same shape as `GET /v1/resources/{id}` (no `webhook_secret`).

| Status | Code | Cause |
|--------|------|-------|
| 400 | `bad_request` | Malformed JSON, unknown field or wrong field type |
| 401 | `unauthorized` | Missing, invalid or expired token |
| 403 | `insufficient_scope` | The token is a read token |
| 403 | `email_unverified` | The account email is not verified |
| 403 | `forbidden` | You are not a member of the owning organisation, do not own the personal resource, or the token is restricted to another organisation |
| 404 | `not_found` | The ID does not exist, or the resource was deleted |
| 422 | `immutable_field` | The body contains `name`, `type` or `namespace` |
| 422 | `invalid_repository_url` | The URL is not an https URL on an allowed git host |
| 422 | `invalid_tags` | A tag is not lowercase 2-20 characters of `a-z`, `0-9` and `-`, or appears twice |
| 429 | `rate_limited` | Write rate limit exceeded |

### Delete a Resource

`DELETE /v1/resources/{id}` deletes a resource. It needs a `write` token and counts against the write rate limit.

```bash
curl -X DELETE https://ramble.openwander.org/v1/resources/42 \
  -H "Authorization: Bearer <your-token>"
```

Only an owner may delete: the owning user of a personal resource, or an owner of the organisation (a plain member gets `403 forbidden`). The response is `204` with no body. Afterwards the resource is `404` on every endpoint, including a second `DELETE`.

A deleted name can be reused: creating a resource (here or in the web UI) with the same name, compared case-insensitively, in the same namespace permanently removes the deleted resource together with its versions, stars and tag links, then creates the new one in the same transaction. A resource that is still live with that name is a `409 conflict`.

| Status | Code | Cause |
|--------|------|-------|
| 401 | `unauthorized` | Missing, invalid or expired token |
| 403 | `insufficient_scope` | The token is a read token |
| 403 | `email_unverified` | The account email is not verified |
| 403 | `forbidden` | You are not an owner, or the token is restricted to another organisation |
| 404 | `not_found` | The ID does not exist, or the resource was already deleted |
| 429 | `rate_limited` | Write rate limit exceeded |

### List Versions of a Resource

`GET /v1/resources/{id}/versions` lists the versions of a resource, newest first. It needs a `read` token.

```bash
curl https://ramble.openwander.org/v1/resources/42/versions \
  -H "Authorization: Bearer <your-token>"
```

```json
[
  {
    "version": "v1.1.0",
    "fetch_status": "completed",
    "fetch_error": "",
    "created_at": "2027-01-20T09:00:00Z"
  }
]
```

`fetch_status` is `pending`, `fetching`, `completed` or `failed`; `fetch_error` explains a failed fetch and is empty otherwise.

| Status | Code | Cause |
|--------|------|-------|
| 401 | `unauthorized` | Missing, invalid or expired token |
| 403 | `forbidden` | The token is restricted to another organisation |
| 404 | `not_found` | The ID does not exist, or the resource was deleted |

### Add a Version

`POST /v1/resources/{id}/versions` adds a version. It needs a `write` token and counts against the write rate limit. Any member of the owning organisation may add versions; a personal resource can only be changed by its owner.

```bash
curl -X POST https://ramble.openwander.org/v1/resources/42/versions \
  -H "Authorization: Bearer <your-token>" \
  -H "Content-Type: application/json" \
  -d '{"version": "v1.1.0"}'
```

`version` is required and at most 64 characters (surrounding whitespace is trimmed). The response is `202` with the new version in the shape of the list above, with `fetch_status` `pending`; the content is fetched from the repository in the background. Poll the list to see the result.

| Status | Code | Cause |
|--------|------|-------|
| 400 | `bad_request` | Malformed JSON, unknown field or trailing data |
| 401 | `unauthorized` | Missing, invalid or expired token |
| 403 | `insufficient_scope` | The token is a `read` token |
| 403 | `forbidden` | You are not a member of the owning organisation, do not own the personal resource, or the token is restricted to another organisation |
| 404 | `not_found` | The ID does not exist, or the resource was deleted |
| 409 | `conflict` | The version already exists |
| 422 | `invalid_version` | `version` is empty or longer than 64 characters |
| 429 | `rate_limited` | Write rate limit exceeded |

### Retry a Version Fetch

`POST /v1/resources/{id}/versions/{version}/retry` fetches the content of a failed version again. It needs a `write` token and counts against the write rate limit. `{version}` is URL-encoded: write `v1.0.0+build` as `v1.0.0%2Bbuild`.

```bash
curl -X POST https://ramble.openwander.org/v1/resources/42/versions/v1.1.0/retry \
  -H "Authorization: Bearer <your-token>"
```

The version is reset to `pending` and fetched in the background; the response is `202` with the version. Only a `failed` version can be retried. Of two concurrent retries, one is accepted and the other gets `409`.

| Status | Code | Cause |
|--------|------|-------|
| 401 | `unauthorized` | Missing, invalid or expired token |
| 403 | `insufficient_scope` | The token is a `read` token |
| 403 | `forbidden` | You are not a member of the owning organisation, do not own the personal resource, or the token is restricted to another organisation |
| 404 | `not_found` | The resource or the version does not exist |
| 409 | `conflict` | The version is not in the `failed` state |
| 429 | `rate_limited` | Write rate limit exceeded |

### Rotate the Webhook Secret

`POST /v1/resources/{id}/webhook-secret` replaces the webhook secret. It needs a `write` token and counts against the write rate limit.

```bash
curl -X POST https://ramble.openwander.org/v1/resources/42/webhook-secret \
  -H "Authorization: Bearer <your-token>"
```

```json
{"webhook_secret": "<secret>"}
```

The new secret is returned only in this response; the old one stops working at once. Update the secret in your repository's webhook settings.

| Status | Code | Cause |
|--------|------|-------|
| 401 | `unauthorized` | Missing, invalid or expired token |
| 403 | `insufficient_scope` | The token is a `read` token |
| 403 | `forbidden` | You are not a member of the owning organisation, do not own the personal resource, or the token is restricted to another organisation |
| 404 | `not_found` | The ID does not exist, or the resource was deleted |
| 429 | `rate_limited` | Write rate limit exceeded |

## Organizations

### Create an Organization

`POST /v1/orgs` creates an organization and makes the caller its owner. It needs a `write` token and a verified email address, and counts against the write rate limit. A token restricted to one organization cannot create organizations.

```bash
curl -X POST https://ramble.openwander.org/v1/orgs \
  -H "Authorization: Bearer <your-token>" \
  -H "Content-Type: application/json" \
  -d '{"name": "myorg", "description": "Our packs and jobs"}'
```

`name` is required: 1-64 characters of letters, digits, dot, dash or underscore, starting with a letter or digit. It must be unique ignoring case, must not match a username and must not be a reserved name. `description` is optional. Unknown fields and malformed JSON are `400 bad_request`.

The response is `201` with the organization:

```json
{
  "name": "myorg",
  "description": "Our packs and jobs",
  "created_at": "2026-11-26T20:00:00Z",
  "member_count": 1
}
```

| Status | Code | Cause |
|--------|------|-------|
| 400 | `bad_request` | Malformed JSON or unknown field |
| 401 | `unauthorized` | Missing, invalid or expired token |
| 403 | `insufficient_scope` | The token is a read token |
| 403 | `email_unverified` | The account email is not verified |
| 403 | `forbidden` | The token is restricted to an organization |
| 409 | `conflict` | The name is taken by an organization or a user |
| 422 | `invalid_name` | The name is empty, malformed or reserved |
| 429 | `rate_limited` | Write rate limit exceeded |

### Get an Organization

`GET /v1/orgs/{name}` returns an organization, looked up ignoring case. It needs a `read` token. Name, description and member count are public on the web, so any token may read any organization; a token restricted to one organization may read only that one. `member_count` counts current members.

| Status | Code | Cause |
|--------|------|-------|
| 401 | `unauthorized` | Missing, invalid or expired token |
| 403 | `forbidden` | The token is restricted to another organization |
| 404 | `not_found` | The organization does not exist |

### Update an Organization

`PATCH /v1/orgs/{name}` changes the description. It needs a `write` token and counts against the write rate limit. Only an owner of the organization may update it.

```bash
curl -X PATCH https://ramble.openwander.org/v1/orgs/myorg \
  -H "Authorization: Bearer <your-token>" \
  -H "Content-Type: application/json" \
  -d '{"description": "New description"}'
```

An omitted or `null` `description` is left unchanged; an empty string clears it. `{}` is accepted and changes nothing. `name` cannot be changed; sending it is rejected with `422 immutable_field`. The response is `200` with the organization, in the same shape as `GET /v1/orgs/{name}`.

| Status | Code | Cause |
|--------|------|-------|
| 400 | `bad_request` | Malformed JSON, unknown field or wrong field type |
| 401 | `unauthorized` | Missing, invalid or expired token |
| 403 | `insufficient_scope` | The token is a read token |
| 403 | `email_unverified` | The account email is not verified |
| 403 | `forbidden` | You are not an owner of the organization, or the token is restricted to another organization |
| 404 | `not_found` | The organization does not exist |
| 422 | `immutable_field` | The body contains `name` |
| 429 | `rate_limited` | Write rate limit exceeded |

### Delete an Organization

`DELETE /v1/orgs/{name}` deletes an organization and its memberships. It needs a `write` token and counts against the write rate limit. Only an owner of the organization may delete it, and only when it has no live resources: delete or move them first. The response is `204` with no body.

```bash
curl -X DELETE https://ramble.openwander.org/v1/orgs/myorg \
  -H "Authorization: Bearer <your-token>"
```

A deleted name can be reused: creating an organization with the same name, compared case-insensitively, permanently removes the deleted organization with its memberships and deleted resources. The one exception is an organization an administrator deleted while it still owned live resources; its name stays taken.

| Status | Code | Cause |
|--------|------|-------|
| 401 | `unauthorized` | Missing, invalid or expired token |
| 403 | `insufficient_scope` | The token is a read token |
| 403 | `email_unverified` | The account email is not verified |
| 403 | `forbidden` | You are not an owner of the organization, or the token is restricted to another organization |
| 404 | `not_found` | The organization does not exist or is already deleted |
| 409 | `org_not_empty` | The organization still has resources; the message gives the count, for example `organization still has 3 resources` |
| 429 | `rate_limited` | Write rate limit exceeded |

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
| 400 | `bad_request` | The request body could not be parsed |
| 401 | `unauthorized` | Missing, invalid or expired API token |
| 403 | `insufficient_scope` | The token lacks the required scope |
| 403 | `email_unverified` | The account email is not verified |
| 403 | `forbidden` | The request is not allowed |
| 404 | `not_found` | Namespace, pack, job or path does not exist |
| 405 | `method_not_allowed` | Method not supported on this path |
| 409 | `conflict` | The name or version is already taken |
| 422 | `invalid_name`, `invalid_type`, `invalid_repository_url`, `invalid_version`, `invalid_tags`, `immutable_field` | A field failed validation or cannot be changed (see the endpoint); `invalid_name` and `immutable_field` apply to resources and organizations |
| 429 | `rate_limited` | Too many requests |
| 500 | `internal_error` | Unexpected server or database error |

**Breaking change:** earlier versions returned `{"error": "message"}` (a
string). Clients that read `error` as a string must read `error.message`
instead.

Raw content endpoints (`/{namespace}/{name}/raw`) and the HTML pages are not
part of `/v1`; they return plain-text errors.

## Swagger Documentation

Full API documentation is available at `/swagger/` on development instances.
