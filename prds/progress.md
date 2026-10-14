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

## 2026-10-11 — 7.2 webhook lines carry request_id

resource_webhooks.go webhookLog from logger.FromContext(c.UserContext()); both goroutines
take a zerolog.Logger parameter (no *fiber.Ctx capture). HandleGitHubIssueWebhook uses the
request logger; handleNewIssueDB/handleIssueStateChangeDB use
logger.FromContextOr(db.Statement.Context, &logger.Log). Keys and levels unchanged.
webhook_log_test.go: table test of early-reject paths plus a HandleWebhook case.
Merged with 7.1: both extracted the same db_test.go helper; kept 7.1's newRequestLogApp.
Orchestrator mutation: HandleGitHubIssueWebhook back to &logger.Log -> red. Suite ok.
Advisor REJECT, fix round, MINOR. Follow-ups: fetchVersionContent (background.go) logs
version_fetch.* via the global logger, so webhook-triggered fetch lines lack request_id;
goroutine DB writes in resource_webhooks.go stay on database.DB; no test covers the
github.webhook_request_* lines.

## 2026-10-11 — 7.3 ramble server writes only JSON

root.go: wantsUpdateCheck (no update check or notice for version/help/server);
Execute uses ExecuteC + reportExecError, which skips errors marked loggedError.
server.go: runServer seam, runServerCmd logs server.exit_error and wraps in loggedError;
SilenceErrors/SilenceUsage on serverCmd only. Tests in internal/cli/cmd/server_test.go
including an end-to-end `ramble server` with a failing runServer. Worker ran in a
worktree based on main (cli untouched by the branch, applied cleanly).
Orchestrator mutation: drop "server" from wantsUpdateCheck -> red. Suite ok.
Advisor MINOR: predicate matches by name (a future `x server` subcommand would also
skip); flag parse errors stay plain text; tests mutate package-level cobra state.

## 2026-10-11 — 7.4 and 7.5 test the production logging wiring

7.4: logger.Init -> initWithWriter(os.Stdout). TestInit_ProductionFormat no longer swaps
the writer with Log.Output, snapshots and restores every global Init touches.
7.5: database.gormConfig() used by Connect; TestGormConfigUsesZerologAdapter.
Run in parallel in the main checkout (different packages, no worktrees). The 7.5 worker
saw a transient TestInit_ProductionFormat failure from 7.4's mutation in progress.
Orchestrator: full suite ok; mutations ConsoleWriter in prod branch -> red,
gormlogger.Default -> red. Both advisor PASS.

PATTERN: parallel-in-one-checkout - workers sharing a checkout see each other's
in-flight mutations; tell them to report cross-package failures, and re-run the suite
after both finish.

## 2026-10-11 — 8.1 real request chain test

Moved requestLogger from internal/server to logger.RequestLogger (git mv requestlog.go ->
logger/fiber.go, test likewise) so handler tests use the production middleware; the copy
in handlers/db_test.go now builds on it. New TestRequestChain_QueryErrorSharesRequestIDWithAccessLine
(one buffer for request and GORM logger, two requests, generated ids). Orchestrator
mutation: SetUserContext removed -> red. Full suite ok. Advisor MINOR: server.Run's
middleware wiring/order is only compile-time checked (a no-op swap stays green) -> 8.8;
logLines helper now exists in three test packages.

## 2026-10-11 — 8.6 SMTP errors carry no addresses or link

email.go: redactError(err, secrets...) wraps the send result of SendResetEmail and
SendVerificationEmail; regex over addr-spec tokens plus literal secrets. smtpRootCAs
seam lets email_send_test.go run the real STARTTLS send path against an in-process fake
server that echoes addresses at MAIL, RCPT and DATA. No Unwrap on purpose.
Orchestrator follow-up from review: sendSecrets(link, to, from) adds the link's query
values (token) and both addresses verbatim; new row where the reply echoes only the token.
Mutations: no redaction -> 4 rows red; token literals dropped -> token row red. Suite ok.
Known limit: URL-encoded or line-wrapped echoes of the link are not matched.

## 2026-10-11 — 8.7 flag errors as JSON, no FATAL level

internal/cli/cmd/server.go: initLogger seam + serverFlagError set via SetFlagErrorFunc on
serverCmd only. logger.InitWithWriter exported for tests in other packages.
database.go: three Fatal() -> Error() + exit(1) + return (exit = os.Exit seam).
AST guard TestServerCodeHasNoFatalOrPanicEvents. Orchestrator mutations: FlagErrorFunc
removed -> TestServerCommandBadFlag red; Fatal re-added -> guard red; exit(1) dropped ->
TestConnectConnectFailed red. Suite ok.
Advisor MINOR: db.migrate_failed branch only AST-covered; InitWithWriter widens the API;
level change FATAL -> ERROR may affect external alerts. Reported pre-existing bug:
gorm logger db.error args fail JSON marshalling (pgconn.DialFunc) -> 8.9. 8.1 review
-> 8.8 (server.Run middleware wiring untested).

