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

## Phase 6: Structured logging

Source: infra `docs/prod-logging.md` item 2 (TydeWhatMay/infra, surveyed 2026-10-09). Prod
logged ~37,000 text lines/day: GORM printed every query with ANSI colours, Fiber's text
access log, stdlib `log.Printf`. Standard: one JSON object per line, `time`/`level`/`msg`,
`msg` a stable event key, no SQL at INFO, no secrets or query strings.

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 6.1 | JSON logging: GORM through a zerolog adapter (silent <200ms, `db.slow_query` WARN, `db.query_error` ERROR, `$n` placeholders only), `http.request` middleware replacing fiberlogger, all server-side `log.*` calls to event keys, stdlib `log` redirected, upper-case `level` + `msg` | infra `docs/prod-logging.md` | DONE on branch (2026-10-09) | Branch `feat/structured-logging` (bb1c408..c59d669), not merged. Tests/vet/gosec clean, re-run independently. Mutation-tested. |
| 6.2 | Merge 6.1, release, deploy; then verify in Loki that `{container="ramble"} !~ "^\\{"` returns only panic traces, and that line volume dropped | infra `docs/prod-logging.md` | DONE | Released v0.5.17, deployed 2026-10-10 21:00. Loki: only the Fiber startup banner is non-JSON (6.6); line volume ~1,390/h to ~220/h. |
| 6.3 | Thread the request context into DB calls (`database.DB.WithContext(c.UserContext())` or a helper) so `db.slow_query`/`db.query_error` lines carry `request_id` | infra `docs/prod-logging.md` | DONE (2026-10-11) | `reqDB(c)` helper (internal/handlers/db.go) on every request path; `resource.Service.WithContext(ctx)` so service transactions inherit it; session middleware inlined. Goroutines, background fetches, startup keep `database.DB`. `TestReqDB_QueryErrorCarriesRequestID` (mutation-checked). Advisor REJECT then MINOR after fix round. |
| 6.4 | Convert remaining sentence-style zerolog messages to stable event keys (`github_sync.go`, `background.go`, `audit.go`, e.g. "webhook validation failed" -> `webhook.validation_failed`), variable data in fields | infra `docs/prod-logging.md` | DONE (2026-10-11) | 34 messages in github_sync.go, background.go, resource_webhooks.go, audit.go converted (github.*, webhook.*, download.*, version_fetch.*). `msg="audit"` is now `audit.recorded`. AST guard `TestServerLogMessagesAreEventKeys` (internal/services/logger/msgkey_test.go) blocks regressions; CLI excluded. Advisor MINOR. |
| 6.5 | Decide on static-asset access lines: log `/public/*` at DEBUG or skip them | infra `docs/prod-logging.md` | DONE (2026-10-11) | Closed, no code change. Loki, 12h to 2026-10-11: 59 of 904 `http.request` lines were `/public/*` (6.5%, ~5/h). Not worth hiding asset 404s for. |
| 6.6 | Disable Fiber's startup banner (`fiber.Config{DisableStartupMessage: true}`) and log one `server.started` JSON line with addr and version instead | infra `docs/prod-logging.md` | DONE (2026-10-11) | fiberConfig() sets DisableStartupMessage; logStarted() writes `server.started` with `addr`, `version` before Listen. Tests pin the proxy settings too. Advisor PASS. |

## Phase 7: Logging standard gaps (from verify 2026-10-11)

