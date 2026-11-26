# PRD: Programmatic Management (API tokens, write API, CLI publish, Terraform provider)

Source: issue #65 ("programmatic way to upload jobs/packs").
Target release: Ramble v0.6.0, plus terraform-provider-ramble v0.1.0.

## 1. Introduction / Overview

Ramble has a read-only JSON API under `/v1` (`ListAllPacksAPI`, `SearchPacksAPI`,
`ListAllJobsAPI`, registries, pack/job detail in `internal/handlers/nomad_pack.go`).
Everything that changes state (registering a pack or job, editing it, publishing a
version, retrying a fetch, deleting, managing organisations and members) is only
reachable through HTML forms that need a browser session cookie and a CSRF token
(`internal/server/server.go`, routes `/new`, `/resource/:id/...`, `/orgs/...`).

This PRD adds:

1. Scoped personal API tokens.
2. Write endpoints under `/v1` for resources (packs and jobs) and organisations.
3. An OpenAPI 3 spec generated from the server and published with every release.
4. `ramble` CLI commands that use the write API.
5. A Terraform provider (`open-wander/terraform-provider-ramble`) whose Go API
   client is generated from that spec, released automatically when a Ramble
   release changes the API.

### Decisions already made

| Topic | Decision |
|---|---|
| Provider build | Hand-written provider on `terraform-plugin-framework`. Go API client generated from Ramble's OpenAPI 3 spec with `oapi-codegen`. HashiCorp's `terraform-plugin-codegen-*` tools are not used: tech preview, last releases 2024, and they generate schemas only, not CRUD logic. |
| Provider versioning | Own semver. Every Ramble release triggers the provider pipeline; a provider release happens only when the spec changed. Provider docs state the minimum Ramble version. |
| Tokens | Personal access tokens per user, hashed at rest, with expiry, `read`/`write` scope, optionally restricted to one organisation. A token acts with its user's permissions, never more. |
| Scope | Packs, jobs, organisations and organisation members. Admin operations are out of scope. |

## 2. Goals

- A user can register, update, version and delete a pack or job with `curl` and a token, with no browser.
- A user can manage an organisation and its members with `curl` and a token.
- `terraform apply` with the provider creates a pack, an organisation and a membership; a second `terraform plan` shows no changes; `terraform destroy` removes them.
- A Ramble release whose spec changed produces a provider pull request with a regenerated client, without manual steps.
- No regression in the existing web UI, CSRF protection or security tests (`go test -tags=security ./...`).

## 3. User Stories

### Phase 1: API tokens

### US-001: Token model and hashing
**Description:** As a maintainer, I want tokens stored so that a database leak does not expose usable tokens.

**Acceptance Criteria:**
- [ ] New model `APIToken` with fields: `ID`, `UserID` (FK, cascade delete), `Name` (1-64 chars), `Prefix` (first 12 chars of the token, indexed), `Hash` (SHA-256 hex of the full token, unique index), `Scope` (`read` or `write`), `OrganizationID` (*uint, nullable, FK cascade delete), `ExpiresAt` (time, not null), `LastUsedAt` (*time), `CreatedAt`, `RevokedAt` (*time). Added to `AutoMigrate` in `internal/database/database.go` and `test_helper.go`.
- [ ] Generated tokens have the form `rmbl_` + 40 chars of base62 from `crypto/rand` (at least 232 bits of entropy); a test asserts the regex `^rmbl_[0-9A-Za-z]{40}$`.
- [ ] The plaintext token is never written to the database or logs; a test creates a token, then asserts no column of the stored row contains the plaintext.
- [ ] `ExpiresAt` defaults to 90 days from creation; values over 365 days are rejected with an error.

### US-002: Token management UI
**Description:** As a user, I want to create and revoke tokens on a settings page so that I can give the CLI or Terraform access.

**Acceptance Criteria:**
- [ ] `GET /settings/tokens` (RequireAuth) lists the user's tokens: name, prefix, scope, organisation, expiry, last used. It never shows the hash or full token.
- [ ] `POST /settings/tokens` (RequireAuth, RequireVerifiedEmail, CSRF) with `name=ci&scope=write&expires_in_days=30` creates a token and renders the full token exactly once with the text "Copy this token now. It will not be shown again."
- [ ] Choosing an organisation the user is not a member of returns 403 and creates no token.
- [ ] `DELETE /settings/tokens/:id` sets `RevokedAt`; revoking another user's token returns 404.
- [ ] Each create and revoke writes an audit event: `token.create`, `token.revoke` (target type `token`, never including the token value).
- [ ] All new template strings follow the project's i18n pattern (second grep pass done).
- [ ] A user can hold at most 25 active tokens; the 26th create returns a form error.

