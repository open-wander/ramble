package email

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// smtpRootCAs is the trust pool for the STARTTLS handshake. nil means the
// system roots; tests set it to trust an in-process fake server.
var smtpRootCAs *x509.CertPool

// addrPattern matches addr-spec-like tokens (local@domain) in error text.
var addrPattern = regexp.MustCompile(`[^\s<>()\[\],;:"@]+@[^\s<>()\[\],;:"]+`)

// redactError returns an error whose text has every email address and every
// given secret literal (such as the reset link) replaced with [redacted].
// SMTP servers echo addresses in their replies and callers log the error.
// The result deliberately does not wrap err: an Unwrap would hand the
// unredacted *textproto.Error to anything walking the chain, so errors.Is/As
// on the original are not supported (no caller uses them).
// sendSecrets lists the literals a send error must not carry: the link, its
// token on its own (a server may echo only part of the link), and both
// addresses verbatim (covers forms addrPattern misses, such as quoted local parts).
func sendSecrets(link, to, from string) []string {
	secrets := []string{link, to, from}
	if u, err := url.Parse(link); err == nil {
		for _, vs := range u.Query() {
			secrets = append(secrets, vs...)
		}
	}
	return secrets
}

func redactError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	text := err.Error()
	for _, s := range secrets {
		if s != "" {
			text = strings.ReplaceAll(text, s, "[redacted]")
		}
	}
	return errors.New(addrPattern.ReplaceAllString(text, "[redacted]"))
}

func SendResetEmail(toEmail string, resetLink string) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("FROM_ADDRESS")

	if host == "" || user == "" || password == "" {
		return fmt.Errorf("SMTP configuration missing")
	}

	msg := []byte(fmt.Sprintf("To: %s\r\n"+
		"Subject: RMBL Password Reset\r\n"+
		"\r\n"+
		"You requested a password reset. Click the link below to reset your password:\r\n"+
		"\r\n"+
		"%s\r\n", toEmail, resetLink))

	return redactError(sendEmailWithTLS(host, port, user, password, from, []string{toEmail}, msg), sendSecrets(resetLink, toEmail, from)...)
}

func SendVerificationEmail(toEmail string, verificationLink string) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("FROM_ADDRESS")

	if host == "" || user == "" || password == "" {
		return fmt.Errorf("SMTP configuration missing")
	}

	msg := []byte(fmt.Sprintf("To: %s\r\n"+
		"Subject: Verify Your RMBL Email Address\r\n"+
		"\r\n"+
		"Welcome to RMBL! Please verify your email address by clicking the link below:\r\n"+
		"\r\n"+
		"%s\r\n"+
		"\r\n"+
		"This link will expire in 24 hours.\r\n", toEmail, verificationLink))

	return redactError(sendEmailWithTLS(host, port, user, password, from, []string{toEmail}, msg), sendSecrets(verificationLink, toEmail, from)...)
}

// validateEmail checks that an email address is well-formed and safe for SMTP use.
func validateEmail(addr string) (string, error) {
	parsed, err := mail.ParseAddress(addr)
	if err != nil {
		return "", fmt.Errorf("invalid email address: %w", err)
	}
	return parsed.Address, nil
}

// sendEmailWithTLS sends email using STARTTLS for secure transmission
func sendEmailWithTLS(host, port, user, password, from string, to []string, msg []byte) error {
	// Validate email addresses to prevent SMTP header injection
	sanitizedFrom, err := validateEmail(from)
	if err != nil {
		return fmt.Errorf("invalid sender address: %w", err)
	}
	from = sanitizedFrom

	for i, recipient := range to {
		sanitized, err := validateEmail(recipient)
		if err != nil {
			return fmt.Errorf("invalid recipient address: %w", err)
		}
		to[i] = sanitized
	}

	addr := net.JoinHostPort(host, port)

	// TLS configuration with minimum TLS 1.2
	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
		RootCAs:    smtpRootCAs,
	}

	// Connect to SMTP server
	conn, err := net.Dial("tcp", addr) // #nosec G704 -- addr is from server config, not user input
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	// Create SMTP client
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// Start TLS
	if err = client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("STARTTLS failed: %w", err)
	}

	// Authenticate
	auth := smtp.PlainAuth("", user, password, host)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Set sender (from is validated by validateEmail above)
	if err = client.Mail(from); err != nil { // #nosec G707 -- from is validated via mail.ParseAddress above
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipients
	for _, recipient := range to {
		if err = client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to set recipient: %w", err)
		}
	}

	// Send message
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}

	_, err = w.Write(msg)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	err = w.Close()
	if err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return client.Quit()
}
