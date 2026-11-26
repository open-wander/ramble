//go:build security

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
