# Tasks

## Phase 1: Authorization

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 1.1 | US-001: Block unauthorized organization resource creation — validate org membership before persisting a resource with `OrganizationID`; reject invalid/nonexistent org IDs (FR-1, FR-2) | [prd-security-bugfix.md](backlog/prd-security-bugfix.md) | TODO | Prefer centralizing the org-authorization decision in the resource service so 1.2 can reuse it |
| 1.2 | US-004: Clarify organization resource management roles — encode intended permission model and reuse it across edit, version, retry, webhook reset, delete (FR-6, FR-7) | [prd-security-bugfix.md](backlog/prd-security-bugfix.md) | TODO | Needs: 1.1. Open question: owner-only vs member edit (deletion stays owner-only) |

## Phase 2: Fetch Hardening

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 2.1 | US-002: Limit remote repository content size — shared size-limited download helper, propagate oversized errors to fetch status (FR-3, FR-4) | [prd-security-bugfix.md](backlog/prd-security-bugfix.md) | TODO | DECISION: single shared limit 1 MiB (`maxRemoteFileBytes = 1 << 20`) for README/LICENSE/metadata/variables/job files; use `io.LimitReader(r, maxBytes+1)` to detect exactly-at-limit vs oversized |
| 2.2 | US-003: Tighten SSRF host allowlisting — exact host or subdomain-of-allowed-host only, remove reverse suffix matching (FR-5) | [prd-security-bugfix.md](backlog/prd-security-bugfix.md) | TODO | Add test for parent-domain bypass (e.g. `com`, `githubusercontent.com`) |

## Phase 3: Auth Hardening

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 3.1 | US-005: Harden OAuth account linking — require trusted/verified email or explicit authenticated link flow before linking (FR-8) | [prd-security-bugfix.md](backlog/prd-security-bugfix.md) | TODO | INVESTIGATED: goth v1.82.0 exposes NO verified-email flag for GitHub or GitLab (only Google via RawData email_verified). DECISION: stop silent email-based linking (auth.go:93-94) — match only on (provider, providerID); if an account with same email exists, return clear error telling user to log in and link explicitly |