## 2026-10-11 — 8.3 github.webhook_request_* lines tested

webhook_log_test.go TestHandleGitHubIssueWebhook_DBLinesCarryRequestID: signed opened
(pack-request label) and closed payloads through HandleGitHubIssueWebhook + newRequestLogApp;
asserts DB row Open -> Completed, exactly one created and one status_updated line, each
with its own request's id, nothing on the global logger. Mutations (each helper back to
&logger.Log) red; orchestrator re-ran one. Advisor PASS.

## 2026-10-11 — 8.2 secrets never reach logs

audit.go: redactDetails/redactValue/isSensitiveKey (lowercase, '-' -> '_', substring match
on password, passwd, secret, token, api_key, apikey, key, authorization, cookie, session,
csrf, signature), applied before log, stored Details and checksum. Existing audit test
key "key" renamed "note". Real leak found and fixed: downloadFileWithRetry logged repo_url
with userinfo (https://user:pw@ / https://token@) on download.* lines -> stripURLUserinfo
(not url.Redacted, which keeps the username/token). secret_log_test.go captureAllLogs puts
global, request and GORM (threshold 0) loggers on one buffer; each test asserts the
expected event line exists and no sentinel appears anywhere. Orchestrator mutations:
redactDetails returns input -> red; repo_url raw -> 3 rows red. Suite ok.
Advisor MINOR: struct/pointer detail values are not redacted (no caller passes them);
substring "key"/"session" over-redacts harmless keys - pick audit detail keys accordingly;
fetch subtest builds resource.Service before the GORM capture swap.

## 2026-10-11 — 8.5 request-triggered fetches log with request_id

background.go: fetchVersionContent and downloadFileWithRetry take the logger from ctx;
fetch DB writes (both version updates, RecordFetchFailed on failure and panic paths) bound
to ctx. detachedLogCtx(c) rebuilds the bare request logger on context.Background() (not
cancelled with the request, no *fiber.Ctx captured). resource_webhooks.go goroutines and
resource_crud.go create / add-version / retry fetches pass it. fetch_log_test.go covers
webhook new-tag and latest-refresh, request-triggered fetches, and the no-context fallback.
Merged with 8.2 in downloadFileWithRetry (kept FromContextOr + stripURLUserinfo).
Orchestrator mutation: background.go back to &logger.Log -> 3 tests red. Suite ok.
Advisor MINOR: three surviving mutants (panic-path WithContext, webhook follow-up First,
success-path update) and duplicated goroutine bodies -> 8.10.

## 2026-10-11 — 8.8, 8.9, 8.10

8.8: server.go useRequestLogging(app, base) - Run's request-logging wiring is now tested
(TestUseRequestLogging; four mutations red). Advisor PASS.
8.9: gormlogger formatText + safeArg (orchestrator extension: typed-nil errors render
<nil> instead of panicking, named scalar kinds keep their value, TestSafeArg). Raw args no
longer serialised: fixes the useless `marshaling error` on connect failures and removes
the risk of serialising pgconn.Config with the DB password. Password-sentinel test is a
regression guard (ConnectError.Error() omits the password).
8.10: finishWebhookFetch dedupes the webhook goroutines; fetchVersionHook seam; the three
previously surviving WithContext mutants now killed. Deslop find-similar unavailable to
the worker; the helper replaces two copies rather than adding one.
Orchestrator mutations: 8.8 RequestLogger removed, 8.9 raw args restored, typed-nil guard
disabled, 8.10 follow-up First without WithContext - all red. Suite ok.
8.8 review found a real gap -> 8.11: handler panics produce no http.request line, and
without SENTRY_DSN no middleware recovers them.

## 2026-10-11 — 8.11 panic recovery, 8.4 end-to-end production run

8.11: server.go recoverPanics() registered in useRequestLogging after RequestLogger. Avoids
Fiber's recover middleware (default ErrorHandler would put the panic value in the body).
Reports through sentryfiber.GetHubFromContext(c).RecoverWithContext; outer sentryfiber
(Repanic:false) stays as backstop for panics in requestid/RequestLogger themselves (only
with SENTRY_DSN). First review REJECT: a worker mutation had been left in the session
middleware log calls; fixed, re-review PASS. Orchestrator mutation: Sentry report removed
-> TestRequestLoggingPanicReportedToSentry red.
8.4: cmd/ramble/e2e_test.go builds and runs the real binary against testcontainers
Postgres in production mode. Orchestrator mutation: println in handlers.Home -> red
(stderr). Notes: the ConsoleWriter mutation fails via the 60s readiness timeout; the go
test cache does not track ./views and ./public, so run with -count=1 after template
changes. Suite ok.
8.4 found a production bug outside logging: the global csrf middleware has no exemption
for POST /resource/:id/webhook and POST /webhooks/github/issues, so webhook deliveries
get 403 before the handler runs. Not fixed here - reported to the user.

PATTERN: e2e - an end-to-end run of the real binary finds what unit tests with hand-built
apps cannot (here: middleware order blocking webhooks entirely).

## 2026-10-11 — Phase 9 started; 9.3 staticcheck findings

Phase 9 scope: webhook CSRF bug (8.4 finding), review debt from Phases 6-8, staticcheck
(latest via go run; installed binary too old for go1.26), go fix modernizers, Go 1.27 bump
(user request, 9.8). gosec clean.
9.3: pages.go titleWords replaces deprecated strings.Title; TestGetPageTitle written and
passing against the old code first. Orchestrator mutation ToTitle -> ToLower red. Suite ok.

PATTERN: tooling - the installed staticcheck (go1.25 build) cannot load go1.26+ modules;
use `go run honnef.co/go/tools/cmd/staticcheck@latest ./...`.

## 2026-10-11 — 9.2 dead code and test-only wrappers

Deleted: validateGitHubSignature (duplicate of github.ValidateWebhookSignature),
setupAuthenticatedApp (unused test helper), SyncGitHubToRequest + syncGitHubToRequest (no
callers), and the database.DB wrappers only tests used (processOAuthUser, handleNewIssue,
handleIssueStateChange, GetPopularTags). Tests now call processOAuthUserDB, handleNewIssueDB,
handleIssueStateChangeDB, popularTags(database.DB); TestGetPopularTags_* renamed
TestPopularTags_* by the orchestrator.
isAllowedContentType (added 9c5135f, never called) DELETED rather than wired, because:
(1) downloaded content is never served to a client with the remote Content-Type - it is
parsed as HCL, stored as a string, or HTML-escaped - so the header adds no security; the
SSRF host allowlist and the 1 MiB read cap already bound the download; (2) downloadFile
builds its client and hosts internally, so wiring it testably needs a new seam;
(3) GitLab raw files go through Workhorse and can return application/octet-stream or
sniffed types, and the case-sensitive prefix check would reject valid responses.

## 2026-10-11 — 9.1 webhook CSRF exemption; SQL injection found (9.0)

9.1: server.go isWebhookRequest (POST + exact path, resource id digits only via regexp) as
csrf Next; HandleWebhook parses the id with ParseUint before querying. e2e seeds a user and
resource with a sentinel webhook secret via database/sql (pgx stdlib, now a direct dep) and
proves signed resource and GitHub webhooks reach their handlers, bad signatures get the
handler's 401, /login without a token gets 403. Orchestrator mutations: predicate false ->
unit + e2e red (403 on signed webhooks, i.e. the original bug); predicate true -> unit red;
ParseUint removed -> TestHandleWebhook_NonNumericID red.
SECURITY (advisor on 9.1, confirmed by the orchestrator with a DryRun probe): GORM's
First(&x, idString) treats a non-numeric string as a raw WHERE condition
(statement.go BuildCondition). ~30 handlers pass c.Params("id") strings. Public:
GET /requests/:id, GET /resource/:id/fetch-readme; the rest need a login (open registration)
or admin. Loki (~14 days, ramble + traefik JSON and CLF) shows no probes. Since ~January.
User chose to fix it as 9.0 inside Phase 9's PR.

PATTERN: gorm - never pass a request string as the primary-key argument of First/Find/
Delete/Take; parse to uint first or constrain the route (:id<int>). GORM only treats the
string as a key if strconv.Atoi succeeds; otherwise it is raw SQL.
PATTERN: review - exempting a route from a middleware (CSRF) can remove an accidental
guard; check what the handler does with untrusted input before widening exposure.

## 2026-10-11 — 9.0 SQL injection fixed

internal/handlers/params.go: paramID(c, name) -> parseID(s): ParseUint base 10, 64-bit,
rejects 0. All 31 id-like c.Params sites (28 "id", 3 "member_id", 1 "org_id"; admin 16,
requests 6, web FetchReadme, resource_crud 5, resource_social, resource_webhooks 2, org 1)
use it and return their existing not-found response. registerRoutes(app) extracted from
Run (static files through catch-all, limiters, dev route); 30 params carry <int>.
Guards: TestRegisterRoutes_IDParamsAreIntConstrained (route table), TestParamsIDOnlyInParamID
(AST; flags id-like literal, non-literal names, ParamsInt, AllParams outside paramID),
TestIDHandlers_RejectInjectionPayloads (8 handlers incl. two-param admin routes, payloads
id>0 / 1 or 1=1 / pg_sleep / -1 / +1 / abc, GORM capture at 1ns threshold, payload never in
SQL), TestIDHandlers_BadIDIs404BeforeAnyWork, e2e probes (404, fast). Before the fix the
handler test showed WHERE id>0 returning a row.
Orchestrator: suite, gosec and staticcheck clean; mutations <int> dropped -> route guard red;
GetRequest back to raw string -> AST guard + injection test red; zero accepted ->
TestParamID/0 red. Orchestrator tidy: parseID extracted so TestParamID tests the parse
directly (the " 1" and "" rows previously never reached the handler); dead valid/control
case fields removed; section comments fixed.
Review notes: scanParamsCalls inspects FuncDecl bodies only (package-level func literals
not covered; none exist); idLike/idParamName duplicated across two test packages; /requests/abc
now 404s via the catch-all falling through -> 9.9 (no reserved usernames).

## 2026-10-11 — 9.8 Go 1.27

User request. go 1.26.1 -> 1.27.2 (latest; 1.26.9 still supported), golang:1.27-bookworm,
setup-go 1.27.x in ci, release and security-nightly. GOTOOLCHAIN=auto switched the local
toolchain to 1.27.2. go test -tags=security ./..., govulncheck (latest; vulns present in
required modules but none called), gosec clean; `docker build --target builder` ok.
Not done (offered to the user): `go get -u` dependency refresh; pinning alpine:latest.

## 2026-10-11 — 9.6 coverage gaps

TestConnectMigrateFailed: SetupTestDB, users table replaced by a view -> AutoMigrate fails
(42P07); asserts exit codes [1], ERROR db.migrate_failed with error, no migrate_completed.
Orchestrator fix: restore the caller's DB handle (SetupTestDB had left database.DB pointing
at the closed test handle). e2e trackAssets opens every file under views/ and public/ in the
test process. Note: in practice the e2e test is never served from the go test cache here
(two consecutive runs both executed), so the stale-cache risk was not observed; trackAssets
is kept as cheap insurance. msgkey guard now rejects stdlib log.Print*/Fatal*/Panic* in
server code. Orchestrator mutations: exit(1) removed on migrate branch -> red; log.Printf
added to a handler -> guard red. Advisor PASS.

