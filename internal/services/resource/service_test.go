package resource

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rmbl/internal/database"
	"rmbl/internal/models"
)

func TestMain(m *testing.M) {
	// Setup Test DB (Container)
	_, cleanup := database.SetupTestDB()

	// Run tests
	code := m.Run()

	// Cleanup container
	cleanup()

	os.Exit(code)
}

// createTestUser creates a test user and returns it (cleanup via defer in test)
func createTestUser(t *testing.T, username string) models.User {
	user := models.User{
		Username: username,
		Email:    username + "@test.example.com",
	}
	err := database.DB.Create(&user).Error
	require.NoError(t, err)
	return user
}

func TestCreateResource_Success(t *testing.T) {
	service := NewResourceService(database.DB)

	// Create test user
	user := createTestUser(t, "testuser")
	defer database.DB.Delete(&user)

	// Test input
	input := CreateInput{
		Name:          "test-resource",
		Type:          "pack",
		Version:       "v1.0.0",
		Description:   "Test resource",
		RepositoryURL: "https://github.com/test/test-resource",
		License:       "MIT",
		Tags:          "test, demo",
	}

	// Execute
	resource, err := service.CreateResource(input, user.ID, nil)

	// Verify
	require.NoError(t, err)
	assert.NotNil(t, resource)
	assert.Equal(t, "test-resource", resource.Name)
	assert.Equal(t, models.ResourceTypePack, resource.Type)
	assert.Equal(t, user.ID, resource.UserID)
	assert.Equal(t, "MIT", resource.License)
	assert.NotEmpty(t, resource.WebhookSecret)
	assert.Len(t, resource.Versions, 1)
	assert.Equal(t, "v1.0.0", resource.Versions[0].Version)
	assert.Greater(t, len(resource.Tags), 0)

	// Cleanup
	database.DB.Delete(&resource)
}

func TestCreateResource_DuplicateName(t *testing.T) {
	service := NewResourceService(database.DB)

	user := createTestUser(t, "dupuser")
	defer database.DB.Delete(&user)

	// Create first resource
	input := CreateInput{
		Name:    "duplicate",
		Type:    "pack",
		Version: "v1.0.0",
	}
	res1, err := service.CreateResource(input, user.ID, nil)
	require.NoError(t, err)
	defer database.DB.Delete(&res1)

	// Try to create duplicate
	res2, err := service.CreateResource(input, user.ID, nil)

	// Verify error
	assert.Error(t, err)
	assert.Nil(t, res2)
	assert.Contains(t, err.Error(), "already exists")
}

func TestCreateResource_EmptyName(t *testing.T) {
	service := NewResourceService(database.DB)

	user := createTestUser(t, "emptyuser")
	defer database.DB.Delete(&user)

	// Empty name input
	input := CreateInput{
		Name:    "",
		Type:    "pack",
		Version: "v1.0.0",
	}

	res, err := service.CreateResource(input, user.ID, nil)

	// Verify validation error
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "required")
}

func TestDeleteResource_AsOwner(t *testing.T) {
	service := NewResourceService(database.DB)

	user := createTestUser(t, "owner")
	defer database.DB.Delete(&user)

	// Create resource
	input := CreateInput{
		Name:    "delete-test",
		Type:    "pack",
		Version: "v1.0.0",
	}
	resource, err := service.CreateResource(input, user.ID, nil)
	require.NoError(t, err)

	// Delete as owner
	err = service.DeleteResource(resource.ID, user.ID)

	// Verify success
	assert.NoError(t, err)

	// Verify resource is deleted
	var found models.NomadResource
	err = database.DB.First(&found, resource.ID).Error
	assert.Error(t, err) // Should not find deleted resource
}

func TestDeleteResource_NotOwner(t *testing.T) {
	service := NewResourceService(database.DB)

	owner := createTestUser(t, "resourceowner")
	defer database.DB.Delete(&owner)

	nonOwner := createTestUser(t, "notowner")
	defer database.DB.Delete(&nonOwner)

	// Create resource as owner
	input := CreateInput{
		Name:    "protected-resource",
		Type:    "pack",
		Version: "v1.0.0",
	}
	resource, err := service.CreateResource(input, owner.ID, nil)
	require.NoError(t, err)
	defer database.DB.Delete(&resource)

	// Try to delete as non-owner
	err = service.DeleteResource(resource.ID, nonOwner.ID)

	// Verify error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestToggleStar_StarAndUnstar(t *testing.T) {
	service := NewResourceService(database.DB)

	user := createTestUser(t, "staruser")
	defer database.DB.Delete(&user)

	// Create resource
	input := CreateInput{
		Name:    "star-test",
		Type:    "pack",
		Version: "v1.0.0",
	}
	resource, err := service.CreateResource(input, user.ID, nil)
	require.NoError(t, err)
	defer database.DB.Delete(&resource)

	// Star the resource
	isStarred, count, err := service.ToggleStar(resource.ID, user.ID)
	require.NoError(t, err)
	assert.True(t, isStarred)
	assert.Equal(t, int64(1), count)

	// Unstar the resource
	isStarred, count, err = service.ToggleStar(resource.ID, user.ID)
	require.NoError(t, err)
	assert.False(t, isStarred)
	assert.Equal(t, int64(0), count)
}

