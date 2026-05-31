# Tasks Archive

Collapsed, fully-completed phases moved out of TASKS.md to keep it lean.
PRD: [done/prd-security-bugfix.md](done/prd-security-bugfix.md).

## Phase 1: Authorization — 2 tasks completed 2026-05-31

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 1.1 | US-001: Block unauthorized organization resource creation (FR-1, FR-2) | [prd-security-bugfix.md](done/prd-security-bugfix.md) | DONE (2026-05-30) | Inline membership check in PostNewResource (`role IN ('member','owner')`); owner parsed as numeric `org:<id>`; non-member -> 403; verify PASS |
| 1.2 | US-004: Clarify organization resource management roles (FR-6, FR-7) | [prd-security-bugfix.md](done/prd-security-bugfix.md) | DONE (2026-05-31) | Centralized `AuthorizeResourceAction(resourceID, userID, requireOwner)` (org-before-personal); members edit/version/retry/reset-webhook, delete owner-only; PostNewVersion was unauthenticated, now gated; review PASS (minor: GetEditResource still on a separate at-least-as-strict path) |

## Phase 2: Fetch Hardening — 2 tasks completed 2026-05-30

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 2.1 | US-002: Limit remote repository content size (FR-3, FR-4) | [prd-security-bugfix.md](done/prd-security-bugfix.md) | DONE (2026-05-30) | `MaxRemoteFileBytes = 1 << 20`; `io.LimitReader(r, max+1)` + explicit length check in downloadFile; oversized required content marks fetch failed; verify PARTIAL -> resolved by 4.1 |
| 2.2 | US-003: Tighten SSRF host allowlisting (FR-5) | [prd-security-bugfix.md](done/prd-security-bugfix.md) | DONE (2026-05-30) | Removed reverse-suffix branch in `isHostAllowed`; lowercase + trailing-dot normalization + empty-entry guard; parent-domain bypass tests added; verify PASS |

## Phase 3: Auth Hardening — 2 tasks completed 2026-05-31

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 3.1 | US-005: Harden OAuth account linking (FR-8) | [prd-security-bugfix.md](done/prd-security-bugfix.md) | DONE (2026-05-30) | Removed email-fallback link; match on (provider, providerID); refuse same-email collision; `errors.Is(gorm.ErrRecordNotFound)`; verify PASS |
| 3.2 | Follow-up: do not trust unverified provider email as `EmailVerified=true` for new OAuth accounts | [prd-security-bugfix.md](done/prd-security-bugfix.md) | DONE (2026-05-31) | Already satisfied by 3.1 — new OAuth accounts created with EmailVerified=false (auth.go:151-154); confirmed by verification |

## Phase 4: Cleanup — 1 task completed 2026-05-31

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 4.1 | Resolve `RecordFetchFailed` dead code (US-002 verify gap) | [prd-security-bugfix.md](done/prd-security-bugfix.md) | DONE (2026-05-31) | Wired `fetchVersionContent` (panic-recovery + error branch) through `RecordFetchFailed` as single source of truth; helper now live, existing tests cover real path; review PASS |

## Phase 5: Review Follow-ups — 5 tasks completed 2026-05-31

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 5.1 | Cap `fetch_error` length in `RecordFetchFailed` | [prd-security-bugfix.md](done/prd-security-bugfix.md) | DONE (2026-05-31) | 1000-rune UTF-8-safe cap with truncation marker; commit d1e3e87 |
| 5.2 | Replace `err.Error()` string-compare auth in `ToggleStar` with typed sentinel | [prd-security-bugfix.md](done/prd-security-bugfix.md) | DONE (2026-05-31) | service `ToggleStar` returns `ErrResourceNotFound`; handler uses `errors.Is`; needed the two-file fix flagged when run isolated; commit d1e3e87 |
| 5.3 | Size-cap GitHub/GitLab JSON API responses | [prd-security-bugfix.md](done/prd-security-bugfix.md) | DONE (2026-05-31) | `io.LimitReader(resp.Body, 10 MiB)` on all 6 API decoders; commit d195d9c |
| 5.4 | Clearer message for cross-provider same-email OAuth signup | [prd-security-bugfix.md](done/prd-security-bugfix.md) | DONE (2026-05-31) | Broadened collision probe to any existing email; provider+id login still runs first; commit d2d3da3 |
| 5.5 | gofmt-clean tree-wide (Go 1.26) | [prd-security-bugfix.md](done/prd-security-bugfix.md) | DONE (2026-05-31) | `gofmt -w internal/ cmd/`, 23 files; commit b3f8b1d. NOTE: interface{}->any / modernize hints are staticcheck-level, not gofmt — separate pass if wanted |
