package handlers

import (
	"net/http/httptest"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

// cleanupOrgTestData removes org-related test data
func cleanupOrgTestData(t *testing.T) {
	database.DB.Exec("DELETE FROM memberships")
	database.DB.Exec("DELETE FROM organizations WHERE name LIKE 'test%'")
	cleanupTestData(t)
}

func TestPostCreateOrg_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	// Route should be behind RequireAuth middleware
	app.Post("/orgs/new", RequireAuth, PostCreateOrg)

	payload := strings.NewReader("name=testorg&description=Test+Org")
	req := httptest.NewRequest("POST", "/orgs/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Should redirect to login
	assert.Equal(t, 302, resp.StatusCode)
}

func TestPostCreateOrg_Success(t *testing.T) {
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "orgcreator")

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

	app.Post("/orgs/new", PostCreateOrg)

	payload := strings.NewReader("name=testorg1&description=Test+Organization")
	req := httptest.NewRequest("POST", "/orgs/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify org was created
	var org models.Organization
	err = database.DB.Where("name = ?", "testorg1").First(&org).Error
	assert.NoError(t, err)
	assert.Equal(t, "Test Organization", org.Description)

	// Verify membership was created
	var membership models.Membership
	err = database.DB.Where("user_id = ? AND organization_id = ?", user.ID, org.ID).First(&membership).Error
	assert.NoError(t, err)
	assert.Equal(t, "owner", membership.Role)
}

func TestPostCreateOrg_DuplicateName(t *testing.T) {
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "orgcreator2")

	// Create org first
	org := models.Organization{Name: "testexisting", Description: "Existing Org"}
	database.DB.Create(&org)

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

	app.Post("/orgs/new", PostCreateOrg)

	// Try to create org with same name
	payload := strings.NewReader("name=testexisting&description=Duplicate")
	req := httptest.NewRequest("POST", "/orgs/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostCreateOrg_NameConflictWithUser(t *testing.T) {
	defer cleanupOrgTestData(t)

	existingUser := createTestUser(t, "testuserorg")
	creator := createTestUser(t, "orgcreator3")

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", creator.ID)
		sess.Save()
		c.Locals("UserID", creator.ID)
		c.Locals("User", creator)
		return c.Next()
	})

	app.Post("/orgs/new", PostCreateOrg)

	// Try to create org with same name as existing user
	payload := strings.NewReader("name=" + existingUser.Username + "&description=Conflict")
	req := httptest.NewRequest("POST", "/orgs/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestRequireOrgOwner_NotMember(t *testing.T) {
	defer cleanupOrgTestData(t)

	owner := createTestUser(t, "orgowner")
	nonMember := createTestUser(t, "nonmember")

	org := models.Organization{Name: "testprivateorg", Description: "Private"}
	database.DB.Create(&org)

	// Make owner a member
	database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"})

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", nonMember.ID)
		sess.Save()
		c.Locals("UserID", nonMember.ID)
		c.Locals("User", nonMember)
		return c.Next()
	})

	app.Get("/orgs/:orgname/settings", RequireOrgOwner, GetOrgSettings)

	req := httptest.NewRequest("GET", "/orgs/testprivateorg/settings", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestRequireOrgOwner_IsOwner(t *testing.T) {
	defer cleanupOrgTestData(t)

	owner := createTestUser(t, "realowner")

	org := models.Organization{Name: "testownedorg", Description: "Owned"}
	database.DB.Create(&org)
	database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"})

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", owner.ID)
		sess.Save()
		c.Locals("UserID", owner.ID)
		c.Locals("User", owner)
		return c.Next()
	})

	app.Get("/orgs/:orgname/settings", RequireOrgOwner, GetOrgSettings)

	req := httptest.NewRequest("GET", "/orgs/testownedorg/settings", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPostAddMember(t *testing.T) {
	defer cleanupOrgTestData(t)

	owner := createTestUser(t, "addmemberowner")
	newMember := createTestUser(t, "newmember")

	org := models.Organization{Name: "testaddmemberorg", Description: "Test"}
	database.DB.Create(&org)
	database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"})

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", owner.ID)
		sess.Save()
		c.Locals("UserID", owner.ID)
		c.Locals("User", owner)
		return c.Next()
	})

	// RequireOrgOwner sets c.Locals("OrgID")
	app.Post("/orgs/:orgname/members/add", RequireOrgOwner, PostAddMember)

	payload := strings.NewReader("username=" + newMember.Username)
	req := httptest.NewRequest("POST", "/orgs/testaddmemberorg/members/add", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify membership was created
	var membership models.Membership
	err = database.DB.Where("user_id = ? AND organization_id = ?", newMember.ID, org.ID).First(&membership).Error
	assert.NoError(t, err)
	assert.Equal(t, "member", membership.Role)
}

