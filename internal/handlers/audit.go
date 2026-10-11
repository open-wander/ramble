package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"

	"github.com/gofiber/fiber/v2"
)

// computeAuditChecksum generates a SHA-256 checksum of all audit log fields for integrity
func computeAuditChecksum(audit *models.AuditLog) string {
	data := fmt.Sprintf("%s|%d|%s|%s|%d|%s|%s|%s|%s|%s|%s",
		audit.CreatedAt.Format(time.RFC3339Nano),
		audit.ActorID,
		audit.ActorName,
		audit.Action,
		audit.TargetID,
		audit.TargetName,
		audit.TargetType,
		audit.Details,
		audit.IPAddress,
		audit.UserAgent,
		audit.RequestID,
	)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// sensitiveKeyParts are substrings of a normalised (lowercase, '-' to '_') detail
// key whose value must never be logged or stored. Substring matching catches
// webhook_secret, reset_token, x-api-key; it also over-redacts e.g. "keyword",
// which is the safe direction.
var sensitiveKeyParts = []string{
	"password", "passwd", "secret", "token", "api_key", "apikey", "key",
	"authorization", "cookie", "session", "csrf", "signature",
}

func isSensitiveKey(k string) bool {
	k = strings.ReplaceAll(strings.ToLower(k), "-", "_")
	for _, p := range sensitiveKeyParts {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}

// redactValue returns a copy of v with sensitive map entries replaced,
// recursing into maps and slices. The caller's data is never modified.
func redactValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		return redactDetails(t)
	case fiber.Map: // named type: does not match the case above
		return redactDetails(t)
	case map[string]string:
		out := make(map[string]string, len(t))
		for k, val := range t {
			if isSensitiveKey(k) {
				out[k] = "[REDACTED]"
			} else {
				out[k] = val
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, e := range t {
			out[i] = redactValue(e)
		}
		return out
	case []map[string]interface{}:
		out := make([]map[string]interface{}, len(t))
		for i, e := range t {
			out[i] = redactDetails(e)
		}
		return out
	}
	return v
}

// redactDetails returns a copy of m with values under sensitive keys replaced
// by "[REDACTED]". nil stays nil.
func redactDetails(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if isSensitiveKey(k) {
			out[k] = "[REDACTED]"
		} else {
			out[k] = redactValue(v)
		}
	}
	return out
}

// AuditLog records an action to both structured logs and database
func AuditLog(c *fiber.Ctx, action string, targetType string, targetID uint, targetName string, details map[string]interface{}) {
	var actorID uint
	var actorName string

	if c.Locals("UserID") != nil {
		actorID = c.Locals("UserID").(uint)
		if user, ok := c.Locals("User").(models.User); ok {
			actorName = user.Username
		}
	}

	// Get request ID for tracing
	requestID := ""
	if rid := c.Locals("requestid"); rid != nil {
		requestID = rid.(string)
	}

	details = redactDetails(details)
	detailsJSON := ""
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			detailsJSON = string(b)
		}
	}

	// Log to structured logger
	logEvent := logger.Log.Info().
		Str("action", action).
		Uint("actor_id", actorID).
		Str("actor_name", actorName).
		Str("target_type", targetType).
		Uint("target_id", targetID).
		Str("target_name", targetName).
		Str("ip", c.IP()).
		Str("request_id", requestID)

	if details != nil {
		logEvent = logEvent.Interface("details", details)
	}

	logEvent.Msg("audit.recorded")

	// Log to database
	audit := models.AuditLog{
		CreatedAt:  time.Now(),
		Action:     action,
		ActorID:    actorID,
		ActorName:  actorName,
		TargetType: targetType,
		TargetID:   targetID,
		TargetName: targetName,
		Details:    detailsJSON,
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
		RequestID:  requestID,
	}
	audit.Checksum = computeAuditChecksum(&audit)
	reqDB(c).Create(&audit)
}

// AuditLogNoContext records an action without request context (for background jobs)
func AuditLogNoContext(action string, actorID uint, actorName string, targetType string, targetID uint, targetName string, details map[string]interface{}) {
	details = redactDetails(details)
	detailsJSON := ""
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			detailsJSON = string(b)
		}
	}

	// Log to structured logger
	logEvent := logger.Log.Info().
		Str("action", action).
		Uint("actor_id", actorID).
		Str("actor_name", actorName).
		Str("target_type", targetType).
		Uint("target_id", targetID).
		Str("target_name", targetName)

	if details != nil {
		logEvent = logEvent.Interface("details", details)
	}

	logEvent.Msg("audit.recorded")

	// Log to database
	audit := models.AuditLog{
		CreatedAt:  time.Now(),
		Action:     action,
		ActorID:    actorID,
		ActorName:  actorName,
		TargetType: targetType,
		TargetID:   targetID,
		TargetName: targetName,
		Details:    detailsJSON,
	}
	audit.Checksum = computeAuditChecksum(&audit)
	database.DB.Create(&audit)
}
