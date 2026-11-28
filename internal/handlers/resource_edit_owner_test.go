package handlers

import (
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
)

func editApp(user models.User) *fiber.App {
	app := setupTestApp()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		_ = sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})
	app.Get("/:username/:resourcename/edit", GetEditResource)
	app.Post("/resource/:id/edit", PostEditResource)
	return app
}

func orgResource(t *testing.T, name string, creator uint, orgID *uint) models.NomadResource {
	t.Helper()
	res := models.NomadResource{
		Name: name, Type: models.ResourceTypePack, UserID: creator, OrganizationID: orgID,
		RepositoryURL: "https://github.com/test/x", License: "MIT",
	}
	require.NoError(t, database.DB.Create(&res).Error)
	return res
}

func postEdit(t *testing.T, app *fiber.App, res models.NomadResource, form url.Values) int {
	t.Helper()
	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/edit", res.ID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	return resp.StatusCode
}

func editForm(res models.NomadResource) url.Values {
	return url.Values{
		"name": {res.Name}, "type": {string(res.Type)}, "repository_url": {res.RepositoryURL},
		"license": {res.License}, "description": {res.Description},
	}
}

func auditCount(t *testing.T, action string, id uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, database.DB.Model(&models.AuditLog{}).Where("action = ? AND target_id = ?", action, id).Count(&n).Error)
	return n
}

func TestEditResource_OrgOptionSelectedAndRoundTrips(t *testing.T) {
	defer cleanupAuthzTestData(t)
	member := createTestUser(t, "authzedit1")
	org := setupAuthzTestOrg(t, "authzorgedit1")
	addMember(t, member.ID, org.ID, "member")
	res := orgResource(t, "edit-roundtrip", member.ID, &org.ID)
	app := editApp(member)

	req := httptest.NewRequest("GET", "/"+org.Name+"/"+res.Name+"/edit", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Regexp(t, regexp.MustCompile(fmt.Sprintf(`value="org:%d"\s+selected`, org.ID)), string(body))
	assert.NotRegexp(t, regexp.MustCompile(`value="personal"\s+selected`), string(body))

	// Post what the rendered form carries: the selected option.
	form := editForm(res)
	form.Set("owner", fmt.Sprintf("org:%d", org.ID))
	assert.Equal(t, 200, postEdit(t, app, res, form))
	var got models.NomadResource
	require.NoError(t, database.DB.First(&got, res.ID).Error)
	require.NotNil(t, got.OrganizationID)
	assert.Equal(t, org.ID, *got.OrganizationID)
	assert.Zero(t, auditCount(t, "resource.transfer", res.ID))

	// Omitted owner means unchanged.
	assert.Equal(t, 200, postEdit(t, app, res, editForm(res)))
	require.NoError(t, database.DB.First(&got, res.ID).Error)
	require.NotNil(t, got.OrganizationID)
	assert.Zero(t, auditCount(t, "resource.transfer", res.ID))
}

func TestEditResource_PersonalOptionSelected(t *testing.T) {
	defer cleanupAuthzTestData(t)
	user := createTestUser(t, "authzedit2")
	org := setupAuthzTestOrg(t, "authzorgedit2")
	addMember(t, user.ID, org.ID, "member")
	res := orgResource(t, "edit-personal", user.ID, nil)

	req := httptest.NewRequest("GET", "/"+user.Username+"/"+res.Name+"/edit", nil)
	resp, err := editApp(user).Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Regexp(t, regexp.MustCompile(`value="personal"\s+selected`), string(body))
}

func TestEditResource_MemberCannotMoveOut(t *testing.T) {
	defer cleanupAuthzTestData(t)
	member := createTestUser(t, "authzedit3")
	org := setupAuthzTestOrg(t, "authzorgedit3")
	other := setupAuthzTestOrg(t, "authzorgedit3b")
	addMember(t, member.ID, org.ID, "member")
	addMember(t, member.ID, other.ID, "owner")
	res := orgResource(t, "edit-nomove", member.ID, &org.ID)
	app := editApp(member)

	for _, owner := range []string{"personal", fmt.Sprintf("org:%d", other.ID)} {
		form := editForm(res)
		form.Set("owner", owner)
		assert.Equal(t, 403, postEdit(t, app, res, form), owner)
		var got models.NomadResource
		require.NoError(t, database.DB.First(&got, res.ID).Error)
		require.NotNil(t, got.OrganizationID, owner)
		assert.Equal(t, org.ID, *got.OrganizationID)
	}
	assert.Zero(t, auditCount(t, "resource.transfer", res.ID))
}

func TestEditResource_OwnerMoveOutAudited(t *testing.T) {
	defer cleanupAuthzTestData(t)
	owner := createTestUser(t, "authzedit4")
	org := setupAuthzTestOrg(t, "authzorgedit4")
	addMember(t, owner.ID, org.ID, "owner")
	res := orgResource(t, "edit-moveout", owner.ID, &org.ID)
	t.Cleanup(func() {
		database.DB.Exec("DELETE FROM audit_logs WHERE target_id = ? AND target_type = 'resource'", res.ID)
	})

	form := editForm(res)
	form.Set("owner", "personal")
	assert.Equal(t, 200, postEdit(t, editApp(owner), res, form))
	var got models.NomadResource
	require.NoError(t, database.DB.First(&got, res.ID).Error)
	assert.Nil(t, got.OrganizationID)
	assert.Equal(t, owner.ID, got.UserID)

	var al models.AuditLog
	require.NoError(t, database.DB.Where("action = ? AND target_id = ?", "resource.transfer", res.ID).First(&al).Error)
	assert.Contains(t, al.Details, `"from":"`+org.Name+`"`)
	assert.Contains(t, al.Details, `"to":"`+owner.Username+`"`)
}

func TestEditResource_OtherUsersPersonalResource(t *testing.T) {
	defer cleanupAuthzTestData(t)
	victim := createTestUser(t, "authzedit5")
	attacker := createTestUser(t, "authzedit6")
	res := orgResource(t, "edit-victim", victim.ID, nil)
	form := editForm(res)
	form.Set("owner", "personal")
	assert.Equal(t, 403, postEdit(t, editApp(attacker), res, form))
	var got models.NomadResource
	require.NoError(t, database.DB.First(&got, res.ID).Error)
	assert.Equal(t, victim.ID, got.UserID)
}
