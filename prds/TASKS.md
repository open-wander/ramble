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

## Phase 14: Resource write API — 6 tasks completed 2026-11-26 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 15: Organisation API — 4 tasks completed 2026-11-26 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 16: CLI — 2 tasks completed 2026-11-26 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 17: Terraform provider and release

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 17.1 | OPERATOR: create repo `open-wander/terraform-provider-ramble`; give step-by-step instructions when reached | [prd-programmatic-management.md](active/prd-programmatic-management.md) | DONE (2026-11-27) | repo created from the scaffolding template; DCO app; ruleset in .github/rulesets |
| 17.2 | Provider scaffold (plugin-framework), config, `oapi-codegen` client, `ramble_resource` resource + data source, import, acceptance tests vs Ramble container (US-014) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | DONE (2026-11-27) | split: provider#4 skeleton, generated client, acceptance env (Ramble from source + Postgres); provider#5 ramble_resource |
| 17.3 | `ramble_organization`, `ramble_organization_member` resources + data source, import, acc tests, tfplugindocs, examples (US-014) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | DONE (2026-11-27) | provider#6 ramble_organization, ramble_organization_member, data source; examples/complete |
| 17.4 | Automated provider pipeline: repository_dispatch from Ramble release, regenerate + PR, tag on merge, signed GoReleaser publish (US-015) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | DONE (2026-11-27) | ramble#74 notify-provider; provider#7 regenerate.yml, tag-release.yml, release.yml (GPG), failure issues. Regeneration verified by hand for v0.6.0 (no diff); ramble notify-provider failed at v0.6.0 (variable added after the run) and is first proven at the next release |
| 17.5 | Invite the issue author (#65) to the provider repo; collect feedback before tagging v0.1.0 | [prd-programmatic-management.md](active/prd-programmatic-management.md) | DONE (2026-11-27) | issue author invited to the provider repo by the operator; v0.1.0 tagged without waiting for feedback, at the operator's call |
| 17.6 | Release Ramble v0.6.0 and provider v0.1.0; update #65 | [prd-programmatic-management.md](active/prd-programmatic-management.md) | DONE (2026-11-27) | Ramble v0.6.0 released; provider v0.1.0 tagged. norman verify and a trimmed norman secure (#65 PRD) run after the release, at the operator's call |

## Phase 18: #65 review fixes (v0.6.1)

From the norman verify and trimmed norman secure run of 2026-11-27 ([verification.md](verification.md)). Each fix starts with a failing test that reproduces the finding.

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 18.1 | HIGH: CLI pack cache path traversal. A registry-supplied version like `v1.tar.gz#/../../..` escapes the cache dir and extractTarGz overwrites files in the user's home. Validate versions on the server (create/version API and web: a safe charset, no `/`, `..`, `#`, `?`) and in the CLI before any path or URL use; extractTarGz must check against the real cache root | - | DONE (2026-11-27) | names.IsValidVersion `^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$` on create, add version and webhook tags (tags with `/` now rejected, webhook answers 422 and records a failure); CLI validates host/namespace/name/version, writes through os.OpenRoot, extractTarGz rejects absolute and `..` entries and skips links; download URL path-escapes the version. Was: internal/pack/cache.go, internal/services/resource/create.go validateVersion, internal/cli/cmd/pack_run.go. Reproduction: cache.Store with that version resolves to the home dir |
| 18.2 | HIGH: web edit of an org-owned resource moves it to the creator's personal namespace (no `<option selected>` for the current org; empty owner clears OrganizationID). Also: a plain member can move a resource into an org they own. Preserve the current owner when unchanged; moving out of or into an org needs owner rights on the source org | - | TODO | views/edit_resource.html:99-101, internal/handlers/resource_crud.go:279, internal/services/resource/service.go:183-227 |
| 18.3 | HIGH, OPERATOR + agent: release supply chain. Any Write collaborator can deploy to production, sign a provider release, or read release secrets; fork PRs can target the self-hosted runner. Agent: `environment: release` on secret-using jobs, release only from main, tag-release checks every commit author on the regen PR. Operator: environments with required reviewers, tag rulesets (`v*` creation for admins and the App only) in both repos, fork-PR approval for all outside contributors, runner restricted to release.yml, last-push approval in the provider ruleset | - | TODO | ramble .github/workflows/{release,ci}.yml; provider .github/workflows/{release,tag-release}.yml, .github/rulesets |
| 18.4 | MEDIUM: API tokens survive a password reset. Revoke all of a user's tokens on password reset or change, and audit it | - | TODO | internal/services/apitoken, password reset handler |
| 18.5 | MEDIUM: unbounded tag list on POST/PATCH /v1/resources (per-tag queries in one transaction). Cap the count and length, with 422 | - | TODO | internal/services/resource/create.go:86 |
| 18.6 | MEDIUM: an organisation can be created over the namespace of an admin-deleted (soft-deleted) user | - | TODO | internal/services/org/org.go:38, namespace uniqueness across users and orgs |
| 18.7 | LOW batch: revoke without verified email; terminal escapes in CLI list/info/search/run output; RAMBLE_TOKEN tied to one registry; a deleted member breaks org pages and cannot be removed via the API; `Session expired` JSON shape on /v1 (FR-6); verify and fix the soft-delete filter on GET /v1/registries and /v1/registries/search (critic gap, not yet confirmed); web edit name/type validation | - | TODO | see verification.md |
| 18.9 | Check `ramble pack run` against a real GitHub archive: extractTarGz takes the root dir from the first tar entry, which may be `pax_global_header`, so the repo-name prefix may not be stripped (pre-existing, found in the 18.1 review) | - | TODO | internal/pack/cache.go |
| 18.8 | PRD text: US-009 audit name is `resource.reset_webhook` (FR-8); US-014/US-015 acceptance tests build Ramble from source at the tag; US-015 spec from the tag's raw URL | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | docs only |
