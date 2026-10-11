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

## 2026-05-31 — Phase 5 review follow-ups (5.1-5.5)

Implemented the five deferred review follow-ups. 5.1/5.3/5.4 ran as parallel workers;
5.2 correctly self-blocked (needed a two-file fix: service ToggleStar still used a plain
fmt.Errorf, so converting only the handler would break 404 detection) and was completed
after 5.1 freed up service.go. 5.5 (gofmt) ran last so new code is included.

`go build ./...` clean; `go test ./...` exit 0 (whole suite).

- 5.1 d1e3e87 - RecordFetchFailed truncates fetch_error to 1000 runes (UTF-8 safe).
- 5.2 d1e3e87 - ToggleStar returns ErrResourceNotFound; handler uses errors.Is.
- 5.3 d195d9c - 10 MiB io.LimitReader on all 6 GitHub/GitLab JSON API decoders.
- 5.4 d2d3da3 - OAuth email-collision probe broadened to any existing email (no more
  generic 500 on cross-provider same-email); provider+id login path still runs first.
- 5.5 b3f8b1d - gofmt -w across internal/ and cmd/ (23 files), formatting only.

All Phase 5 commits carry DCO Signed-off-by (per [[dco-signoff-required]]). Branch had
its earlier 11 commits sign-off-rebased; the new commits were created with `git commit -s`.

PATTERN: norman-parallelism - do not run workers in parallel when a "handler-only" change
actually requires a matching service-layer change in a file another worker owns; sequence them.

## 2026-05-31 — Correction: 5.2 was not actually applied in first pass

The first Phase 5 pass committed dad17f9 with a message claiming the ToggleStar
typed-error conversion, but the Edit attempts had failed (wrong match strings) so 5.2
was never applied — the Opus batch review correctly REJECTED it. Re-applied properly in
commit 8b9b8aa: ToggleStar returns ErrResourceNotFound, handler uses errors.Is, and the
API-limit test was de-cruft'd (removed dead 10 MiB padding alloc + unused table, added a
real small-body assertion). build + full test green; gofmt-clean; DCO signed.
LESSON: never trust a worker's DONE/commit-message claim over the actual diff — the
adversarial code review is what caught this. Verify edits landed before committing.

## 2026-10-11 — 6.6 startup banner -> server.started

Batch with 6.4 (parallel worktrees, workflow). internal/server/server.go: Fiber config
extracted to fiberConfig(views) with DisableStartupMessage: true; logStarted(l, addr,
version) logs INFO `server.started` just before app.Listen, same addr variable for both.
internal/server/startup_test.go pins DisableStartupMessage plus the trusted-proxy fields
and the server.started line. Mutation-checked (flag false, msg renamed -> red).
Advisor review PASS. Re-verified by orchestrator: go vet, go test -tags=security ./... ok.
Note: server.started is written before bind, so a bind failure still logs it first.

## 2026-10-11 — 6.4 sentence-style log messages -> event keys

Converted every remaining server-side zerolog message to a dotted event key (fields and
levels unchanged, no Msgf existed). Prefixes: github.*, webhook.*, download.*,
version_fetch.*, audit.recorded (was msg="audit"; no consumer found, JSON logs live only
since 2026-10-10). internal/cli excluded on purpose (terminal tool). Guard:
TestServerLogMessagesAreEventKeys walks internal/ and cmd/ with go/parser, rejects Msgf,
non-literal Msg args and non-matching keys; requires >= 40 calls seen. The AST walk found
8 multi-line chained calls the advisor's grep missed. Orchestrator re-ran the mutation
(audit.recorded -> "audit recorded" -> red) and go test -tags=security ./... ok.
Advisor MINOR notes: guard matches any .Msg selector, not only zerolog; a webhook fetch
failure emits both webhook.fetch_failed and version_fetch.failed (two layers).
Incident: the reviewer ran `git checkout` on a file with uncommitted worker edits during
its mutation check and restored them by hand; diff verified intact before applying.

PATTERN: review - a read-only reviewer must not use `git checkout <file>` to undo a
mutation on a file with uncommitted changes; copy aside first or reverse the edit.
PATTERN: logging - guard log-key conventions with a go/parser walk, not grep: grep misses
multi-line chained zerolog calls.
PATTERN: tooling - BSD sed ignores GNU `0,/re/`; a "mutation" that silently didn't apply
looks like a surviving mutant. Confirm the mutation landed before trusting the result.

## 2026-10-11 — 6.3 request context into DB calls

reqDB(c) = database.DB.WithContext(c.UserContext()) in internal/handlers/db.go, used on
all request paths (14 handler files). resource.Service gained WithContext(ctx); all 7
handler-side service calls use it, so service transactions inherit the request logger.
Session middleware in server.go inlines database.DB.WithContext(c.UserContext()).
github_sync internals and createGitHubIssueForRequest now take a *gorm.DB; goroutines
and the CLI pass database.DB, request paths pass reqDB(c). Unchanged on purpose:
startup, AuditLogNoContext, background.go fetches, goroutine bodies in admin.go,
requests.go, resource_webhooks.go. No c.Context() and no goroutine captures *fiber.Ctx.
Tests: internal/handlers/db_test.go (query against a missing table -> db.query_error in
the request logger buffer with the request's X-Request-ID; base buffer empty) and
internal/services/resource/withcontext_test.go. Orchestrator mutation: reqDB returning
database.DB -> red; go vet, go test -tags=security ./... ok.
Advisor: first review REJECT (service calls not context-bound, WithContext unused), one
fix round, re-review MINOR. Follow-ups (not blocking): SyncGitHubToRequest has no
callers; processOAuthUser, handleNewIssue, handleIssueStateChange and GetPopularTags
are now database.DB wrappers used only by tests - point tests at the *DB variants and
delete the wrappers in a later cleanup.

PATTERN: context-threading - a global service built at startup with the global DB hides
from a grep for `database.DB`; give it WithContext(ctx) and bind at each handler call.

## 2026-10-11 — 6.5 closed; Phase 6 collapsed

6.5 (static-asset access lines) closed without a change on measurement: over 12h,
59 of 904 http.request lines were /public/* (6.5%, ~5/h, against ~1,390 lines/h before
6.2). Lowering them to DEBUG would hide asset 404s for negligible volume. Decision by
the user. Phase 6 fully DONE and moved to TASKS-archive.md.

## 2026-10-11 — 7.1 plain-text writes on request paths

auth.go (signup, forgot password, resend verification) and utils.go SetFlash log via
logger.FromContext(c.UserContext()); no address, link or token in the lines (user_id only).
email.go validateEmail and Rcpt errors no longer embed the address. Guard
TestServerCodeDoesNotWriteStdout shares walkServerFiles with the event-key guard; flags
fmt.Print*, print/println, fmt.Fprint*(os.Stdout|os.Stderr); cmd/nomad-vars excluded
(operator CLI). flash_log_test.go proves session.save_failed carries request_id.
Orchestrator mutation: fmt.Println added to utils.go -> guard red. Full suite ok.
Advisor: plan REVISE (scope cmd/ramble only, second address leak at email.go:117),
review REJECT, fix round, MINOR. Follow-ups: SMTP server replies wrapped by %w can still
echo the recipient; guard matches identifier names (aliases not caught); stdlib log.* not covered.

PATTERN: workflow - Workflow `isolation: worktree` branches from the default branch, not
the current HEAD. On a feature branch with unmerged commits, run isolated workers only
when the base doesn't matter, or have them `git checkout <branch-sha>` first.
