package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestIsLegacyAuthAllowed(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     bool
	}{
		{
			name:     "enabled when true",
			envValue: "true",
			want:     true,
		},
		{
			name:     "disabled when false",
			envValue: "false",
			want:     false,
		},
		{
			name:     "disabled when empty",
			envValue: "",
			want:     false,
		},
		{
			name:     "disabled when other value",
			envValue: "yes",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue == "" {
				os.Unsetenv("ALLOW_LEGACY_WEBHOOK_AUTH")
			} else {
				os.Setenv("ALLOW_LEGACY_WEBHOOK_AUTH", tt.envValue)
			}
			defer os.Unsetenv("ALLOW_LEGACY_WEBHOOK_AUTH")

			got := IsLegacyAuthAllowed()
			if got != tt.want {
				t.Errorf("IsLegacyAuthAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateTimestamp(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name      string
		timestamp string
		wantErr   bool
	}{
		{
			name:      "valid current timestamp",
			timestamp: fmt.Sprintf("%d", now),
			wantErr:   false,
		},
		{
			name:      "valid timestamp 4m59s in past",
			timestamp: fmt.Sprintf("%d", now-299),
			wantErr:   false,
		},
		{
			name:      "valid timestamp 4m59s in future",
			timestamp: fmt.Sprintf("%d", now+299),
			wantErr:   false,
		},
		{
			name:      "invalid timestamp 5m1s in past",
			timestamp: fmt.Sprintf("%d", now-301),
			wantErr:   true,
		},
		{
			name:      "invalid timestamp 5m1s in future",
			timestamp: fmt.Sprintf("%d", now+301),
			wantErr:   true,
		},
		{
			name:      "invalid format",
			timestamp: "not-a-number",
			wantErr:   true,
		},
		{
			name:      "empty string",
			timestamp: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimestamp(tt.timestamp)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTimestamp() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestComputeSignature(t *testing.T) {
	tests := []struct {
		name      string
		body      []byte
		timestamp string
		secret    string
		want      string
	}{
		{
			name:      "known input produces expected output",
			body:      []byte("test-body"),
			timestamp: "1234567890",
			secret:    "test-secret",
			want:      computeExpectedSignature([]byte("test-body"), "1234567890", "test-secret"),
		},
		{
			name:      "empty body",
			body:      []byte(""),
			timestamp: "1234567890",
			secret:    "test-secret",
			want:      computeExpectedSignature([]byte(""), "1234567890", "test-secret"),
		},
		{
			name:      "different timestamp changes signature",
			body:      []byte("test-body"),
			timestamp: "9876543210",
			secret:    "test-secret",
			want:      computeExpectedSignature([]byte("test-body"), "9876543210", "test-secret"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeSignature(tt.body, tt.timestamp, tt.secret)
			if got != tt.want {
				t.Errorf("ComputeSignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function to compute expected signature for testing
func computeExpectedSignature(body []byte, timestamp, secret string) string {
	payload := timestamp + "." + string(body)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

func TestValidateWebhookSignature(t *testing.T) {
	body := []byte("test-body")
	timestamp := "1234567890"
	secret := "test-secret"
	validSignature := computeExpectedSignature(body, timestamp, secret)

	tests := []struct {
		name      string
		body      []byte
		signature string
		timestamp string
		secret    string
		want      bool
	}{
		{
			name:      "valid signature",
			body:      body,
			signature: validSignature,
			timestamp: timestamp,
			secret:    secret,
			want:      true,
		},
		{
			name:      "invalid signature",
			body:      body,
			signature: "sha256=wrong",
			timestamp: timestamp,
			secret:    secret,
			want:      false,
		},
		{
			name:      "missing sha256 prefix",
			body:      body,
			signature: "deadbeef",
			timestamp: timestamp,
			secret:    secret,
			want:      false,
		},
		{
			name:      "empty signature",
			body:      body,
			signature: "",
			timestamp: timestamp,
			secret:    secret,
			want:      false,
		},
		{
			name:      "wrong secret",
			body:      body,
			signature: validSignature,
			timestamp: timestamp,
			secret:    "wrong-secret",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateWebhookSignature(tt.body, tt.signature, tt.timestamp, tt.secret)
			if got != tt.want {
				t.Errorf("ValidateWebhookSignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateWebhookRequest(t *testing.T) {
	now := time.Now().Unix()
	validTimestamp := fmt.Sprintf("%d", now)
	expiredTimestamp := fmt.Sprintf("%d", now-400) // More than 5 minutes old
	secret := "test-secret"
	body := []byte("test-body")
	validSignature := computeExpectedSignature(body, validTimestamp, secret)

	tests := []struct {
		name          string
		body          []byte
		signature     string
		timestamp     string
		querySecret   string
		storedSecret  string
		legacyEnabled bool
		wantValid     bool
		wantIsLegacy  bool
		wantReason    string
	}{
		{
			name:          "new auth success",
			body:          body,
			signature:     validSignature,
			timestamp:     validTimestamp,
			querySecret:   "",
			storedSecret:  secret,
			legacyEnabled: false,
			wantValid:     true,
			wantIsLegacy:  false,
			wantReason:    "",
		},
		{
			name:          "new auth timestamp expired",
			body:          body,
			signature:     computeExpectedSignature(body, expiredTimestamp, secret),
			timestamp:     expiredTimestamp,
			querySecret:   "",
			storedSecret:  secret,
			legacyEnabled: false,
			wantValid:     false,
			wantIsLegacy:  false,
			wantReason:    "timestamp",
		},
		{
			name:          "new auth invalid signature",
			body:          body,
			signature:     "sha256=wrong",
			timestamp:     validTimestamp,
			querySecret:   "",
			storedSecret:  secret,
			legacyEnabled: false,
			wantValid:     false,
			wantIsLegacy:  false,
			wantReason:    "signature",
		},
		{
			name:          "legacy auth enabled and valid",
			body:          body,
			signature:     "",
			timestamp:     "",
			querySecret:   secret,
			storedSecret:  secret,
			legacyEnabled: true,
			wantValid:     true,
			wantIsLegacy:  true,
			wantReason:    "",
		},
		{
			name:          "legacy auth disabled",
			body:          body,
			signature:     "",
			timestamp:     "",
			querySecret:   secret,
			storedSecret:  secret,
			legacyEnabled: false,
			wantValid:     false,
			wantIsLegacy:  false,
			wantReason:    "missing headers",
		},
		{
			name:          "legacy auth enabled but invalid secret",
			body:          body,
			signature:     "",
			timestamp:     "",
			querySecret:   "wrong-secret",
			storedSecret:  secret,
			legacyEnabled: true,
			wantValid:     false,
			wantIsLegacy:  false,
			wantReason:    "invalid secret",
		},
		{
			name:          "empty stored secret",
			body:          body,
			signature:     validSignature,
			timestamp:     validTimestamp,
			querySecret:   "",
			storedSecret:  "",
			legacyEnabled: false,
			wantValid:     false,
			wantIsLegacy:  false,
			wantReason:    "signature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.legacyEnabled {
				os.Setenv("ALLOW_LEGACY_WEBHOOK_AUTH", "true")
			} else {
				os.Unsetenv("ALLOW_LEGACY_WEBHOOK_AUTH")
			}
			defer os.Unsetenv("ALLOW_LEGACY_WEBHOOK_AUTH")

			gotValid, gotIsLegacy, gotReason := ValidateWebhookRequest(
				tt.body,
				tt.signature,
				tt.timestamp,
				tt.querySecret,
				tt.storedSecret,
			)

			if gotValid != tt.wantValid {
				t.Errorf("ValidateWebhookRequest() valid = %v, want %v", gotValid, tt.wantValid)
			}
			if gotIsLegacy != tt.wantIsLegacy {
				t.Errorf("ValidateWebhookRequest() isLegacy = %v, want %v", gotIsLegacy, tt.wantIsLegacy)
			}
			if tt.wantReason != "" && gotReason == "" {
				t.Errorf("ValidateWebhookRequest() expected reason containing %q, got empty", tt.wantReason)
			}
		})
	}
}

func TestWebhookAuthEdgeCases(t *testing.T) {
	now := time.Now().Unix()
	secret := "test-secret"
	body := []byte("test-body")

	t.Run("clock skew boundary - 4m59s past should pass", func(t *testing.T) {
		timestamp := fmt.Sprintf("%d", now-299)
		err := ValidateTimestamp(timestamp)
		if err != nil {
			t.Errorf("ValidateTimestamp() with 4m59s past should pass, got error: %v", err)
		}
	})

	t.Run("clock skew boundary - 4m59s future should pass", func(t *testing.T) {
		timestamp := fmt.Sprintf("%d", now+299)
		err := ValidateTimestamp(timestamp)
		if err != nil {
			t.Errorf("ValidateTimestamp() with 4m59s future should pass, got error: %v", err)
		}
	})

	t.Run("clock skew boundary - 5m1s past should fail", func(t *testing.T) {
		timestamp := fmt.Sprintf("%d", now-301)
		err := ValidateTimestamp(timestamp)
		if err == nil {
			t.Error("ValidateTimestamp() with 5m1s past should fail")
		}
	})

	t.Run("clock skew boundary - 5m1s future should fail", func(t *testing.T) {
		timestamp := fmt.Sprintf("%d", now+301)
		err := ValidateTimestamp(timestamp)
		if err == nil {
			t.Error("ValidateTimestamp() with 5m1s future should fail")
		}
	})

	t.Run("empty secret handling - returns invalid", func(t *testing.T) {
		validTimestamp := fmt.Sprintf("%d", now)
		signature := ComputeSignature(body, validTimestamp, secret)

		valid := ValidateWebhookSignature(body, signature, validTimestamp, "")
		if valid {
			t.Error("ValidateWebhookSignature() with empty secret should return false")
		}
	})

	t.Run("empty timestamp string - returns error", func(t *testing.T) {
		err := ValidateTimestamp("")
		if err == nil {
			t.Error("ValidateTimestamp() with empty string should return error")
		}
	})

	t.Run("malformed signature - non-hex after prefix", func(t *testing.T) {
		validTimestamp := fmt.Sprintf("%d", now)
		malformedSig := "sha256=notahexstring!!!"

		valid := ValidateWebhookSignature(body, malformedSig, validTimestamp, secret)
		if valid {
			t.Error("ValidateWebhookSignature() with malformed signature should return false")
		}
	})

	t.Run("legacy auth transition - from allowed to disallowed", func(t *testing.T) {
		// First, test with legacy enabled
		os.Setenv("ALLOW_LEGACY_WEBHOOK_AUTH", "true")
		valid, isLegacy, _ := ValidateWebhookRequest(body, "", "", secret, secret)
		if !valid || !isLegacy {
			t.Error("Legacy auth should work when enabled")
		}

		// Now disable and verify it fails
		os.Unsetenv("ALLOW_LEGACY_WEBHOOK_AUTH")
		valid, isLegacy, _ = ValidateWebhookRequest(body, "", "", secret, secret)
		if valid || isLegacy {
			t.Error("Legacy auth should fail when disabled")
		}
	})

	t.Run("reason strings contain no sensitive data", func(t *testing.T) {
		validTimestamp := fmt.Sprintf("%d", now)
		expiredTimestamp := fmt.Sprintf("%d", now-400)

		// Test timestamp failure reason
		_, _, reason := ValidateWebhookRequest(
			body,
			computeExpectedSignature(body, expiredTimestamp, secret),
			expiredTimestamp,
			"",
			secret,
		)
		if reason == "" {
			t.Error("Expected non-empty reason for timestamp validation failure")
		}
		// Verify no secret in reason - simple substring check
		if len(reason) > 0 && len(secret) > 0 {
			for i := 0; i <= len(reason)-len(secret); i++ {
				if reason[i:i+len(secret)] == secret {
					t.Errorf("Reason string should not contain secret: %s", reason)
				}
			}
		}

		// Test signature failure reason
		_, _, reason = ValidateWebhookRequest(
			body,
			"sha256=wrong",
			validTimestamp,
			"",
			secret,
		)
		if reason == "" {
			t.Error("Expected non-empty reason for signature validation failure")
		}
		// Verify no secret in reason
		if len(reason) > 0 && len(secret) > 0 {
			for i := 0; i <= len(reason)-len(secret); i++ {
				if reason[i:i+len(secret)] == secret {
					t.Errorf("Reason string should not contain secret: %s", reason)
				}
			}
		}
	})
}
