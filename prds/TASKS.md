# Tasks

Phases 1-11 complete (prd-security-bugfix; Phase 6 from infra `docs/prod-logging.md`). Phases 12-17: programmatic management (#65), target Ramble v0.6.0 + terraform-provider-ramble v0.1.0. Full history in [TASKS-archive.md](TASKS-archive.md);
verification in [verification.md](verification.md). PRD: [done/prd-security-bugfix.md](done/prd-security-bugfix.md).

## Phase 1: Authorization — 2 tasks completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 2: Fetch Hardening — 2 tasks completed 2026-05-30 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 3: Auth Hardening — 2 tasks completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 4: Cleanup — 1 task completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 5: Review Follow-ups — 5 tasks completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 6: Structured logging — 6 tasks completed 2026-10-11 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 7: Logging standard gaps — 5 tasks completed 2026-10-11 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 8: Logging test hardening — 11 tasks completed 2026-10-11 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 9: Webhook CSRF bug and code debt — 13 tasks completed 2026-10-11 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 10: CI and runtime maintenance — 3 tasks completed 2026-10-14 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 11: zerolog to log/slog — 5 tasks completed 2026-10-14 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 12: API tokens — 3 tasks completed 2026-11-26 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 13: API contract — 3 tasks completed 2026-11-26 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 14: Resource write API

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 14.1 | Move resource-create business logic (license detection, org membership check) from `PostNewResource` into `internal/services/resource`; web tests unchanged (US-005, FR-5) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | DONE (2026-11-26) | resource.CreateResource with validation + sentinels + IsInputError, WithFileFetcher, security.ValidateRepositoryURL; web handler thin |
| 14.2 | `POST /v1/resources` (US-005, FR-8) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | DONE (2026-11-26) | CreateResourceAPI, ResourceResponse/CreateResourceResponse (secret only on create), shared apiWriteLimiter, cookie-less /v1 CSRF exemption |
| 14.3 | `GET /v1/resources/{id}` and `GET /v1/namespaces/{ns}/resources/{name}` (US-006) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | DONE (2026-11-26) | GetResourceAPI / GetResourceByNameAPI, service GetResource / GetResourceByNamespace (read.go, withAPIPreloads); POST reloads via GetResource |
| 14.4 | `PATCH` and `DELETE /v1/resources/{id}`; immutable fields; 404/403 as web UI (US-007, US-008) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | DONE (2026-11-26) | UpdateResourceAPI/DeleteResourceAPI, service PatchResource (patch.go), immutable_field; web UpdateResource validates repository_url |
| 14.5 | Versions list/create, retry failed version, rotate webhook secret (US-009) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 14.3 |
| 14.6 | Resource API follow-ups (user decisions 2026-11-26): purge a soft-deleted resource holding the name in the namespace on re-create (same transaction; web + API) so Terraform replace works; API POST/PATCH reject non-canonical tags with 422 invalid_tags (lowercase 2-20 [a-z0-9-], no duplicates, no commas; web stays lenient); limiter test includes POST | [prd-programmatic-management.md](active/prd-programmatic-management.md) | DONE (2026-11-26) | purge-on-re-create in CreateResource tx (namespaceNameScope), ValidateCanonicalTags/ErrInvalidTags + 422 invalid_tags (API only), limiter test covers POST |

## Phase 15: Organisation API

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 15.1 | Move org create/update/member logic from `internal/handlers/org.go` into a shared service; web tests unchanged (FR-5) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | |
| 15.2 | `POST/GET/PATCH /v1/orgs` (US-010) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 12.2, 13.2, 15.1 |
| 15.3 | Owner deletion of empty orgs: `DELETE /v1/orgs/{name}` (409 when resources remain) plus matching button on org settings page (US-010) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 15.2 |
| 15.4 | Members API: list, PUT role, DELETE, last-owner 409, strict role validation (US-011) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 15.2 |

## Phase 16: CLI

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 16.1 | `ramble login` / `logout`, token stored 0600, `RAMBLE_TOKEN` override (US-012) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 13.3 |
| 16.2 | Generated client in `internal/cli/apiclient`; `pack`/`job publish` and `delete`; docs/cli (US-013, FR-9) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 13.1, 14.5, 16.1 |

## Phase 17: Terraform provider and release

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 17.1 | OPERATOR: create repo `open-wander/terraform-provider-ramble`; give step-by-step instructions when reached | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 13.1. Agent stops and hands instructions to operator |
| 17.2 | Provider scaffold (plugin-framework), config, `oapi-codegen` client, `ramble_resource` resource + data source, import, acceptance tests vs Ramble container (US-014) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 14.5, 17.1 |
| 17.3 | `ramble_organization`, `ramble_organization_member` resources + data source, import, acc tests, tfplugindocs, examples (US-014) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 15.4, 17.2 |
| 17.4 | Automated provider pipeline: repository_dispatch from Ramble release, regenerate + PR, tag on merge, signed GoReleaser publish (US-015) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 17.3. OPERATOR setup first (GitHub App/token secret, GPG key, registry) - agent gives instructions |
| 17.5 | Invite the issue author (#65) to the provider repo; collect feedback before tagging v0.1.0 | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 17.3. Manual |
| 17.6 | Release Ramble v0.6.0 and provider v0.1.0; update #65 | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: all above; norman verify + norman secure first |
