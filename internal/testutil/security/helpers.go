package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// ComputeValidSignature computes HMAC-SHA256 signature over "timestamp.body" format.
// Returns signature in "sha256=<hex>" format.
// Used for generating valid test signatures.
func ComputeValidSignature(body []byte, timestamp string, secret string) string {
	payload := timestamp + "." + string(body)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// GenerateExpiredTimestamp returns a Unix timestamp more than 5 minutes in the past.
// Used for testing replay attack protection.
func GenerateExpiredTimestamp() string {
	// 6 minutes in the past (360 seconds)
	expired := time.Now().Unix() - 360
	return fmt.Sprintf("%d", expired)
}

// GenerateFutureTimestamp returns a Unix timestamp more than 5 minutes in the future.
// Used for testing clock skew protection.
func GenerateFutureTimestamp() string {
	// 6 minutes in the future (360 seconds)
	future := time.Now().Unix() + 360
	return fmt.Sprintf("%d", future)
}

// GenerateValidTimestamp returns the current Unix timestamp.
// Used for generating valid test timestamps.
func GenerateValidTimestamp() string {
	return fmt.Sprintf("%d", time.Now().Unix())
}
