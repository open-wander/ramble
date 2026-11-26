package apiclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"rmbl/internal/pack"
)

// maxResponseBody caps every response body the generated client reads, so a
// hostile registry cannot exhaust memory.
const maxResponseBody = 1 << 20

// limitedDoer caps the size of each response body.
type limitedDoer struct {
	hc *http.Client
}

type limitedBody struct {
	io.Reader
	io.Closer
}

func (d limitedDoer) Do(req *http.Request) (*http.Response, error) {
	resp, err := d.hc.Do(req) // #nosec G704 -- New checks the base URL with pack.TokenURLAllowed
	if err != nil {
		return nil, err
	}
	resp.Body = limitedBody{io.LimitReader(resp.Body, maxResponseBody), resp.Body}
	return resp, nil
}

// New returns a /v1 client for the registry at baseURL that authenticates with
// token. It is the only way the CLI builds a client: it refuses an empty
// token and any registry the token may not be sent to (plain http beyond
// loopback), and uses the hardened pack.TokenHTTPClient, which never follows
// redirects.
func New(baseURL, token string) (*ClientWithResponses, error) {
	if token == "" {
		return nil, errors.New("no API token")
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	if err := pack.TokenURLAllowed(baseURL); err != nil {
		return nil, err
	}
	return NewClientWithResponses(baseURL,
		WithHTTPClient(limitedDoer{pack.TokenHTTPClient()}),
		WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		}),
	)
}
