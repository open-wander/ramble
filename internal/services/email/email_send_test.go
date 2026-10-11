package email

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/textproto"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fakeSMTP is an in-process STARTTLS SMTP server. When failAt names a stage
// (MAIL, RCPT or DATA) it replies with the reply template, whose two %s verbs
// receive the address (MAIL/RCPT argument, or the To header at DATA) and the
// link line of the message body.
type fakeSMTP struct {
	cert   tls.Certificate
	failAt string
	reply  string
}

func newFakeSMTP(t *testing.T, failAt, reply string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "fake-smtp"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsed)

	prev := smtpRootCAs
	smtpRootCAs = pool
	t.Cleanup(func() { smtpRootCAs = prev })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	s := &fakeSMTP{
		cert:   tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key},
		failAt: failAt,
		reply:  reply,
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	t.Setenv("SMTP_HOST", "127.0.0.1")
	t.Setenv("SMTP_PORT", port)
	t.Setenv("SMTP_USER", "mailer")
	t.Setenv("SMTP_PASSWORD", "hunter2")
	t.Setenv("FROM_ADDRESS", "noreply@ramble.example.org")
}

func angleArg(line string) string {
	i, j := strings.Index(line, "<"), strings.LastIndex(line, ">")
	if i < 0 || j < i {
		return ""
	}
	return line[i+1 : j]
}

func (s *fakeSMTP) serve(raw net.Conn) {
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
	tc := textproto.NewConn(raw)
	_ = tc.PrintfLine("220 fake ESMTP")
	tlsDone := false
	fail := func(stage, addr, link string) bool {
		if s.failAt != stage {
			return false
		}
		_ = tc.PrintfLine("%s", fmt.Sprintf(s.reply, addr, link))
		return true
	}
	for {
		line, err := tc.ReadLine()
		if err != nil {
			return
		}
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			_ = tc.PrintfLine("250-fake")
			if tlsDone {
				_ = tc.PrintfLine("250 AUTH PLAIN")
			} else {
				_ = tc.PrintfLine("250 STARTTLS")
			}
		case strings.HasPrefix(cmd, "STARTTLS"):
			_ = tc.PrintfLine("220 ready")
			tlsConn := tls.Server(raw, &tls.Config{Certificates: []tls.Certificate{s.cert}, MinVersion: tls.VersionTLS12})
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			tc = textproto.NewConn(tlsConn)
			tlsDone = true
		case strings.HasPrefix(cmd, "AUTH"):
			_ = tc.PrintfLine("235 2.7.0 ok")
		case strings.HasPrefix(cmd, "MAIL"):
			if fail("MAIL", angleArg(line), "") {
				return
			}
			_ = tc.PrintfLine("250 2.1.0 ok")
		case strings.HasPrefix(cmd, "RCPT"):
			if fail("RCPT", angleArg(line), "") {
				return
			}
			_ = tc.PrintfLine("250 2.1.5 ok")
		case strings.HasPrefix(cmd, "DATA"):
			_ = tc.PrintfLine("354 go ahead")
			lines, err := tc.ReadDotLines()
			if err != nil {
				return
			}
			var to, link string
			for _, l := range lines {
				if v, ok := strings.CutPrefix(l, "To: "); ok {
					to = v
				}
				if strings.HasPrefix(l, "http") {
					link = l
				}
			}
			if fail("DATA", to, link) {
				return
			}
			_ = tc.PrintfLine("250 2.0.0 queued")
		case strings.HasPrefix(cmd, "QUIT"):
			_ = tc.PrintfLine("221 bye")
			return
		default:
			_ = tc.PrintfLine("500 unknown")
		}
	}
}

