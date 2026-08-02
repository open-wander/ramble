# Verification — prd-security-bugfix

Date: 2026-05-31
Scope: the four completed user stories (US-001, US-002, US-003, US-005). US-004 is
not yet implemented (task 1.2 still TODO) and was not verified.
Method: independent Opus verifier per story, reading the implementation against the
PRD acceptance criteria and running the package tests.

> **UPDATE (2026-05-31, later same day): everything flagged below has since been
> completed.** This report is a snapshot of the mid-PRD verification pass; do not read
> it as a list of open work. Task 1.2 (US-004) shipped as
> `ResourceService.AuthorizeResourceAction`, and task 4.1 wired `RecordFetchFailed` into
> the live `fetchVersionContent` path, killing the dead-code gap. All 6 tasks plus the 5
> Phase-5 follow-ups are done and the PRD moved to `done/`. See `progress.md`
> (2026-05-31 entries) and `TASKS-archive.md`. See "Remaining work" at the foot for the
> per-item resolution.

## Summary

| Story | Task | Result | Notes |
|-------|------|--------|-------|
| US-001 Block unauthorized org resource creation | 1.1 | PASS | All 6 criteria verified; tests green |
| US-002 Limit remote repository content size | 2.1 | PARTIAL | Behaviour (incl. FR-4) met; coverage gap — `RecordFetchFailed` is dead code |
| US-003 Tighten SSRF host allowlisting | 2.2 | PASS | Reverse-suffix branch removed; bypass tests green (incl. -tags security) |
| US-005 Harden OAuth account linking | 3.1 | PASS | No email-based auto-link; collision rejected; EmailVerified left false |
| US-004 Clarify org resource management roles | 1.2 | NOT IMPLEMENTED **at time of this pass — since completed, see UPDATE above** | Task 1.2 was still TODO when verified |

## Test results

- `go test ./internal/handlers/` — ok
- `go test ./internal/services/resource/` — ok
- `go test ./internal/security/` and `-tags security` — ok

## US-001 (1.1) — PASS
All criteria PASS: membership check (`resource_crud.go:79-90`) runs before
`CreateResource` (`resource_crud.go:122`); non-member -> 403 with no row persisted;
member and owner allowed (`role IN ('member','owner')`); personal namespace skips the
check (`if orgID != nil`); nonexistent org -> 403, no row; userID from session.
Tests: TestPostNewResource_* (5 cases) green.
Minor: malformed `owner=org:abc` becomes org id 0 and is rejected by the existence
check (403) rather than a distinct 400; raw inline query duplicates auth logic that
1.2 should centralize.

## US-002 (2.1) — PARTIAL
Behaviour fully satisfied:
- `readLimited` uses `io.LimitReader(r, MaxRemoteFileBytes+1)` + explicit
  `len(body) > MaxRemoteFileBytes` check (`utils.go:117-127`). Exactly-at-max accepted,
  max+1 rejected.
- All remote-file fetch sites route through `downloadFile` (README, job, metadata,
  variables, LICENSE, webhook metadata, web view). No bypassing site.
- Oversized -> `("", err)`; success update unreachable; required content -> FAILED via
  `fetchVersionContent` (`background.go:200-216`); "blocked" makes it a permanent
  error (no retry).

GAP (why PARTIAL): `RecordFetchFailed` (`service.go:352`) is DEAD CODE — only called
from service_test.go. Its tests (`TestRecordFetchFailed_*`) validate an unused helper,
giving false confidence. FR-4 is genuinely met by background.go's inline failure path,
but there is no test driving the real oversized -> FetchStatusFailed chain end-to-end
(downloadFile hardcodes the protected client/host rewrite, a testability gap).
Recommended fix: wire `fetchVersionContent` to call `RecordFetchFailed` (single source
of truth) or remove the helper, and add a test on the real path.

## US-003 (2.2) — PASS
Reverse-suffix branch removed; only `H==A || HasSuffix(H, "."+A)` remains, with
lowercase + trailing-dot normalization and empty-host/empty-entry guards
(`ssrf.go:152-172`). Parent domains (com, githubusercontent.com), siblings
(notgithub.com), and embedded (github.com.evil.com) rejected; subdomains accepted.
`TestIsHostAllowed_ParentDomainBypass` (11 subtests) + existing suite green.

## US-005 (3.1) — PASS
No remaining email-based auto-link; the only email query in `processOAuthUser`
(`auth.go:122`) is a collision PROBE that refuses (never links). Collision returns
`errOAuthEmailCollision`, existing row unchanged. Already-linked login and brand-new
signup work. `errors.Is(gorm.ErrRecordNotFound)` guards both lookups. Tests assert at
DB level. EmailVerified left false for new OAuth accounts (conservative — github/gitlab
expose no verified flag); this already satisfies the residual concern originally
tracked as task 3.2.
Minor (UX, not security): a same-email account already linked to a *different* provider
falls to Create and hits the Email uniqueIndex, surfacing a generic failure flash
rather than the clear collision message.

## Remaining work — ALL RESOLVED

Both items below were outstanding when this pass ran and were completed later the same
day. Nothing here is open.

- ~~1.2 (US-004): organization resource management roles — not yet implemented.~~
  RESOLVED — `ResourceService.AuthorizeResourceAction(resourceID, userID, requireOwner)`
  added with org-before-personal branching and `ErrUnauthorized`/`ErrResourceNotFound`
  sentinels. Edit/version/retry/reset-webhook are member-allowed, delete is owner-only,
  and `PostNewVersion` (previously UNAUTHENTICATED) is now gated. Tests:
  `resource_authz_test.go` (24) + `TestAuthorizeResourceAction` (10).
- ~~US-002 coverage gap: dead `RecordFetchFailed` helper (wire in or remove).~~
  RESOLVED (task 4.1) — `background.go fetchVersionContent` now calls `RecordFetchFailed`
  on both the panic-recovery and error branches, so the helper is live and its tests
  cover the real path. Later hardened by 5.1 (1000-rune UTF-8-safe truncation).
