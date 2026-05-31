package security

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSSRFProtection_PrivateIPBlocking tests that private IP ranges are blocked
func TestSSRFProtection_PrivateIPBlocking(t *testing.T) {
	tests := []struct {
		name        string
		targetURL   string
		expectBlock bool
	}{
		{
			name:        "block localhost IPv4",
			targetURL:   "http://127.0.0.1/secret",
			expectBlock: true,
		},
		{
			name:        "block 10.x.x.x range",
			targetURL:   "http://10.0.0.1/internal",
			expectBlock: true,
		},
		{
			name:        "block 172.16-31.x.x range",
			targetURL:   "http://172.16.0.1/admin",
			expectBlock: true,
		},
		{
			name:        "block 192.168.x.x range",
			targetURL:   "http://192.168.1.1/config",
			expectBlock: true,
		},
		{
			name:        "block cloud metadata endpoint",
			targetURL:   "http://169.254.169.254/meta-data",
			expectBlock: true,
		},
	}

	cfg := SSRFConfig{
		AllowedHosts: []string{"example.com"},
		MaxRedirects: 3,
		Timeout:      5 * time.Second,
	}

	client, err := NewProtectedClient(cfg)
	if err != nil {
		t.Fatalf("failed to create protected client: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.targetURL, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}

			if tt.expectBlock {
				if err == nil {
					t.Errorf("expected SSRF block for %s, but request succeeded", tt.targetURL)
					return
				}
				errStr := strings.ToLower(err.Error())
				if !strings.Contains(errStr, "ssrf") && !strings.Contains(errStr, "blocked") && !strings.Contains(errStr, "disallowed") {
					t.Errorf("expected SSRF/blocked/disallowed error, got: %v", err)
				}
			}
		})
	}
}

// TestSSRFProtection_AllowlistValidation tests that only allowlisted hosts can be accessed
func TestSSRFProtection_AllowlistValidation(t *testing.T) {
	tests := []struct {
		name          string
		allowedHosts  []string
		targetHost    string
		expectAllowed bool
	}{
		{
			name:          "allow exact match",
			allowedHosts:  []string{"github.com"},
			targetHost:    "github.com",
			expectAllowed: true,
		},
		{
			name:          "block non-allowlisted host",
			allowedHosts:  []string{"github.com"},
			targetHost:    "evil.com",
			expectAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server that will respond successfully
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			}))
			defer server.Close()

			cfg := SSRFConfig{
				AllowedHosts: tt.allowedHosts,
				MaxRedirects: 3,
				Timeout:      5 * time.Second,
			}

			client, err := NewProtectedClient(cfg)
			if err != nil {
				t.Fatalf("failed to create protected client: %v", err)
			}

			// Build URL using the test server's address but with the target host in the request
			// Note: This tests the hostname validation logic
			req, err := http.NewRequest("GET", "http://"+tt.targetHost+"/test", nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}

			if tt.expectAllowed {
				// We expect an error because the hostname won't actually resolve in tests,
				// but it should NOT be an allowlist error
				if err != nil && strings.Contains(strings.ToLower(err.Error()), "allowlist") {
					t.Errorf("expected no allowlist error for allowed host, got: %v", err)
				}
			} else {
				// We expect an allowlist error
				if err == nil {
					t.Errorf("expected allowlist block for %s, but request succeeded", tt.targetHost)
					return
				}
				errStr := strings.ToLower(err.Error())
				if !strings.Contains(errStr, "allowlist") && !strings.Contains(errStr, "disallowed") {
					t.Errorf("expected allowlist/disallowed error, got: %v", err)
				}
			}
		})
	}
}

// TestSSRFProtection_SubdomainMatching tests that subdomains of allowlisted hosts are allowed
func TestSSRFProtection_SubdomainMatching(t *testing.T) {
	tests := []struct {
		name          string
		allowedHosts  []string
		targetHost    string
		expectAllowed bool
	}{
		{
			name:          "allow subdomain of allowlisted host",
			allowedHosts:  []string{"githubusercontent.com"},
			targetHost:    "raw.githubusercontent.com",
			expectAllowed: true,
		},
		{
			name:          "allow api subdomain",
			allowedHosts:  []string{"github.com"},
			targetHost:    "api.github.com",
			expectAllowed: true,
		},
		{
			name:          "block different domain",
			allowedHosts:  []string{"github.com"},
			targetHost:    "githubevil.com",
			expectAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SSRFConfig{
				AllowedHosts: tt.allowedHosts,
				MaxRedirects: 3,
				Timeout:      5 * time.Second,
			}

			client, err := NewProtectedClient(cfg)
			if err != nil {
				t.Fatalf("failed to create protected client: %v", err)
			}

			req, err := http.NewRequest("GET", "http://"+tt.targetHost+"/test", nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}

			if tt.expectAllowed {
				// Should not get an allowlist error (may get DNS error)
				if err != nil && strings.Contains(strings.ToLower(err.Error()), "allowlist") {
					t.Errorf("expected no allowlist error for subdomain of allowed host, got: %v", err)
				}
			} else {
				// Should get an allowlist error
				if err == nil {
					t.Errorf("expected allowlist block for %s, but request succeeded", tt.targetHost)
					return
				}
				errStr := strings.ToLower(err.Error())
				if !strings.Contains(errStr, "allowlist") && !strings.Contains(errStr, "disallowed") {
					t.Errorf("expected allowlist/disallowed error, got: %v", err)
				}
			}
		})
	}
}

