package handlers

import (
	"net/http/httptest"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func countUsersFolded(name string) int64 {
	var n int64
	database.DB.Model(&models.User{}).Where("LOWER(username) = LOWER(?)", name).Count(&n)
	return n
}

func TestPostSignup_CaseVariantOfUserRejected(t *testing.T) {
	defer cleanupTestData(t)

	createTestUser(t, "testunalice")
	app := setupTestApp()
	app.Post("/signup", PostSignup)

	status := postForm(t, app, "/signup", "username=TESTUNALICE&name=A&email=unalice2@test.com&password=SecurePass123!")
	assert.Equal(t, 400, status)
	assert.EqualValues(t, 1, countUsersFolded("testunalice"), "no extra row may be created")

	status = postForm(t, app, "/signup", "username=testunalice2&name=A&email=unalice3@test.com&password=SecurePass123!")
	assert.Equal(t, 200, status, "control: a distinct name signs up")
}

func TestPostSignup_CaseVariantOfOrgRejected(t *testing.T) {
	defer cleanupOrgTestData(t)

	require.NoError(t, database.DB.Create(&models.Organization{Name: "TestUnOrgA"}).Error)
	app := setupTestApp()
	app.Post("/signup", PostSignup)

	assert.Equal(t, 400, postForm(t, app, "/signup", "username=testunorga&name=A&email=unorga@test.com&password=SecurePass123!"))
	assert.Zero(t, countUsersFolded("testunorga"))
}

func TestPostEditUser_RenameUniqueness(t *testing.T) {
	defer cleanupOrgTestData(t)

	admin := createAdminUser(t, "testunadmin")
	target := createTestUser(t, "testuntarget")
	createTestUser(t, "testunother")
	require.NoError(t, database.DB.Create(&models.Organization{Name: "TestUnOrgB"}).Error)
	app := adminEditApp(admin)
	path := "/admin/users/" + toString(target.ID) + "/edit"

	username := func() string {
		var u models.User
		database.DB.First(&u, target.ID)
		return u.Username
	}

	assert.Equal(t, 400, postForm(t, app, path, "username=TESTUNOTHER&name=X&email=testuntarget@test.com"))
	assert.Equal(t, "testuntarget", username(), "rename to a user case variant must not apply")

	assert.Equal(t, 400, postForm(t, app, path, "username=testunorgb&name=X&email=testuntarget@test.com"))
	assert.Equal(t, "testuntarget", username(), "rename to an org case variant must not apply")

	assert.Equal(t, 200, postForm(t, app, path, "username=TestUnTarget&name=X&email=testuntarget@test.com"), "own case variant is allowed")
	assert.Equal(t, "TestUnTarget", username())
}

func TestPostEditUser_LegacyDuplicateStillEditable(t *testing.T) {
	defer cleanupTestData(t)

	admin := createAdminUser(t, "testunadmin2")
	createTestUser(t, "testundup")
	other := createTestUser(t, "TestUnDup")
	app := adminEditApp(admin)

	status := postForm(t, app, "/admin/users/"+toString(other.ID)+"/edit", "username=TestUnDup&name=Changed&email=TestUnDup@test.com")
	assert.Equal(t, 200, status)
	var after models.User
	database.DB.First(&after, other.ID)
	assert.Equal(t, "Changed", after.Name)
}

func TestPostEditOrganization_RenameUniqueness(t *testing.T) {
	defer cleanupOrgTestData(t)

	admin := createAdminUser(t, "testunorgadmin")
	createTestUser(t, "testunorguser")
	target := models.Organization{Name: "testunorgtarget", Description: "d"}
	require.NoError(t, database.DB.Create(&target).Error)
	require.NoError(t, database.DB.Create(&models.Organization{Name: "testunorgother"}).Error)
	app := adminEditApp(admin)
	path := "/admin/organizations/" + toString(target.ID) + "/edit"

	name := func() string {
		var o models.Organization
		database.DB.First(&o, target.ID)
		return o.Name
	}

	assert.Equal(t, 400, postForm(t, app, path, "name=TestUnOrgOther&description=d"))
	assert.Equal(t, "testunorgtarget", name())

	assert.Equal(t, 400, postForm(t, app, path, "name=TESTUNORGUSER&description=d"))
	assert.Equal(t, "testunorgtarget", name())

	assert.Equal(t, 200, postForm(t, app, path, "name=TestUnOrgTarget&description=d"), "own case variant is allowed")
	assert.Equal(t, "TestUnOrgTarget", name())
}

func TestPostEditOrganization_LegacyDuplicateStillEditable(t *testing.T) {
	defer cleanupOrgTestData(t)

	admin := createAdminUser(t, "testunorgadmin2")
	require.NoError(t, database.DB.Create(&models.Organization{Name: "testunorgdup"}).Error)
	dup := models.Organization{Name: "testUnOrgDup", Description: "old"}
	require.NoError(t, database.DB.Create(&dup).Error)
	app := adminEditApp(admin)

	assert.Equal(t, 200, postForm(t, app, "/admin/organizations/"+toString(dup.ID)+"/edit", "name=testUnOrgDup&description=new"))
	var after models.Organization
	database.DB.First(&after, dup.ID)
	assert.Equal(t, "new", after.Description)
}

// orgStubApp mounts handlers directly, with RequireOrgOwner replaced by a stub
// pointing at an organisation id that does not exist.
func orgStubApp(user models.User) *fiber.App {
	app := setupTestApp()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		c.Locals("OrgID", uint(999999))
		return c.Next()
	})
	app.Get("/orgs/:orgname/settings", GetOrgSettings)
	app.Post("/orgs/:orgname/update", PostUpdateOrg)
	app.Post("/orgs/:orgname/members", PostAddMember)
	app.Post("/orgs/:orgname/members/:member_id/remove", PostRemoveMember)
	return app
}

