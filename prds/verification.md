# Verification and security review: programmatic management (#65)

Date: 2026-11-27
Scope: ramble main at c757ae0..be86320 (v0.6.0) and terraform-provider-ramble v0.1.0, against prds/active/prd-programmatic-management.md. Run after the release, at the operator's call.

Method:
- norman verify, single rigor: one verifier for each of US-001..US-015, FR-5, FR-6, FR-7 and FR-8.
- Trimmed norman secure, Track A: 8 security requirements, each checked with 3 lenses (exists, tested, edge cases). Every requirement that passed was then put through a mutation sweep.
- 14 further security requirements went over the cap of 8 and were not lens-verified. Most of them are covered by the single-rigor verify.
- Trimmed norman secure, Track B: 5 threat-led areas (tokens, resource writes, organisations, CLI, provider and CI). Every finding went to 3 skeptics (reproduce, reachable, mitigated).
- A completeness critic ran over all of the above.
- 99 agents in total.

All 16 Track B findings were confirmed by at least 2 of 3 skeptics. In this mode the skeptics are known to be too agreeable, so a confirmation here is not proof. The failing test written in each fix task is the proof.

## Confirmed findings (Track B), most severe first

| Area | Severity | Finding | File | Fix task |
|---|---|---|---|---|
| cli | HIGH | Path traversal in the pack cache: a version string from the registry writes files anywhere in the victim's home (e.g. ~/.bashrc, ~/.ssh/authorized_keys) | `internal/pack/cache.go:43` | 18.1 |
| provider-ci | HIGH | Any write collaborator can deploy arbitrary code to production and steal the Nomad token and release App key: ramble release.yml is workflow_dispatch on any ref, and all secrets are repository-level with no environment gate | `ramble/.github/workflows/release.yml:3` | 18.3 |
| provider-ci | HIGH | Provider: any write collaborator can produce a GPG-signed Terraform provider release (direct v* tag push, or a self-merged regen PR), and can steal the GPG signing key | `terraform-provider-ramble/.github/workflows/release.yml:6` | 18.3 |
| provider-ci | HIGH | Self-hosted runner that can reach the Nomad API is usable by pull requests on a public repository | `ramble/.github/workflows/ci.yml:6` | 18.3 |
| resources | HIGH | Web edit of an org-owned resource silently moves it out of the organisation into the creator's personal namespace | `ramble/views/edit_resource.html:99` | 18.2 |
| orgs | MEDIUM | An organisation can take over the namespace of an admin-deleted user | `internal/services/org/org.go:38` | 18.6 |
| resources | MEDIUM | Unbounded tag list on POST/PATCH /v1/resources drives per-tag queries inside a transaction (DoS) and floods the global tags table | `ramble/internal/services/resource/create.go:86` | 18.5 |
| tokens | MEDIUM | API tokens survive password reset; a stolen session can be turned into a 365-day write token that account recovery does not remove | `internal/services/apitoken/service.go:202` | 18.4 |
| cli | LOW | RAMBLE_TOKEN is not tied to a registry: it is sent to whichever registry a command targets | `internal/cli/config/config.go:176` | 18.7 |
| cli | LOW | Terminal escape injection: descriptions and versions set by other users are printed raw by pack/job list, info, search and run | `internal/cli/cmd/pack_info.go:72` | 18.7 |
| orgs | LOW | A member whose account an admin deleted breaks the organisation's pages and cannot be removed through the API | `views/profile.html:73` | 18.7 |
| orgs | LOW | Any owner can force a user into an organisation, and the user cannot leave | `internal/handlers/api_org_members.go:108` | 18.7 |
| orgs | LOW | Some owner checks run outside the organisation row lock (time-of-check to time-of-use gap) | `internal/handlers/api_org_members.go:172` | 18.7 |
| resources | LOW | Web resource edit skips name/type validation and uses a case-sensitive collision check | `ramble/internal/services/resource/service.go:210` | 18.7 |
| tokens | LOW | Token revocation sits behind RequireVerifiedEmail, so a user whose verification is cleared cannot revoke live tokens | `internal/server/server.go:352` | 18.7 |
| tokens | LOW | Write rate limit can be multiplied by issuing many tokens, and failed or read token requests have no limit | `internal/handlers/api_auth.go:161` | 18.7 |

The scenarios and the skeptics' reasons are in the workflow journal for run wf_79a8b810-abf, which is session-local. Each fix task restates its own reproduction.

## Requirement verdicts