Source: [verification.md](verification.md) (Phase 6 verify, gaps 1-6). Fixed on
`feat/logging-followups` before PR #52 merges. Gap 7 (minor) accepted as-is.

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 7.1 | Replace the three `fmt.Printf` calls on request paths (`auth.go:384`, `auth.go:488`, `utils.go:49`) with request-scoped zerolog event keys; stop email validation errors carrying the recipient address; extend the AST guard to fail on `fmt.Print*`, `print`/`println` and `fmt.Fprint*(os.Stdout/os.Stderr)` in server packages | verification.md gaps 1, 2 | DONE (2026-10-11) | email.verification_send_failed / email.reset_send_failed / session.save_failed (op=set_flash) via the request logger; email.go errors no longer embed the address. `TestServerCodeDoesNotWriteStdout` (AST, excludes internal/cli and cmd/nomad-vars operator CLI). Advisor REVISE plan, REJECT then MINOR. |
| 7.2 | Webhook handlers log through the request logger: `resource_webhooks.go:228` webhookLog, `HandleGitHubIssueWebhook` and the `github.webhook_request_*` lines in handleNewIssueDB/handleIssueStateChangeDB | verification.md gap 3 | DONE (2026-10-11) | HandleWebhook / HandleGitHubIssueWebhook use the request logger; goroutines take a zerolog.Logger value; handleNewIssueDB/handleIssueStateChangeDB derive it from db.Statement.Context. webhook_log_test.go. Advisor REJECT then MINOR. |
| 7.3 | `ramble server` writes only JSON: SilenceErrors/SilenceUsage for the server command, log a JSON error line when `server.Run` returns an error, skip the update check for `server` | verification.md gap 6 | DONE (2026-10-11) | wantsUpdateCheck skips server; serverCmd SilenceErrors/SilenceUsage; runServerCmd logs `server.exit_error` and returns a loggedError that Execute does not reprint. Flag errors (`--bogus`) still plain text. Advisor MINOR. |
| 7.4 | Test what `logger.Init` itself writes in production (injectable writer), so a ConsoleWriter in the production branch goes red | verification.md gap 4 | DONE (2026-10-11) | Init calls initWithWriter(os.Stdout); TestInit_ProductionFormat tests Init's own output and restores all globals. ConsoleWriter / lower-case level / os.Stdout mutations red. Advisor PASS. |
| 7.5 | Test that `database.Connect`'s GORM config uses the zerolog adapter, so reverting to GORM's default logger goes red | verification.md gap 5 | DONE (2026-10-11) | gormConfig() used by Connect; TestGormConfigUsesZerologAdapter asserts adapter type, threshold, base logger and db.query_error output. gormlogger.Default / nil mutations red. Advisor PASS. |

## Phase 8: Logging test hardening (open items from verify 2026-10-11)

