package handlers

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"rmbl/internal/database"
	"rmbl/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cleanupWildcardData removes the named users and their resources by id.
// cleanupTestData deletes in bulk, which fails as a whole when any unrelated
// leftover row is still referenced, leaving these behind.
func cleanupWildcardData(t *testing.T, usernames ...string) {
	cleanupTestData(t)
	for _, q := range []string{
		"DELETE FROM resource_versions WHERE resource_id IN (SELECT id FROM nomad_resources WHERE user_id IN (SELECT id FROM users WHERE username IN ?))",
		"DELETE FROM nomad_resources WHERE user_id IN (SELECT id FROM users WHERE username IN ?)",
		"DELETE FROM users WHERE username IN ?",
	} {
		if err := database.DB.Exec(q, usernames).Error; err != nil {
			t.Logf("cleanup: %v", err)
		}
	}
}

func wildcardGet(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func TestGetUserProfile_UnderscoreIsNotWildcard(t *testing.T) {
	defer cleanupWildcardData(t, "abc", "a_c")

	// abc is inserted first: under ILIKE the pattern a_c matches abc and
	// First() returns the lower id.
	abc := createTestUser(t, "abc")
	createTestPack(t, abc.ID, "pack-of-abc")
	ac := createTestUser(t, "a_c")
	createTestPack(t, ac.ID, "pack-of-a-c")

	app := setupTestApp()
	app.Get("/:username", GetUserProfile)

	status, body := wildcardGet(t, app, "/a_c")
	assert.Equal(t, 200, status)
	assert.Contains(t, body, "pack-of-a-c")
	assert.NotContains(t, body, "pack-of-abc")

	status, body = wildcardGet(t, app, "/abc")
	assert.Equal(t, 200, status)
	assert.Contains(t, body, "pack-of-abc")
	assert.NotContains(t, body, "pack-of-a-c")

	// Case-insensitive lookups keep working.
	status, body = wildcardGet(t, app, "/ABC")
	assert.Equal(t, 200, status)
	assert.Contains(t, body, "pack-of-abc")
}

func TestGetUserProfile_UnderscoreNoMatch(t *testing.T) {
	defer cleanupWildcardData(t, "abc")

	createTestUser(t, "abc")

	app := setupTestApp()
	app.Get("/:username", GetUserProfile)

	status, _ := wildcardGet(t, app, "/a_c")
	assert.Equal(t, 404, status)
}

func TestGetUserProfile_PercentIsNotWildcard(t *testing.T) {
	defer cleanupWildcardData(t, "bob25")

	// Fiber passes the raw "%25" param; as an ILIKE pattern it matches any
	// username ending in "25".
	createTestUser(t, "bob25")

	app := setupTestApp()
	app.Get("/:username", GetUserProfile)

	status, _ := wildcardGet(t, app, "/%25")
	assert.Equal(t, 404, status)
}

func TestGetResource_UnderscoreIsNotWildcard(t *testing.T) {
	defer cleanupWildcardData(t, "alice")

	alice := createTestUser(t, "alice")
	createTestPack(t, alice.ID, "myXpack")

	app := setupTestApp()
	app.Get("/:username/:resourcename", GetResource)

	status, _ := wildcardGet(t, app, "/alice/my_pack")
	assert.Equal(t, 404, status)

	createTestPack(t, alice.ID, "my_pack")
	status, body := wildcardGet(t, app, "/alice/my_pack")
	assert.Equal(t, 200, status)
	assert.Contains(t, body, "Test pack: my_pack")

	status, body = wildcardGet(t, app, "/ALICE/MY_PACK")
	assert.Equal(t, 200, status)
	assert.Contains(t, body, "Test pack: my_pack")
}

func createOrgApp(user models.User) *fiber.App {
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
	return app
}

func TestPostCreateOrg_UniquenessIsExactCaseInsensitive(t *testing.T) {
	defer cleanupWildcardData(t, "orgwild", "testuxser")
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "orgwild")
	// Under ILIKE a new name containing _ would match these existing rows.
	require.NoError(t, database.DB.Create(&models.Organization{Name: "testmyXorg"}).Error)
	require.NoError(t, database.DB.Create(&models.User{Username: "testuxser", Email: "testuxser@test.com"}).Error)
	app := createOrgApp(user)

	assert.Equal(t, 200, postForm(t, app, "/orgs/new", "name=testmy_org"), "wildcard match of an org must be accepted")
	assert.Equal(t, 400, postForm(t, app, "/orgs/new", "name=TestMyXOrg"), "case variant must be rejected")
	assert.Equal(t, 200, postForm(t, app, "/orgs/new", "name=testu_ser"), "wildcard match of a username must be accepted")
	assert.Equal(t, 400, postForm(t, app, "/orgs/new", "name=TESTUXSER"), "case variant of a username must be rejected")
	assert.Equal(t, 400, postForm(t, app, "/orgs/new", "name=Testmy_Org"), "case variant of the new org must be rejected")
}

func TestPostSignup_OrgNameWildcard(t *testing.T) {
	defer cleanupWildcardData(t, "testsg_org", "TestsgXorg")
	defer cleanupOrgTestData(t)

	require.NoError(t, database.DB.Create(&models.Organization{Name: "testsgXorg"}).Error)
	app := setupTestApp()
	app.Post("/signup", PostSignup)

	assert.Equal(t, 200, postForm(t, app, "/signup", "username=testsg_org&name=N&email=sgx@test.com&password=SecurePass123!"))
	assert.Equal(t, 400, postForm(t, app, "/signup", "username=TestSgXOrg&name=N&email=sgy@test.com&password=SecurePass123!"))
}

func TestOAuthNewUser_TakenNickname(t *testing.T) {
	defer cleanupOrgTestData(t)

	gu := func(id, nick string) goth.User {
		return goth.User{Provider: "github", UserID: id, Email: id + "@test.com", Name: "N", NickName: nick, AccessToken: "tok"}
	}
	require.NoError(t, database.DB.Create(&models.User{Username: "Octo", Email: "octo-existing@test.com"}).Error)
	require.NoError(t, database.DB.Create(&models.Organization{Name: "testorgnick"}).Error)

	u, err := processOAuthUserDB(database.DB, gu("tk1", "octo"))
	require.NoError(t, err)
	assert.Equal(t, "octo-1", u.Username, "taken by a user, case-variant")

	u, err = processOAuthUserDB(database.DB, gu("tk2", "octo"))
	require.NoError(t, err)
	assert.Equal(t, "octo-2", u.Username, "octo-1 also taken")

	u, err = processOAuthUserDB(database.DB, gu("tk3", "testorgnick"))
	require.NoError(t, err)
	assert.Equal(t, "testorgnick-1", u.Username, "taken by an org")

	u, err = processOAuthUserDB(database.DB, gu("tk4", "freshnick"))
	require.NoError(t, err)
	assert.Equal(t, "freshnick", u.Username, "free nickname unchanged")

	noName := goth.User{Provider: "github", UserID: "tk5", Email: "tk5@test.com", AccessToken: "tok"}
	u, err = processOAuthUserDB(database.DB, noName)
	require.NoError(t, err)
	assert.NotEmpty(t, u.Username, "no nickname or name must not create an empty username")
	assert.True(t, strings.HasPrefix(u.Username, "user"), "got %q", u.Username)
}
