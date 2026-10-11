package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// computeTestSignature computes HMAC-SHA256 signature over "timestamp.body" format
func computeTestSignature(body []byte, timestamp, secret string) string {
	payload := fmt.Sprintf("%s.%s", timestamp, body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// toString converts uint to string for URL params
func toString(id uint) string {
	return fmt.Sprintf("%d", id)
}
