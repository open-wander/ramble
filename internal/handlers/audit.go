package handlers

import (
	"encoding/json"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"

	"github.com/gofiber/fiber/v2"
)

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
		Str("ip", c.IP())

	if details != nil {
		logEvent = logEvent.Interface("details", details)
	}

	logEvent.Msg("audit")

	// Log to database
	audit := models.AuditLog{
		Action:     action,
		ActorID:    actorID,
		ActorName:  actorName,
		TargetType: targetType,
		TargetID:   targetID,
		TargetName: targetName,
		Details:    detailsJSON,
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	}
	database.DB.Create(&audit)
}

// AuditLogNoContext records an action without request context (for background jobs)
func AuditLogNoContext(action string, actorID uint, actorName string, targetType string, targetID uint, targetName string, details map[string]interface{}) {
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

	logEvent.Msg("audit")

	// Log to database
	audit := models.AuditLog{
		Action:     action,
		ActorID:    actorID,
		ActorName:  actorName,
		TargetType: targetType,
		TargetID:   targetID,
		TargetName: targetName,
		Details:    detailsJSON,
	}
	database.DB.Create(&audit)
}
