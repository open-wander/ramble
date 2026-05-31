# Tasks

## Phase 1: Authorization

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 1.1 | US-001: Block unauthorized organization resource creation — validate org membership before persisting a resource with `OrganizationID`; reject invalid/nonexistent org IDs (FR-1, FR-2) | [prd-security-bugfix.md](active/prd-security-bugfix.md) | DONE (2026-05-30) | Inline membership check in PostNewResource (`role IN ('member','owner')`); owner parsed as numeric `org:<id>`; non-member -> 403; verify PASS |
| 1.2 | US-004: Clarify organization resource management roles — encode intended permission model and reuse it across edit, version, retry, webhook reset, delete (FR-6, FR-7) | [prd-security-bugfix.md](active/prd-security-bugfix.md) | TODO | Unblocked (1.1 done). DECISION: org members CAN edit/version/retry/reset-webhook; delete owner-only. Plan reviewed: add `AuthorizeResourceAction(resourceID, userID, requireOwner)` to resource service (no `organization.CheckPermission` exists); `PostNewVersion` currently has NO auth |

## Phase 2: Fetch Hardening — 2 tasks completed 2026-05-30 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 3: Auth Hardening

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 3.1 | US-005: Harden OAuth account linking — require trusted/verified email or explicit authenticated link flow before linking (FR-8) | [prd-security-bugfix.md](active/prd-security-bugfix.md) | DONE (2026-05-30) | Removed email-fallback link; match on (provider, providerID); refuse same-email collision; EmailVerified left false; verify PASS |
| 3.2 | Follow-up: do not trust unverified provider email as `EmailVerified=true` for brand-new OAuth accounts | [prd-security-bugfix.md](active/prd-security-bugfix.md) | DONE (2026-05-31) | Already satisfied by 3.1 — new OAuth accounts are created with EmailVerified=false (auth.go:151-154). Confirmed by verification |

## Phase 4: Cleanup (from verification)

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 4.1 | Resolve `RecordFetchFailed` dead code (US-002 verify gap) — wire `fetchVersionContent` to call it (single source of truth) or remove it; add a test driving the real oversized -> FetchStatusFailed path | [prd-security-bugfix.md](active/prd-security-bugfix.md) | TODO | FR-4 behaviour already works via background.go inline failure handling; this is cleanliness + real-path coverage, not a missing feature |
