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

## Phase 8: Logging test hardening — 11 tasks completed 2026-10-11 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 9: Webhook CSRF bug and code debt — 13 tasks completed 2026-10-11 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 10: CI and runtime maintenance — 3 tasks completed 2026-10-14 (see [TASKS-archive.md](TASKS-archive.md))

## Phase 11: zerolog to log/slog

Source: user request (2026-10-14), to match nectvia and havencast. Output contract unchanged:
one JSON object per line, `time`, upper-case `level`, `msg`, ramble's event keys (`http.request`,
`db.slow_query`, `db.query_error`, ...), `request_id` on every line of a request including SQL.
Decisions (user): keep ramble's event keys (nectvia's GORM logger uses `gorm.*` keys and drops
request_id - ramble keeps its stronger behaviour); JSON in every environment (development
included) for one consistent format - no console/text handler.

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 11.1 | Logger package on slog: JSON handler in every environment (drop the dev console writer), request-scoped logger via context, RequestLogger attaches both loggers | user request | DONE | `logger.S`, `SFromContext`/`SFromContextOr`/`SWithContext` (temporary names). zerolog dev output now JSON too. stdlib bridge and panic recovery moved to 11.3/11.5. |
| 11.2 | GORM adapter on slog: db.slow_query / db.query_error with request_id, `$n` placeholders, safeArg rendering | user request | DONE | (needs: 11.1) Sequential with 11.3: they share handler test helpers. Each stage ports the log-capturing tests its call sites break. |
| 11.3 | Convert server call sites (~90 in 12 files) incl. http.request and panic recovery; goroutines take `*slog.Logger`; RequestLogger takes one `*slog.Logger` | user request | DONE | (needs: 11.1) |
| 11.4 | Port guards (event keys, no direct stdout, no stdlib log, no fatal/panic) and the ~15 log-capturing test files | user request | TODO | (needs: 11.2, 11.3) |
| 11.5 | Rename S/SFromContext/SFromContextOr/SWithContext to Log/FromContext/FromContextOr/WithContext; stdlib log bridged to slog (msg stdlib.log); delete WithUser/WithRequest (unused); remove zerolog from go.mod; e2e production test unchanged and green; release; verify in Loki; update CLAUDE.md, infra doc, prod-logs skill | user request | TODO | (needs: 11.4) |
