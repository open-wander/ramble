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
| 17.4 | Automated provider pipeline: repository_dispatch from Ramble release, regenerate + PR, tag on merge, signed GoReleaser publish (US-015) | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 17.3. OPERATOR setup first (GitHub App/token secret, GPG key, registry) - agent gives instructions |
| 17.5 | Invite the issue author (#65) to the provider repo; collect feedback before tagging v0.1.0 | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: 17.3. Manual |
| 17.6 | Release Ramble v0.6.0 and provider v0.1.0; update #65 | [prd-programmatic-management.md](active/prd-programmatic-management.md) | TODO | needs: all above; norman verify + norman secure first |