func TestPostRemoveMember(t *testing.T) {
	defer cleanupOrgTestData(t)

	owner := createTestUser(t, "removememberowner")
	member := createTestUser(t, "removablemember")

	org := models.Organization{Name: "testremovememberorg", Description: "Test"}
	database.DB.Create(&org)
	database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"})

	// Add member to remove
	memberMembership := models.Membership{UserID: member.ID, OrganizationID: org.ID, Role: "member"}
	database.DB.Create(&memberMembership)

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", owner.ID)
		sess.Save()
		c.Locals("UserID", owner.ID)
		c.Locals("User", owner)
		return c.Next()
	})

	// RequireOrgOwner sets c.Locals("OrgID")
	app.Post("/orgs/:orgname/members/:member_id/remove", RequireOrgOwner, PostRemoveMember)

	// member_id in the URL is actually the user ID, not the membership ID
	req := httptest.NewRequest("POST", "/orgs/testremovememberorg/members/"+toString(member.ID)+"/remove", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify membership was deleted by checking user_id and org_id
	var count int64
	database.DB.Model(&models.Membership{}).Where("user_id = ? AND organization_id = ?", member.ID, org.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestPostUpdateOrg_NotOwner(t *testing.T) {
	defer cleanupOrgTestData(t)

	owner := createTestUser(t, "updateowner")
	nonOwner := createTestUser(t, "updatenonowner")

	org := models.Organization{Name: "testupdateorg", Description: "Original"}
	database.DB.Create(&org)
	database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"})

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", nonOwner.ID)
		sess.Save()
		c.Locals("UserID", nonOwner.ID)
		c.Locals("User", nonOwner)
		return c.Next()
	})

	app.Post("/orgs/:orgname/settings", RequireOrgOwner, PostUpdateOrg)

	payload := strings.NewReader("description=Hacked+Description")
	req := httptest.NewRequest("POST", "/orgs/testupdateorg/settings", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)

	// Verify description unchanged
	var updated models.Organization
	database.DB.First(&updated, org.ID)
	assert.Equal(t, "Original", updated.Description)
}

func TestPostUpdateOrg_Success(t *testing.T) {
	defer cleanupOrgTestData(t)

	owner := createTestUser(t, "updateowner2")

	org := models.Organization{Name: "testupdateorg2", Description: "Original"}
	database.DB.Create(&org)
	database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"})

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", owner.ID)
		sess.Save()
		c.Locals("UserID", owner.ID)
		c.Locals("User", owner)
		return c.Next()
	})

	app.Post("/orgs/:orgname/settings", RequireOrgOwner, PostUpdateOrg)

	payload := strings.NewReader("description=Updated+Description")
	req := httptest.NewRequest("POST", "/orgs/testupdateorg2/settings", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify description was updated
	var updated models.Organization
	database.DB.First(&updated, org.ID)
	assert.Equal(t, "Updated Description", updated.Description)
}

// GetOrgSettings Page Tests