func TestGenerateWebhookSecret(t *testing.T) {
	service := NewResourceService(database.DB)

	secret, err := service.GenerateWebhookSecret()

	require.NoError(t, err)
	assert.NotEmpty(t, secret)
	assert.Equal(t, 64, len(secret)) // 32 bytes hex encoded = 64 characters
}

func TestProcessTagEvent_NewVersion(t *testing.T) {
	service := NewResourceService(database.DB)

	user := createTestUser(t, "taguser")
	defer database.DB.Delete(&user)

	// Create resource
	input := CreateInput{
		Name:    "tag-test",
		Type:    "pack",
		Version: "v1.0.0",
	}
	resource, err := service.CreateResource(input, user.ID, nil)
	require.NoError(t, err)
	defer database.DB.Delete(&resource)

	// Process new tag event
	created, err := service.ProcessTagEvent(resource.ID, "v2.0.0")

	require.NoError(t, err)
	assert.True(t, created)

	// Verify version was created
	var version models.ResourceVersion
	err = database.DB.Where("resource_id = ? AND version = ?", resource.ID, "v2.0.0").First(&version).Error
	require.NoError(t, err)
	assert.Equal(t, "v2.0.0", version.Version)
}

func TestProcessTagEvent_ExistingVersion(t *testing.T) {
	service := NewResourceService(database.DB)

	user := createTestUser(t, "existuser")
	defer database.DB.Delete(&user)

	// Create resource with initial version
	input := CreateInput{
		Name:    "exist-test",
		Type:    "pack",
		Version: "v1.0.0",
	}
	resource, err := service.CreateResource(input, user.ID, nil)
	require.NoError(t, err)
	defer database.DB.Delete(&resource)

	// Process tag event for existing version
	created, err := service.ProcessTagEvent(resource.ID, "v1.0.0")

	require.NoError(t, err)
	assert.False(t, created) // Should not create duplicate
}

// TestRecordFetchFailed_SizeLimit verifies that when a required remote file
// exceeds the size limit the fetch status is recorded as FAILED and no success
// content is stored.  This satisfies FR-4 (US-002): oversized downloads must
// not produce a completed fetch record.
func TestRecordFetchFailed_SizeLimit(t *testing.T) {
	service := NewResourceService(database.DB)

	user := createTestUser(t, "sizelimituser")
	defer database.DB.Delete(&user)

	// Create resource and a version that will simulate a failed oversized fetch.
	input := CreateInput{
		Name:    "size-limit-test",
		Type:    "pack",
		Version: "v1.0.0",
	}
	res, err := service.CreateResource(input, user.ID, nil)
	require.NoError(t, err)
	defer database.DB.Delete(&res)

	// Simulate the error message produced by handlers.readLimited when
	// MaxRemoteFileBytes is exceeded.
	sizeErrMsg := "remote file blocked: exceeds maximum allowed size of 1048576 bytes"

	// RecordFetchFailed is what the fetch pipeline calls when downloadFile
	// returns a size-limit error for a REQUIRED file (job or metadata.hcl).
	err = service.RecordFetchFailed(res.ID, "v1.0.0", sizeErrMsg)
	require.NoError(t, err)

	// Reload the version from the DB.
	var version models.ResourceVersion
	err = database.DB.
		Where("resource_id = ? AND version = ?", res.ID, "v1.0.0").
		First(&version).Error
	require.NoError(t, err)

	// FR-4: fetch status must be FAILED.
	assert.Equal(t, models.FetchStatusFailed, version.FetchStatus,
		"oversized fetch must be recorded as FAILED")

	// The error field must capture the size-limit message.
	assert.Contains(t, version.FetchError, "exceeds maximum allowed size",
		"fetch_error must describe the size violation")

	// No partial content must be stored as a successful fetch.
	assert.Empty(t, version.Content,
		"content must remain empty when the fetch is marked FAILED")
	assert.Empty(t, version.Readme,
		"readme must remain empty when the fetch is marked FAILED")

	// FetchCompletedAt must be set (not nil) so the UI can show a timestamp.
	assert.NotNil(t, version.FetchCompletedAt,
		"fetch_completed_at must be set on failure")
}

