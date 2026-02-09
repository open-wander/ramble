package handlers

import (
	"rmbl/internal/models"

	"github.com/gofiber/fiber/v2"
)

// RequireAuth middleware checks if user is authenticated
func RequireAuth(c *fiber.Ctx) error {
	sess, err := Store.Get(c)
	if err != nil || sess.Get("user_id") == nil {
		return c.Redirect("/login")
	}
	return c.Next()
}

// RequireVerifiedEmail ensures the user has verified their email
func RequireVerifiedEmail(c *fiber.Ctx) error {
	userLoc := c.Locals("User")
	if userLoc == nil {
		return c.Redirect("/login")
	}

	user := userLoc.(models.User)

	// OAuth users are always verified
	if user.Provider != "" {
		return c.Next()
	}

	// Check if email is verified
	if !user.EmailVerified {
		SetFlash(c, "error", "Please verify your email address before performing this action. Check your inbox for the verification link.")
		return c.Redirect("/")
	}

	return c.Next()
}