func TestGetOrgSettings_Unauthenticated(t *testing.T) {
	app := setupTestApp()
	app.Get("/orgs/:orgname/settings", RequireAuth, RequireOrgOwner, GetOrgSettings)

	req := httptest.NewRequest("GET", "/orgs/someorg/settings", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

func TestGetOrgSettings_NotOwner(t *testing.T) {
	defer cleanupOrgTestData(t)

	owner := createTestUser(t, "settingsowner")
	nonOwner := createTestUser(t, "settingsnonowner")

	org := models.Organization{Name: "testsettingsorg", Description: "Test"}
	database.DB.Create(&org)
	database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"})

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", nonOwner.ID)
		sess.Save()
		c.Locals("UserID", nonOwner.ID)
		c.Locals("User", nonOwner)
		return c.Next()
	})

	app.Get("/orgs/:orgname/settings", RequireOrgOwner, GetOrgSettings)

	req := httptest.NewRequest("GET", "/orgs/testsettingsorg/settings", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestGetOrgSettings_Owner(t *testing.T) {
	defer cleanupOrgTestData(t)

	owner := createTestUser(t, "settingsowner2")

	org := models.Organization{Name: "testsettingsorg2", Description: "Test"}
	database.DB.Create(&org)
	database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"})

	app := setupTestApp()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", owner.ID)
		sess.Save()
		c.Locals("UserID", owner.ID)
		c.Locals("User", owner)
		return c.Next()
	})

	app.Get("/orgs/:orgname/settings", RequireOrgOwner, GetOrgSettings)

	req := httptest.NewRequest("GET", "/orgs/testsettingsorg2/settings", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// PostCreateOrg Validation Tests

func TestPostCreateOrg_EmptyName(t *testing.T) {
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "emptyorgcreator")

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

	app.Post("/orgs/new", PostCreateOrg)

	// Empty name
	payload := strings.NewReader("name=&description=Test+Org")
	req := httptest.NewRequest("POST", "/orgs/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostCreateOrg_WhitespaceName(t *testing.T) {
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "whitespaceorgcreator")

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

	app.Post("/orgs/new", PostCreateOrg)

	// Whitespace only name
	payload := strings.NewReader("name=+++&description=Test+Org")
	req := httptest.NewRequest("POST", "/orgs/new", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

// PostAddMember Edge Cases

func TestPostAddMember_UserNotFound(t *testing.T) {
	defer cleanupOrgTestData(t)

	owner := createTestUser(t, "addmemberowner2")

	org := models.Organization{Name: "testaddmemberorg2", Description: "Test"}
	database.DB.Create(&org)
	database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"})

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", owner.ID)
		sess.Save()
		c.Locals("UserID", owner.ID)
		c.Locals("User", owner)
		return c.Next()
	})

	app.Post("/orgs/:orgname/members/add", RequireOrgOwner, PostAddMember)

	payload := strings.NewReader("username=nonexistentuser")
	req := httptest.NewRequest("POST", "/orgs/testaddmemberorg2/members/add", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPostAddMember_AlreadyMember(t *testing.T) {
	defer cleanupOrgTestData(t)

	owner := createTestUser(t, "addmemberowner3")
	existingMember := createTestUser(t, "existingmember")

	org := models.Organization{Name: "testaddmemberorg3", Description: "Test"}
	database.DB.Create(&org)
	database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"})
	database.DB.Create(&models.Membership{UserID: existingMember.ID, OrganizationID: org.ID, Role: "member"})

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", owner.ID)
		sess.Save()
		c.Locals("UserID", owner.ID)
		c.Locals("User", owner)
		return c.Next()
	})

	app.Post("/orgs/:orgname/members/add", RequireOrgOwner, PostAddMember)

	payload := strings.NewReader("username=" + existingMember.Username)
	req := httptest.NewRequest("POST", "/orgs/testaddmemberorg3/members/add", payload)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostRemoveMember_SelfRemoveOnlyOwner(t *testing.T) {
	defer cleanupOrgTestData(t)

	owner := createTestUser(t, "onlyowner")

	org := models.Organization{Name: "testonlyownerorg", Description: "Test"}
	database.DB.Create(&org)
	database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"})

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", owner.ID)
		sess.Save()
		c.Locals("UserID", owner.ID)
		c.Locals("User", owner)
		return c.Next()
	})

	app.Post("/orgs/:orgname/members/:member_id/remove", RequireOrgOwner, PostRemoveMember)

	// Try to remove self when only owner
	req := httptest.NewRequest("POST", "/orgs/testonlyownerorg/members/"+toString(owner.ID)+"/remove", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestRequireOrgOwner_OrgNotFound(t *testing.T) {
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "orgnotfounduser")

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

	app.Get("/orgs/:orgname/settings", RequireOrgOwner, GetOrgSettings)

	req := httptest.NewRequest("GET", "/orgs/nonexistentorg/settings", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

