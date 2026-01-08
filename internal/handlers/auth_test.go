package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "Valid password",
			password: "SecurePass123!",
			wantErr:  false,
		},
		{
			name:     "Too short",
			password: "Short1!",
			wantErr:  true,
			errMsg:   "at least 12 characters",
		},
		{
			name:     "No uppercase",
			password: "securepass123!",
			wantErr:  true,
			errMsg:   "uppercase letter",
		},
		{
			name:     "No lowercase",
			password: "SECUREPASS123!",
			wantErr:  true,
			errMsg:   "lowercase letter",
		},
		{
			name:     "No number",
			password: "SecurePassword!",
			wantErr:  true,
			errMsg:   "number",
		},
		{
			name:     "No special character",
			password: "SecurePass1234",
			wantErr:  true,
			errMsg:   "special character",
		},
		{
			name:     "Exactly 12 chars valid",
			password: "Abcdefgh123!",
			wantErr:  false,
		},
		{
			name:     "Complex valid password",
			password: "MyP@ssw0rd!2024",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.password)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRequireAuth_Unauthenticated(t *testing.T) {
	app := setupTestApp()

	app.Use(RequireAuth)
	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.SendString("protected content")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode) // Redirect to login
	assert.Contains(t, resp.Header.Get("Location"), "/login")
}