### US-003: Bearer authentication middleware
**Description:** As an API client, I want to authenticate with `Authorization: Bearer <token>` so that I can call write endpoints.

**Acceptance Criteria:**
- [ ] New middleware `RequireAPIToken(scope string)` in `internal/handlers/api_auth.go`. Lookup is by SHA-256 hash with a constant-time comparison.
- [ ] Missing header, malformed header, unknown token, revoked token or expired token returns 401 with body `{"error":{"code":"unauthorized","message":"invalid or expired token"}}`; the message is identical for all five cases.
- [ ] A `read` token on a write endpoint returns 403 `{"error":{"code":"insufficient_scope",...}}`.
- [ ] A token restricted to org A used on a resource or org that belongs to org B or to the user personally returns 403.
- [ ] A token belonging to a user with an unverified email is rejected on write endpoints with 403 (same rule as `RequireVerifiedEmail`).
- [ ] Admin rights are never granted through a token: a token of an admin user calling `/admin/*` with only a Bearer header gets the same response as an anonymous request.
- [ ] On success the middleware sets `c.Locals("User")` and `c.Locals("APIToken")`, and updates `LastUsedAt` at most once per minute per token.
- [ ] Requests to `/v1/*` with an `Authorization: Bearer` header and no session cookie skip CSRF. The exemption is in the same function as the webhook exemption (`server.go` ~line 563) and a security test proves a cookie-authenticated POST to `/v1/*` without a CSRF token still fails.
- [ ] Rate limit: 120 write requests per minute per token; the 121st returns 429.
- [ ] Every request authenticated by token logs `token_prefix` (not the token) on the `http.request` line, and audit events record `via: "token"` plus the token ID in `Details`.

### Phase 2: API contract

### US-004: OpenAPI 3 spec and error format
**Description:** As the provider maintainer, I want an OpenAPI 3 spec of the full `/v1` API so that a client can be generated from it.

**Acceptance Criteria:**
- [ ] `make openapi` produces `api-docs/openapi.yaml` in OpenAPI 3.x and it passes a validator (e.g. `vacuum lint` or `redocly lint`) in CI.
- [ ] Every `/v1` write endpoint has an `operationId`, request schema, response schemas and the `bearerAuth` security scheme.
- [ ] CI fails if `make openapi` produces a diff against the committed file (`git diff --exit-code api-docs/`).
- [ ] All `/v1` errors use `{"error":{"code":"<snake_case>","message":"<text>"}}`; existing read endpoints are migrated and their tests updated.
- [ ] The release workflow attaches `openapi.yaml` to the GitHub release as an asset.

### Phase 3: Resource write API

All endpoints below require `RequireAPIToken("write")` unless stated. Authorisation reuses `resource.Service.AuthorizeResourceAction` (members may edit/version/retry/rotate; delete is owner-only, matching the web UI).

### US-005: Create a resource
**Acceptance Criteria:**
- [ ] `POST /v1/resources` with JSON `{"name":"mysql","type":"pack","namespace":"myorg","description":"...","repository_url":"https://github.com/x/y","file_path":"packs/mysql","version":"v1.0.0","license":"","tags":["db"]}` returns 201 with the resource object (`id`, `name`, `type`, `namespace`, `description`, `repository_url`, `file_path`, `license`, `tags`, `webhook_url`, `webhook_secret`, `created_at`, `updated_at`, `latest_version`).
- [ ] `namespace` is the user's username or an org name; an org the user is not a member of returns 403.
- [ ] The license detection currently inline in `PostNewResource` moves into the resource service so web and API share it; existing `resource_crud` tests still pass.
- [ ] Duplicate name in the namespace returns 409 `conflict`.
- [ ] A repository URL rejected by the existing SSRF checks (`internal/security/ssrf.go`) returns 422 `invalid_repository_url`.
- [ ] Audit event `resource.create` is written, same as the web path.

### US-006: Read and look up a resource
**Acceptance Criteria:**
- [ ] `GET /v1/resources/{id}` (read scope) returns the resource object without `webhook_secret`.
- [ ] `GET /v1/namespaces/{namespace}/resources/{name}` returns the same object; this is what Terraform import uses.
- [ ] Soft-deleted or unknown resources return 404.

### US-007: Update a resource
**Acceptance Criteria:**
- [ ] `PATCH /v1/resources/{id}` accepts any subset of `description`, `repository_url`, `file_path`, `license`, `tags`; omitted fields are unchanged; returns 200 with the updated object.
- [ ] `name`, `type` and `namespace` in the body return 422 `immutable_field` (Terraform will treat them as replace-on-change).
- [ ] Matches the web UI (`resource_crud.go` PostNewVersion/PostRetryFetch): unknown id returns 404 `not_found`; an existing resource the caller may not change returns 403 `forbidden`. Resources are public, so 403 leaks nothing. Applies to every resource endpoint.