## 2026-10-11 — 9.4 outer panic backstop, Sentry scrub, update-check predicate

server.go useOuterMiddleware(app, base, withSentry): recoverOuter first (logs http.panic
with outer=true via the base logger, returns 500), then sentryfiber. Advisor REVISE: with
Repanic:false sentryfiber swallowed middleware panics as a 200 with no log line, so it is
now Repanic:true; handler panics are recovered inside by recoverPanics and reported once.
sentryOptions(dsn, env, release) sets BeforeSend: scrubSentryEvent (QueryString, URL
?/#, Cookies, Data cleared; Authorization, Cookie, X-Hub-Signature-256, X-Webhook-Timestamp
removed case-insensitively). Tests go through a capture Transport, not a test BeforeSend.
root.go wantsUpdateCheck compares versionCmd/serverCmd by pointer; help matched as the
root's direct child (rootCmd itself would be an init cycle).
Orchestrator: TestOuterBackstop now panics only on /boom and checks a follow-up /ok
succeeds (the /ok route was dead). Mutations: Repanic false -> with_sentry red; BeforeSend
removed -> TestSentryEventsCarryNoRequestSecrets red; name compare -> nested_server red.

## 2026-10-11 — 9.5 handler and test debt

audit.go redactValue sends any other composite through redactComposite (marshal, decode
with UseNumber so big ints stay exact, recurse); unmarshalable values -> "[UNSERIALIZABLE]";
caller's value untouched. fetch_log_test.go: follow-up lookup assertions folded into
TestHandleWebhook_FetchLogsCarryRequestID. New internal/testutil/logtest (Lines, Find; imports
nothing under rmbl) replaces the logLines copies in server, logger and handlers tests; note
Lines fails on non-JSON lines where the old handlers copy skipped them. email sendSecrets adds
url.QueryEscape/PathEscape of the link and token values.
Orchestrator: fixed a stale logLines call in TestOuterBackstop (added after 9.5's worktree was
cut), moved the logtest import into the rmbl group in five files, replaced a name-matched
skip in TestRedactDetails with a marshal-error check. Mutations: composite fallback removed
-> struct rows red; encoded secrets removed -> two encoded-echo rows red; follow-up First
without WithContext -> merged webhook test red. Suite ok.