Source: [verification.md](verification.md) (Phase 6 verify gap 7 and the Phase 7 notes).
Goal: every logging guarantee is proven by a test that a mutation turns red. Branch
`test/logging-hardening`.

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 8.1 | Integration test of the real request chain: production `requestid` + `requestLogger` middleware and a real handler whose query goes through `reqDB`; `db.query_error` carries the request's `request_id`. Remove the copied middleware from handler tests (or make it call the real one) | verification.md gap 7 | DONE (2026-10-11) | Middleware moved to `logger.RequestLogger` (internal/services/logger/fiber.go), used by server.Run and every handler log test. `TestRequestChain_QueryErrorSharesRequestIDWithAccessLine`: two requests, generated ids, http.request and db.query_error share request_id. Advisor MINOR. |
| 8.2 | Secrets never reach logs: `AuditLog`/`AuditLogNoContext` redact sensitive `details` keys (password, token, secret, authorization, key...); tests feed known secret values through audit, auth (login, reset, OAuth errors), webhook (secret/signature) and fetch-error paths and assert none appear in captured log output | verification.md S6 | DONE (2026-10-11) | `redactDetails` in AuditLog/AuditLogNoContext (substring key match, nested maps/slices, copy) for log line, stored Details and checksum. **Leak fixed:** download.* lines logged `repo_url` with userinfo; now `stripURLUserinfo`. secret_log_test.go: sentinels through audit, failed login, forgot/reset, query-string token, both webhooks, download/fetch. Advisor MINOR. |
| 8.3 | Test the `github.webhook_request_created` / `github.webhook_request_status_updated` lines carry the request's `request_id` (valid signed webhook through `HandleGitHubIssueWebhook`, DB-backed) | 7.2 review | DONE (2026-10-11) | `TestHandleGitHubIssueWebhook_DBLinesCarryRequestID`: signed opened + closed payloads through the real handler; DB row created then Completed; each line carries its own request's id, none on the global logger. Mutations of both helpers red. Advisor PASS. |
| 8.4 | End-to-end production-mode test: build and run `ramble server` with `ENV=production` against a testcontainers Postgres, make requests (200, 404, a failing query or slow path, a webhook), stop it, assert every stdout line is one JSON object with time/level/msg (no ANSI) and stderr is empty | verification.md S1 | DONE (2026-10-11) | cmd/ramble/e2e_test.go `TestE2EProductionLogging`: real binary, testcontainers Postgres, ENV=production, 5 requests; every stdout line JSON with time/upper-case level/dotted msg, no ANSI, stderr empty, startup lines present, one http.request per response X-Request-ID, no sentinel anywhere. Fails rather than skips without Docker when CI is set. Advisor MINOR. |
| 8.5 | Webhook-triggered version fetches log with the request's `request_id`: `fetchVersionContent` logs through `logger.FromContextOr(ctx, ...)` and webhook goroutines pass a context carrying the request logger (not cancelled with the request) | Phase 7 notes | DONE (2026-10-11) | fetchVersionContent/downloadFileWithRetry log via `FromContextOr(ctx)`; DB writes bound to ctx. `detachedLogCtx(c)` gives goroutines the request logger on context.Background(). Webhook, create, add-version and retry fetches covered by fetch_log_test.go. Advisor MINOR -> 8.10. |
| 8.6 | SMTP errors never carry the recipient address: sanitise errors returned by the email sender (e.g. replace the address, or keep only the SMTP code) and test with a fake SMTP server whose replies echo the recipient | Phase 7 notes | DONE (2026-10-11) | `redactError` on both send functions: addresses (regex), link, its token, recipient and sender literals -> `[redacted]`; SMTP codes kept; no Unwrap (would expose the raw reply). STARTTLS fake-server test drives the real send code (`smtpRootCAs` seam). Advisor MINOR; orchestrator added token/address literals + token-only row. |
| 8.7 | Small standard gaps: `ramble server` flag-parse errors log JSON (`server.flag_error`), and `database` startup failures log at ERROR then exit 1 instead of `Fatal` (level FATAL is outside the standard's four levels); tests for both | verification.md gap 7 | DONE (2026-10-11) | serverCmd FlagErrorFunc logs `server.flag_error` (logger initialised first) and returns loggedError. database.go: Fatal -> Error + `exit(1)` seam, same keys. Guard `TestServerCodeHasNoFatalOrPanicEvents`. **Level FATAL -> ERROR for db.credentials_missing/connect_failed/migrate_failed: check alerts.** Advisor MINOR. |
| 8.8 | Test the middleware wiring in `server.Run`: requestid before `logger.RequestLogger`, both present (today a no-op swap stays green). Extract the middleware setup into a function the test can call on a bare app | 8.1 review | DONE (2026-10-11) | `useRequestLogging(app, base)` (requestid then RequestLogger) called once from Run after Sentry. `TestUseRequestLogging`: request_id equals X-Request-ID header and is shared by a handler line. Mutations (removed, swapped, no-op, wrong base) red. Advisor PASS. |
| 8.9 | GORM adapter `db.error`/`db.warn`/`db.info` lines marshal raw `args` and fail on unserialisable values (`args="marshaling error: json: unsupported type: pgconn.DialFunc"` on a connect failure). Format args safely (e.g. `%v` strings) and test with an unserialisable arg | 8.7 review | DONE (2026-10-11) | `formatText`/`safeArg`: db.info/warn/error render args into `text` (errors via Error(), scalars incl. named kinds, everything else as `<type>`); raw `args` field removed, so no `marshaling error` and no pgconn.Config (password) serialisation. Orchestrator added typed-nil and named-scalar handling + `TestSafeArg`. Advisor MINOR. |
| 8.10 | Close 8.5's surviving mutants and duplication: extract the two identical webhook goroutine bodies in `resource_webhooks.go` into one helper; add tests that go red when `WithContext(ctx)` is dropped from the panic-path `RecordFetchFailed`, the webhook follow-up `.First`, and the success-path `resource_versions` update | 8.5 review | DONE (2026-10-11) | `finishWebhookFetch` replaces the two identical goroutine bodies; `fetchVersionHook` test seam (nil in prod). Tests for panic-path RecordFetchFailed, webhook follow-up lookup and success-path update each go red without their WithContext. Advisor MINOR. |
| 8.11 | Panics are logged and recovered regardless of `SENTRY_DSN`: a recover middleware inside the request logger so a handler panic yields one `http.panic` ERROR line (with request_id, panic value, stack) and the `http.request` line with status 500; Sentry still sees the panic when enabled | 8.8 review | DONE (2026-10-11) | `recoverPanics()` inside `useRequestLogging` (after RequestLogger): one `http.panic` ERROR line (panic, stack, request_id), 500 with no panic text in the body, `http.request` status 500, reported to Sentry via the hub when present. panic_test.go with and without Sentry. Advisor REJECT (stray mutation left in) then PASS. |
