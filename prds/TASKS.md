# Tasks

## Phase 1: Authorization

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 1.1 | US-001: Block unauthorized organization resource creation — validate org membership before persisting a resource with `OrganizationID`; reject invalid/nonexistent org IDs (FR-1, FR-2) | [prd-security-bugfix.md](active/prd-security-bugfix.md) | DONE (2026-05-30) | Reused org service `CheckPermission(RoleMember)` in CreateResource; owner resolved by name/slug; review PASS |
| 1.2 | US-004: Clarify organization resource management roles — encode intended permission model and reuse it across edit, version, retry, webhook reset, delete (FR-6, FR-7) | [prd-security-bugfix.md](active/prd-security-bugfix.md) | TODO | Unblocked (1.1 done). DECISION: org members CAN edit/version/retry/reset-webhook; deletion stays owner-only |

## Phase 2: Fetch Hardening — 2 tasks completed 2026-05-30 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 3: Auth Hardening

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 3.1 | US-005: Harden OAuth account linking — require trusted/verified email or explicit authenticated link flow before linking (FR-8) | [prd-security-bugfix.md](active/prd-security-bugfix.md) | DONE (2026-05-30) | Removed email-fallback link; match on (provider, providerID); refuse same-email collision; review MINOR (see 3.2) |
| 3.2 | Follow-up: stop trusting unverified provider email as `EmailVerified=true` for brand-new OAuth accounts (or document why it is acceptable) | [prd-security-bugfix.md](active/prd-security-bugfix.md) | TODO | Raised by 3.1 Opus code review (MINOR). PRD scoped this out of US-005; tracked here so the residual trust assumption is not lost |
