package handlers

import (
	"fmt"
	"net/http/httptest"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAuthzTestOrg creates an org and returns it.
func setupAuthzTestOrg(t *testing.T, name string) models.Organization {
	t.Helper()
	org := models.Organization{
		Name:        name,
		Description: "Authz test org",
	}
	require.NoError(t, database.DB.Create(&org).Error)
	return org
}

// addMember adds a user to an org with the given role.
func addMember(t *testing.T, userID, orgID uint, role string) {
	t.Helper()
	m := models.Membership{UserID: userID, OrganizationID: orgID, Role: role}
	require.NoError(t, database.DB.Create(&m).Error)
}

// cleanupAuthzTestData removes all test data created by the authz tests.
func cleanupAuthzTestData(t *testing.T) {
	t.Helper()
	database.DB.Exec("DELETE FROM resource_versions")
	database.DB.Exec("DELETE FROM resource_tags")
	database.DB.Exec("DELETE FROM nomad_resources")
	database.DB.Exec("DELETE FROM memberships")
	database.DB.Exec("DELETE FROM organizations WHERE name LIKE 'authzorg%'")
	database.DB.Exec("DELETE FROM users WHERE email LIKE '%@test.com'")
}

// authzApp builds an authenticated Fiber app for authz tests.
func authzApp(user models.User) *fiber.App {
	app := fiber.New()
	InitSession()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		_ = sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})
	app.Post("/new", PostNewResource)
	return app
}

// postNewResource sends a POST /new form request and returns the response.
func postNewResource(t *testing.T, app *fiber.App, owner, name string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf("name=%s&type=pack&version=v1.0.0&description=test&owner=%s", name, owner)
	req := httptest.NewRequest("POST", "/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	rec := &httptest.ResponseRecorder{Code: resp.StatusCode}
	return rec
}

// resourceExists checks whether a NomadResource with the given name and orgID exists in the DB.
func resourceExists(t *testing.T, name string, orgID *uint) bool {
	t.Helper()
	q := database.DB.Model(&models.NomadResource{}).Where("name = ?", name)
	if orgID != nil {
		q = q.Where("organization_id = ?", *orgID)
	} else {
		q = q.Where("organization_id IS NULL")
	}
	var count int64
	q.Count(&count)
	return count > 0
}

// TestPostNewResource_OrgNonMember_Forbidden verifies that a logged-in user who is NOT
// a member of the target org receives 403 and no resource row is persisted.
func TestPostNewResource_OrgNonMember_Forbidden(t *testing.T) {
	defer cleanupAuthzTestData(t)

	owner := createTestUser(t, "authznomember1")
	nonMember := createTestUser(t, "authznomember2")
	org := setupAuthzTestOrg(t, "authzorg1")
	// owner is a member of the org; nonMember is not.
	addMember(t, owner.ID, org.ID, "owner")

	app := authzApp(nonMember)
	ownerStr := fmt.Sprintf("org:%d", org.ID)
	resp := postNewResource(t, app, ownerStr, "forbidden-pack")

	assert.Equal(t, fiber.StatusForbidden, resp.Code, "non-member should receive 403")

	// No resource row should have been persisted.
	assert.False(t, resourceExists(t, "forbidden-pack", &org.ID), "resource must not be persisted for non-member")
}

// TestPostNewResource_OrgMember_Allowed verifies that a logged-in member can create a
// resource in the target org.
func TestPostNewResource_OrgMember_Allowed(t *testing.T) {
	defer cleanupAuthzTestData(t)

	member := createTestUser(t, "authzmember1")
	org := setupAuthzTestOrg(t, "authzorg2")
	addMember(t, member.ID, org.ID, "member")

	app := authzApp(member)
	ownerStr := fmt.Sprintf("org:%d", org.ID)
	resp := postNewResource(t, app, ownerStr, "member-pack")

	assert.Equal(t, fiber.StatusOK, resp.Code, "member should be allowed to create a resource")
	assert.True(t, resourceExists(t, "member-pack", &org.ID), "resource must be persisted for member")
}

// TestPostNewResource_OrgOwner_Allowed verifies that the org owner can create a resource
// (owner satisfies the member requirement).
func TestPostNewResource_OrgOwner_Allowed(t *testing.T) {
	defer cleanupAuthzTestData(t)

	owner := createTestUser(t, "authzowner1")
	org := setupAuthzTestOrg(t, "authzorg3")
	addMember(t, owner.ID, org.ID, "owner")

	app := authzApp(owner)
	ownerStr := fmt.Sprintf("org:%d", org.ID)
	resp := postNewResource(t, app, ownerStr, "owner-pack")

	assert.Equal(t, fiber.StatusOK, resp.Code, "owner should be allowed to create a resource")
	assert.True(t, resourceExists(t, "owner-pack", &org.ID), "resource must be persisted for owner")
}

// TestPostNewResource_PersonalNamespace_Allowed verifies that a logged-in user can always
// create a resource in their own personal namespace (no org owner field).
func TestPostNewResource_PersonalNamespace_Allowed(t *testing.T) {
	defer cleanupAuthzTestData(t)

	user := createTestUser(t, "authzpersonal1")

	app := authzApp(user)
	// No owner field — defaults to personal namespace.
	resp := postNewResource(t, app, "", "personal-pack")

	assert.Equal(t, fiber.StatusOK, resp.Code, "personal namespace creation should succeed")
	assert.True(t, resourceExists(t, "personal-pack", nil), "resource must be persisted in personal namespace")
}

// TestPostNewResource_NonexistentOrg_Rejected verifies that submitting an owner that
// references a non-existent org ID is rejected and no resource row is persisted.
func TestPostNewResource_NonexistentOrg_Rejected(t *testing.T) {
	defer cleanupAuthzTestData(t)

	user := createTestUser(t, "authznoorg1")

	app := authzApp(user)
	// Use a very large org ID that cannot exist.
	ownerStr := "org:999999999"
	resp := postNewResource(t, app, ownerStr, "noorg-pack")

	assert.True(t, resp.Code == fiber.StatusForbidden || resp.Code == fiber.StatusBadRequest,
		"non-existent org should be rejected (got %d)", resp.Code)

	// No resource row should be persisted.
	assert.False(t, resourceExists(t, "noorg-pack", nil), "resource must not be persisted for non-existent org")

	// Also check there is no row with any org ID.
	var count int64
	database.DB.Model(&models.NomadResource{}).Where("name = ?", "noorg-pack").Count(&count)
	assert.Equal(t, int64(0), count, "resource must not be persisted for non-existent org (any namespace)")
}