## 2026-10-11 — 9.9 reserved usernames and org names

internal/handlers/reserved_names.go IsReservedName; enforced in PostSignup, PostCreateOrg,
admin PostEditUser/PostEditOrganization (only when the name changes case-insensitively),
and processOAuthUserDB (reserved base -> base-1..base-20, skipping reserved and taken names
by exact LOWER() equality). TestRegisterRoutes_TopLevelSegmentsAreReserved derives the
required list from registerRoutes (ENV=test so the dev route is included).
Not changed: database.SeedInitialUser (operator-chosen; config.example.yml defaults to "admin").
Existing offenders: SELECT id, username FROM users WHERE lower(username) IN (...);
SELECT id, name FROM organizations WHERE lower(name) IN (...); (add deleted_at IS NULL).
Orchestrator: suite ok (one testcontainers startup timeout in internal/services/resource on the
full run, passed on re-run - infrastructure, not code); mutations: "requests" dropped -> router
guard red; signup check bypassed -> TestPostSignup_ReservedUsername red.
Review follow-ups -> 9.10: ordinary taken OAuth nickname fails on the unique index;
PostCreateOrg uniqueness uses ILIKE (_ and % act as wildcards).

PATTERN: tests - testcontainers startup can time out when many packages start Postgres at
once; a single package failing with "wait until ready: context deadline exceeded" is
infrastructure - re-run that package before suspecting the code.

