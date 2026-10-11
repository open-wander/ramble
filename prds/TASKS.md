# Tasks

All phases complete (prd-security-bugfix; Phase 6 from infra `docs/prod-logging.md`). Full history in [TASKS-archive.md](TASKS-archive.md);
verification in [verification.md](verification.md). PRD: [done/prd-security-bugfix.md](done/prd-security-bugfix.md).

## Phase 1: Authorization — 2 tasks completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 2: Fetch Hardening — 2 tasks completed 2026-05-30 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 3: Auth Hardening — 2 tasks completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 4: Cleanup — 1 task completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 5: Review Follow-ups — 5 tasks completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 6: Structured logging — 6 tasks completed 2026-10-11 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 7: Logging standard gaps (from verify 2026-10-11)

Source: [verification.md](verification.md) (Phase 6 verify, gaps 1-6). Fixed on
`feat/logging-followups` before PR #52 merges. Gap 7 (minor) accepted as-is.

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 7.1 | Replace the three `fmt.Printf` calls on request paths (`auth.go:384`, `auth.go:488`, `utils.go:49`) with request-scoped zerolog event keys; stop email validation errors carrying the recipient address; extend the AST guard to fail on `fmt.Print*`, `print`/`println` and `fmt.Fprint*(os.Stdout/os.Stderr)` in server packages | verification.md gaps 1, 2 | DONE (2026-10-11) | email.verification_send_failed / email.reset_send_failed / session.save_failed (op=set_flash) via the request logger; email.go errors no longer embed the address. `TestServerCodeDoesNotWriteStdout` (AST, excludes internal/cli and cmd/nomad-vars operator CLI). Advisor REVISE plan, REJECT then MINOR. |
| 7.2 | Webhook handlers log through the request logger: `resource_webhooks.go:228` webhookLog, `HandleGitHubIssueWebhook` and the `github.webhook_request_*` lines in handleNewIssueDB/handleIssueStateChangeDB | verification.md gap 3 | DONE (2026-10-11) | HandleWebhook / HandleGitHubIssueWebhook use the request logger; goroutines take a zerolog.Logger value; handleNewIssueDB/handleIssueStateChangeDB derive it from db.Statement.Context. webhook_log_test.go. Advisor REJECT then MINOR. |
| 7.3 | `ramble server` writes only JSON: SilenceErrors/SilenceUsage for the server command, log a JSON error line when `server.Run` returns an error, skip the update check for `server` | verification.md gap 6 | DONE (2026-10-11) | wantsUpdateCheck skips server; serverCmd SilenceErrors/SilenceUsage; runServerCmd logs `server.exit_error` and returns a loggedError that Execute does not reprint. Flag errors (`--bogus`) still plain text. Advisor MINOR. |
| 7.4 | Test what `logger.Init` itself writes in production (injectable writer), so a ConsoleWriter in the production branch goes red | verification.md gap 4 | TODO | Surviving mutant in verify. |
| 7.5 | Test that `database.Connect`'s GORM config uses the zerolog adapter, so reverting to GORM's default logger goes red | verification.md gap 5 | TODO | Extract the gorm.Config into a helper if needed. |
