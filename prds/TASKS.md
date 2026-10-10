# Tasks

All phases complete (prd-security-bugfix). Full history in [TASKS-archive.md](TASKS-archive.md);
verification in [verification.md](verification.md). PRD: [done/prd-security-bugfix.md](done/prd-security-bugfix.md).

## Phase 1: Authorization — 2 tasks completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 2: Fetch Hardening — 2 tasks completed 2026-05-30 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 3: Auth Hardening — 2 tasks completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 4: Cleanup — 1 task completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))
## Phase 5: Review Follow-ups — 5 tasks completed 2026-05-31 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 6: Structured logging

Source: infra `docs/prod-logging.md` item 2 (TydeWhatMay/infra, surveyed 2026-10-09). Prod
logged ~37,000 text lines/day: GORM printed every query with ANSI colours, Fiber's text
access log, stdlib `log.Printf`. Standard: one JSON object per line, `time`/`level`/`msg`,
`msg` a stable event key, no SQL at INFO, no secrets or query strings.

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 6.1 | JSON logging: GORM through a zerolog adapter (silent <200ms, `db.slow_query` WARN, `db.query_error` ERROR, `$n` placeholders only), `http.request` middleware replacing fiberlogger, all server-side `log.*` calls to event keys, stdlib `log` redirected, upper-case `level` + `msg` | infra `docs/prod-logging.md` | DONE on branch (2026-10-09) | Branch `feat/structured-logging` (bb1c408..c59d669), not merged. Tests/vet/gosec clean, re-run independently. Mutation-tested. |
| 6.2 | Merge 6.1, release, deploy; then verify in Loki that `{container="ramble"} !~ "^\\{"` returns only panic traces, and that line volume dropped | infra `docs/prod-logging.md` | DONE | Released v0.5.17, deployed 2026-10-10 21:00. Loki: only the Fiber startup banner is non-JSON (6.6); line volume ~1,390/h to ~220/h. |
| 6.3 | Thread the request context into DB calls (`database.DB.WithContext(c.UserContext())` or a helper) so `db.slow_query`/`db.query_error` lines carry `request_id` | infra `docs/prod-logging.md` | TODO | Handlers call `database.DB` directly, so SQL lines fall back to the global logger. Wide but mechanical; add a test that a slow query inside a request logs the request's `request_id`. (needs: 6.1) |
| 6.4 | Convert remaining sentence-style zerolog messages to stable event keys (`github_sync.go`, `background.go`, `audit.go`, e.g. "webhook validation failed" -> `webhook.validation_failed`), variable data in fields | infra `docs/prod-logging.md` | TODO | Already JSON; this is consistency. Grep `Msg("` / `Msgf(` for messages with spaces. (needs: 6.1) |
| 6.5 | Decide on static-asset access lines: log `/public/*` at DEBUG or skip them | infra `docs/prod-logging.md` | TODO | Optional. Measure share of `http.request` lines with `path=~"/public/.*"` after 6.2 first. (needs: 6.2) |
| 6.6 | Disable Fiber's startup banner (`fiber.Config{DisableStartupMessage: true}`) and log one `server.started` JSON line with addr and version instead | infra `docs/prod-logging.md` | TODO | 10 text lines per start in prod, seen after 6.2. |