## 2026-10-11 — 9.10 ILIKE wildcards in exact lookups; OAuth taken nicknames

utils.go iEq(col) -> LOWER(col) = LOWER(?); all 31 unescaped `ILIKE ?` exact-match sites
(web, resource_crud, org, auth, nomad_pack) converted; compound clauses chained with Where.
Escaped searches (ILIKE ? ESCAPE) untouched. sql_like_guard_test.go TestNoUnescapedLikeQueries
(regexp; known limit: misses LIKE built via || or Sprintf). lookup_wildcard_test.go covers
underscore and percent in user/resource lookups, org/signup uniqueness, OAuth taken nicknames.
availableOAuthUsername(tx, base) replaces unreservedOAuthUsername: free base kept, else base-1..20;
orchestrator added: empty base -> "user" (provider with no nickname and no name previously
created an empty username).
Orchestrator mutations: ILIKE restored in GetResource -> guard red; in GetUserProfile -> guard and
three profile tests red; empty-base default removed -> OAuth test red. Suite ok.
Not added: LOWER() functional unique indexes (prod may hold case-variant duplicates) -> 9.11
handles the app-level checks. Noted: GetOrgSettings ignores First() error -> 9.11.

## 2026-10-11 — 9.11 case-insensitive name uniqueness, missing-org 404s

auth.go nameTaken(tx, name, exceptUserID, exceptOrgID): users and orgs via iEq, excluding the
given ids; usernameFree builds on it. PostSignup: separate email check + nameTaken (users and
orgs). Admin PostEditUser/PostEditOrganization: when the name changes exactly, nameTaken with
self-exclusion (own case-variant allowed; legacy case-duplicates stay editable). org.go:
GetOrgSettings, PostUpdateOrg (previously Save()d a zero-value org = INSERT of a blank org on
a miss), PostAddMember, PostRemoveMember now 404 on a missing org (lookups moved before writes).
No LOWER() unique index. Duplicate audit SQL:
SELECT LOWER(name) AS folded, COUNT(*) AS n, STRING_AGG(kind||':'||id||':'||name||CASE WHEN
deleted_at IS NOT NULL THEN ' (deleted)' ELSE '' END, ', ' ORDER BY kind, id) AS rows FROM
(SELECT 'user' AS kind, id, username AS name, deleted_at FROM users UNION ALL SELECT 'org', id,
name, deleted_at FROM organizations) x GROUP BY LOWER(name) HAVING COUNT(*) > 1 ORDER BY folded;
Review REJECT: cleanupOrgTestData missed mixed-case test orgs (fixed: LOWER(name) LIKE 'test%').
Orchestrator mutations: admin user rename check removed -> red; signup nameTaken removed ->
both signup tests red. Suite ok. Leftovers -> 9.12.

## 2026-10-11 — 9.12 review leftovers

org.go orgLoadError: 404 only for gorm.ErrRecordNotFound, else 500 + ERROR org.load_failed
(RequireOrgOwner membership lookup: 403 / 500 org.membership_load_failed). PostAddMember: user
lookup via iEq; DB errors 500 (org.member_lookup_failed / org.member_add_failed). Real bug fixed:
idx_user_org includes soft-deleted memberships, so remove + re-add hit the index and returned 200
with no membership; the pre-check is now Unscoped and restores a soft-deleted membership.
nameTaken counts Unscoped (soft-deleted names now give 400, not a 500 at Save).
Test idempotency: cleanupTestData deletes resource_tags, user_stars, test memberships and the
popular-tag tag, and reports every Exec error; four tests fixed (org rename into test% namespace,
signup user cleanup, sitemap hard deletes). go test -count=2 ./internal/handlers/ passes.
Forced DB errors are tested with a separately opened gorm.DB on the shared pool with a failing
callback (never registered on database.DB, never closing the pool).
Orchestrator: gofmt fixes (org.go imports, handlers_test.go alignment); fixture cleanup now reports
errors; addMemberApp unused param removed, t.Helper added; role from the add-member form limited
to owner/member like the admin handlers (TestPostAddMember_UnknownRoleBecomesMember).
Mutations: role check removed -> red; nameTaken without Unscoped -> both soft-deleted tests red.
One-off flake seen by the worker: TestDownloadAndFetch_NeverLogRepoURLUserinfo failed once, not
reproducible (-count=5 green).

## 2026-10-11 — 9.7 go fix + CI checks; Phase 9 collapsed

go fix ./... and go fix -tags=security ./... (Go 1.27 modernizers): 46 files, mostly
interface{} -> any. CI test job gains go vet (both tags), gofmt -l and go fix -diff gates (it
had none). staticcheck 2026.2.1 cannot read Go 1.27 export data (v5) -> not added; add it to
CI once a release supports Go 1.27. Local: vet, gofmt, suite, swagger (no diff), gosec clean.
Phase 9 (13 tasks incl. 9.0 SQL injection) done and archived.
Remaining known debt (not in Phase 9): staticcheck in CI (blocked on upstream); a LOWER()
unique index on users.username / organizations.name after auditing prod for case-variant
duplicates (SQL in 9.11); optional `go get -u` dependency refresh and pinning alpine:latest.