### US-008: Delete a resource
**Acceptance Criteria:**
- [ ] `DELETE /v1/resources/{id}` by the owner returns 204; by an org member who is not owner returns 403; audit `resource.delete`.
- [ ] A second DELETE returns 404.

### US-009: Versions, retry and webhook secret
**Acceptance Criteria:**
- [ ] `POST /v1/resources/{id}/versions` with `{"version":"v1.1.0"}` returns 202 and starts the same background fetch as `PostNewVersion`.
- [ ] `GET /v1/resources/{id}/versions` lists versions with `version`, `fetch_status`, `fetch_error`, `created_at`.
- [ ] `POST /v1/resources/{id}/versions/{version}/retry` returns 202 for a version in `failed` status, 409 otherwise.
- [ ] `POST /v1/resources/{id}/webhook-secret` rotates the secret and returns it once in the response; audit `resource.webhook_reset`.

### Phase 4: Organisation write API

### US-010: Organisations
**Acceptance Criteria:**
- [ ] `POST /v1/orgs` with `{"name":"myorg","description":"..."}` returns 201; the caller becomes owner. Name rules and reserved-name checks are the same as `PostCreateOrg` (shared function, not duplicated).
- [ ] `GET /v1/orgs/{name}` (read scope) returns `name`, `description`, `created_at`, `member_count`.
- [ ] `PATCH /v1/orgs/{name}` (owner only) updates `description`; `name` returns 422 `immutable_field`.
- [ ] `DELETE /v1/orgs/{name}` (owner only) soft-deletes an organisation that has no resources and returns 204; memberships are removed; audit `org.delete`. An org that still has resources returns 409 `org_not_empty` with the count. A non-owner member returns 403.
- [ ] The org settings page gets the same "Delete organisation" action with the same rules, so web and API stay identical (FR-5). Admin deletion in `/admin` is unchanged.

### US-011: Organisation members
**Acceptance Criteria:**
- [ ] `GET /v1/orgs/{name}/members` (any member, read scope) lists `username` and `role`.
- [ ] `PUT /v1/orgs/{name}/members/{username}` with `{"role":"member"}` or `{"role":"owner"}` (owner only) adds or updates a member; returns 200 with the membership; unknown username returns 404; unknown role returns 422 (the web path silently maps unknown roles to member; the API must not).
- [ ] `DELETE /v1/orgs/{name}/members/{username}` (owner only) returns 204; removing the last owner returns 409 `last_owner`.
- [ ] Audit events match the web equivalents.

### Phase 5: CLI

### US-012: CLI authentication
**Acceptance Criteria:**
- [ ] `ramble login [--registry name]` reads a token from stdin (no echo) and stores it in the CLI config with file mode 0600; `RAMBLE_TOKEN` env var overrides the stored token.
- [ ] `GET /v1/user` (read scope) returns `username`, `token_scope`, `token_org`, `token_expires_at`; `ramble login` calls it and prints "Logged in to <registry> as <username>"; a bad token prints the 401 message and stores nothing.
- [ ] `ramble logout` removes the stored token.

### US-013: CLI publish and manage
**Acceptance Criteria:**
- [ ] `ramble pack publish --repo URL --path DIR --version vX --namespace NS [--name N]` creates the pack or, if `NS/N` exists, adds the version. Same for `ramble job publish`.
- [ ] `ramble pack delete NS/N` asks for confirmation unless `--yes`.
- [ ] Commands use the generated client (shared with the provider is not required; the CLI may use its own `oapi-codegen` output under `internal/cli/apiclient`).
- [ ] `docs/cli` is updated.

### Phase 6: Terraform provider (new repo `open-wander/terraform-provider-ramble`)

### US-014: Provider and resources
**Acceptance Criteria:**
- [ ] Provider config: `address` (default `https://ramble.openwander.org`, env `RAMBLE_ADDR`), `token` (sensitive, env `RAMBLE_TOKEN`).
- [ ] Resources: `ramble_resource`, `ramble_organization`, `ramble_organization_member`. Data sources: `ramble_resource`, `ramble_organization`.
- [ ] `ramble_resource.webhook_secret` is a sensitive computed attribute, set from the create response and kept in state (it is not returned by GET).
- [ ] `name`, `type`, `namespace` force replacement.
- [ ] Import: `terraform import ramble_resource.x myorg/mysql`, `ramble_organization.x myorg`, `ramble_organization_member.x myorg/alice`.
- [ ] Acceptance tests (`TF_ACC=1`) run against the Ramble container image plus Postgres in CI and cover create, plan-is-empty, update, import and destroy for each resource.
- [ ] Docs generated with `tfplugindocs`; an `examples/` directory with one complete example.

