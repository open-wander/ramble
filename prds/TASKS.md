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

## Phase 10: CI and runtime maintenance

Source: review after v0.5.20 (2026-10-14). GitHub's `ubuntu-latest` moves to Ubuntu 26 from
2026-10-19 and Node 20 actions are deprecated; build tools and the runtime user were never pinned
or hardened. Decision (user): pin GitHub-hosted jobs to `ubuntu-26.04` now.

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 10.1 | Bring GitHub Actions current: docker/login, setup-buildx, setup-qemu, build-push, attest-build-provenance, goreleaser-action to current majors (Node 24); re-check checkout, setup-go, codeql; pin GitHub-hosted jobs to `ubuntu-26.04` | review 2026-10-14 | ACTIVE | Release workflow is proven by the next release. |
| 10.2 | Pin build tools in the Makefile: `SWAG_VERSION`, `TAILWIND_VERSION`; CI uses `make swagger`; Dockerfile takes them as build args; CI check fails if Dockerfile and Makefile pins differ | review 2026-10-14 | TODO | Same pattern as GOSEC_VERSION / STATICCHECK_VERSION. |
| 10.3 | Run the container as non-root: fixed unprivileged user, `/app` instead of `/root`, ownership right; confirm the server writes nothing to disk and the Nomad job needs no root; verify image (`id -u` != 0, server starts) | review 2026-10-14 | DONE (2026-10-14) | Runtime stage: user `ramble` uid/gid 10001 (no shell, no home), `WORKDIR /app`, files root-owned and read-only to the app, `USER 10001:10001`. Nomad job needed no change. Verified: image builds, `id`, writes denied, server starts and serves / /login /docs against a throwaway Postgres. `TestDockerfileRuntimeRunsNonRoot` guard. Advisor PASS. Ships with the next release. |

## Phase 11: zerolog to log/slog

Source: user request (2026-10-14), to match nectvia and havencast. Output contract unchanged:
one JSON object per line, `time`, upper-case `level`, `msg`, ramble's event keys (`http.request`,
`db.slow_query`, `db.query_error`, ...), `request_id` on every line of a request including SQL.
Decisions (user): keep ramble's event keys (nectvia's GORM logger uses `gorm.*` keys and drops
request_id - ramble keeps its stronger behaviour); JSON in every environment (development
included) for one consistent format - no console/text handler.

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 11.1 | Logger package on slog: JSON handler in every environment (drop the dev console writer), request-scoped logger via context (FromContext/WithContext), stdlib log bridged, RequestLogger middleware and panic recovery ported | user request | TODO | Foundation. |
| 11.2 | GORM adapter on slog: db.slow_query / db.query_error with request_id, `$n` placeholders, safeArg rendering | user request | TODO | (needs: 11.1) |
| 11.3 | Convert server call sites (~90 in 12 files); goroutines take `*slog.Logger` | user request | TODO | (needs: 11.1) |
| 11.4 | Port guards (event keys, no direct stdout, no stdlib log, no fatal/panic) and the ~15 log-capturing test files | user request | TODO | (needs: 11.2, 11.3) |
| 11.5 | Remove zerolog from go.mod; e2e production test unchanged and green; release; verify in Loki; update CLAUDE.md, infra doc, prod-logs skill | user request | TODO | (needs: 11.4) |
