# Tasks

All phases complete (prd-security-bugfix; Phase 6 from infra `docs/prod-logging.md`). Full history in [TASKS-archive.md](TASKS-archive.md);
verification in [verification.md](verification.md). PRD: [done/prd-security-bugfix.md](done/prd-security-bugfix.md).

## Phase 1: Authorization — 2 tasks completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 2: Fetch Hardening — 2 tasks completed 2026-05-30 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 3: Auth Hardening — 2 tasks completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 4: Cleanup — 1 task completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 5: Review Follow-ups — 5 tasks completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 6: Structured logging — 6 tasks completed 2026-10-11 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 7: Logging standard gaps — 5 tasks completed 2026-10-11 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 8: Logging test hardening (open items from verify 2026-10-11)

Source: [verification.md](verification.md) (Phase 6 verify gap 7 and the Phase 7 notes).
Goal: every logging guarantee is proven by a test that a mutation turns red. Branch
`test/logging-hardening`.

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 8.1 | Integration test of the real request chain: production `requestid` + `requestLogger` middleware and a real handler whose query goes through `reqDB`; `db.query_error` carries the request's `request_id`. Remove the copied middleware from handler tests (or make it call the real one) | verification.md gap 7 | DONE (2026-10-11) | Middleware moved to `logger.RequestLogger` (internal/services/logger/fiber.go), used by server.Run and every handler log test. `TestRequestChain_QueryErrorSharesRequestIDWithAccessLine`: two requests, generated ids, http.request and db.query_error share request_id. Advisor MINOR. |
| 8.2 | Secrets never reach logs: `AuditLog`/`AuditLogNoContext` redact sensitive `details` keys (password, token, secret, authorization, key...); tests feed known secret values through audit, auth (login, reset, OAuth errors), webhook (secret/signature) and fetch-error paths and assert none appear in captured log output | verification.md S6 | TODO | Include a URL with userinfo credentials in a fetch error. |
| 8.3 | Test the `github.webhook_request_created` / `github.webhook_request_status_updated` lines carry the request's `request_id` (valid signed webhook through `HandleGitHubIssueWebhook`, DB-backed) | 7.2 review | TODO | |
| 8.4 | End-to-end production-mode test: build and run `ramble server` with `ENV=production` against a testcontainers Postgres, make requests (200, 404, a failing query or slow path, a webhook), stop it, assert every stdout line is one JSON object with time/level/msg (no ANSI) and stderr is empty | verification.md S1 | TODO | Run last so it covers 8.x changes. |
| 8.5 | Webhook-triggered version fetches log with the request's `request_id`: `fetchVersionContent` logs through `logger.FromContextOr(ctx, ...)` and webhook goroutines pass a context carrying the request logger (not cancelled with the request) | Phase 7 notes | TODO | No `*fiber.Ctx` in goroutines. |
| 8.6 | SMTP errors never carry the recipient address: sanitise errors returned by the email sender (e.g. replace the address, or keep only the SMTP code) and test with a fake SMTP server whose replies echo the recipient | Phase 7 notes | DONE (2026-10-11) | `redactError` on both send functions: addresses (regex), link, its token, recipient and sender literals -> `[redacted]`; SMTP codes kept; no Unwrap (would expose the raw reply). STARTTLS fake-server test drives the real send code (`smtpRootCAs` seam). Advisor MINOR; orchestrator added token/address literals + token-only row. |
| 8.7 | Small standard gaps: `ramble server` flag-parse errors log JSON (`server.flag_error`), and `database` startup failures log at ERROR then exit 1 instead of `Fatal` (level FATAL is outside the standard's four levels); tests for both | verification.md gap 7 | ACTIVE | Keep non-server CLI commands' plain-text errors. |