## 2026-10-11 — CI: gosec pin for Go 1.27

PR #54 security-scan failed: the CI pin gosec v2.22.11 cannot load Go 1.27 packages
("package encoding/json without types"); reproduced locally. Bumped the pin to v2.29.0
(2026-08-25), which runs clean on Go 1.27. Missed in 9.8: when bumping Go, check every
pinned analysis tool (gosec, staticcheck, swag) against the new toolchain.

PATTERN: toolchain-bump - Go major bumps break pinned go/analysis tools (gosec, staticcheck)
before they break the build; run each pinned tool at its pinned version on the new toolchain.

## 2026-10-11 — CORRECTION: gosec on Go 1.27

The "gosec clean" results recorded for 9.8, 9.4-9.6 checks and 9.7 were wrong: gosec exited 1
with 241 "export data version 5 is greater than maximum supported version 4" errors and
"Issues: 0"; the orchestrator read the output and piped through tail, losing the exit code.
No tagged gosec (<= v2.29.0, built against x/tools v0.49 / go 1.25) reads Go 1.27 export data.
gosec master v2.29.1-0.20261009120814-7b1b5cebe007 does: exit 0, 0 issues on the branch. CI and
the nightly workflow now pin that commit, with a comment to return to a tagged release.
govulncheck@latest verified with its real exit code: 0.

PATTERN: verification - never judge a tool by its output piped through tail/grep; capture the
exit code ($? of the tool itself, or set -o pipefail). A tool can print "Issues: 0" and fail.

## 2026-10-11 — case-insensitive unique name indexes (post v0.5.18)

The user ran the duplicate audit against prod: no case-variant duplicates. database.Connect now
runs ensureCaseInsensitiveNameIndexes after AutoMigrate: CREATE UNIQUE INDEX IF NOT EXISTS
idx_users_username_lower ON users (LOWER(username)) and idx_organizations_name_lower ON
organizations (LOWER(name)) (soft-deleted rows included, like the existing indexes). A failure
logs ERROR db.name_index_failed and does not exit (handlers already reject case variants).
User-vs-org clashes stay an application check (nameTaken). The handler test DB helper does not
create these indexes (two tests seed legacy case-duplicates on purpose).
Tests: TestCaseInsensitiveNameIndexes (idempotent, Alice/alice and Acme/ACME rejected),
TestConnectNameIndexFailureIsNotFatal. Mutations: plain-column index -> red; exit(1) on failure
-> red. vet, gofmt, go fix check, suite, gosec (exit 0) ok.

## 2026-10-11 — staticcheck back in CI; analyser pins in one place

User asked to stay current with gosec and staticcheck. Neither tagged release reads Go 1.27
export data (gosec v2.29.0, staticcheck 2026.2.1); both masters do: gosec 7b1b5ce and
staticcheck 452d5bb (both 2026-10-09) exit 0 with no findings. Makefile now owns the pins
(GOSEC_VERSION, STATICCHECK_VERSION) with `make staticcheck` (both tag sets) and `make gosec`;
CI calls them; bootstrap and `make security` use the same pins. The weekly security workflow
runs both at master (plus govulncheck@latest) so new checks and toolchain breaks surface there
first; bump the Makefile pins when it passes.

## 2026-10-12 — dependency refresh and alpine pin

go get -u ./... + go mod tidy: ~60 modules. Direct: ssrf 0.2.0->0.3.0 (refactor, stdlib slices,
adds 3fff::/20 to the deny list - stricter), goth_fiber 0.3.3->0.5.0 (fixes only: Save error
returned, safe session value assertion, provider via Locals), cobra 1.8.1->1.10.2, pgx 5.9->5.11,
fasthttp 1.71->1.75, sentry-go 0.49->0.50, fiber 2.52.15, gorm 1.31.2, hcl 2.25, zerolog 1.35.1,
x/crypto 0.58. Dockerfile runtime alpine:latest -> alpine:3.24 (3.24.2 today; patch updates on
rebuild, no surprise minor bumps). Checks: mod verify, vet (both tags), gofmt, go fix, make
staticcheck, make gosec (0 issues), govulncheck (exit 0; GO-2026-5932 in x/crypto has no fix and
is not called), full suite incl. e2e, docker build + binary runs on alpine 3.24.2.

## 2026-10-14 — Phase 10 planned; 10.3 non-root container

Phase 10 (CI/runtime maintenance) and 11 (slog) planned with the user: ubuntu-26.04 for hosted
jobs; slog keeps ramble's event keys and request_id on SQL lines; JSON in every environment.
10.3: Dockerfile runtime stage creates ramble (uid/gid 10001, nologin, no home), WORKDIR /app,
copies root-owned read-only assets, USER 10001:10001 (numeric). Server writes nothing to disk
(internal/pack and internal/nomad are CLI-only). Nomad job unchanged (port 3000, no volumes).
Worker verified the server in the container against postgres:17 (server.started, / /login /docs
200, no permission errors). TestDockerfileRuntimeRunsNonRoot (internal/security) guards USER,
WORKDIR, uid and no --chown. Orchestrator: build, id, touch denied; mutation USER root -> red.

