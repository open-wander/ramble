# Research Note: Investigate Deferred Observability / Architecture Work

**Status:** RESEARCH — not yet scoped into a PRD. Investigate, then decide what (if any)
becomes a milestone.
**Created:** 2026-05-31
**Source:** Deferred "Future Requirements" from the shipped v0.5.0 milestone, found in
`.planning/milestones/v0.5.0-REQUIREMENTS.md` (the gsd planning system, separate from
norman). These were explicitly scoped OUT of v0.5.0 — they are candidate future work,
not owed/committed work.

## Why this note exists

After the security-bugfix PRD shipped, an audit confirmed there are no outstanding
norman PRDs and the gsd `.planning/` system is at "milestone complete, nothing pending."
The only candidate future work is the deferred list below. This note parks it so it is
not lost, and records what to investigate before committing to any of it.

## Deferred items to investigate

### Advanced Observability
- **OBS-01** — Background job status tracking with lifecycle visibility.
  - Builds on the existing `FetchStatus`/`FetchError`/timestamp fields shipped in v0.5.0.
  - Investigate: is the current per-version FetchStatus + `/admin/errors` dashboard
    already "good enough", or is a dedicated job-status view/table wanted?
- **OBS-02** — Prometheus `/metrics` endpoint (counters/histograms for background ops).
  - CONFIRMED ABSENT today: no `/metrics`, no `/healthz`/`/readyz` route exists
    (grep of internal/ found none; go.mod has otel indirect deps only).
  - Highest-value, lowest-risk pick for an operator-focused tool. Pairs naturally with
    a `/healthz` + `/readyz` liveness/readiness endpoint (not in the original list but
    obviously adjacent — investigate adding it alongside).
- **OBS-03** — Webhook replay capability from the admin panel.
  - Investigate: replay needs the original payload retained; check whether webhook
    payloads are currently persisted or only their error/timestamp.
- **OBS-04** — Notification / alerting when error thresholds are exceeded.
  - Investigate delivery channel (email is already wired via internal/services/email;
    Slack/webhook out?) and where thresholds would be configured.

### Architecture (only "when scale justifies it")
- **ARCH-01** — Full job queue system (River / Asynq).
  - v0.5.0 rationale for deferring: overkill for current scale (<100 ops/hour);
    logging + DB tracking deemed sufficient. Re-evaluate only if op volume grows.
- **ARCH-02** — Worker pool with configurable concurrency limits.
  - Same "premature optimization for current traffic" caveat. Tie any decision to
    real measured load (which OBS-02 metrics would actually provide — so OBS-02 is a
    sensible prerequisite for justifying ARCH-01/02).

## Related known tech debt (from v0.5.0 milestone summary)
- `fetchVersionContent` still lives in `internal/handlers/background.go` (handler layer)
  rather than the resource service, deferred due to deep HTTP/errgroup dependencies.
  Note: the security-bugfix pass (task 4.1) already routed its failure path through
  `ResourceService.RecordFetchFailed`, so a fuller extraction is now slightly easier.
  Investigate whether a clean extraction is worth it or should stay put.

## Suggested investigation order
1. OBS-02 (`/metrics`) + `/healthz`/`/readyz` — concrete gap, clear value, self-contained.
2. OBS-01 — decide if existing FetchStatus tooling already covers it.
3. OBS-03 / OBS-04 — depend on payload retention + alerting channel decisions.
4. ARCH-01 / ARCH-02 — defer until OBS-02 metrics show load that justifies them.

## Next step
When ready to act on any of these, run `norman prd` to turn the chosen item(s) into a
proper PRD with acceptance criteria, then `norman import`. Do NOT treat this note as an
approved backlog — it is a parking spot for investigation.
