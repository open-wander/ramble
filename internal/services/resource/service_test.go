package resource

import (
	"os"
	"testing"

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
