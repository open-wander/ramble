package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"
)

// IsLegacyAuthAllowed checks if legacy webhook authentication is enabled.
// Only returns true if ALLOW_LEGACY_WEBHOOK_AUTH environment variable is set to "true".
func IsLegacyAuthAllowed() bool {
	return os.Getenv("ALLOW_LEGACY_WEBHOOK_AUTH") == "true"
}

// ValidateTimestamp parses a Unix timestamp string and validates it's within 5 minutes of now.
// Returns nil if valid, error if timestamp is outside the allowed window or invalid format.
func ValidateTimestamp(timestampStr string) error {
	if timestampStr == "" {
		return fmt.Errorf("empty timestamp")
	}

	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp format: %w", err)
	}

	now := time.Now().Unix()
	diff := now - timestamp
	if diff < 0 {
		diff = -diff
	}

	// 5 minute window (300 seconds)
	if diff > 300 {
		return fmt.Errorf("timestamp outside allowed window")
	}

	return nil
}

// ComputeSignature computes HMAC-SHA256 signature over "timestamp.body" format.
// Returns signature in "sha256=<hex>" format.
func ComputeSignature(body []byte, timestamp string, secret string) string {
	payload := timestamp + "." + string(body)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// ValidateWebhookSignature validates the provided signature against computed signature.
// Uses constant-time comparison to prevent timing attacks.
func ValidateWebhookSignature(body []byte, signature, timestamp, secret string) bool {
	if signature == "" || secret == "" {
		return false
	}

	expected := ComputeSignature(body, timestamp, secret)

	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
}

// ValidateWebhookRequest validates a webhook request using either new (HMAC+timestamp)
// or legacy (query param) authentication.
//
// Returns:
//   - valid: true if request is authenticated
//   - isLegacy: true if legacy auth was used
//   - reason: description of validation failure (for logging only, never return to client)
func ValidateWebhookRequest(body []byte, signature, timestamp, querySecret, storedSecret string) (valid bool, isLegacy bool, reason string) {
	// Try new authentication first (signature + timestamp headers)
	if signature != "" && timestamp != "" {
		// Validate timestamp first (faster rejection for replay attacks)
		if err := ValidateTimestamp(timestamp); err != nil {
			return false, false, fmt.Sprintf("timestamp validation failed: %v", err)
		}

		// Validate signature
		if !ValidateWebhookSignature(body, signature, timestamp, storedSecret) {
			return false, false, "signature validation failed"
		}

		return true, false, ""
	}

	// Fall back to legacy authentication if enabled
	if IsLegacyAuthAllowed() && querySecret != "" {
		// Use constant-time comparison for secret comparison
		if subtle.ConstantTimeCompare([]byte(querySecret), []byte(storedSecret)) == 1 {
			return true, true, ""
		}
		return false, false, "invalid secret"
	}

	// No valid authentication method provided
	if signature == "" || timestamp == "" {
		return false, false, "missing headers"
	}

	return false, false, "authentication failed"
}