## 2026-10-14 — 10.1 GitHub Actions current

Majors: checkout v5->v7, setup-go v6->v7, docker setup-qemu/setup-buildx/login v3->v4,
build-push v6->v7, attest-build-provenance v2->v4, goreleaser-action v6->v7 (codeql v4 current).
Release notes read per major; no input/permission changes required. Notes: checkout v6 keeps
persisted credentials under $RUNNER_TEMP (bump-version git push still works); attest v4 on an
org repo with push-to-registry tries to create a storage record needing `artifact-metadata:
write` - it only warns, permissions left unchanged pending a decision. Hosted jobs ubuntu-latest
-> ubuntu-26.04 (user decision; GA per actions/runner-images). Self-hosted runner already runs
node24 actions (checkout@v5), so it meets v2.327.1. actionlint v1.7.12 does not know
ubuntu-26.04: .github/actionlint.yaml declares it (remove when actionlint ships the label);
quoted $GITHUB_OUTPUT (SC2086) so actionlint exits 0.
Orchestrator dropped the worker's internal/workflows/workflows_test.go: it hard-coded action
majors in the Go suite, so every bump would break go test. actionlint in CI (10.2) is the guard.

## 2026-10-14 — 10.2 build tool pins; Phase 10 collapsed

Makefile: SWAG_VERSION v1.16.6 (latest; what @latest resolved to), TAILWIND_VERSION v4.1.18
(Docker already pinned it; `make tailwind-install` fetched latest = v4.3.3, so local CSS could
differ from prod - now both 4.1.18; moving to 4.3 is a separate, visual change), ACTIONLINT_VERSION
v1.7.12. Targets: actionlint, check-pins (Dockerfile ARG defaults must equal the Makefile pins),
print-%. Dockerfile ARG SWAG_VERSION / TAILWIND_VERSION. CI test job: make actionlint, make
check-pins, make swagger (no separate swag@latest install).
Checks: check-pins drift mutation (Dockerfile tailwind v4.3.3) -> exit 2; make swagger -> no
api-docs diff; docker build ok; suite ok (handlers/database hit testcontainers startup timeouts
while ~34 containers from other sessions shared Docker; re-run green).
Release workflow changes (10.1) are only proven by the next release.
PR #61 CI: the go fix gate caught strings.Split -> SplitSeq in the new dockerfile_test.go (the
orchestrator ran the fix/gofmt checks before 10.3 landed, not after). The gate also failed
silently: go fix -diff exits non-zero on changes and bash -e aborted before printing; the step
now uses `|| true` inside the substitution so the diff is shown. Verified both ways locally.

## 2026-10-14 — release workflow: artifact-metadata permission

User approved adding `artifact-metadata: write` to release.yml so attest-build-provenance v4 can
create the artifact storage record instead of warning on every release. actionlint clean.

## 2026-10-14 — 11.1 slog foundation

logger.S (slog JSON handler, LevelDebug, no AddSource; discards until Init) with ReplaceAttr that
keeps errors strings even for json.Marshaler error types. SWithContext/SFromContext/SFromContextOr
(unexported sloggerKey). RequestLogger(base, sbase) attaches both loggers with the same request_id;
http.request stays on zerolog until 11.3. zerolog development output switched to JSON (ConsoleWriter,
defaultLevelMarshal and the dev-only Caller() removed), so both loggers are JSON everywhere now.
Side effect: org_errors_test logHasEvent matched lower-case levels that only appeared because the
dev marshaller leaked through a process-global; it now matches case-insensitively (pin the level
when it moves to slog in 11.3).
Mutations red: drop SWithContext attach; lower-case level via ReplaceAttr; SFromContextOr never
returns the attached logger; drop the error-to-string ReplaceAttr; (orchestrator) drop the attach
again and a wrong WithUser field after making TestWithUser/TestWithRequest assert.
Note: server.go reads logger.S when useRequestLogging runs, after Init; reordering would send
request slog lines to the discard handler. Goes away in 11.3.
PATTERN: slog - look slog lines up by "msg"; logtest.Find uses zerolog.MessageFieldName until 11.4.

## 2026-10-14 — 11.2 GORM logger on slog

