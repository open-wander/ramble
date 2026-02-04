# Codebase Concerns

**Analysis Date:** 2026-02-04

## Tech Debt

**Goroutine Error Handling - Background File Fetching:**
- Issue: Multiple goroutines fire background tasks to fetch and update resource content without error recovery or panic handling
- Files: `internal/handlers/resource.go` (lines 298-315, 448-465, 608-634, 643-661)
- Impact: Failed goroutines silently fail without logging. If downloadFile() errors occur, database updates never happen, leaving stale content. No way to retry or alert admin.
- Fix approach: Add panic recovery, structured error logging, and consider using a task queue (background job system) instead of fire-and-forget goroutines

**Unhandled Error Ignoring:**
- Issue: Consistent pattern of ignoring errors with blank identifiers in goroutines
- Files: `internal/handlers/resource.go` (lines 299, 449, 609, 644: `readme, _ := downloadFile(...)`)
- Impact: Silent failures when fetching README files, incomplete resource metadata
- Fix approach: At minimum, log errors inside goroutines; better solution is task queue with retry logic

**Session Error Silently Swallowed:**
- Issue: SetFlash() silently returns if session.Get() fails, hiding authentication/session problems
- Files: `internal/handlers/utils.go` (lines 37-46)
- Impact: User never sees error messages if session service has issues; developers unaware of session problems
- Fix approach: Log session errors or propagate to middleware for explicit error handling

## Known Bugs

**Username Collision on OAuth Signup:**
- Symptoms: Comment in code notes "Simple username collision check or suffix" but no actual collision handling implemented
- Files: `internal/handlers/auth.go` (lines 99-103)
- Trigger: Multiple users authenticating with OAuth using same nickname/name
- Workaround: None. Second user gets same username, causes database constraint violation.
- Fix approach: Implement automatic suffix appending (e.g., `username2`, `username3`) before user creation

**Account Lockout After Failed Login:**
- Symptoms: User account locked for 15 minutes, but no notification sent to user. Message only shown if they try login again.
- Files: `internal/handlers/auth.go` (lines 182-189)
- Trigger: 5 failed login attempts
- Workaround: Wait 15 minutes or have admin manually unlock via `LockedUntil` field
- Fix approach: Send email notification when account is locked

**Resource Webhook Secret Generation Can Panic:**
- Symptoms: Application crashes if crypto/rand fails
- Files: `internal/handlers/resource.go` (lines 49-56 in generateWebhookSecret)
- Trigger: Very rare - crypto/rand can only fail if system is severely broken, but panic means no graceful degradation
- Workaround: System restart required; no user action possible
- Fix approach: Return error instead of panic, propagate up to handler for user-friendly error response

## Security Considerations

**Timing Attack Prevention Incomplete:**
- Risk: While bcrypt comparison uses timing-safe comparison, email enumeration via password reset still possible
- Files: `internal/handlers/auth.go` (lines 370-378)
- Current mitigation: Same success message shown whether email exists or not ("If an account exists...")
- Recommendations: This is actually well-mitigated. No changes needed.

**Password Reset Token Storage:**
- Risk: Tokens hashed in database (good), but plain token must be included in email. If email is leaked, token can be used.
- Files: `internal/handlers/auth.go` (lines 282-318 signup, 385-413 password reset)
- Current mitigation: 24-hour token expiration, SHA256 hashing
- Recommendations: Consider shorter expiration (1 hour for reset), implement token rotation or one-time-use tokens

**OAuth Token Storage:**
- Risk: Access tokens stored encrypted but stored in plaintext in database (albeit encrypted column)
- Files: `internal/handlers/auth.go` (lines 93-94, 104, 122)
- Current mitigation: Encryption at rest using crypto module
- Recommendations: Verify encryption key rotation policy; consider external token storage; document token revocation approach

**GitHub Webhook Secret Exposure:**
- Risk: WebhookSecret stored in database but used in query string for legacy fallback method
- Files: `internal/handlers/resource.go` (lines 539-546 in HandleWebhook)
- Current mitigation: HMAC signature validation preferred (X-Hub-Signature-256), fallback uses constant-time comparison
- Recommendations: Remove query parameter secret support, only accept HMAC header; migrate existing resources

**Admin System User Hardcoded:**
- Risk: GitHub sync uses hardcoded UserID = 1 when creating requests from GitHub issues
- Files: `internal/handlers/github_sync.go` (lines 307, 436)
- Current mitigation: None
- Recommendations: Create system user account, store ID in config, or track original GitHub username in request

