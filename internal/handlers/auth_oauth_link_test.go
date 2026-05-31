package handlers

import (
	"errors"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"testing"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOAuthCallback_AlreadyLinkedProviderLogin verifies that a user with a
// pre-existing provider+providerID match is logged in as that same user and
// no new row is inserted.
func TestOAuthCallback_AlreadyLinkedProviderLogin(t *testing.T) {
	defer cleanupTestData(t)

	existing := models.User{
		Username:      "ghlinked1",
		Email:         "ghlinked1@test.com",
		Name:          "GH Linked",
		Provider:      "github",
		ProviderID:    "gh-123",
		EmailVerified: true,
	}
	require.NoError(t, database.DB.Create(&existing).Error)

	gothUser := goth.User{
		Provider:    "github",
		UserID:      "gh-123",
		Email:       "ghlinked1@test.com",
		Name:        "GH Linked",
		NickName:    "ghlinked1",
		AccessToken: "tok",
	}

	var countBefore int64
	database.DB.Model(&models.User{}).Count(&countBefore)

	result, err := processOAuthUser(gothUser)
	require.NoError(t, err)

	// Should log in as the existing user.
	assert.Equal(t, existing.ID, result.ID, "should return the pre-existing user")
	assert.Equal(t, "github", result.Provider)
	assert.Equal(t, "gh-123", result.ProviderID)

	// No new row should have been created.
	var countAfter int64
	database.DB.Model(&models.User{}).Count(&countAfter)
	assert.Equal(t, countBefore, countAfter, "no new user row should be created on already-linked login")
}

// TestOAuthCallback_BrandNewUser verifies that when there is no provider match
// and no email collision, a brand-new user row is created with Provider and
// ProviderID correctly set.
func TestOAuthCallback_BrandNewUser(t *testing.T) {
	defer cleanupTestData(t)

	gothUser := goth.User{
		Provider:    "github",
		UserID:      "gh-new-999",
		Email:       "newghoauth@test.com",
		Name:        "New OAuth User",
		NickName:    "newoauthuser",
		AccessToken: "tok",
	}

	var countBefore int64
	database.DB.Model(&models.User{}).Count(&countBefore)

	result, err := processOAuthUser(gothUser)
	require.NoError(t, err)

	// A new row should exist.
	var countAfter int64
	database.DB.Model(&models.User{}).Count(&countAfter)
	assert.Equal(t, countBefore+1, countAfter, "exactly one new user row should be created")

	// The new user should have Provider/ProviderID set.
	assert.Equal(t, "github", result.Provider)
	assert.Equal(t, "gh-new-999", result.ProviderID)
	assert.Equal(t, "newghoauth@test.com", result.Email)

	// Verify from DB.
	var dbUser models.User
	require.NoError(t, database.DB.First(&dbUser, result.ID).Error)
	assert.Equal(t, "github", dbUser.Provider)
	assert.Equal(t, "gh-new-999", dbUser.ProviderID)
}

// TestOAuthCallback_CrossProviderCollisionRejected verifies that when an account
// already linked to a DIFFERENT provider (e.g. github) exists with the same email,
// a login attempt via a second provider (e.g. gitlab) returns errOAuthEmailCollision,
// leaves the existing row untouched, and does NOT create a new row.
func TestOAuthCallback_CrossProviderCollisionRejected(t *testing.T) {
	defer cleanupTestData(t)

	// Seed an existing account already linked to github.
	existing := models.User{
		Username:      "ghuser-xprov",
		Email:         "xprov@test.com",
		Name:          "GH User",
		Provider:      "github",
		ProviderID:    "gh-1",
		EmailVerified: true,
	}
	require.NoError(t, database.DB.Create(&existing).Error)

	// Attempt login via gitlab with the same email but a different provider+ID.
	gothUser := goth.User{
		Provider:    "gitlab",
		UserID:      "gl-9",
		Email:       "xprov@test.com",
		Name:        "GL User",
		NickName:    "gluser",
		AccessToken: "tok",
	}

	_, err := processOAuthUser(gothUser)

	// Must return the collision error — not a generic DB error and not nil.
	require.Error(t, err, "cross-provider email collision should produce an error")
	assert.True(t, errors.Is(err, errOAuthEmailCollision), "error must wrap errOAuthEmailCollision, got: %v", err)

	// The existing github row must be completely unchanged.
	var reloaded models.User
	require.NoError(t, database.DB.First(&reloaded, existing.ID).Error)
	assert.Equal(t, "github", reloaded.Provider, "Provider must remain github after collision rejection")
	assert.Equal(t, "gh-1", reloaded.ProviderID, "ProviderID must remain gh-1 after collision rejection")

	// No new row should have been created — still exactly one row with this email.
	var count int64
	database.DB.Model(&models.User{}).Where("email = ?", "xprov@test.com").Count(&count)
	assert.Equal(t, int64(1), count, "exactly one row with this email should exist after rejection")
}

// TestOAuthCallback_EmailCollisionRejected verifies that when a local account
// already exists with the same email and NO provider linkage, the OAuth callback
// does NOT silently link the account. It must return an error, and the existing
// row's Provider/ProviderID must remain unchanged.
func TestOAuthCallback_EmailCollisionRejected(t *testing.T) {
	defer cleanupTestData(t)

	// Seed a local-auth user (no provider).
	localUser := models.User{
		Username:      "localonly1",
		Email:         "collision@test.com",
		Name:          "Local Only",
		PasswordHash:  "somehash",
		EmailVerified: true,
		Provider:      "", // no OAuth provider
		ProviderID:    "",
	}
	require.NoError(t, database.DB.Create(&localUser).Error)

	gothUser := goth.User{
		Provider:    "github",
		UserID:      "gh-attacker-456",
		Email:       "collision@test.com", // same email
		Name:        "Attacker",
		NickName:    "attacker",
		AccessToken: "tok",
	}

	_, err := processOAuthUser(gothUser)

	// Must return an error signalling the collision.
	require.Error(t, err, "email collision should produce an error, not silently link")
	assert.Contains(t, err.Error(), "existing account", "error should mention the existing account")

	// The existing user's Provider and ProviderID must be unchanged.
	var reloaded models.User
	require.NoError(t, database.DB.First(&reloaded, localUser.ID).Error)
	assert.Empty(t, reloaded.Provider, "Provider must remain empty after collision rejection")
	assert.Empty(t, reloaded.ProviderID, "ProviderID must remain empty after collision rejection")

	// No new user should have been created.
	var count int64
	database.DB.Model(&models.User{}).Where("email = ?", "collision@test.com").Count(&count)
	assert.Equal(t, int64(1), count, "exactly one row with this email should exist")
}
