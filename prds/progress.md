# Progress Log

Append-only execution log for crash recovery.

---

## 2026-05-30 — Import

Imported `prd-security-bugfix.md` from research to backlog. Extracted 5 tasks
across 3 phases (Authorization, Fetch Hardening, Auth Hardening) from US-001..US-005
and FR-1..FR-10.

All PRD-referenced files confirmed present:
- internal/handlers/resource_crud.go, utils.go, auth.go
- internal/services/resource/service.go
- internal/security/ssrf.go

Existing security tests confirmed present (extend rather than replace):
- internal/handlers/{auth_security,resource_security,utils_security,auth_oauth_security}_test.go
- internal/security/ssrf_test.go
- internal/services/resource/service_test.go

Open questions carried into task notes (1.2 org edit roles, 2.1 size limit default,
3.1 Goth verified-email exposure).

## 2026-05-30 — Open questions resolved

- 1.2 (org roles): org MEMBERS can edit/version/retry/reset-webhook; deletion owner-only.
- 2.1 (size limit): single shared 1 MiB limit (1<<20) for all fetched files;
  io.LimitReader(r, maxBytes+1) to distinguish exactly-at-limit from oversized.
- 3.1 (OAuth verified email): investigated goth v1.82.0. No verified-email signal
  for GitHub (Verified flag only used internally in getPrivateMail fallback, not
  surfaced on User or RawData) or GitLab (/api/v4/user email only, no confirmed flag).
  Ramble configures only github + gitlab providers (auth.go:65-67). Google would
  expose email_verified via RawData but is not configured.
  DECISION: remove silent email-based linking at auth.go:93-94; match only on
  (provider, providerID). Same-email collisions return a clear "log in and link
  explicitly" error.

PATTERN: oauth-security - goth's generic User struct carries no verified-email flag;
do not treat gothUser.Email as a trusted identity for account linking.

## 2026-05-30 — Executed US-001/002/003/005 in parallel

All four independent tasks implemented by Sonnet workers (golang-pro), each preceded
by an Opus plan review and followed by an Opus code review. Full suite green:
`go build ./...` OK, `go test ./...` all packages ok.

- 1.1 (US-001) DONE — internal/handlers/resource_crud.go: org membership check via
  organization.CheckPermission(RoleMember) before persisting; owner resolved by
  name/slug; non-member -> 403, no row persisted. Tests: resource_crud_authz_test.go.
  Review PASS.
- 2.1 (US-002) DONE — internal/handlers/utils.go MaxRemoteFileBytes=1<<20 +
  io.LimitReader(r,max+1)+len check; service.go marks required-content oversize as
  failed. All 5 fetch sites confirmed routed through downloadFile. Review PASS.
- 2.2 (US-003) DONE — internal/security/ssrf.go: removed reverse-suffix branch;
  exact-or-subdomain only + normalization + empty-entry guard. Review PASS.
- 3.1 (US-005) DONE — internal/handlers/auth.go: removed email-fallback auto-link;
  match on (provider, providerID); refuse email collision with clear flash + redirect;
  errors.Is(gorm.ErrRecordNotFound); removed bogus OAuthProvider assignment. Review
  MINOR: new OAuth accounts still set EmailVerified=true (PRD non-goal) -> tracked as 3.2.

Phase 2 collapsed into TASKS-archive.md (both tasks DONE).
PRD kept in active/ (shared by all 5 tasks; 1.2 still TODO).

Remaining: 1.2 (org roles, now unblocked) and 3.2 (EmailVerified follow-up).

PATTERN: authorization - authorization-before-persist; assert row-count==0 on negative tests.
PATTERN: security-limits - io.LimitReader(r, max+1) + len(body)>max for bounded remote reads.
PATTERN: security-ssrf - host allowlist match only `h==a || HasSuffix(h, "."+a)`; never reverse-suffix.
PATTERN: oauth-security - match OAuth on (provider, provider_id); never link by unverified email.

## 2026-05-31 — norman verify (US-001/002/003/005)

Ran independent Opus verifiers per completed story. Tests green across
internal/handlers, internal/services/resource, internal/security (incl. -tags security).
- US-001 PASS, US-003 PASS, US-005 PASS.
- US-002 PARTIAL: behaviour incl. FR-4 satisfied via background.go, but RecordFetchFailed
  (service.go:352) is dead code and its tests give false confidence -> tracked as 4.1.
- US-004 (1.2) not implemented yet.
- 3.2 resolved: 3.1 already leaves EmailVerified=false for new OAuth accounts.
Also committed previously-missed SSRF test file (internal/security/ssrf_test.go) as fff20f6.
Report: prds/verification.md.

## 2026-05-31 — Completed 1.2 (US-004) and 4.1; PRD done

- 1.2 (US-004) DONE — internal/services/resource/service.go: new
  AuthorizeResourceAction(resourceID, userID, requireOwner) with org-before-personal
  branching + ErrUnauthorized/ErrResourceNotFound sentinels + RoleOwner/RoleMember.
  Rewired edit/version/retry/reset-webhook (member-allowed) and delete (owner-only);
  PostNewVersion was previously UNAUTHENTICATED, now gated. Handlers use errors.Is.
  Tests: resource_authz_test.go (24) + TestAuthorizeResourceAction (10). Review PASS
  (minor: GetEditResource render path not centralized but at-least-as-strict).
- 4.1 DONE — background.go fetchVersionContent (panic-recovery + error branch) now
  call resource.NewService(database.DB).RecordFetchFailed; helper is live (single
  source of truth, 1000-char error truncation preserved), existing tests cover real
  path. Review PASS.

All 6 tasks complete. `go build ./...` clean; `go test ./...` all packages ok.
PRD moved active/ -> done/. All phases collapsed into TASKS-archive.md.

PATTERN: authorization - central AuthorizeResourceAction with org-before-personal
branching + sentinel errors + errors.Is in handlers; assert no side effect on deny.
PATTERN: refactoring - consolidate duplicated inline DB status-writes into one service
method to kill dead code and guarantee field-write parity.
