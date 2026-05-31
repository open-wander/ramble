package security

import (
	"context"
	"net"
	"sync/atomic"
)

// RebindingResolver simulates a DNS rebinding attack by returning different IPs on successive lookups.
// First lookup returns a public IP (bypass allowlist checks), subsequent lookups return attack target IP.
type RebindingResolver struct {
	FirstIP  string // Returns this on first lookup (e.g., "93.184.216.34" - public IP)
	SecondIP string // Returns this on subsequent lookups (e.g., "127.0.0.1" - attack target)
	calls    int64  // Atomic counter for lookup count
}

// NewRebindingResolver creates a DNS rebinding simulator.
// publicIP is returned on first lookup, privateIP on subsequent lookups.
func NewRebindingResolver(publicIP, privateIP string) *RebindingResolver {
	return &RebindingResolver{
		FirstIP:  publicIP,
		SecondIP: privateIP,
		calls:    0,
	}
}

// LookupIPAddr implements DNS resolution with rebinding behavior.
// First call returns FirstIP, subsequent calls return SecondIP.
func (r *RebindingResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	callNum := atomic.AddInt64(&r.calls, 1)

	var ipStr string
	if callNum == 1 {
		ipStr = r.FirstIP
	} else {
		ipStr = r.SecondIP
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, &net.DNSError{
			Err:  "invalid IP address in test resolver",
			Name: host,
		}
	}

	return []net.IPAddr{{IP: ip}}, nil
}

// SSRFBypassPayload represents a single SSRF bypass attempt.
type SSRFBypassPayload struct {
	Name        string // Descriptive name
	URL         string // Malicious URL
	BypassType  string // Category: encoded_ip, ipv6, cloud_metadata, link_local
	ExpectBlock bool   // Should be true for all
}

// SSRFBypassPayloads contains comprehensive SSRF bypass techniques documented in OWASP.
var SSRFBypassPayloads = []SSRFBypassPayload{
	{
		Name:        "Decimal encoded localhost",
		URL:         "http://2130706433/admin",
		BypassType:  "encoded_ip",
		ExpectBlock: true,
	},
	{
		Name:        "Hex encoded localhost",
		URL:         "http://0x7f000001/admin",
		BypassType:  "encoded_ip",
		ExpectBlock: true,
	},
	{
		Name:        "Octal encoded localhost",
		URL:         "http://0177.0.0.1/admin",
		BypassType:  "encoded_ip",
		ExpectBlock: true,
	},
	{
		Name:        "IPv6 localhost",
		URL:         "http://[::1]/admin",
		BypassType:  "ipv6",
		ExpectBlock: true,
	},
	{
		Name:        "IPv6 mapped IPv4 localhost",
		URL:         "http://[::ffff:127.0.0.1]/admin",
		BypassType:  "ipv6",
		ExpectBlock: true,
	},
	{
		Name:        "IPv6 private range",
		URL:         "http://[fc00::1]/admin",
		BypassType:  "ipv6",
		ExpectBlock: true,
	},
	{
		Name:        "AWS metadata endpoint",
		URL:         "http://169.254.169.254/latest/meta-data/",
		BypassType:  "cloud_metadata",
		ExpectBlock: true,
	},
	{
		Name:        "GCP metadata endpoint",
		URL:         "http://metadata.google.internal/",
		BypassType:  "cloud_metadata",
		ExpectBlock: true,
	},
	{
		Name:        "Link-local address",
		URL:         "http://169.254.1.1/",
		BypassType:  "link_local",
		ExpectBlock: true,
	},
	{
		Name:        "Localhost with different port",
		URL:         "http://127.0.0.1:8080/admin",
		BypassType:  "encoded_ip",
		ExpectBlock: true,
	},
	{
		Name:        "Decimal encoded with port",
		URL:         "http://2130706433:8080/admin",
		BypassType:  "encoded_ip",
		ExpectBlock: true,
	},
	{
		Name:        "Private IP 10.0.0.1",
		URL:         "http://10.0.0.1/internal",
		BypassType:  "encoded_ip",
		ExpectBlock: true,
	},
	{
		Name:        "Private IP 192.168.1.1",
		URL:         "http://192.168.1.1/internal",
		BypassType:  "encoded_ip",
		ExpectBlock: true,
	},
	{
		Name:        "Private IP 172.16.0.1",
		URL:         "http://172.16.0.1/internal",
		BypassType:  "encoded_ip",
		ExpectBlock: true,
	},
}