| Requirement | Verdict | Notes |
|---|---|---|
| US-001 | PASS |  |
| US-002 | PASS |  |
| US-003 | PASS |  |
| US-004 | PASS |  |
| US-005 | PASS |  |
| US-006 | PASS |  |
| US-007 | PASS |  |
| US-008 | PASS |  |
| US-009 | PARTIAL | The audit action is `resource.reset_webhook`, the same as the web path (FR-8). The PRD says `resource.webhook_reset`, so the PRD text is wrong. |
| US-010 | PASS |  |
| US-011 | PASS |  |
| US-012 | PASS |  |
| US-013 | PASS |  |
| US-014 | PARTIAL | Acceptance tests build Ramble from source rather than using the ghcr image. Namespace replacement is covered only by a unit test. |
| US-015 | PARTIAL | Deviations: the spec is fetched from a raw URL at the tag, not a release asset, and tests build from source rather than the image. notify-provider has not been proven yet (it failed at v0.6.0 because the variable was missing). No test checks the scope of the App token; mutation survived. |
| FR-5 | PASS |  |
| FR-6 | PARTIAL | The global session middleware returns `{"error":"Session expired"}` on /v1 for expired cookie sessions sending Accept: application/json. Fiber 413 maps to 500 internal_error. |
| FR-7 | PASS |  |
| FR-8 | PASS |  |

Track A (lenses, then mutation):

| Requirement | Verdict | Mutation |
|---|---|---|
| 65:US-001 | PASS | HashToken returns plain (service.go:103), so plaintext lands in api_tokens.hash: TestPlaintextNeverStored FAIL (service_security_test.go:39). GAP: adding "token", plain to the token.created log (service.go:177) left all  |
| 65:US-003-auth | PASS | Dropped the RevokedAt check in Authenticate (service.go:202): TestAuthenticate_Invalid FAIL (service_test.go:319) and TestRequireAPIToken_Rejections_Identical401/revoked FAIL. Separately, a distinct message for an unknow |
| 65:US-003-org | PASS | tokenAllowsOrg returns true for restricted tokens (api_auth.go:~126): 11 tests FAIL incl. TestTokenAllowsOrg, ...ThroughRoute, CreateOrg/OrgAPI/DeleteOrg/OrgMembers/CreateResource/GetResource/WriteResource OrgRestricted, |
| 65:US-003-admin | PASS | Removed `u.IsAdmin = false` in RequireAPIToken (api_auth.go:88): TestRequireAPIToken_SetsLocals FAIL (api_auth_test.go:172). TestAdminRoutesIgnoreBearerToken (server) stayed green under this mutation; it guards admin rou |
| 65:US-003-csrf | PASS | isCookielessAPIRequest drops the session-cookie check (server.go:626), so any /v1 request skips CSRF: TestCSRF_BearerExemptionThroughStack FAIL (api_csrf_security_test.go:86,101) and TestIsCookielessAPIRequest FAIL (serv |
| 65:US-008 | PASS | DeleteResource calls AuthorizeResourceAction(id, userID, false) (resource/service.go:251), so org members can delete: handlers TestResourceAPI_OrgRoles and TestDeleteResource_OrgMember_Forbidden FAIL. Resource-package te |
| 65:US-011 | PASS | keepsAnOwner no longer returns ErrLastOwner (org.go:405): TestRemoveMember, TestSetMemberRole, TestLastOwner_IgnoresDeletedUsers, TestPutOrgMemberAPI and TestRemoveOrgMemberAPI FAIL. Removing RequireOwner from SetMemberR |
| 65:US-015 | PARTIAL | In notify-provider (release.yml:229) I removed `repositories: terraform-provider-ramble`, so the token covers every repo in the installation, and added `permission-administration: write`. actionlint on a reduced copy of  |

## Critic gaps

- UNCOVERED GET /v1/registries and GET /v1/registries/search (internal/handlers/nomad_pack.go:232-280, routes internal/server/server.go:422-423): None of the sweeps examined these public routes. The tokens lens says it skipped the public list and search handlers, and the resources lens did not name them. ListUserRegistriesAPI (nomad_pack.go:234-240) and SearchRegistriesAPI (nomad_pack.go:266-273) query reqDB(c).Table("nomad_resources") with LEFT JOINs on users and organizations. They use no Model, so GORM's soft-delete scope does not apply, and neither query has a deleted_at filter (NomadResource, User and Organization all embed gorm.Model, internal/models). As a result, after DELETE /v1/resources/{id} (US-008) or DELETE /v1/orgs/{name} (US-010), the deleted namespace still appears in the public registry list and search. The same appl

## Out of scope or capped

- 14 security requirements over the cap of 8 were not lens-verified.
- GitHub settings the auditors could not read, because the API returned 403: fork-PR approval policy, runner groups, whether the runner is ephemeral, secret names, App installation scope.
- Provider acceptance tests: one verifier ran them against a local Ramble (23 passing). Nothing was run against production.
- The previous report (Phase 6 logging, 2026-10-11) is in git history.