func TestSend_ErrorsRedactAddressesAndLink(t *testing.T) {
	const (
		rcpt       = "alice@example.com"
		sender     = "noreply@ramble.example.org"
		verifyLink = "https://ramble.example.org/verify?token=VERIFYTOKEN123"
		resetLink  = "https://ramble.example.org/reset?token=RESETTOKEN456"
	)

	tests := []struct {
		name     string
		failAt   string
		reply    string
		send     func() error
		wantCode string
		wantText string
		hidden   []string
	}{
		{
			name:     "rcpt rejected echoes recipient",
			failAt:   "RCPT",
			reply:    "550 5.1.1 <%s>: Recipient address rejected",
			send:     func() error { return SendVerificationEmail(rcpt, verifyLink) },
			wantCode: "550",
			wantText: "failed to set recipient",
			hidden:   []string{rcpt, "example.com", verifyLink, "VERIFYTOKEN123"},
		},
		{
			name:     "mail rejected echoes sender",
			failAt:   "MAIL",
			reply:    "553 5.1.8 <%s>: Sender address rejected",
			send:     func() error { return SendVerificationEmail(rcpt, verifyLink) },
			wantCode: "553",
			wantText: "failed to set sender",
			hidden:   []string{sender, "ramble.example.org", "VERIFYTOKEN123"},
		},
		{
			name:     "end of data echoes recipient and link",
			failAt:   "DATA",
			reply:    "554 5.7.1 message to %s refused, body said %s",
			send:     func() error { return SendResetEmail(rcpt, resetLink) },
			wantCode: "554",
			wantText: "failed to close data writer",
			hidden:   []string{rcpt, resetLink, "RESETTOKEN456"},
		},
		{
			name:     "reply echoes only the token",
			failAt:   "DATA",
			reply:    "554 5.7.1 refused%.0s%.0s token RESETTOKEN456 looks like spam",
			send:     func() error { return SendResetEmail(rcpt, resetLink) },
			wantCode: "554",
			wantText: "failed to close data writer",
			hidden:   []string{"RESETTOKEN456"},
		},
		{
			name:     "reply echoes the query-encoded link",
			failAt:   "DATA",
			reply:    "554 5.7.1 refused%.0s%.0s body had " + strings.ReplaceAll(url.QueryEscape(resetLink), "%", "%%"),
			send:     func() error { return SendResetEmail(rcpt, resetLink) },
			wantCode: "554",
			wantText: "failed to close data writer",
			hidden:   []string{url.QueryEscape(resetLink), "ramble.example.org%2Freset%3Ftoken%3D"},
		},
		{
			name:     "reply echoes the path-encoded link",
			failAt:   "DATA",
			reply:    "554 5.7.1 refused%.0s%.0s body had " + strings.ReplaceAll(url.PathEscape(resetLink), "%", "%%"),
			send:     func() error { return SendResetEmail(rcpt, resetLink) },
			wantCode: "554",
			wantText: "failed to close data writer",
			hidden:   []string{url.PathEscape(resetLink), "ramble.example.org%2Freset%3Ftoken="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newFakeSMTP(t, tt.failAt, tt.reply)
			err := tt.send()
			if err == nil {
				t.Fatal("expected an error")
			}
			msg := err.Error()
			// The code and stage text guard against a vacuous pass: a TLS or
			// AUTH failure would carry no address either.
			if !strings.Contains(msg, tt.wantCode) {
				t.Errorf("error lacks SMTP code %s: %q", tt.wantCode, msg)
			}
			if !strings.Contains(msg, tt.wantText) {
				t.Errorf("error lacks stage text %q: %q", tt.wantText, msg)
			}
			if !strings.Contains(msg, "[redacted]") {
				t.Errorf("error lacks redaction marker: %q", msg)
			}
			for _, h := range tt.hidden {
				if strings.Contains(msg, h) {
					t.Errorf("error leaks %q: %q", h, msg)
				}
			}
		})
	}
}

// TestSend_FakeServerSuccess proves the fake drives the real send path end to end.
func TestSend_FakeServerSuccess(t *testing.T) {
	newFakeSMTP(t, "", "")
	if err := SendResetEmail("alice@example.com", "https://ramble.example.org/reset?token=t"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := SendVerificationEmail("alice@example.com", "https://ramble.example.org/verify?token=t"); err != nil {
		t.Fatalf("verification: %v", err)
	}
}

func TestRedactError(t *testing.T) {
	if got := redactError(nil, "x"); got != nil {
		t.Fatalf("nil in must give nil out, got %v", got)
	}

	tests := []struct {
		name    string
		in      string
		secrets []string
		want    string
	}{
		{"plus dots subdomain", "failed to set recipient: 550 5.1.1 <a.b+tag@mail.example.co.uk>: Recipient address rejected",
			nil, "failed to set recipient: 550 5.1.1 <[redacted]>: Recipient address rejected"},
		{"no address unchanged", "failed to connect to SMTP server: dial tcp 10.0.0.1:587: connection refused",
			nil, "failed to connect to SMTP server: dial tcp 10.0.0.1:587: connection refused"},
		{"status codes survive", "554 5.7.1 rejected", nil, "554 5.7.1 rejected"},
		{"link literal", "554 body https://x.example/reset?t=abc end", []string{"https://x.example/reset?t=abc", ""},
			"554 body [redacted] end"},
		{"two addresses", "550 a@b.com and c@d.org", nil, "550 [redacted] and [redacted]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactError(errors.New(tt.in), tt.secrets...)
			if got == nil || got.Error() != tt.want {
				t.Errorf("got %v, want %q", got, tt.want)
			}
		})
	}
}
