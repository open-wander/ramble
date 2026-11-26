//go:build security

package server

import (
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/handlers"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"
)

// The token routes are registered normally, so the global CSRF middleware
// must reject mutating requests that carry no CSRF token.
func TestCSRF_TokenRoutesRejectMissingToken(t *testing.T) {
	app := stackApp(t, true)
	cookie := map[string]string{"Cookie": "session_id=anything"}

	status, _ := send(t, app, "POST", "/settings/tokens", cookie)
	assert.Equal(t, 403, status)
	status, _ = send(t, app, "DELETE", "/settings/tokens/1", cookie)
	assert.Equal(t, 403, status)
}

// An owner's session cannot delete the organisation without a CSRF token;
// with one, the same request reaches the handler and deletes it.
func TestCSRF_OrgDeleteNeedsToken(t *testing.T) {
	owner, _, _ := mkUserTok(t, "csrfowner", models.ScopeRead, false)
	org := models.Organization{Name: "csrforg-" + strings.ToLower(owner.Username[len("csrfowner-"):])}
	require.NoError(t, database.DB.Create(&org).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"}).Error)

	app := fiber.New(fiberConfig(nil))
	useRequestLogging(app, logger.Log)
	useSecurityMiddleware(app, false)
	useSessionMiddleware(app, "test")
	app.Get("/_test/login", func(c *fiber.Ctx) error {
		sess, err := handlers.Store.Get(c)
		if err != nil {
			return err
		}
		sess.Set("user_id", owner.ID)
		if err := sess.Save(); err != nil {
			return err
		}
		return c.SendString(fmt.Sprint(c.Locals("csrf")))
	})
	registerRoutes(app)

	resp, err := app.Test(httptest.NewRequest("GET", "/_test/login", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	csrfToken := string(body)
	var cookies []string
	for _, ck := range resp.Cookies() {
		cookies = append(cookies, ck.Name+"="+ck.Value)
	}
	cookie := strings.Join(cookies, "; ")
	require.Contains(t, cookie, handlers.SessionCookieName+"=")
	require.Contains(t, cookie, "csrf_token=")

	live := func() bool {
		var n int64
		database.DB.Model(&models.Organization{}).Where("id = ?", org.ID).Count(&n)
		return n == 1
	}

	path := "/orgs/" + org.Name + "/delete"
	status, _ := send(t, app, "POST", path, map[string]string{"Cookie": cookie})
	assert.Equal(t, 403, status)
	assert.True(t, live(), "organisation must survive a request without a CSRF token")

	status, _ = send(t, app, "POST", path, map[string]string{"Cookie": cookie, "X-CSRF-Token": csrfToken})
	assert.Equal(t, 200, status)
	assert.False(t, live(), "the same request with a CSRF token deletes the organisation")
}