func countOrgsAndMembers() (orgs, members int64) {
	database.DB.Unscoped().Model(&models.Organization{}).Count(&orgs)
	database.DB.Unscoped().Model(&models.Membership{}).Count(&members)
	return
}

func TestGetOrgSettings_MissingOrg404(t *testing.T) {
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "testunsettings")
	app := orgStubApp(user)
	resp, err := app.Test(httptest.NewRequest("GET", "/orgs/testnosuchorg/settings", nil))
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPostUpdateOrg_MissingOrg404NoInsert(t *testing.T) {
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "testunupdate")
	app := orgStubApp(user)
	orgsBefore, _ := countOrgsAndMembers()

	assert.Equal(t, 404, postForm(t, app, "/orgs/x/update", "description=hi"))
	orgsAfter, _ := countOrgsAndMembers()
	assert.Equal(t, orgsBefore, orgsAfter, "no blank organisation may be inserted")
}

func TestPostAddMember_MissingOrg404NoMembership(t *testing.T) {
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "testunaddowner")
	createTestUser(t, "testunaddee")
	app := orgStubApp(user)
	_, before := countOrgsAndMembers()

	assert.Equal(t, 404, postForm(t, app, "/orgs/x/members", "username=testunaddee&role=member"))
	_, after := countOrgsAndMembers()
	assert.Equal(t, before, after, "no membership may be created")
}

func TestPostRemoveMember_MissingOrg404NoDelete(t *testing.T) {
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "testunrmowner")
	member := createTestUser(t, "testunrmmember")
	require.NoError(t, database.DB.Create(&models.Organization{Name: "testunrmorg"}).Error)
	var real models.Organization
	database.DB.Where("name = ?", "testunrmorg").First(&real)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: member.ID, OrganizationID: real.ID, Role: "member"}).Error)
	app := orgStubApp(user)
	assert.Equal(t, 404, postForm(t, app, "/orgs/x/members/"+toString(member.ID)+"/remove", ""))
}
