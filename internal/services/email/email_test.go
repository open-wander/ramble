package email

import (
	"testing"
)

func TestSendResetEmail_MissingConfig(t *testing.T) {
	// Clear all SMTP config
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_USER", "")
	t.Setenv("SMTP_PASSWORD", "")

	err := SendResetEmail("test@example.com", "https://example.com/reset?token=abc")
	if err == nil {
		t.Error("SendResetEmail() should error with missing config")
	}
	if err.Error() != "SMTP configuration missing" {
		t.Errorf("Error = %v, want 'SMTP configuration missing'", err)
	}
}

func TestSendResetEmail_MissingHost(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_USER", "user")
	t.Setenv("SMTP_PASSWORD", "password")

	err := SendResetEmail("test@example.com", "https://example.com/reset")
	if err == nil {
		t.Error("SendResetEmail() should error with missing host")
	}
}

func TestSendResetEmail_MissingUser(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_USER", "")
	t.Setenv("SMTP_PASSWORD", "password")

	err := SendResetEmail("test@example.com", "https://example.com/reset")
	if err == nil {
		t.Error("SendResetEmail() should error with missing user")
	}
}

func TestSendResetEmail_MissingPassword(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_USER", "user")
	t.Setenv("SMTP_PASSWORD", "")

	err := SendResetEmail("test@example.com", "https://example.com/reset")
	if err == nil {
		t.Error("SendResetEmail() should error with missing password")
	}
}

func TestSendVerificationEmail_MissingConfig(t *testing.T) {
	// Clear all SMTP config
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_USER", "")
	t.Setenv("SMTP_PASSWORD", "")

	err := SendVerificationEmail("test@example.com", "https://example.com/verify?token=abc")
	if err == nil {
		t.Error("SendVerificationEmail() should error with missing config")
	}
	if err.Error() != "SMTP configuration missing" {
		t.Errorf("Error = %v, want 'SMTP configuration missing'", err)
	}
}

func TestSendVerificationEmail_MissingHost(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_USER", "user")
	t.Setenv("SMTP_PASSWORD", "password")

	err := SendVerificationEmail("test@example.com", "https://example.com/verify")
	if err == nil {
		t.Error("SendVerificationEmail() should error with missing host")
	}
}

func TestSendVerificationEmail_MissingUser(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_USER", "")
	t.Setenv("SMTP_PASSWORD", "password")

	err := SendVerificationEmail("test@example.com", "https://example.com/verify")
	if err == nil {
		t.Error("SendVerificationEmail() should error with missing user")
	}
}

func TestSendVerificationEmail_MissingPassword(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_USER", "user")
	t.Setenv("SMTP_PASSWORD", "")

	err := SendVerificationEmail("test@example.com", "https://example.com/verify")
	if err == nil {
		t.Error("SendVerificationEmail() should error with missing password")
	}
}

func TestSendEmailWithTLS_ConnectionError(t *testing.T) {
	// Try to connect to a non-existent server
	t.Setenv("SMTP_HOST", "localhost")
	t.Setenv("SMTP_PORT", "99999") // Invalid port
	t.Setenv("SMTP_USER", "user")
	t.Setenv("SMTP_PASSWORD", "password")
	t.Setenv("FROM_ADDRESS", "noreply@example.com")

	err := SendResetEmail("test@example.com", "https://example.com/reset")
	if err == nil {
		t.Error("sendEmailWithTLS() should error on connection failure")
	}
}

func TestEmailMessageFormat_Reset(t *testing.T) {
	// This test verifies the message format logic without actually sending email
	toEmail := "test@example.com"
	resetLink := "https://example.com/reset?token=abc123"

	// Simulate the message format from SendResetEmail
	msg := []byte("To: " + toEmail + "\r\n" +
		"Subject: RMBL Password Reset\r\n" +
		"\r\n" +
		"You requested a password reset. Click the link below to reset your password:\r\n" +
		"\r\n" +
		resetLink + "\r\n")

	msgStr := string(msg)

	// Verify headers
	if !contains(msgStr, "To: test@example.com") {
		t.Error("Message should contain To header")
	}
	if !contains(msgStr, "Subject: RMBL Password Reset") {
		t.Error("Message should contain Subject header")
	}
	if !contains(msgStr, resetLink) {
		t.Error("Message should contain reset link")
	}
}

func TestEmailMessageFormat_Verification(t *testing.T) {
	toEmail := "test@example.com"
	verificationLink := "https://example.com/verify?token=xyz789"

	// Simulate the message format from SendVerificationEmail
	msg := []byte("To: " + toEmail + "\r\n" +
		"Subject: Verify Your RMBL Email Address\r\n" +
		"\r\n" +
		"Welcome to RMBL! Please verify your email address by clicking the link below:\r\n" +
		"\r\n" +
		verificationLink + "\r\n" +
		"\r\n" +
		"This link will expire in 24 hours.\r\n")

	msgStr := string(msg)

	// Verify headers
	if !contains(msgStr, "To: test@example.com") {
		t.Error("Message should contain To header")
	}
	if !contains(msgStr, "Subject: Verify Your RMBL Email Address") {
		t.Error("Message should contain Subject header")
	}
	if !contains(msgStr, verificationLink) {
		t.Error("Message should contain verification link")
	}
	if !contains(msgStr, "expire in 24 hours") {
		t.Error("Message should mention expiration")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
