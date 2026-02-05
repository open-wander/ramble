//go:build security

package handlers

import (
	"fmt"
	"testing"
	"time"

	"rmbl/internal/testutil/security"
)

// TestWebhookSignature_AttackMatrix validates webhook signature validation against comprehensive attack scenarios.
// This test ensures that ValidateWebhookRequest properly rejects all known attack patterns.
func TestWebhookSignature_AttackMatrix(t *testing.T) {
	secret := "test-secret"

	// Include a valid request as sanity check
	validTimestamp := security.GenerateValidTimestamp()
	validBody := []byte("test-body")
	validSignature := security.ComputeValidSignature(validBody, validTimestamp, secret)

	tests := []struct {
		name        string
		signature   string
		timestamp   string
		body        []byte
		expectValid bool
		description string
	}{
		{
			name:        "valid_request",
			signature:   validSignature,
			timestamp:   validTimestamp,
			body:        validBody,
			expectValid: true,
			description: "Sanity check - valid request should pass",
		},
	}

	// Add all attack cases from the security package
	for _, attack := range security.WebhookAttacks {
		tests = append(tests, struct {
			name        string
			signature   string
			timestamp   string
			body        []byte
			expectValid bool
			description string
		}{
			name:        attack.Name,
			signature:   attack.Signature,
			timestamp:   attack.Timestamp,
			body:        attack.Body,
			expectValid: attack.ExpectValid,
			description: attack.Description,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, _, reason := ValidateWebhookRequest(
				tt.body,
				tt.signature,
				tt.timestamp,
				"", // No query secret for these tests
				secret,
			)

			t.Logf("Attack %s: valid=%v, reason=%s", tt.name, valid, reason)

			if valid != tt.expectValid {
				t.Errorf("ValidateWebhookRequest() = %v, want %v\nDescription: %s\nReason: %s",
					valid, tt.expectValid, tt.description, reason)
			}

			// For attack cases, ensure we got a reason for rejection
			if !tt.expectValid && valid == false && reason == "" {
				t.Errorf("Expected non-empty reason for attack rejection: %s", tt.name)
			}
		})
	}
}

// TestWebhookSignature_ReplayProtection specifically tests replay attack scenarios.
// Validates that signatures with expired timestamps are properly rejected.
func TestWebhookSignature_ReplayProtection(t *testing.T) {
	secret := "test-secret"
	body := []byte("test-body")
	now := time.Now().Unix()

	tests := []struct {
		name      string
		timestamp string
		wantValid bool
		reason    string
	}{
		{
			name:      "current timestamp passes",
			timestamp: fmt.Sprintf("%d", now),
			wantValid: true,
			reason:    "Current timestamp should be valid",
		},
		{
			name:      "299 seconds past passes",
			timestamp: fmt.Sprintf("%d", now-299),
			wantValid: true,
			reason:    "Within 5 minute window (boundary -1)",
		},
		{
			name:      "300 seconds past passes",
			timestamp: fmt.Sprintf("%d", now-300),
			wantValid: true,
			reason:    "Exactly at 5 minute boundary",
		},
		{
			name:      "301 seconds past fails",
			timestamp: fmt.Sprintf("%d", now-301),
			wantValid: false,
			reason:    "Outside 5 minute window (replay attack)",
		},
		{
			name:      "299 seconds future passes",
			timestamp: fmt.Sprintf("%d", now+299),
			wantValid: true,
			reason:    "Within 5 minute window (boundary -1)",
		},
		{
			name:      "300 seconds future passes",
			timestamp: fmt.Sprintf("%d", now+300),
			wantValid: true,
			reason:    "Exactly at 5 minute boundary",
		},
		{
			name:      "301 seconds future fails",
			timestamp: fmt.Sprintf("%d", now+301),
			wantValid: false,
			reason:    "Outside 5 minute window (clock skew attack)",
		},
		{
			name:      "360 seconds past fails",
			timestamp: security.GenerateExpiredTimestamp(),
			wantValid: false,
			reason:    "Well outside window (replay attack)",
		},
		{
			name:      "360 seconds future fails",
			timestamp: security.GenerateFutureTimestamp(),
			wantValid: false,
			reason:    "Well outside window (clock skew attack)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate valid signature for the timestamp
			signature := security.ComputeValidSignature(body, tt.timestamp, secret)

			valid, _, reason := ValidateWebhookRequest(
				body,
				signature,
				tt.timestamp,
				"",
				secret,
			)

			t.Logf("Replay test %s: valid=%v, reason=%s", tt.name, valid, reason)

			if valid != tt.wantValid {
				t.Errorf("ValidateWebhookRequest() = %v, want %v\nTest reason: %s\nValidation reason: %s",
					valid, tt.wantValid, tt.reason, reason)
			}
		})
	}
}

// TestWebhookSignature_TimingConsistency documents the constant-time comparison requirement.
// Runtime timing tests are unreliable due to OS scheduling and CPU variance.
// This test serves as documentation of the security requirement.
func TestWebhookSignature_TimingConsistency(t *testing.T) {
	// NOTE: This test documents the constant-time requirement for signature validation.
	// ValidateWebhookSignature uses subtle.ConstantTimeCompare to prevent timing attacks.
	//
	// WHY constant-time is important:
	// - Timing attacks can leak information about the secret by measuring response times
	// - Early-exit string comparison leaks position of first mismatch
	// - Attackers can brute-force secrets byte-by-byte using timing measurements
	//
	// IMPLEMENTATION:
	// The ValidateWebhookSignature function uses crypto/subtle.ConstantTimeCompare
	// which is specifically designed to prevent timing side-channels.
	//
	// VERIFICATION:
	// This is verified by code review of webhook_auth.go line 65:
	//   return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
	//
	// Runtime timing tests are explicitly NOT performed because:
	// 1. OS scheduling makes timing measurements unreliable
	// 2. CPU caching and branch prediction introduce noise
	// 3. Virtual environments have non-deterministic timing
	// 4. The Go runtime scheduler adds unpredictable delays

	t.Log("Constant-time comparison requirement documented")
	t.Log("Implementation uses crypto/subtle.ConstantTimeCompare")
	t.Log("Verified by code review of webhook_auth.go")

	// Verify the function exists and is being used (compile-time check)
	secret := "test-secret"
	body := []byte("test-body")
	timestamp := security.GenerateValidTimestamp()
	validSignature := security.ComputeValidSignature(body, timestamp, secret)

	// This call exercises the constant-time comparison path
	valid := ValidateWebhookSignature(body, validSignature, timestamp, secret)
	if !valid {
		t.Error("ValidateWebhookSignature should validate correct signature")
	}

	// This call also exercises the constant-time comparison path
	invalidSignature := "sha256=0000000000000000000000000000000000000000000000000000000000000000"
	valid = ValidateWebhookSignature(body, invalidSignature, timestamp, secret)
	if valid {
		t.Error("ValidateWebhookSignature should reject invalid signature")
	}

	t.Log("Constant-time comparison code path exercised successfully")
}