// TestSSRFProtection_RedirectLimits tests that redirect limits are enforced
func TestSSRFProtection_RedirectLimits(t *testing.T) {
	// Test max redirects by checking the CheckRedirect logic directly
	t.Run("enforce max redirect limit", func(t *testing.T) {
		cfg := SSRFConfig{
			AllowedHosts: []string{"example.com"},
			MaxRedirects: 3,
			Timeout:      5 * time.Second,
		}

		client, err := NewProtectedClient(cfg)
		if err != nil {
			t.Fatalf("failed to create protected client: %v", err)
		}

		// Simulate the CheckRedirect being called with 4 prior requests (exceeding limit of 3)
		testReq, _ := http.NewRequest("GET", "http://example.com/page4", nil)
		via := make([]*http.Request, 3)
		for i := range via {
			via[i], _ = http.NewRequest("GET", fmt.Sprintf("http://example.com/page%d", i), nil)
		}

		// Call the CheckRedirect function
		err = client.CheckRedirect(testReq, via)
		if err == nil {
			t.Error("expected redirect limit error after 3 redirects")
			return
		}

		errStr := strings.ToLower(err.Error())
		if !strings.Contains(errStr, "redirect") && !strings.Contains(errStr, "stopped") {
			t.Errorf("expected redirect limit error, got: %v", err)
		}
	})

	// Test redirect to disallowed host
	t.Run("block redirect to disallowed host", func(t *testing.T) {
		cfg := SSRFConfig{
			AllowedHosts: []string{"example.com"},
			MaxRedirects: 3,
			Timeout:      5 * time.Second,
		}

		client, err := NewProtectedClient(cfg)
		if err != nil {
			t.Fatalf("failed to create protected client: %v", err)
		}

		// Simulate redirect to a non-allowlisted host
		testReq, _ := http.NewRequest("GET", "http://evil.com/malicious", nil)
		via := []*http.Request{
			{},
		}

		// Call the CheckRedirect function
		err = client.CheckRedirect(testReq, via)
		if err == nil {
			t.Error("expected redirect block for disallowed host")
			return
		}

		errStr := strings.ToLower(err.Error())
		if !strings.Contains(errStr, "disallowed") {
			t.Errorf("expected disallowed host error, got: %v", err)
		}
	})
}

// TestIsHostAllowed_ParentDomainBypass verifies that parent domains, TLDs,
// and lookalike domains are rejected when only specific hosts are allowed.
// AllowedHosts = ["github.com", "raw.githubusercontent.com"]
func TestIsHostAllowed_ParentDomainBypass(t *testing.T) {
	allowed := []string{"github.com", "raw.githubusercontent.com"}

	tests := []struct {
		name      string
		host      string
		wantAllow bool
	}{
		// --- permitted ---
		{name: "exact github.com", host: "github.com", wantAllow: true},
		{name: "subdomain api.github.com", host: "api.github.com", wantAllow: true},
		{name: "exact raw.githubusercontent.com", host: "raw.githubusercontent.com", wantAllow: true},
		// --- rejected: parent-domain / TLD bypass (core regression) ---
		{name: "parent githubusercontent.com", host: "githubusercontent.com", wantAllow: false},
		{name: "TLD com", host: "com", wantAllow: false},
		// --- rejected: unrelated domains ---
		{name: "notgithub.com", host: "notgithub.com", wantAllow: false},
		{name: "evil.com", host: "evil.com", wantAllow: false},
		// --- rejected: subdomain-of-attacker wrapping allowed suffix ---
		{name: "github.com.evil.com", host: "github.com.evil.com", wantAllow: false},
		// --- edge cases ---
		{name: "uppercase GitHub.COM normalised", host: "GitHub.COM", wantAllow: true},
		{name: "trailing dot github.com.", host: "github.com.", wantAllow: true},
		{name: "empty host", host: "", wantAllow: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHostAllowed(tt.host, allowed)
			if got != tt.wantAllow {
				t.Errorf("isHostAllowed(%q, %v) = %v, want %v", tt.host, allowed, got, tt.wantAllow)
			}
		})
	}
}

// TestDefaultAllowedHosts tests that default allowed hosts are returned correctly
func TestDefaultAllowedHosts(t *testing.T) {
	hosts := DefaultAllowedHosts()

	expectedHosts := []string{"github.com", "raw.githubusercontent.com", "api.github.com", "gitlab.com"}

	for _, expected := range expectedHosts {
		found := false
		for _, host := range hosts {
			if host == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected default host %s not found in %v", expected, hosts)
		}
	}
}
