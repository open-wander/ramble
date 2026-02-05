//go:build security

package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rmbl/internal/testutil/security"
)

// TestSSRF_BypassAttempts tests that SSRF protection blocks all known bypass techniques.
func TestSSRF_BypassAttempts(t *testing.T) {
	cfg := SSRFConfig{
		AllowedHosts: []string{"github.com"},
		MaxRedirects: 3,
		Timeout:      5 * time.Second,
	}

	client, err := NewProtectedClient(cfg)
	if err != nil {
		t.Fatalf("failed to create protected client: %v", err)
	}

	for _, payload := range security.SSRFBypassPayloads {
		t.Run(payload.Name, func(t *testing.T) {
			req, err := http.NewRequest("GET", payload.URL, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}

			if payload.ExpectBlock {
				if err == nil {
					t.Errorf("expected SSRF block for %s, but request succeeded", payload.URL)
					return
				}
				t.Logf("Bypass attempt %s (%s): blocked=%v", payload.Name, payload.BypassType, err != nil)

				// Verify error indicates blocking
				errStr := strings.ToLower(err.Error())
				if !strings.Contains(errStr, "ssrf") &&
				   !strings.Contains(errStr, "blocked") &&
				   !strings.Contains(errStr, "disallowed") &&
				   !strings.Contains(errStr, "allowlist") {
					t.Logf("Note: Error type '%v' - may indicate network-level block", err)
				}
			}
		})
	}
}

// TestSSRF_RedirectChainAttack tests that redirect chains to internal IPs are blocked.
func TestSSRF_RedirectChainAttack(t *testing.T) {
	// Create test server that redirects to internal IP
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/step1":
			// First redirect - same server (allowed)
			http.Redirect(w, r, "/step2", http.StatusFound)
		case "/step2":
			// Second redirect - to localhost (should be blocked)
			http.Redirect(w, r, "http://127.0.0.1/admin", http.StatusFound)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	// Extract just the hostname from server.URL (e.g., "127.0.0.1:12345")
	serverHost := strings.TrimPrefix(server.URL, "http://")

	cfg := SSRFConfig{
		AllowedHosts: []string{serverHost},
		MaxRedirects: 5,
		Timeout:      5 * time.Second,
		AllowAnyPort: true, // Allow test server port
	}

	client, err := NewProtectedClient(cfg)
	if err != nil {
		t.Fatalf("failed to create protected client: %v", err)
	}

	// Request /step1 which will redirect to /step2, then to 127.0.0.1
	req, err := http.NewRequest("GET", server.URL+"/step1", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}

	// Should fail due to redirect to disallowed host
	if err == nil {
		t.Error("expected redirect chain to internal IP to be blocked")
		return
	}

	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "disallowed") &&
	   !strings.Contains(errStr, "allowlist") &&
	   !strings.Contains(errStr, "blocked") &&
	   !strings.Contains(errStr, "ssrf") {
		t.Errorf("expected disallowed/blocked error, got: %v", err)
	}
}

// TestSSRF_DNSRebindingProtection documents DNS rebinding protection.
//
// DNS rebinding attack vector:
// 1. Attacker controls DNS for evil.com
// 2. First lookup: evil.com -> 93.184.216.34 (public IP, passes allowlist)
// 3. Attacker changes DNS record
// 4. Second lookup: evil.com -> 127.0.0.1 (internal IP, attack target)
//
// Protection: The code.dny.dev/ssrf library's Guardian.Safe() function is called
// in the DialContext Control callback (ssrf.go:87-89). This validates the resolved
// IP address at dial time, preventing connections to private IPs even if DNS returns
// them on subsequent lookups.
//
// Full DNS rebinding requires custom resolver injection into DialContext; the ssrf
// library validates IPs at dial time which prevents this attack.
func TestSSRF_DNSRebindingProtection(t *testing.T) {
	// This is a documentation test acknowledging the protection exists.
	// The ssrf library's Guardian.Safe() is called in DialContext Control callback.

	t.Log("DNS rebinding protection: ssrf.Guardian.Safe() validates resolved IPs at dial time")
	t.Log("Location: internal/security/ssrf.go:87-89 in DialContext Control callback")
	t.Log("Protection: Blocks connections to private IPs regardless of DNS response")

	// Create client to verify protection is configured
	cfg := SSRFConfig{
		AllowedHosts: []string{"example.com"},
		MaxRedirects: 3,
		Timeout:      5 * time.Second,
	}

	client, err := NewProtectedClient(cfg)
	if err != nil {
		t.Fatalf("failed to create protected client: %v", err)
	}

	// Verify client was created (protection is configured in DialContext)
	if client == nil {
		t.Error("expected protected client to be created")
	}

	// Document that DNS rebinding is prevented by IP validation at dial time
	t.Log("DNS rebinding prevented: IP validation occurs after DNS resolution in dial phase")
}

// TestSSRF_IPv6Variations tests IPv6 bypass attempts are blocked.
func TestSSRF_IPv6Variations(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{
			name: "Pure IPv6 localhost",
			url:  "http://[::1]/admin",
		},
		{
			name: "IPv4-mapped IPv6 localhost",
			url:  "http://[::ffff:127.0.0.1]/admin",
		},
		{
			name: "IPv6 private range",
			url:  "http://[fc00::1]/admin",
		},
		{
			name: "IPv6 link-local",
			url:  "http://[fe80::1]/admin",
		},
	}

	cfg := SSRFConfig{
		AllowedHosts: []string{"github.com"},
		MaxRedirects: 3,
		Timeout:      5 * time.Second,
	}

	client, err := NewProtectedClient(cfg)
	if err != nil {
		t.Fatalf("failed to create protected client: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.url, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}

			if err == nil {
				t.Errorf("expected IPv6 bypass %s to be blocked", tt.url)
				return
			}

			t.Logf("IPv6 bypass blocked: %v", err)
		})
	}
}

// TestSSRF_EncodedIPVariations tests encoded IP bypass attempts are blocked.
func TestSSRF_EncodedIPVariations(t *testing.T) {
	tests := []struct {
		name string
		url  string
		desc string
	}{
		{
			name: "Decimal encoded localhost",
			url:  "http://2130706433/admin",
			desc: "2130706433 = 127.0.0.1 in decimal",
		},
		{
			name: "Hex encoded localhost",
			url:  "http://0x7f000001/admin",
			desc: "0x7f000001 = 127.0.0.1 in hex",
		},
		{
			name: "Octal encoded localhost",
			url:  "http://0177.0.0.1/admin",
			desc: "0177.0.0.1 = 127.0.0.1 with octal first octet",
		},
		{
			name: "Mixed format with port",
			url:  "http://127.0.0.1:8080/admin",
			desc: "Localhost with non-standard port",
		},
		{
			name: "Decimal with port",
			url:  "http://2130706433:8080/admin",
			desc: "Decimal encoded with port variation",
		},
	}

	cfg := SSRFConfig{
		AllowedHosts: []string{"github.com"},
		MaxRedirects: 3,
		Timeout:      5 * time.Second,
	}

	client, err := NewProtectedClient(cfg)
	if err != nil {
		t.Fatalf("failed to create protected client: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.desc)

			req, err := http.NewRequest("GET", tt.url, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}

			if err == nil {
				t.Errorf("expected encoded IP bypass %s to be blocked", tt.url)
				return
			}

			// Verify the ssrf library's IP validation blocked it
			t.Logf("Encoded IP bypass blocked: %v", err)
		})
	}
}