### US-015: Automated provider releases
**Acceptance Criteria:**
- [ ] The Ramble release workflow's last job sends a `repository_dispatch` (`ramble-released`, payload: version, spec asset URL) to the provider repo, using a GitHub App token with `contents:write` on that repo only.
- [ ] The provider workflow downloads the spec, runs `oapi-codegen`, and if the generated client differs, opens a PR titled `chore: regenerate client for ramble vX.Y.Z`; if no diff, it exits successfully without a PR.
- [ ] The PR's CI runs acceptance tests against `ghcr.io/open-wander/ramble:X.Y.Z`.
- [ ] On merge of such a PR, the provider release workflow tags the next minor version and GoReleaser publishes signed (GPG) artifacts accepted by the Terraform Registry and OpenTofu Registry.
- [ ] `README` states the provider/Ramble compatibility table.

## 4. Functional Requirements

- FR-1: The system must authenticate `/v1` requests by Bearer token, with scopes `read` and `write`, optional organisation restriction, expiry and revocation.
- FR-2: Tokens must be stored only as SHA-256 hashes and shown in plaintext once.
- FR-3: Tokens must never grant admin rights or access to `/admin`.
- FR-4: Token-authenticated `/v1` requests without a session cookie must be exempt from CSRF; cookie-authenticated requests must not.
- FR-5: Write endpoints must reuse the resource and organisation services the web handlers use; authorisation rules must be identical to the web UI.
- FR-6: All `/v1` errors must use the JSON error shape in US-004.
- FR-7: An OpenAPI 3 spec must be generated from code, checked for drift in CI and attached to each release.
- FR-8: Every state change through the API must write the same audit event as the web equivalent, marked as via token.
- FR-9: The CLI must support login, logout, publish and delete against any configured registry.
- FR-10: The Terraform provider must manage resources, organisations and members, support import, and be released when the spec changes.

## 5. Non-Goals

- Admin endpoints (users, site settings, audit, requests) in the API, CLI or provider.
- Organisation-owned service tokens not tied to a person (possible follow-up).
- OAuth2 flows or device-code login for the CLI.
- Uploading pack/job content directly; content still comes from a Git repository.
- Managing stars, pack requests or votes via API.
- Lockstep provider versions.
- Using HashiCorp's `terraform-plugin-codegen-*` tools.

## 6. Technical Considerations

- **Spec generation:** `swag` (Makefile `swagger` target) produces Swagger 2.0. Options: `swag` v2 with OpenAPI 3 output (check its release status at implementation time), or keep `swag` and convert with `swagger2openapi`/`kin-openapi`. Pick whichever produces a spec `oapi-codegen` accepts without hand edits.
- **Handler refactor:** `PostNewResource` mixes form parsing, license detection, service calls and HTMX redirects. Extract the business logic into `internal/services/resource` so the API handlers are thin; same for org handlers in `internal/handlers/org.go`.
- **Background fetch:** API create and new-version start `fetchVersionContent` the same way; the 202 response means "accepted, fetch pending", and clients poll `GET .../versions`.
- **Migrations:** schema is managed by GORM `AutoMigrate`; the new `api_tokens` table needs no manual migration.
- **Cross-repo dispatch:** needs a GitHub App or fine-grained token stored as a repository secret in `ramble`; to be set up by the operator, not by an agent. When US-015 starts, stop and give the operator step-by-step setup instructions (create the provider repo, the GitHub App or token, the secret name, the GPG signing key and registry publishing) before writing the workflow.
- **Logs:** Bearer tokens must be added to the redaction list in `internal/handlers/audit.go` (it already redacts `authorization`); add a test that a logged request line never contains `rmbl_`.
- **Security tests:** new `*_security_test.go` files under the `security` build tag for token auth, CSRF exemption, scope and org restriction.

## 7. Success Metrics

- Issue #65's request is met: a pack can be registered from Terraform.
- At least one external user (e.g. the issue author) uses the provider or API within a month of release.
- Zero security findings in `norman secure` / `gosec` on the new code at release.
- The provider pipeline produces a PR or a clean no-op for every Ramble release after v0.6.0 with no manual steps.

## 8. Resolved Questions (2026-11-26)

1. **Organisation deletion:** owners may delete an organisation with no resources, via API and org settings page; otherwise 409. (US-010)
2. **Webhook secret in Terraform state:** returned on create and kept as a sensitive attribute in state, consistent with how Terraform state handles secrets today. (US-014)
3. **404 vs 403:** same as the web UI: 404 for unknown, 403 for not permitted. (US-007)
4. **Provider ownership:** invite the issue author as a contributor before v0.1.0 is tagged, once US-014 acceptance tests pass, so their feedback lands before the schema is frozen. Tag v0.1.0 after their review.
5. **Release split:** everything ships together as Ramble v0.6.0 + provider v0.1.0.
