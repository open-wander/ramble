package handlers

import (
	"strconv"
	"strings"
	"time"

	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
	"rmbl/internal/services/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

const (
	// LocalAPIToken is the c.Locals key holding the *models.APIToken of a
	// token-authenticated request.
	LocalAPIToken = "APIToken"

	// SessionCookieName is the session cookie name; InitSession pins the
	// session store to it and the CSRF exemption checks for it.
	SessionCookieName = "session_id"

	apiWriteLimit  = 120
	apiWriteWindow = time.Minute
)

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header. The scheme is matched case-insensitively (RFC 7235). The header
// value is never logged.
func bearerToken(c *fiber.Ctx) (string, bool) {
	scheme, tok, found := strings.Cut(c.Get(fiber.HeaderAuthorization), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	if tok == "" || strings.ContainsAny(tok, " \t\r\n") {
		return "", false
	}
	return tok, true
}

// APITokenFromCtx returns the API token that authenticated the request, if any.
func APITokenFromCtx(c *fiber.Ctx) (*models.APIToken, bool) {
	tok, ok := c.Locals(LocalAPIToken).(*models.APIToken)
	return tok, ok && tok != nil
}

// emailUnverified mirrors the rule in RequireVerifiedEmail: OAuth users count
// as verified, local accounts need EmailVerified.
func emailUnverified(u models.User) bool {
	return u.Provider == "" && !u.EmailVerified
}

// RequireAPIToken authenticates a request by Bearer token for the /v1 API.
// scope is models.ScopeRead or models.ScopeWrite; anything else panics at
// construction so a typo cannot fail open. A write route also needs a write
// token and a verified email.
//
// Mount it per route or on a /v1 group only; never globally and never on a
// group that includes /admin. Tokens never grant admin: the stored User has
// IsAdmin forced to false, and API handlers must not reload IsAdmin from the
// database or add admin bypasses.
func RequireAPIToken(scope string) fiber.Handler {
	if scope != models.ScopeRead && scope != models.ScopeWrite {
		panic("handlers: RequireAPIToken: invalid scope " + strconv.Quote(scope))
	}
	return func(c *fiber.Ctx) error {
		plain, ok := bearerToken(c)
		if !ok {
			return APIError(c, fiber.StatusUnauthorized, codeUnauthorized, msgUnauthenticated)
		}
		res, err := apitoken.Svc.WithContext(c.UserContext()).Authenticate(plain)
		if err != nil {
			return APIError(c, fiber.StatusUnauthorized, codeUnauthorized, msgUnauthenticated)
		}

		if scope == models.ScopeWrite {
			if res.Token.Scope != models.ScopeWrite {
				return APIError(c, fiber.StatusForbidden, codeInsufficientScope, "token scope does not allow this request")
			}
			if emailUnverified(*res.User) {
				return APIError(c, fiber.StatusForbidden, codeEmailUnverified, "verify your email address before using write access")
			}
		}

		u := *res.User
		u.IsAdmin = false
		// Parity with the session middleware, which preloads memberships.
		// An org-restricted token only carries that org's membership, so a
		// handler that authorises from u.Memberships cannot reach other orgs
		// even if it forgets TokenAllowsOrg.
		q := reqDB(c).Preload("Organization").Where("user_id = ?", u.ID)
		if res.Token.OrganizationID != nil {
			q = q.Where("organization_id = ?", *res.Token.OrganizationID)
		}
		if err := q.Find(&u.Memberships).Error; err != nil {
			logger.FromContext(c.UserContext()).Error("apitoken.memberships_failed", logger.Err(err), "token_id", res.Token.ID)
			return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
		}

		// Overwrite, not add: a request with both a session cookie and a
		// token must act as the token's user.
		c.Locals("UserID", u.ID)
		c.Locals("User", u)
		c.Locals(LocalAPIToken, res.Token)
		c.Locals(logger.LocalTokenPrefix, res.Token.Prefix)

		if err := apitoken.Svc.WithContext(c.UserContext()).TouchLastUsed(res.Token.ID); err != nil {
			logger.FromContext(c.UserContext()).Warn("apitoken.touch_failed", logger.Err(err), "token_id", res.Token.ID)
		}
		return c.Next()
	}
}

// tokenAllowsOrg is the pure org-restriction rule. A nil token is a session
// request. An unrestricted token may act anywhere the user may. A restricted
// token may only act inside its organisation, never on personally owned
// resources (nil target).
func tokenAllowsOrg(tok *models.APIToken, targetOrgID *uint) bool {
	if tok == nil || tok.OrganizationID == nil {
		return true
	}
	return targetOrgID != nil && *targetOrgID == *tok.OrganizationID
}

// TokenAllowsOrg reports whether the request's token may act on a target owned
// by targetOrgID. Callers pass the resource's or organisation's OrganizationID,
// nil for user-owned resources and when creating an organisation (so a
// restricted token cannot create one). Session requests are always allowed.
func TokenAllowsOrg(c *fiber.Ctx, targetOrgID *uint) bool {
	tok, _ := APITokenFromCtx(c)
	return tokenAllowsOrg(tok, targetOrgID)
}

// APIOrgForbidden writes the 403 forbidden API error for a token that may not
// act on the target's organisation. Use it as
//
//	if !TokenAllowsOrg(c, target) { return APIOrgForbidden(c) }
func APIOrgForbidden(c *fiber.Ctx) error {
	return APIError(c, fiber.StatusForbidden, codeForbidden, "token is restricted to another organization")
}

// NewAPIWriteLimiter returns a fresh per-token write limiter (120 per minute,
// keyed by token ID, not IP, which collapses behind the proxy). It must be
// chained after RequireAPIToken, since it reads the token from locals. Create
// one per route table and share it across endpoints so the budget is shared;
// do not make it a package variable (state would leak across tests). The
// store is in memory, so the limit applies per replica.
func NewAPIWriteLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        apiWriteLimit,
		Expiration: apiWriteWindow,
		Next: func(c *fiber.Ctx) bool {
			switch c.Method() {
			case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
				return true
			}
			return false
		},
		KeyGenerator: func(c *fiber.Ctx) string {
			if tok, ok := APITokenFromCtx(c); ok {
				return "apitoken:" + strconv.FormatUint(uint64(tok.ID), 10)
			}
			return "ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return APIError(c, fiber.StatusTooManyRequests, codeRateLimited, "rate limit exceeded")
		},
	})
}
