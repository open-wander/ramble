# PRD: Security Bugfix

## 1. Introduction / Overview

This PRD defines a focused security bugfix pass for Ramble. The goal is to address concrete issues found during the security review without expanding into broad security redesign work.

The work protects organization namespaces from unauthorized writes, prevents resource fetches from consuming unbounded memory or storage, tightens outbound host validation, clarifies organization member permissions for resource management, and reduces account takeover risk from OAuth email-based account linking.

## 2. Goals

- Prevent authenticated users from creating resources in organizations they do not belong to.
- Enforce explicit size limits for remote repository files fetched and stored by the application.
- Ensure SSRF host allowlisting only permits exact allowed hosts and valid subdomains of allowed hosts.
- Make organization resource management permissions consistent and intentional.
- Ensure OAuth account linking only occurs when the provider email can be trusted or the user is already authenticated.
- Add regression tests for each fixed vulnerability.

## 3. User Stories

### US-001: Block Unauthorized Organization Resource Creation

**Description:** As an organization owner, I want only authorized organization members to create resources in my organization so that outsiders cannot publish under my namespace.

**Acceptance Criteria:**
- [ ] A logged-in non-member submitting `owner=org:<id>` receives `403 Forbidden` or a clear authorization error.
- [ ] A logged-in member of the target organization can create a resource in that organization.
- [ ] A logged-in user can still create resources in their own personal namespace.
- [ ] Tests cover non-member, member, and personal resource creation paths.

### US-002: Limit Remote Repository Content Size

**Description:** As an operator, I want repository file downloads to have enforced byte limits so that malicious or accidental large files cannot exhaust memory or fill the database.

**Acceptance Criteria:**
- [ ] `downloadFile` rejects any remote file larger than a configured maximum size.
- [ ] README, LICENSE, metadata, variables, and job file fetches all use the same size-limited read path.
- [ ] Oversized downloads return an error and do not store partial content as successful fetch content.
- [ ] Fetch status is recorded as failed when required content exceeds the limit.
- [ ] Tests cover an oversized response body and a response exactly at the maximum accepted size.

### US-003: Tighten SSRF Host Allowlisting

**Description:** As an operator, I want outbound fetches restricted to intended hosts so that allowlist logic cannot accidentally permit parent domains or unrelated hosts.

**Acceptance Criteria:**
- [ ] Host validation permits exact matches in `AllowedHosts`.
- [ ] Host validation permits subdomains of allowed hosts, for example `api.github.com` when `github.com` is allowed.
- [ ] Host validation does not permit parent domains of allowed hosts, for example `com` or `githubusercontent.com` when `raw.githubusercontent.com` is allowed.
- [ ] Existing SSRF tests pass and new tests cover the parent-domain bypass case.

### US-004: Clarify Organization Resource Management Roles

**Description:** As an organization owner, I want resource edit and webhook management permissions to follow explicit role rules so that membership does not grant unintended control.

**Acceptance Criteria:**
- [ ] The intended permission model is encoded in code and tests.
- [ ] If only owners should manage organization resources, non-owner members cannot edit resources, add versions, retry fetches, or reset webhook secrets.
- [ ] If members should manage organization resources, this behavior is documented and deletion remains owner-only.
- [ ] Tests cover owner, member, and non-member access for edit, version, retry, webhook reset, and delete operations.

### US-005: Harden OAuth Account Linking

**Description:** As a user with a local account, I want OAuth sign-in to link to my account only when the email identity is trustworthy so that another account cannot claim my email through an OAuth provider edge case.

**Acceptance Criteria:**
- [ ] OAuth callback does not silently link to an existing account unless the provider email is verified or the user is already authenticated and initiating an explicit link flow.
- [ ] If verification status is unavailable from the OAuth library/provider, the callback creates a distinct account or rejects linking with a clear message.
- [ ] Existing OAuth login for already-linked provider IDs still works.
- [ ] Tests cover existing provider login, safe account linking, and rejected untrusted email linking.

## 4. Functional Requirements

- FR-1: Resource creation must validate target organization membership before persisting a resource with `OrganizationID`.
- FR-2: Resource creation must reject invalid or nonexistent organization IDs instead of treating them as valid ownership targets.
- FR-3: Remote file download code must use a shared helper that enforces a maximum response size before converting the body to a string.
- FR-4: Oversized remote file errors must propagate to background fetch status for required resource content.
- FR-5: SSRF host matching must remove reverse suffix matching and use only exact host or subdomain-of-allowed-host logic.
- FR-6: Organization resource authorization must be centralized or consistently reused by resource edit, version, retry, webhook reset, and delete operations.
- FR-7: Organization membership role checks must be covered by handler or service tests.
- FR-8: OAuth account linking must require a trusted email signal or an explicit authenticated linking flow.
- FR-9: All bug fixes must preserve existing successful flows covered by current tests.
- FR-10: `go test ./...` must pass.

## 5. Non-Goals

- Replacing the Fiber session store or changing the entire authentication architecture.
- Adding a new admin role management UI beyond what is necessary to clarify permissions.
- Implementing a full OAuth account-linking settings page unless required by the chosen fix.
- Supporting arbitrary self-hosted Git providers beyond the existing allowlist behavior.
- Adding a complete WAF, request body gateway, or deployment-level rate limiting system.
- Rewriting the resource fetch pipeline.

## 6. Technical Considerations

- The likely primary files are:
  - `internal/handlers/resource_crud.go`
  - `internal/services/resource/service.go`
  - `internal/handlers/utils.go`
  - `internal/security/ssrf.go`
  - `internal/handlers/auth.go`
- Prefer moving authorization decisions into the resource service when possible, then keeping handlers thin.
- Choose a conservative default remote file size limit. A suggested starting point is 1 MiB for README/LICENSE/metadata/variables/job files unless project requirements justify a larger limit.
- Use `io.LimitReader` with `maxBytes + 1` so the implementation can distinguish an exactly allowed file from an oversized file.
- Review tests already present under `internal/handlers/*security*_test.go`, `internal/security/*test.go`, and `internal/services/resource/service_test.go` before adding new test files.
- If OAuth provider email verification is not exposed by the current library, reject automatic email-based linking and require an explicit future linking flow.

## 7. Success Metrics

- All acceptance criteria pass in automated tests.
- `go test ./...` passes.
- A non-member cannot create a resource under another organization by forging `owner=org:<id>`.
- A large remote file cannot be fully loaded into memory or stored by the resource fetch path.
- SSRF host tests demonstrate that parent-domain allowlist bypasses are rejected.
- Permission tests document the intended org role behavior.

## 8. Open Questions

- Should organization members be allowed to edit organization resources, or should that be owner-only?
- What exact maximum remote file size should be enforced for fetched repository files?
- Does the current Goth provider data expose provider-verified email status for GitHub and GitLab?
- Should rejected OAuth email-linking attempts create a new account automatically or return a clear error asking the user to log in first?