**Unvalidated External File Downloads:**
- Risk: RepositoryURL passed from user is trusted for file downloads without URL validation
- Files: `internal/handlers/resource.go` (lines 299-313), `internal/handlers/utils.go` (lines 101-144)
- Current mitigation: Basic GitHub/GitLab host check, but no validation of special URL schemes or redirects
- Recommendations: Add allowlist of git hosts, validate URL structure before HTTP requests, set request timeouts

## Performance Bottlenecks

**N+1 Queries on Resource Listings:**
- Problem: While Preload is used, multiple preloads on large result sets cause excessive queries
- Files: `internal/handlers/nomad_pack.go` (lines 481-483: Preload("User"), Preload("Organization"), Preload("Tags"), Preload("Versions"))
- Cause: Tags and Versions loaded for all resources even when not needed in response
- Improvement path: Use conditional Preload based on response format (HTML vs API), add pagination limits

**Background Goroutine Unbounded Concurrency:**
- Problem: Each resource creation spawns goroutine to fetch files. No limit on concurrent goroutines.
- Files: `internal/handlers/resource.go` (lines 298-315, 448-465, 608-634, 643-661)
- Cause: Many simultaneous resource creation requests cause many concurrent HTTP requests to GitHub/GitLab
- Improvement path: Implement worker pool with bounded concurrency (use channels or sync.Semaphore)

**Resource Search Uses ILIKE Without Index:**
- Problem: Search queries use ILIKE pattern matching which is slow on large datasets
- Files: `internal/handlers/nomad_pack.go` (lines 277, 352)
- Cause: Text search not optimized; ILIKE operators require full table scans
- Improvement path: Add PostgreSQL full-text search indexes or trigram indexes (CREATE INDEX using gin on name gin_trgm_ops)

**No Query Result Limits on Admin Pages:**
- Problem: Admin dashboard loads all resources/users/requests without pagination
- Files: `internal/handlers/admin.go` (lines 67, 186, 311)
- Cause: Some queries have Limit(5) but many critical pages fetch entire table
- Improvement path: Add pagination (LIMIT/OFFSET) and lazy-load options to admin dashboard

**Webhook Update Refreshes Entire Latest Version:**
- Problem: Webhook handler re-downloads and updates all files for latest version on every push
- Files: `internal/handlers/resource.go` (lines 641-661)
- Cause: No distinction between README-only push vs full resource update
- Improvement path: Parse webhook payload to determine what changed, only update necessary files

## Fragile Areas

**Resource Permission Checks Duplicated:**
- Files: `internal/handlers/resource.go` (lines 335-339 GetEditResource, 374-379 PostEditResource, 488-502 DeleteResource, 678-690 PostResetWebhookSecret)
- Why fragile: Same authorization logic (user owns resource OR user is org owner) repeated 4+ times. Any future change to permission logic requires updating all locations.
- Safe modification: Extract to `canModifyResource(userID, resource)` helper function
- Test coverage: Authorization tested in ResourceTest but single point of truth would reduce fragile duplication

**GitHub Sync Bidirectional Logic:**
- Files: `internal/handlers/github_sync.go` (lines 206-320 runBidirectionalSync)
- Why fragile: Complex state machine with multiple edge cases (newly created issues, externally changed status, app-changed status). Easy to introduce race conditions.
- Safe modification: Add comprehensive unit tests for each sync scenario, document edge cases, consider adding sync logging/debugging output
- Test coverage: Some coverage in github_sync_test.go but missing edge case tests

**Email Verification/Password Reset Token System:**
- Files: `internal/handlers/auth.go` (lines 282-318 signup verification, 385-413 password reset, 469-495 email verify)
- Why fragile: Same token pattern (generate, hash, store, compare) used 3 places with slight variations. Token format not validated, expiration check relies on database time.
- Safe modification: Create reusable Token type and validation function, add token format validation
- Test coverage: auth_test.go has tests but edge cases like expired token during verification not fully tested

**Goroutine Spawning in Handlers:**
- Files: `internal/handlers/resource.go` (lines 298-315, 448-465, 608-634, 643-661)
- Why fragile: No way to track goroutine completion, test goroutine behavior, or add timeout/context. Tests may pass while background task fails.
- Safe modification: Use context with timeout, implement WaitGroup or channel-based coordination, add logging
- Test coverage: No tests for background goroutine behavior; tests complete before goroutines finish

## Scaling Limits

