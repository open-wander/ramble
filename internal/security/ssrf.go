package security

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"code.dny.dev/ssrf"
)

// SSRFConfig holds configuration for SSRF-protected HTTP client
type SSRFConfig struct {
	// AllowedHosts is the list of hostnames that are permitted to be accessed
	AllowedHosts []string
	// MaxRedirects is the maximum number of redirects to follow (default: 3)
	MaxRedirects int
	// Timeout is the HTTP client timeout (default: 30 seconds)
	Timeout time.Duration
	// AllowAnyPort allows connections to any port (useful for testing, less secure)
	AllowAnyPort bool
}

// DefaultAllowedHosts returns the default list of allowed hosts for Git operations
func DefaultAllowedHosts() []string {
	hosts := []string{
		"github.com",
		"raw.githubusercontent.com",
		"api.github.com",
		"gitlab.com",
	}

	// Add hosts from environment variable
	if envHosts := os.Getenv("ALLOWED_GIT_HOSTS"); envHosts != "" {
		for _, host := range strings.Split(envHosts, ",") {
			host = strings.TrimSpace(host)
			if host != "" {
				hosts = append(hosts, host)
			}
		}
	}

	return hosts
}

// NewProtectedClient creates an HTTP client with SSRF protection
func NewProtectedClient(cfg SSRFConfig) (*http.Client, error) {
	// Validate config
	if len(cfg.AllowedHosts) == 0 {
		return nil, fmt.Errorf("AllowedHosts cannot be empty")
	}

	if cfg.MaxRedirects == 0 {
		cfg.MaxRedirects = 3
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	// Create SSRF guardian to validate IP addresses
	var guardianOpts []ssrf.Option
	if cfg.AllowAnyPort {
		guardianOpts = []ssrf.Option{
			ssrf.WithAnyPort(),
			ssrf.WithNetworks("tcp4", "tcp6"),
		}
	} else {
		guardianOpts = []ssrf.Option{
			ssrf.WithPorts(80, 443, 8080, 8443, 9418), // Common HTTP/HTTPS and Git ports
			ssrf.WithNetworks("tcp4", "tcp6"),
		}
	}
	guardian := ssrf.New(guardianOpts...)

	// Create custom dialer with SSRF protection
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			// Check SSRF guardian for IP-based protection
			// This will block private IPs and cloud metadata endpoints
			if err := guardian.Safe(network, address, c); err != nil {
				return fmt.Errorf("SSRF protection blocked address %s: %w", address, err)
			}

			return nil
		},
	}

	// Create transport with custom dialer and hostname validation
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Extract hostname from address (format: "host:port")
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address format: %w", err)
			}

			// Validate hostname against allowlist before DNS resolution
			// Skip validation if the host is already an IP address (from redirects, etc.)
			if net.ParseIP(host) == nil {
				if !isHostAllowed(host, cfg.AllowedHosts) {
					return nil, fmt.Errorf("host %s is not in allowlist", host)
				}
			}

			return dialer.DialContext(ctx, network, addr)
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Create HTTP client with redirect validation
	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Limit number of redirects
			if len(via) >= cfg.MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", cfg.MaxRedirects)
			}

			// Validate redirect destination hostname
			redirectHost := req.URL.Hostname()
			if !isHostAllowed(redirectHost, cfg.AllowedHosts) {
				return fmt.Errorf("redirect to disallowed host: %s", redirectHost)
			}

			return nil
		},
	}

	return client, nil
}

// isHostAllowed checks if a hostname is in the allowlist.
// A candidate host H matches an allowed entry A iff:
//   - H == A  (exact match), or
//   - H ends with "."+A  (H is a subdomain of A)
//
// Parent domains are never permitted: if "raw.githubusercontent.com" is
// allowed, "githubusercontent.com" and "com" are still rejected.
func isHostAllowed(host string, allowedHosts []string) bool {
	// Normalise: lowercase and strip a single trailing dot (FQDN form).
	host = strings.ToLower(strings.TrimSuffix(host, "."))

	if host == "" {
		return false
	}

	for _, allowed := range allowedHosts {
		allowed = strings.ToLower(strings.TrimSuffix(allowed, "."))
		if allowed == "" {
			continue
		}

		// Exact match.
		if host == allowed {
			return true
		}

		// Subdomain match: host must end with ".<allowed>".
		if strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}

	return false
}
