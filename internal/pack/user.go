package pack

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// CurrentUser is the body of GET /v1/user.
type CurrentUser struct {
	Username       string    `json:"username"`
	TokenScope     string    `json:"token_scope"`
	TokenOrg       *string   `json:"token_org"`
	TokenExpiresAt time.Time `json:"token_expires_at"`
}

// APIError is a non-200 answer from the /v1 API. Message is the server's
// error message, or empty when the body was not the /v1 error shape.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("unexpected status code: %d", e.StatusCode)
}

// maxUserBody caps how much of a GET /v1/user answer is read.
const maxUserBody = 64 << 10

// TokenURLAllowed reports whether a bearer token may be sent to rawURL: https
// anywhere, plain http only to a loopback host.
func TokenURLAllowed(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("invalid registry URL %q", rawURL)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return fmt.Errorf("refusing to send a token to %s: use https (plain http is allowed only for localhost)", u.Scheme+"://"+u.Host)
}

// CurrentUser calls GET /v1/user with token as the Bearer credential. It
// refuses plain-http non-loopback registries and never follows redirects, so
// the token cannot leave the configured host.
func (c *Client) CurrentUser(ctx context.Context, token string) (*CurrentUser, error) {
	if err := TokenURLAllowed(c.BaseURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/user", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	hc := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := hc.Do(req) // #nosec G704 -- BaseURL is the user's configured registry, checked by TokenURLAllowed
	if err != nil {
		// Do's error embeds the URL, which never contains the token.
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: resp.StatusCode, Message: errorMessage(resp)}
	}
	var u CurrentUser
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxUserBody)).Decode(&u); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &u, nil
}