**Webhook Processing Not Asynchronous:**
- Current capacity: Webhook endpoint blocks until all file downloads complete (can be 10+ seconds for slow GitHub)
- Limit: Simultaneous webhook requests blocked by I/O to GitHub/GitLab
- Scaling path: Move webhook processing to background queue (Redis, database task table), return 202 Accepted immediately

**Database Connections Untuned:**
- Current capacity: Default GORM/PostgreSQL pool with no custom configuration
- Limit: Connection pool exhaustion under concurrent admin request load
- Scaling path: Configure MaxOpenConns, MaxIdleConns in database.go, add connection pool monitoring

**File Fetch Operations Sequential:**
- Current capacity: downloadFile() makes sequential HTTP requests (main branch, then master branch fallback)
- Limit: Slow for repositories with poor network connectivity
- Scaling path: Add HTTP client timeout configuration, parallel requests with race condition resolution

**No Rate Limiting:**
- Current capacity: No rate limits on resource creation, version creation, or voting
- Limit: User spam abuse (create 1000 resources, spam votes)
- Scaling path: Add rate limiter middleware per user/IP for POST endpoints

## Dependencies at Risk

**goth & goth_fiber OAuth Libraries:**
- Risk: goth_fiber is third-party integration layer, less maintained than goth itself
- Impact: OAuth flow broken if dependencies become incompatible with newer goth versions
- Migration plan: Monitor GitHub issues, pin versions carefully, have fallback to manual OAuth implementation if needed

**hashicorp/hcl Parser:**
- Risk: HCL parsing errors silently ignored in many places
- Impact: Malformed HCL content (in metadata.hcl, variables.hcl) doesn't trigger user-facing errors, causes silent data loss
- Migration plan: Add validation layer that tests HCL before accepting resources, propagate parse errors to handlers

## Missing Critical Features

**No Rate Limiting:**
- Problem: Users can spam resource creation, votes, comments without restriction
- Blocks: Abuse mitigation, production readiness
- Solution priority: High - implement per-user rate limiter middleware

**No Audit Trail Search/Export:**
- Problem: AuditLog table has no search interface, admins can't query security events
- Blocks: Security investigations, compliance reporting
- Solution priority: Medium - add admin audit log viewer with filtering

**No Bulk Operations:**
- Problem: Admins must delete/moderate resources one at a time
- Blocks: Efficient content moderation
- Solution priority: Medium - add bulk delete/status change in admin panel

**No Resource Versioning in UI:**
- Problem: All versions stored but UI only shows latest, no way to view/rollback to older versions
- Blocks: Users can't access stable older versions
- Solution priority: Medium-Low - add version history UI

## Test Coverage Gaps

**Goroutine Background Tasks Untested:**
- What's not tested: Success/failure of background file fetching in resource creation and webhook handlers
- Files: `internal/handlers/resource.go` (background fetch logic at lines 298-315, etc)
- Risk: Goroutines failing silently, resource content never updates, no notification to user
- Priority: High - critical path for core feature

**OAuth Account Linking Logic:**
- What's not tested: Edge cases for account linking when multiple providers link to same email
- Files: `internal/handlers/auth.go` (lines 84-96 AuthCallback linking logic)
- Risk: Account takeover or data loss if linking logic has race conditions
- Priority: High - security-critical

**Webhook Signature Validation Edge Cases:**
- What's not tested: Invalid signature formats, missing secrets, payload tampering
- Files: `internal/handlers/resource.go` (validateGitHubSignature and HandleWebhook)
- Risk: Invalid webhooks could be processed as valid, allowing unauthorized resource updates
- Priority: High - security-critical

**GitHub Sync Bidirectional Logic:**
- What's not tested: Sync races (simultaneous GitHub and app status changes), orphaned issues, external GitHub deletions
- Files: `internal/handlers/github_sync.go` (entire sync logic)
- Risk: Data inconsistency between app and GitHub
- Priority: Medium - feature-specific but complex logic

**Email Sending Failures:**
- What's not tested: Behavior when email service is down (signup, password reset, verification)
- Files: `internal/handlers/auth.go` (lines 315-318, 405-408 email send failures ignored)
- Risk: User locked out unable to verify or reset, errors silently swallowed
- Priority: Medium - affects user experience

**Resource Deletion Authorization:**
- What's not tested: Organization resource deletion with revoked member, orphaned resources
- Files: `internal/handlers/resource.go` (DeleteResource authorization logic)
- Risk: Permission bypass through timing/race conditions
- Priority: Medium - security-adjacent

---

*Concerns audit: 2026-02-04*
