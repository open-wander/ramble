# CLAUDE.md

## Reading production logs

Every nomad-prod task's stdout/stderr is in Loki on the home Pi cluster (about
two weeks), readable from any tailnet device with no credential. Agents: use
the `prod-logs` skill (`loki.sh '<logql>' [since] [limit]`). Raw API and the
shared rules: `TydeWhatMay/infra` `platform/alloy/README.md`.

- Selector: `{container="ramble"}` (`deploy/nomad/ramble.nomad.hcl`).
- Format: zerolog JSON on stdout since v0.5.17 (2026-10-10 21:00 UTC). Before
  that it was text: Fiber access lines and GORM SQL traces with ANSI colours.
  Since v0.5.18 each start logs one `server.started` line (`addr`, `version`)
  instead of Fiber's text banner.
- Requests are `msg="http.request"` with `request_id`, `method`, `path`,
  `status`, `duration_ms`, `client_ip`. Database: `db.slow_query` (WARN,
  over 200 ms) and `db.query_error` (ERROR); fast queries are not logged.
  Since v0.5.18 SQL, handler and webhook lines carry the request's
  `request_id`; panics log `http.panic`; audit events are `audit.recorded`.
- `level` is not a label; filter after `| json`. Add `| __error__=""` to any
  metric query: the pre-v0.5.17 lines do not parse, and Loki
  fails the whole query on a parse error.

```
{container="ramble"} | json | level=~"ERROR|WARN" | __error__=""
{container="ramble"} | json | msg="http.request" | status >= 500
{container="ramble"} | json | msg=~"db\\.(slow_query|query_error)"
{container="ramble"} |= "<request-id>"
{container="ramble", stream="stderr"}                                          # panics
sum by (msg, level) (count_over_time({container="ramble"} | json | __error__="" [1h]))   # metric (-m)
```

Edge view of the same request (Traefik, JSON):
`{job="traefik-access"} |~ "^\\{" | json | RequestHost="ramble.openwander.org"`.

Logs carry IP addresses and usernames: quote only the lines you need, never
bulk output in commits, issues or PRs. Never use a Nomad token to read logs.
