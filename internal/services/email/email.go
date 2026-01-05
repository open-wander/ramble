package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
)

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

	return sendEmailWithTLS(host, port, user, password, from, []string{toEmail}, msg)
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

	return sendEmailWithTLS(host, port, user, password, from, []string{toEmail}, msg)
}

// sendEmailWithTLS sends email using STARTTLS for secure transmission
func sendEmailWithTLS(host, port, user, password, from string, to []string, msg []byte) error {
	addr := net.JoinHostPort(host, port)

	// TLS configuration with minimum TLS 1.2
	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}

	// Connect to SMTP server
	conn, err := net.Dial("tcp", addr)
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

	// Set sender
	if err = client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipients
	for _, recipient := range to {
		if err = client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", recipient, err)
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
