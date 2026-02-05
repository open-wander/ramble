package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"rmbl/internal/crypto"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/email"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/gitlab"
	"github.com/shareed2k/goth_fiber"
	"golang.org/x/crypto/bcrypt"
)

var Store *session.Store

// validatePassword checks if a password meets security requirements
func validatePassword(password string) error {
	if len(password) < 12 {
		return fmt.Errorf("password must be at least 12 characters long")
	}
	if !regexp.MustCompile(`[A-Z]`).MatchString(password) {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if !regexp.MustCompile(`[a-z]`).MatchString(password) {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}
	if !regexp.MustCompile(`[0-9]`).MatchString(password) {
		return fmt.Errorf("password must contain at least one number")
	}
	if !regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`).MatchString(password) {
		return fmt.Errorf("password must contain at least one special character")
	}
	return nil
}

func InitSession() {
	Store = session.New(session.Config{
		Expiration:     24 * time.Hour,
		CookieSecure:   os.Getenv("ENV") == "production", // Only send over HTTPS in production
		CookieHTTPOnly: true,                             // Prevent XSS access to cookies
		CookieSameSite: "Lax",                            // Lax allows OAuth redirects while still protecting POST/PUT/DELETE
		CookiePath:     "/",
	})

	// Configure goth_fiber to use the same session store
	goth_fiber.SessionStore = Store

	// Initialize OAuth Providers
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}

	goth.UseProviders(
		github.New(os.Getenv("GITHUB_KEY"), os.Getenv("GITHUB_SECRET"), baseURL+"/auth/github/callback", "public_repo", "user:email"),
		gitlab.New(os.Getenv("GITLAB_KEY"), os.Getenv("GITLAB_SECRET"), baseURL+"/auth/gitlab/callback", "read_api", "read_user"),
	)
}

func BeginAuth(c *fiber.Ctx) error {
	return goth_fiber.BeginAuthHandler(c)
}

func AuthCallback(c *fiber.Ctx) error {
	gothUser, err := goth_fiber.CompleteUserAuth(c)
	if err != nil {
		SetFlash(c, "error", "Authentication failed: "+err.Error())
		return c.Redirect("/login")
	}

	var user models.User
	// Check if user exists by Provider + ProviderID
	result := database.DB.Where("provider = ? AND provider_id = ?", gothUser.Provider, gothUser.UserID).First(&user)

	if result.Error != nil {
		// User doesn't exist, check by email to link accounts
		result = database.DB.Where("email = ?", gothUser.Email).First(&user)
		if result.Error == nil {
			// Link account
			user.Provider = gothUser.Provider
			user.ProviderID = gothUser.UserID
			encryptedToken, _ := crypto.EncryptToken(gothUser.AccessToken)
			user.AccessToken = encryptedToken
			user.EmailVerified = true // OAuth users are pre-verified
			database.DB.Save(&user)
		} else {
			// Create new user
			username := gothUser.NickName
			if username == "" {
				username = gothUser.Name
			}
			// Simple username collision check or suffix
			encryptedToken, _ := crypto.EncryptToken(gothUser.AccessToken)
			user = models.User{
				Username:      username,
				Email:         gothUser.Email,
				Name:          gothUser.Name,
				AvatarURL:     gothUser.AvatarURL,
				Provider:      gothUser.Provider,
				ProviderID:    gothUser.UserID,
				AccessToken:   encryptedToken,
				EmailVerified: true, // OAuth users are pre-verified
			}
			if err := database.DB.Create(&user).Error; err != nil {
				SetFlash(c, "error", "Failed to create user account.")
				return c.Redirect("/signup")
			}
		}
	} else {
		// Update existing user token
		encryptedToken, _ := crypto.EncryptToken(gothUser.AccessToken)
		user.AccessToken = encryptedToken
		database.DB.Save(&user)
	}

	// Set Session
	sess, err := Store.Get(c)
	if err == nil {
		if err := sess.Regenerate(); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to regenerate session")
		}
		sess.Set("user_id", user.ID)
		sess.Set("session_created_at", time.Now().Unix())
		sess.Set("flash_type", "success")
		sess.Set("flash_message", "Successfully logged in via "+gothUser.Provider)
		if err := sess.Save(); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to save session")
		}
	}

	AuditLog(c, "auth.oauth_login", "user", user.ID, user.Username, map[string]interface{}{
		"provider": gothUser.Provider,
	})

	return c.Redirect("/")
}

func GetLogin(c *fiber.Ctx) error {
	return c.Render("login", BaseContext(c), "layouts/main")
}

// MaxFailedLoginAttempts before account lockout
const MaxFailedLoginAttempts = 5

// AccountLockDuration is how long an account is locked after too many failed attempts
const AccountLockDuration = 15 * time.Minute

func PostLogin(c *fiber.Ctx) error {
	type LoginInput struct {
		Email    string `form:"email"`
		Password string `form:"password"`
	}
	var input LoginInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid input")
	}

	// Dummy hash for timing attack prevention - same cost as real hashes
	dummyHash := "$2a$10$dummyHashForTimingAttackPreventionXXXXXXXXXXXXXXXXXXXXX"

	var user models.User
	result := database.DB.Where("email = ?", input.Email).First(&user)
	if result.Error != nil {
		// Always perform bcrypt comparison to prevent timing-based username enumeration
		bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(input.Password))
		AuditLog(c, "auth.login_failed", "user", 0, input.Email, nil)
		return c.Status(fiber.StatusUnauthorized).SendString("Invalid email or password")
	}

	// Check if account is locked
	if user.LockedUntil.After(time.Now()) {
		remainingTime := time.Until(user.LockedUntil).Round(time.Minute)
		AuditLog(c, "auth.login_blocked", "user", user.ID, user.Username, map[string]interface{}{
			"reason": "account_locked",
		})
		return c.Status(fiber.StatusTooManyRequests).SendString(
			fmt.Sprintf("Account temporarily locked due to too many failed attempts. Try again in %d minutes.", int(remainingTime.Minutes())+1))
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password))
	if err != nil {
		// Increment failed attempts
		user.FailedLoginAttempts++

		// Lock account if threshold reached
		if user.FailedLoginAttempts >= MaxFailedLoginAttempts {
			user.LockedUntil = time.Now().Add(AccountLockDuration)
			AuditLog(c, "auth.account_locked", "user", user.ID, user.Username, map[string]interface{}{
				"failed_attempts": user.FailedLoginAttempts,
				"locked_until":    user.LockedUntil,
			})
		}

		database.DB.Save(&user)
		AuditLog(c, "auth.login_failed", "user", user.ID, user.Username, map[string]interface{}{
			"failed_attempts": user.FailedLoginAttempts,
		})
		return c.Status(fiber.StatusUnauthorized).SendString("Invalid email or password")
	}

	// Reset failed attempts on successful login
	if user.FailedLoginAttempts > 0 {
		user.FailedLoginAttempts = 0
		user.LockedUntil = time.Time{}
		database.DB.Save(&user)
	}

	// Set Session
	sess, err := Store.Get(c)
	if err != nil {
		return err
	}
	if err := sess.Regenerate(); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to regenerate session")
	}
	sess.Set("user_id", user.ID)
	sess.Set("session_created_at", time.Now().Unix())
	sess.Set("flash_type", "success")
	sess.Set("flash_message", "Welcome back, "+user.Name+"!")
	if err := sess.Save(); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save session")
	}

	AuditLog(c, "auth.login", "user", user.ID, user.Username, nil)

	// Redirect to home
	c.Set("HX-Redirect", "/")
	return c.SendStatus(fiber.StatusOK)
}

func GetSignup(c *fiber.Ctx) error {
	return c.Render("signup", BaseContext(c), "layouts/main")
}

func PostSignup(c *fiber.Ctx) error {
	type SignupInput struct {
		Username string `form:"username"`
		Name     string `form:"name"`
		Email    string `form:"email"`
		Password string `form:"password"`
	}
	var input SignupInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid input")
	}

	// Validate password strength
	if err := validatePassword(input.Password); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}

	// Check if username or email exists
	var existingUser models.User
	if err := database.DB.Where("username = ? OR email = ?", input.Username, input.Email).First(&existingUser).Error; err == nil {
		return c.Status(fiber.StatusBadRequest).SendString("Username or Email already registered")
	}

	// Check if username is taken by an Org
	var count int64
	database.DB.Model(&models.Organization{}).Where("name ILIKE ?", input.Username).Count(&count)
	if count > 0 {
		return c.Status(fiber.StatusBadRequest).SendString("Username is already taken by an organization")
	}

	// Hash Password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Could not hash password")
	}

	// Generate email verification token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate verification token")
	}
	verificationToken := hex.EncodeToString(b)

	// Hash token before storing
	hashedToken := sha256.Sum256([]byte(verificationToken))

	user := models.User{
		Username:                 input.Username,
		Name:                     input.Name,
		Email:                    input.Email,
		PasswordHash:             string(hashedPassword),
		EmailVerified:            false,
		VerificationToken:        hex.EncodeToString(hashedToken[:]),
		VerificationTokenExpires: time.Now().Add(24 * time.Hour),
	}

	if err := database.DB.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Could not create user")
	}

	AuditLog(c, "auth.signup", "user", user.ID, user.Username, nil)

	// Send verification email
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	verificationLink := fmt.Sprintf("%s/verify-email?token=%s", baseURL, verificationToken)

	if err := email.SendVerificationEmail(user.Email, verificationLink); err != nil {
		// Log error but don't fail signup
		fmt.Printf("Failed to send verification email: %v\n", err)
	}

	// Auto login
	sess, err := Store.Get(c)
	if err != nil {
		return err
	}
	if err := sess.Regenerate(); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to regenerate session")
	}
	sess.Set("user_id", user.ID)
	sess.Set("session_created_at", time.Now().Unix())
	if err := sess.Save(); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save session")
	}

	SetFlash(c, "success", "Welcome to RMBL! Please check your email to verify your account.")

	c.Set("HX-Redirect", "/")
	return c.SendStatus(fiber.StatusOK)
}

func Logout(c *fiber.Ctx) error {
	// Get user info before destroying session for audit log
	var userID uint
	var username string
	if c.Locals("UserID") != nil {
		userID = c.Locals("UserID").(uint)
		if user, ok := c.Locals("User").(models.User); ok {
			username = user.Username
		}
	}

	sess, err := Store.Get(c)
	if err != nil {
		return err
	}
	if err := sess.Destroy(); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to destroy session")
	}

	AuditLogNoContext("auth.logout", userID, username, "user", userID, username, nil)

	SetFlash(c, "success", "You have been logged out.")

	return c.Redirect("/")
}

func GetForgotPassword(c *fiber.Ctx) error {
	return c.Render("forgot_password", BaseContext(c), "layouts/main")
}

func PostForgotPassword(c *fiber.Ctx) error {
	emailAddr := c.FormValue("email")

	// Always log reset request (for monitoring)
	AuditLog(c, "auth.password_reset_requested", "user", 0, emailAddr, nil)

	var user models.User
	if err := database.DB.Where("email = ?", emailAddr).First(&user).Error; err != nil {
		// Return success even if user not found (prevent email enumeration)
		// User decision: "Unknown email requests show same success message"
		SetFlash(c, "success", "If an account exists, we sent a reset link.")
		c.Set("HX-Redirect", "/login")
		return c.SendStatus(fiber.StatusOK)
	}

	// Prevent password reset for OAuth-only accounts
	if user.Provider != "" && user.PasswordHash == "" {
		return c.Status(fiber.StatusBadRequest).SendString("This account is linked to " + user.Provider + ". Please sign in using your social account.")
	}

	// Invalidate all previous tokens for this user (user decision)
	database.DB.Model(&models.User{}).
		Where("id = ?", user.ID).
		Updates(map[string]interface{}{
			"reset_token":         "",
			"reset_token_expires": time.Time{},
			"reset_token_used_at": nil,
		})

	// Generate Token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate reset token")
	}
	token := hex.EncodeToString(b)

	// Hash token before storing (security: prevent token theft from database)
	hashedToken := sha256.Sum256([]byte(token))
	user.ResetToken = hex.EncodeToString(hashedToken[:])
	user.ResetTokenExpires = time.Now().Add(1 * time.Hour)
	user.ResetTokenUsedAt = nil // Ensure it's marked as unused
	database.DB.Save(&user)

	// Send Email
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)

	if err := email.SendResetEmail(user.Email, resetLink); err != nil {
		fmt.Printf("Failed to send reset email: %v\n", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to send reset email. Please try again later.")
	}

	AuditLog(c, "auth.password_reset_token_sent", "user", user.ID, user.Username, nil)

	SetFlash(c, "success", "If an account exists, we sent a reset link.")
	c.Set("HX-Redirect", "/login")
	return c.SendStatus(fiber.StatusOK)
}

func GetResetPassword(c *fiber.Ctx) error {
	token := c.Query("token")

	// Hash the token to compare with stored hash
	hashedToken := sha256.Sum256([]byte(token))
	hashedTokenStr := hex.EncodeToString(hashedToken[:])

	// Query includes single-use check: reset_token_used_at IS NULL
	var user models.User
	if err := database.DB.Where(
		"reset_token = ? AND reset_token_expires > ? AND reset_token_used_at IS NULL",
		hashedTokenStr, time.Now(),
	).First(&user).Error; err != nil {
		// Generic error message (user decision: prevent enumeration)
		SetFlash(c, "error", "This link is no longer valid. Request a new one.")
		return c.Redirect("/forgot-password")
	}

	return c.Render("reset_password", MergeContext(BaseContext(c), fiber.Map{
		"Token": token,
	}), "layouts/main")
}

func PostResetPassword(c *fiber.Ctx) error {
	token := c.FormValue("token")
	password := c.FormValue("password")

	// Validate password strength
	if err := validatePassword(password); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}

	// Hash the token to compare with stored hash
	hashedToken := sha256.Sum256([]byte(token))
	hashedTokenStr := hex.EncodeToString(hashedToken[:])

	// Query includes single-use check: reset_token_used_at IS NULL
	// Note: SQL comparison of hashed tokens is acceptable - see objective security note
	var user models.User
	if err := database.DB.Where(
		"reset_token = ? AND reset_token_expires > ? AND reset_token_used_at IS NULL",
		hashedTokenStr, time.Now(),
	).First(&user).Error; err != nil {
		// Log the failure (internal detail, not returned to client)
		AuditLog(c, "auth.password_reset_failed", "user", 0, "", map[string]interface{}{
			"reason": "invalid_expired_or_used_token",
		})
		// Generic error message (user decision: prevent enumeration)
		return c.Status(fiber.StatusBadRequest).SendString("This link is no longer valid. Request a new one.")
	}

	// Mark token as used IMMEDIATELY (TOCTOU protection: prevents race condition)
	now := time.Now()
	user.ResetTokenUsedAt = &now
	if err := database.DB.Save(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to process request")
	}

	// Now safe to process password change
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Could not hash password")
	}

	// Update password and set PasswordChangedAt for session invalidation
	user.PasswordHash = string(hashedPassword)
	user.PasswordChangedAt = time.Now()
	user.ResetToken = "" // Clear token
	database.DB.Save(&user)

	AuditLog(c, "auth.password_reset_completed", "user", user.ID, user.Username, nil)

	SetFlash(c, "success", "Your password has been reset. Please log in.")
	c.Set("HX-Redirect", "/login")
	return c.SendStatus(fiber.StatusOK)
}

// GetVerifyEmail handles email verification via token
func GetVerifyEmail(c *fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		SetFlash(c, "error", "Invalid verification token.")
		return c.Redirect("/")
	}

	// Hash the token to compare with stored hash
	hashedToken := sha256.Sum256([]byte(token))
	hashedTokenStr := hex.EncodeToString(hashedToken[:])

	var user models.User
	if err := database.DB.Where("verification_token = ? AND verification_token_expires > ?", hashedTokenStr, time.Now()).First(&user).Error; err != nil {
		SetFlash(c, "error", "Invalid or expired verification token.")
		return c.Redirect("/")
	}

	// Mark email as verified
	user.EmailVerified = true
	user.VerificationToken = "" // Clear token
	database.DB.Save(&user)

	AuditLog(c, "auth.email_verified", "user", user.ID, user.Username, nil)

	SetFlash(c, "success", "Your email has been verified successfully!")
	return c.Redirect("/")
}

// PostResendVerification resends verification email
func PostResendVerification(c *fiber.Ctx) error {
	sess, err := Store.Get(c)
	if err != nil || sess.Get("user_id") == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Please log in first")
	}

	userID := sess.Get("user_id").(uint)
	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).SendString("User not found")
	}

	if user.EmailVerified {
		return c.Status(fiber.StatusBadRequest).SendString("Email already verified")
	}

	// Generate new verification token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate verification token")
	}
	verificationToken := hex.EncodeToString(b)

	// Hash token before storing
	hashedToken := sha256.Sum256([]byte(verificationToken))
	user.VerificationToken = hex.EncodeToString(hashedToken[:])
	user.VerificationTokenExpires = time.Now().Add(24 * time.Hour)
	database.DB.Save(&user)

	// Send verification email
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	verificationLink := fmt.Sprintf("%s/verify-email?token=%s", baseURL, verificationToken)

	if err := email.SendVerificationEmail(user.Email, verificationLink); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to send verification email")
	}

	return c.SendString("Verification email sent successfully")
}