gormlogger.go uses slog (LogAttrs; error written as slog.String so it stays a string without the
production ReplaceAttr); events, fields, levels, safeArg/formatText/ParamsFilter unchanged;
newDefaultGormLogger reads logger.S at call time. Advisor caught a spec gap: detachedLogCtx
(background.go) rebuilt the goroutine context with only the zerolog logger, so fetch/webhook SQL
would have lost request_id; it now carries both. Test ports: gormlogger_test, database_test
(swapFailSeams swaps S; migrate test now requires db.query_error at ERROR), handlers db_test,
secret_log_test (captureAllLogs swaps S so "absent from all output" is not vacuous), fetch_log,
params, webhook_log, flash_log, org_errors, resource withcontext/service tests. handlers and
resource TestMains set zerolog.MessageFieldName="msg" (as Init does) so mixed buffers share a key.
Mutations red: logger ignores ctx; ParamsFilter returns params; slow query at INFO;
ErrRecordNotFound logged; detachedLogCtx without SWithContext (webhook latest_refresh/new_tag).
Orchestrator re-ran the ctx mutation: TestReqDB_QueryErrorCarriesRequestID,
TestRequestChain_QueryErrorSharesRequestIDWithAccessLine, TestHandleWebhook_FetchLogsCarryRequestID red.
Note for 11.3/11.4: the rid-810 fetch tests build their ctx with reqLogCtx by hand, so only
TestHandleWebhook_FetchLogsCarryRequestID guards detachedLogCtx.

## 2026-10-14 — 11.3 call sites on slog

All non-test zerolog use is gone except internal/services/logger/logger.go (Log, stdlib bridge,
FromContext/FromContextOr, WithUser/WithRequest; removed in 11.5). logger.Err(err) slog.Attr (zero
Attr for nil, string otherwise) for every error field; logger.NewJSON(w) builds the production
handler (Init and tests use it). RequestLogger(base *slog.Logger); levelForStatus returns slog.Level;
detachedLogCtx carries only slog; http.panic via SFromContext. audit.recorded and
syncRequestToGitHubWithRetry stay on logger.S because they set their own request_id (a request
logger would emit the key twice). zerolog .Stack() dropped: it was a no-op (no .Err, no
ErrorStackMarshaler). Advisor caught: slog.Duration writes int nanoseconds where zerolog Dur wrote
float ms - durations are slog.Float64 ms; the event-key guard (msgkey_test) only scanned .Msg and
would go red - ported to slog Debug/Info/Warn/Error (+Context, Log/LogAttrs) with TestScanFileMsgKeys.
Mutations red: 5xx at INFO; RequestLogger no attach; http.panic without request_id; Err as slog.Any;
detachedLogCtx drops logger; org.go wrong key; non-dotted key (guard); slog.Duration.
Orchestrator re-ran: http.panic via logger.S (TestUseRequestLoggingRecoversPanic and the Sentry test
red); slog.Duration in background.go (TestFetchVersionContent_DurationIsMilliseconds red).
Gap: github.sync_completed duration has no own test - 11.4 adds an AST guard against slog.Duration.

## 2026-10-14 — 11.4 guards and 11.5 code: zerolog removed

Final names: logger.Log (*slog.Logger), FromContext, FromContextOr, WithContext, ctxKey, Err, NewJSON.
Deleted the zerolog Log, upperLevelMarshal, zerolog FromContext/FromContextOr, WithUser/WithRequest.
initWithWriter: Log = NewJSON(w); slog.SetDefault(Log); THEN stdlog.SetFlags(0)/SetOutput(stdlibWriter)
(SetDefault redirects stdlib log into slog with the raw text as msg). stdlibWriter logs
stdlib.log with field text. Test restore helpers call slog.SetDefault(prev) before
stdlog.SetOutput(prev) (otherwise stdlog stays pointed at a dead buffer handler; golang/go#61892
deadlock class). go mod tidy removed rs/zerolog from go.mod/go.sum; TestMain MessageFieldName hacks gone.
Guards (msgkey_test.go, hit/miss fixtures, resolved by import path so local `log` vars are not
flagged): no os.Exit call in server code (database's `var exit = os.Exit` is a reference; internal/cli
is skipped); no stdlib log and no zerolog import (non-test files); no package-level slog calls; no
slog.Duration and no slog.Any("error", ...). Event-key guard: 79 calls checked (floor 40).
Mutations red: SetDefault after SetOutput; SetDefault dropped; NewJSON at LevelInfo; os.Exit in
database.go; slog.Error / slog.Any("error") / slog.Duration / zerolog import in server.go.
Orchestrator re-ran: slog.Error in admin.go -> TestServerCodeDoesNotCallPackageLevelSlog red; tidy no-op.
Open: whether dependencies emit stdlib/slog lines in prod - check `msg="stdlib.log"` in Loki after deploy.

## 2026-10-14 — 11.5 shipped; Phase 11 collapsed

#63 merged (f03561e), v0.5.22 released and deployed 08:50 UTC. Loki: server.started v0.5.22;
no non-JSON and no stderr lines; by msg/level only INFO db.connected/migrate_*, sentry.enabled,
http.request, server.started; an unsigned POST to /webhooks/github/issues -> 401 with
webhook.signature_missing (WARN) carrying the access line's request_id; no stdlib.log lines.
(An unsigned POST to a non-route gets 403 from CSRF, as before - not a regression.)
Docs: ramble CLAUDE.md (in #63), infra docs/prod-logging.md f5b4f89, prod-logs skill.
