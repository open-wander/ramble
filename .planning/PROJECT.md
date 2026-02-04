# Ramble Security Hardening

## What This Is

A security hardening pass for Ramble, the Nomad pack/job registry. Addresses security concerns identified during codebase analysis, plus adds comprehensive test coverage for security-critical paths.

## Core Value

External inputs are validated and security-critical code paths are tested. No silent failures in authentication or authorization flows.

## Requirements

### Validated

- User authentication (local + OAuth via GitHub/GitLab) — existing
- Session management with encrypted cookies — existing
- CSRF protection on forms — existing
- Bcrypt password hashing — existing
- Webhook signature validation (HMAC) — existing
- Rate limiting on auth endpoints — existing

### Active

- [ ] External URL validation with configurable allowlist
- [ ] Remove webhook secret query parameter support
- [ ] Configurable system user for GitHub sync
- [ ] Shorten password reset token expiration to 1 hour
- [ ] Document OAuth token storage and rotation policy
- [ ] Test coverage for webhook signature validation edge cases
- [ ] Test coverage for OAuth account linking edge cases

### Out of Scope

- Performance improvements — separate milestone
- Tech debt cleanup — separate milestone
- UI/UX changes — not security-related
- New features — hardening existing code only

## Context

**Trigger:** Proactive security review based on codebase mapping analysis.

**Codebase state:** Existing Go/Fiber application with established patterns. Security foundations are solid (bcrypt, CSRF, rate limiting) but gaps exist in input validation and test coverage.

**Key findings from analysis:**
- Repository URLs from users passed to HTTP client without validation (SSRF risk)
- Webhook secrets accepted via query parameter (exposure in logs/browser history)
- GitHub sync hardcodes UserID=1 as system user
- Password reset tokens valid for 24 hours (industry standard is 1 hour)
- Security-critical code paths lack edge case tests

## Constraints

- **Tech stack**: Go 1.25, Fiber, PostgreSQL, GORM — no framework changes
- **Config approach**: Environment variables for new settings (ALLOWED_GIT_HOSTS)
- **Backwards compatibility**: Existing webhooks using HMAC continue working; query param method deprecated then removed
- **Test infrastructure**: Use existing testcontainers setup

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Environment variable for allowed hosts | Simple, secure, deploy-friendly | — Pending |
| 1 hour token expiration | Industry standard, balances security/UX | — Pending |
| GitHub + GitLab default allowlist | Most common hosts, self-hosted via env var | — Pending |

---
*Last updated: 2026-02-04 after initialization*
