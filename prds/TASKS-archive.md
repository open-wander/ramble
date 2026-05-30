# Tasks Archive

Collapsed, fully-completed phases moved out of TASKS.md to keep it lean.

## Phase 2: Fetch Hardening — 2 tasks completed 2026-05-30

| # | Task | PRD | Status | Notes |
|---|------|-----|--------|-------|
| 2.1 | US-002: Limit remote repository content size — shared size-limited download helper, propagate oversized errors to fetch status (FR-3, FR-4) | [prd-security-bugfix.md](active/prd-security-bugfix.md) | DONE (2026-05-30) | `MaxRemoteFileBytes = 1 << 20`; `io.LimitReader(r, max+1)` + explicit length check in downloadFile; oversized required content (metadata, variables) marks fetch failed; review PASS |
| 2.2 | US-003: Tighten SSRF host allowlisting — exact host or subdomain-of-allowed-host only, remove reverse suffix matching (FR-5) | [prd-security-bugfix.md](active/prd-security-bugfix.md) | DONE (2026-05-30) | Removed reverse-suffix branch in `isHostAllowed`; lowercase + trailing-dot normalization; parent-domain bypass tests added; review PASS |