func TestRequireAuth_Authenticated(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "authuser1")

	app := fiber.New()
	InitSession()

	// Set up session with user_id
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		return c.Next()
	})

	app.Use(RequireAuth)
	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.SendString("protected content")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRequireVerifiedEmail_Unverified(t *testing.T) {
	defer cleanupTestData(t)

	// Create unverified user
	user := models.User{
		Username:      "unverified1",
		Email:         "unverified1@test.com",
		Name:          "Unverified User",
		EmailVerified: false,
	}
	err := database.DB.Create(&user).Error
	require.NoError(t, err)

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Use(RequireVerifiedEmail)
	app.Post("/action", func(c *fiber.Ctx) error {
		return c.SendString("action completed")
	})

	req := httptest.NewRequest("POST", "/action", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Unverified users are redirected to home with flash message
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/", resp.Header.Get("Location"))
}

func TestRequireVerifiedEmail_Verified(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "verified1") // createTestUser sets EmailVerified: true

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Use(RequireVerifiedEmail)
	app.Post("/action", func(c *fiber.Ctx) error {
		return c.SendString("action completed")
	})

	req := httptest.NewRequest("POST", "/action", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestLogout(t *testing.T) {
	app := setupTestApp()
	app.Get("/logout", Logout)

	req := httptest.NewRequest("GET", "/logout", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/", resp.Header.Get("Location"))
}

func TestPostLogin_InvalidCredentials(t *testing.T) {
	app := setupTestApp()
	app.Post("/login", PostLogin)

	payload := strings.NewReader("email=nonexistent@test.com&password=WrongPass123!")
	req := httptest.NewRequest("POST", "/login", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPostLogin_WrongPassword(t *testing.T) {
	defer cleanupTestData(t)

	// Create user with known password
	app := setupTestApp()
	app.Post("/signup", PostSignup)
	app.Post("/login", PostLogin)

	// First signup
	signupPayload := strings.NewReader("username=logintest&name=Login+Test&email=logintest@test.com&password=CorrectPass123!")
	signupReq := httptest.NewRequest("POST", "/signup", signupPayload)
	signupReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.Test(signupReq)

	// Try login with wrong password
	loginPayload := strings.NewReader("email=logintest@test.com&password=WrongPass123!")
	loginReq := httptest.NewRequest("POST", "/login", loginPayload)
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(loginReq)

	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPostSignup_WeakPassword(t *testing.T) {
	app := setupTestApp()
	app.Post("/signup", PostSignup)

	// Try with weak password
	payload := strings.NewReader("username=weakuser&name=Weak+User&email=weak@test.com&password=weak")
	req := httptest.NewRequest("POST", "/signup", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostSignup_DuplicateUsername(t *testing.T) {
	defer cleanupTestData(t)

	app := setupTestApp()
	app.Post("/signup", PostSignup)

	// First signup
	payload1 := strings.NewReader("username=dupuser&name=First+User&email=first@test.com&password=SecurePass123!")
	req1 := httptest.NewRequest("POST", "/signup", payload1)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp1, _ := app.Test(req1)
	assert.Equal(t, 200, resp1.StatusCode)

	// Try duplicate username
	payload2 := strings.NewReader("username=dupuser&name=Second+User&email=second@test.com&password=SecurePass123!")
	req2 := httptest.NewRequest("POST", "/signup", payload2)
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp2, err := app.Test(req2)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp2.StatusCode)
}

func TestPostSignup_DuplicateEmail(t *testing.T) {
	defer cleanupTestData(t)

	app := setupTestApp()
	app.Post("/signup", PostSignup)

	// First signup
	payload1 := strings.NewReader("username=emailuser1&name=First+User&email=duplicate@test.com&password=SecurePass123!")
	req1 := httptest.NewRequest("POST", "/signup", payload1)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp1, _ := app.Test(req1)
	assert.Equal(t, 200, resp1.StatusCode)

	// Try duplicate email
	payload2 := strings.NewReader("username=emailuser2&name=Second+User&email=duplicate@test.com&password=SecurePass123!")
	req2 := httptest.NewRequest("POST", "/signup", payload2)
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp2, err := app.Test(req2)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp2.StatusCode)
}

// Password Reset Tests

func TestPostForgotPassword_UserNotFound(t *testing.T) {
	app := setupTestApp()
	app.Post("/forgot-password", PostForgotPassword)

	payload := strings.NewReader("email=nonexistent@test.com")
	req := httptest.NewRequest("POST", "/forgot-password", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Should still return 200 to prevent email enumeration
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPostForgotPassword_Success(t *testing.T) {
	defer cleanupTestData(t)

	// Create user with password (not OAuth)
	user := models.User{
		Username:      "resetuser",
		Email:         "resetuser@test.com",
		Name:          "Reset User",
		EmailVerified: true,
		PasswordHash:  "somehash", // Has a password set
	}
	err := database.DB.Create(&user).Error
	require.NoError(t, err)

	app := setupTestApp()
	app.Post("/forgot-password", PostForgotPassword)

	payload := strings.NewReader("email=resetuser@test.com")
	req := httptest.NewRequest("POST", "/forgot-password", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err = app.Test(req)

	assert.NoError(t, err)
	// Note: Returns 500 in test env because SMTP is not configured
	// But the reset token should still be set in the database
	// In production with SMTP configured, this would return 200

	// Verify reset token was set (the important security check)
	var updated models.User
	database.DB.First(&updated, user.ID)
	assert.NotEmpty(t, updated.ResetToken, "Reset token should be set")
	assert.True(t, updated.ResetTokenExpires.After(time.Now()), "Token expiry should be in the future")
}

func TestPostForgotPassword_OAuthAccount(t *testing.T) {
	defer cleanupTestData(t)

	// Create OAuth-only user (no password)
	user := models.User{
		Username:      "oauthuser",
		Email:         "oauthuser@test.com",
		Name:          "OAuth User",
		EmailVerified: true,
		Provider:      "github",
		PasswordHash:  "", // No password set
	}
	err := database.DB.Create(&user).Error
	require.NoError(t, err)

	app := setupTestApp()
	app.Post("/forgot-password", PostForgotPassword)

	payload := strings.NewReader("email=oauthuser@test.com")
	req := httptest.NewRequest("POST", "/forgot-password", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Should return 400 for OAuth-only accounts
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostResetPassword_InvalidToken(t *testing.T) {
	app := setupTestApp()
	app.Post("/reset-password", PostResetPassword)

	payload := strings.NewReader("token=invalidtoken&password=NewSecurePass123!")
	req := httptest.NewRequest("POST", "/reset-password", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostResetPassword_ExpiredToken(t *testing.T) {
	defer cleanupTestData(t)

	// Create user with expired reset token
	token := "testtoken123"
	hashedToken := sha256.Sum256([]byte(token))
	user := models.User{
		Username:          "expireduser",
		Email:             "expireduser@test.com",
		Name:              "Expired User",
		EmailVerified:     true,
		PasswordHash:      "somehash",
		ResetToken:        hex.EncodeToString(hashedToken[:]),
		ResetTokenExpires: time.Now().Add(-1 * time.Hour), // Expired
	}
	err := database.DB.Create(&user).Error
	require.NoError(t, err)

	app := setupTestApp()
	app.Post("/reset-password", PostResetPassword)

	payload := strings.NewReader("token=" + token + "&password=NewSecurePass123!")
	req := httptest.NewRequest("POST", "/reset-password", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostResetPassword_WeakPassword(t *testing.T) {
	defer cleanupTestData(t)

	token := "validtoken456"
	hashedToken := sha256.Sum256([]byte(token))
	user := models.User{
		Username:          "weakpwuser",
		Email:             "weakpwuser@test.com",
		Name:              "Weak PW User",
		EmailVerified:     true,
		PasswordHash:      "somehash",
		ResetToken:        hex.EncodeToString(hashedToken[:]),
		ResetTokenExpires: time.Now().Add(1 * time.Hour), // Valid
	}
	err := database.DB.Create(&user).Error
	require.NoError(t, err)

	app := setupTestApp()
	app.Post("/reset-password", PostResetPassword)

	payload := strings.NewReader("token=" + token + "&password=weak")
	req := httptest.NewRequest("POST", "/reset-password", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostResetPassword_Success(t *testing.T) {
	defer cleanupTestData(t)

	token := "successtoken789"
	hashedToken := sha256.Sum256([]byte(token))
	user := models.User{
		Username:          "successuser",
		Email:             "successuser@test.com",
		Name:              "Success User",
		EmailVerified:     true,
		PasswordHash:      "oldhash",
		ResetToken:        hex.EncodeToString(hashedToken[:]),
		ResetTokenExpires: time.Now().Add(1 * time.Hour),
	}
	err := database.DB.Create(&user).Error
	require.NoError(t, err)

	app := setupTestApp()
	app.Post("/reset-password", PostResetPassword)

	payload := strings.NewReader("token=" + token + "&password=NewSecurePass123!")
	req := httptest.NewRequest("POST", "/reset-password", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify password was changed and token cleared
	var updated models.User
	database.DB.First(&updated, user.ID)
	assert.NotEqual(t, "oldhash", updated.PasswordHash)
	assert.Empty(t, updated.ResetToken)
}

// PostResendVerification Tests

func TestPostResendVerification_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	app.Post("/resend-verification", PostResendVerification)

	req := httptest.NewRequest("POST", "/resend-verification", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPostResendVerification_AlreadyVerified(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "verifiedresend") // createTestUser sets EmailVerified: true

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Post("/resend-verification", PostResendVerification)

	req := httptest.NewRequest("POST", "/resend-verification", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostResendVerification_Success(t *testing.T) {
	defer cleanupTestData(t)

	// Create unverified user
	user := models.User{
		Username:      "unverifiedresend",
		Email:         "unverifiedresend@test.com",
		Name:          "Unverified Resend",
		EmailVerified: false,
	}
	err := database.DB.Create(&user).Error
	require.NoError(t, err)

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Post("/resend-verification", PostResendVerification)

	req := httptest.NewRequest("POST", "/resend-verification", nil)
	_, err = app.Test(req)

	assert.NoError(t, err)
	// Note: Returns 500 in test env because SMTP is not configured
	// But the verification token should still be set

	// Verify token was set
	var updated models.User
	database.DB.First(&updated, user.ID)
	assert.NotEmpty(t, updated.VerificationToken, "Verification token should be set")
}

// GetVerifyEmail Tests

func TestGetVerifyEmail_MissingToken(t *testing.T) {
	app := setupTestApp()
	app.Get("/verify", GetVerifyEmail)

	req := httptest.NewRequest("GET", "/verify", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Should redirect to home
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/", resp.Header.Get("Location"))
}

func TestGetVerifyEmail_InvalidToken(t *testing.T) {
	app := setupTestApp()
	app.Get("/verify", GetVerifyEmail)

	req := httptest.NewRequest("GET", "/verify?token=invalidtoken", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Should redirect to home with error
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/", resp.Header.Get("Location"))
}

func TestGetVerifyEmail_ExpiredToken(t *testing.T) {
	defer cleanupTestData(t)

	token := "expiredverifytoken"
	hashedToken := sha256.Sum256([]byte(token))
	user := models.User{
		Username:                 "expiredverify",
		Email:                    "expiredverify@test.com",
		Name:                     "Expired Verify",
		EmailVerified:            false,
		VerificationToken:        hex.EncodeToString(hashedToken[:]),
		VerificationTokenExpires: time.Now().Add(-1 * time.Hour), // Expired
	}
	err := database.DB.Create(&user).Error
	require.NoError(t, err)

	app := setupTestApp()
	app.Get("/verify", GetVerifyEmail)

	req := httptest.NewRequest("GET", "/verify?token="+token, nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Should redirect to home with error
	assert.Equal(t, 302, resp.StatusCode)

	// Verify user is still unverified
	var updated models.User
	database.DB.First(&updated, user.ID)
	assert.False(t, updated.EmailVerified)
}

func TestGetVerifyEmail_Success(t *testing.T) {
	defer cleanupTestData(t)

	token := "validverifytoken"
	hashedToken := sha256.Sum256([]byte(token))
	user := models.User{
		Username:                 "verifyuser",
		Email:                    "verifyuser@test.com",
		Name:                     "Verify User",
		EmailVerified:            false,
		VerificationToken:        hex.EncodeToString(hashedToken[:]),
		VerificationTokenExpires: time.Now().Add(1 * time.Hour), // Valid
	}
	err := database.DB.Create(&user).Error
	require.NoError(t, err)

	app := setupTestApp()
	app.Get("/verify", GetVerifyEmail)

	req := httptest.NewRequest("GET", "/verify?token="+token, nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Should redirect to home with success
	assert.Equal(t, 302, resp.StatusCode)

	// Verify user is now verified and token cleared
	var updated models.User
	database.DB.First(&updated, user.ID)
	assert.True(t, updated.EmailVerified)
	assert.Empty(t, updated.VerificationToken)
}

// RequireVerifiedEmail tests for OAuth users (always verified)

func TestRequireVerifiedEmail_OAuthUser(t *testing.T) {
	defer cleanupTestData(t)

	// OAuth users are always considered verified
	user := models.User{
		Username:      "oauthtestuser",
		Email:         "oauth@test.com",
		Name:          "OAuth Test",
		EmailVerified: false, // Even if false
		Provider:      "github", // But has OAuth provider
	}
	database.DB.Create(&user)

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Get("/protected", RequireVerifiedEmail, func(c *fiber.Ctx) error {
		return c.SendString("allowed")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode) // OAuth users always pass
}

func TestRequireVerifiedEmail_NoUserLocal(t *testing.T) {
	app := fiber.New()
	InitSession()

	// Don't set c.Locals("User")
	app.Get("/protected", RequireVerifiedEmail, func(c *fiber.Ctx) error {
		return c.SendString("allowed")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode) // Should redirect to login
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

// Additional Password validation tests

func TestPostSignup_NoUppercase(t *testing.T) {
	app := setupTestApp()
	app.Post("/signup", PostSignup)

	// No uppercase letters
	payload := strings.NewReader("username=noupperuser&name=Test&email=noupper@test.com&password=alllowercase123!")
	req := httptest.NewRequest("POST", "/signup", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostSignup_NoSpecialChar(t *testing.T) {
	app := setupTestApp()
	app.Post("/signup", PostSignup)

	// No special characters
	payload := strings.NewReader("username=nospecialuser&name=Test&email=nospecial@test.com&password=NoSpecialChar123")
	req := httptest.NewRequest("POST", "/signup", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}