// TestAuthorizeResourceAction covers the centralized authorization helper.
func TestAuthorizeResourceAction(t *testing.T) {
	svc := NewResourceService(database.DB)

	// Setup: two users, one org
	owner := createTestUser(t, "authzsvc_owner")
	member := createTestUser(t, "authzsvc_member")
	nonMember := createTestUser(t, "authzsvc_nonmember")
	creator := createTestUser(t, "authzsvc_creator") // org resource creator but not a member
	defer func() {
		database.DB.Exec("DELETE FROM memberships WHERE user_id IN (?,?,?,?)", owner.ID, member.ID, nonMember.ID, creator.ID)
		database.DB.Exec("DELETE FROM nomad_resources WHERE user_id IN (?,?,?,?)", owner.ID, member.ID, nonMember.ID, creator.ID)
		database.DB.Exec("DELETE FROM organizations WHERE name = 'authzsvc-org'")
		database.DB.Exec("DELETE FROM users WHERE id IN (?,?,?,?)", owner.ID, member.ID, nonMember.ID, creator.ID)
	}()

	org := models.Organization{Name: "authzsvc-org"}
	require.NoError(t, database.DB.Create(&org).Error)

	// owner is org owner, member is org member, creator is NOT a member
	database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: RoleOwner})
	database.DB.Create(&models.Membership{UserID: member.ID, OrganizationID: org.ID, Role: RoleMember})

	// org resource: created by creator (non-member) but owned by the org
	orgInput := CreateInput{Name: "authzsvc-org-res", Type: "pack", Version: "v1.0.0"}
	orgRes, err := svc.CreateResource(orgInput, creator.ID, &org.ID)
	require.NoError(t, err)
	defer database.DB.Delete(orgRes)

	// personal resource: owned by owner user, no org
	personalInput := CreateInput{Name: "authzsvc-personal-res", Type: "pack", Version: "v1.0.0"}
	personalRes, err := svc.CreateResource(personalInput, owner.ID, nil)
	require.NoError(t, err)
	defer database.DB.Delete(personalRes)

	tests := []struct {
		name         string
		resourceID   uint
		userID       uint
		requireOwner bool
		wantErr      error
	}{
		// Org resource: member + requireOwner=false -> allowed
		{
			name:         "org member edit allowed",
			resourceID:   orgRes.ID,
			userID:       member.ID,
			requireOwner: false,
			wantErr:      nil,
		},
		// Org resource: org owner + requireOwner=false -> allowed
		{
			name:         "org owner edit allowed",
			resourceID:   orgRes.ID,
			userID:       owner.ID,
			requireOwner: false,
			wantErr:      nil,
		},
		// Org resource: org owner + requireOwner=true -> allowed
		{
			name:         "org owner delete allowed",
			resourceID:   orgRes.ID,
			userID:       owner.ID,
			requireOwner: true,
			wantErr:      nil,
		},
		// Org resource: member + requireOwner=true -> ErrUnauthorized
		{
			name:         "org member delete blocked",
			resourceID:   orgRes.ID,
			userID:       member.ID,
			requireOwner: true,
			wantErr:      ErrUnauthorized,
		},
		// Org resource: non-member + requireOwner=false -> ErrUnauthorized
		{
			name:         "org non-member edit blocked",
			resourceID:   orgRes.ID,
			userID:       nonMember.ID,
			requireOwner: false,
			wantErr:      ErrUnauthorized,
		},
		// Org resource: creator who is NOT a member -> ErrUnauthorized (org branch wins)
		{
			name:         "org resource creator-not-member blocked",
			resourceID:   orgRes.ID,
			userID:       creator.ID,
			requireOwner: false,
			wantErr:      ErrUnauthorized,
		},
		// Personal resource: owning user + requireOwner=false -> allowed
		{
			name:         "personal owner edit allowed",
			resourceID:   personalRes.ID,
			userID:       owner.ID,
			requireOwner: false,
			wantErr:      nil,
		},
		// Personal resource: owning user + requireOwner=true -> allowed (owner always has full control)
		{
			name:         "personal owner delete allowed",
			resourceID:   personalRes.ID,
			userID:       owner.ID,
			requireOwner: true,
			wantErr:      nil,
		},
		// Personal resource: other user -> ErrUnauthorized
		{
			name:         "personal non-owner blocked",
			resourceID:   personalRes.ID,
			userID:       member.ID,
			requireOwner: false,
			wantErr:      ErrUnauthorized,
		},
		// Non-existent resource -> ErrResourceNotFound
		{
			name:         "nonexistent resource",
			resourceID:   999999999,
			userID:       owner.ID,
			requireOwner: false,
			wantErr:      ErrResourceNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.AuthorizeResourceAction(tt.resourceID, tt.userID, tt.requireOwner)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr, "expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

// TestRecordFetchFailed_ErrorTruncation verifies that RecordFetchFailed caps the
// stored error message at maxFetchErrorLen runes, never splits a multibyte rune,
// and stores short messages verbatim.
func TestRecordFetchFailed_ErrorTruncation(t *testing.T) {
	service := NewResourceService(database.DB)

	// A multibyte rune (3 bytes each in UTF-8) placed exactly at the cut point
	// to prove no mid-rune split occurs.  We build a string that is
	// maxFetchErrorLen+1 runes long with a 3-byte rune near the boundary.
	longASCII := strings.Repeat("a", maxFetchErrorLen-2) // 998 ASCII runes
	// Place a 3-byte rune (U+4E16, "world" in CJK) at position 999, then one
	// more ASCII char to push the total past the cap.
	longWithMultibyte := longASCII + "世z" // 1000 runes total — NOT over the cap
	// Add one more character so it IS over the cap.
	overCapMsg := longWithMultibyte + "!" // 1001 runes — must be truncated

	tests := []struct {
		name          string
		errMsg        string
		wantExact     string // non-empty: stored value must equal this exactly
		wantMaxRunes  bool   // if true: stored value must be <= maxFetchErrorLen runes
		wantValidUTF8 bool   // if true: stored value must be valid UTF-8
	}{
		{
			name:      "short message stored verbatim",
			errMsg:    "fetch failed: connection refused",
			wantExact: "fetch failed: connection refused",
		},
		{
			name:          "long message is truncated to cap with multibyte rune safety",
			errMsg:        overCapMsg,
			wantMaxRunes:  true,
			wantValidUTF8: true,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := createTestUser(t, fmt.Sprintf("truncuser%d", i))
			defer database.DB.Delete(&user)

			input := CreateInput{
				Name:    fmt.Sprintf("trunc-test-%d", i),
				Type:    "pack",
				Version: "v1.0.0",
			}
			res, err := service.CreateResource(input, user.ID, nil)
			require.NoError(t, err)
			defer database.DB.Delete(&res)

			err = service.RecordFetchFailed(res.ID, "v1.0.0", tt.errMsg)
			require.NoError(t, err)

			var version models.ResourceVersion
			err = database.DB.
				Where("resource_id = ? AND version = ?", res.ID, "v1.0.0").
				First(&version).Error
			require.NoError(t, err)

			stored := version.FetchError

			if tt.wantExact != "" {
				assert.Equal(t, tt.wantExact, stored, "short message must be stored verbatim")
			}
			if tt.wantMaxRunes {
				runeCount := utf8.RuneCountInString(stored)
				assert.LessOrEqual(t, runeCount, maxFetchErrorLen,
					"stored error must not exceed maxFetchErrorLen runes; got %d", runeCount)
			}
			if tt.wantValidUTF8 {
				assert.True(t, utf8.ValidString(stored),
					"stored error must be valid UTF-8")
			}
			// The fetch must still be recorded as FAILED regardless of truncation.
			assert.Equal(t, models.FetchStatusFailed, version.FetchStatus)
		})
	}
}

// TestRecordFetchFailed_DoesNotOverwriteContent verifies that RecordFetchFailed
// updates only the status/error/timestamp columns and does not wipe existing
// content if it were somehow already written (defensive check).
func TestRecordFetchFailed_DoesNotOverwriteContent(t *testing.T) {
	service := NewResourceService(database.DB)

	user := createTestUser(t, "nooverwriteuser")
	defer database.DB.Delete(&user)

	input := CreateInput{
		Name:    "no-overwrite-test",
		Type:    "job",
		Version: "v1.0.0",
	}
	res, err := service.CreateResource(input, user.ID, nil)
	require.NoError(t, err)
	defer database.DB.Delete(&res)

	// Pre-populate content (simulating a partial fetch that stored something).
	database.DB.Model(&models.ResourceVersion{}).
		Where("resource_id = ? AND version = ?", res.ID, "v1.0.0").
		Update("content", "pre-existing content")

	// Record failure for the oversized second attempt.
	err = service.RecordFetchFailed(res.ID, "v1.0.0", "remote file blocked: exceeds maximum allowed size of 1048576 bytes")
	require.NoError(t, err)

	var version models.ResourceVersion
	err = database.DB.
		Where("resource_id = ? AND version = ?", res.ID, "v1.0.0").
		First(&version).Error
	require.NoError(t, err)

	// Status must be FAILED.
	assert.Equal(t, models.FetchStatusFailed, version.FetchStatus)

	// RecordFetchFailed does not clear content — the invariant is that
	// downloadFile returns "" on size error so content is never written
	// from an oversized body. The DB field reflects what was stored before.
	assert.Equal(t, "pre-existing content", version.Content,
		"RecordFetchFailed must not clear pre-existing content columns")
}
