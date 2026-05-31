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

// authzSetup seeds users, org, memberships, and an org-owned resource for
// authorization tests. It returns a cleanup func that removes all seeded rows.
func authzSetup(t *testing.T, suffix string) (
	orgOwner, orgMember, nonMember models.User,
	org models.Organization,
	orgRes models.NomadResource,
	personalOwner models.User,
	personalRes models.NomadResource,
	cleanup func(),
) {
	t.Helper()

	orgOwner = createTestUser(t, "authz_owner_"+suffix)
	orgMember = createTestUser(t, "authz_member_"+suffix)
	nonMember = createTestUser(t, "authz_nonmember_"+suffix)
	personalOwner = createTestUser(t, "authz_personal_"+suffix)

	org = models.Organization{Name: "authzorg-" + suffix, Description: "test"}
	require.NoError(t, database.DB.Create(&org).Error)

	database.DB.Create(&models.Membership{UserID: orgOwner.ID, OrganizationID: org.ID, Role: "owner"})
	database.DB.Create(&models.Membership{UserID: orgMember.ID, OrganizationID: org.ID, Role: "member"})

	orgRes = models.NomadResource{
		Name:           "orgres-" + suffix,
		Type:           models.ResourceTypePack,
		UserID:         orgOwner.ID,
		OrganizationID: &org.ID,
		RepositoryURL:  "https://github.com/test/org-" + suffix,
		WebhookSecret:  "secret-" + suffix,
	}
	require.NoError(t, database.DB.Create(&orgRes).Error)
	database.DB.Create(&models.ResourceVersion{ResourceID: orgRes.ID, Version: "v1.0.0"})

	personalRes = models.NomadResource{
		Name:          "personalres-" + suffix,
		Type:          models.ResourceTypePack,
		UserID:        personalOwner.ID,
		RepositoryURL: "https://github.com/test/personal-" + suffix,
		WebhookSecret: "personal-secret-" + suffix,
	}
	require.NoError(t, database.DB.Create(&personalRes).Error)
	database.DB.Create(&models.ResourceVersion{ResourceID: personalRes.ID, Version: "v1.0.0"})

	cleanup = func() {
		database.DB.Exec("DELETE FROM resource_versions WHERE resource_id IN (?,?)", orgRes.ID, personalRes.ID)
		database.DB.Exec("DELETE FROM resource_tags WHERE nomad_resource_id IN (?,?)", orgRes.ID, personalRes.ID)
		database.DB.Exec("DELETE FROM nomad_resources WHERE id IN (?,?)", orgRes.ID, personalRes.ID)
		database.DB.Exec("DELETE FROM memberships WHERE organization_id = ?", org.ID)
		database.DB.Exec("DELETE FROM organizations WHERE id = ?", org.ID)
		database.DB.Exec("DELETE FROM users WHERE id IN (?,?,?,?)", orgOwner.ID, orgMember.ID, nonMember.ID, personalOwner.ID)
	}
	return
}

// authzAppWith builds a Fiber app that authenticates as the given user and
// mounts the supplied routes via setupFunc.
func authzAppWith(user models.User, setupFunc func(app *fiber.App)) *fiber.App {
	app := fiber.New()
	InitSession()
	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		_ = sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		c.Locals("CSRFToken", "test-token")
		return c.Next()
	})
	setupFunc(app)
	return app
}

// versionCount returns how many ResourceVersion rows exist for the given resource.
func versionCount(t *testing.T, resourceID uint) int64 {
	t.Helper()
	var count int64
	database.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceID).Count(&count)
	return count
}

// --- PostNewVersion authorization tests ---

func TestPostNewVersion_OrgMember_Allowed(t *testing.T) {
	orgOwner, orgMember, _, _, orgRes, _, _, cleanup := authzSetup(t, "pnv1")
	defer cleanup()
	_ = orgOwner

	app := authzAppWith(orgMember, func(a *fiber.App) {
		a.Post("/resource/:id/version", PostNewVersion)
	})

	before := versionCount(t, orgRes.ID)
	body := strings.NewReader("version=v2.0.0")
	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/version", orgRes.ID), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "org member should be allowed to add a version")
	assert.Equal(t, before+1, versionCount(t, orgRes.ID), "version row must be created")
}

func TestPostNewVersion_OrgOwner_Allowed(t *testing.T) {
	orgOwner, _, _, _, orgRes, _, _, cleanup := authzSetup(t, "pnv2")
	defer cleanup()

	app := authzAppWith(orgOwner, func(a *fiber.App) {
		a.Post("/resource/:id/version", PostNewVersion)
	})

	before := versionCount(t, orgRes.ID)
	body := strings.NewReader("version=v2.0.0")
	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/version", orgRes.ID), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "org owner should be allowed to add a version")
	assert.Equal(t, before+1, versionCount(t, orgRes.ID), "version row must be created")
}

func TestPostNewVersion_OrgNonMember_Forbidden(t *testing.T) {
	_, _, nonMember, _, orgRes, _, _, cleanup := authzSetup(t, "pnv3")
	defer cleanup()

	app := authzAppWith(nonMember, func(a *fiber.App) {
		a.Post("/resource/:id/version", PostNewVersion)
	})

	before := versionCount(t, orgRes.ID)
	body := strings.NewReader("version=v2.0.0")
	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/version", orgRes.ID), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 403, resp.StatusCode, "non-member must receive 403")
	assert.Equal(t, before, versionCount(t, orgRes.ID), "no version row must be created for non-member")
}

func TestPostNewVersion_PersonalOwner_Allowed(t *testing.T) {
	_, _, _, _, _, personalOwner, personalRes, cleanup := authzSetup(t, "pnv4")
	defer cleanup()

	app := authzAppWith(personalOwner, func(a *fiber.App) {
		a.Post("/resource/:id/version", PostNewVersion)
	})

	before := versionCount(t, personalRes.ID)
	body := strings.NewReader("version=v2.0.0")
	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/version", personalRes.ID), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "personal owner should be allowed to add a version")
	assert.Equal(t, before+1, versionCount(t, personalRes.ID), "version row must be created")
}

func TestPostNewVersion_PersonalOtherUser_Forbidden(t *testing.T) {
	_, _, nonMember, _, _, _, personalRes, cleanup := authzSetup(t, "pnv5")
	defer cleanup()

	app := authzAppWith(nonMember, func(a *fiber.App) {
		a.Post("/resource/:id/version", PostNewVersion)
	})

	before := versionCount(t, personalRes.ID)
	body := strings.NewReader("version=v2.0.0")
	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/version", personalRes.ID), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 403, resp.StatusCode, "other user must receive 403")
	assert.Equal(t, before, versionCount(t, personalRes.ID), "no version row must be created")
}

func TestPostNewVersion_NonexistentResource_NotFound(t *testing.T) {
	_, orgMember, _, _, _, _, _, cleanup := authzSetup(t, "pnv6")
	defer cleanup()

	app := authzAppWith(orgMember, func(a *fiber.App) {
		a.Post("/resource/:id/version", PostNewVersion)
	})

	body := strings.NewReader("version=v2.0.0")
	req := httptest.NewRequest("POST", "/resource/999999999/version", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 404, resp.StatusCode, "nonexistent resource must return 404")
}

// --- PostEditResource (PostNewVersion) org authorization tests ---

func TestPostEditResource_OrgMember_Allowed(t *testing.T) {
	_, orgMember, _, _, orgRes, _, _, cleanup := authzSetup(t, "per1")
	defer cleanup()

	app := authzAppWith(orgMember, func(a *fiber.App) {
		a.Post("/resource/:id/edit", PostEditResource)
	})

	body := strings.NewReader("name=orgres-per1&type=pack&description=updated+by+member&license=MIT")
	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/edit", orgRes.ID), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "org member should be allowed to edit")
}

func TestPostEditResource_OrgOwner_Allowed(t *testing.T) {
	orgOwner, _, _, _, orgRes, _, _, cleanup := authzSetup(t, "per2")
	defer cleanup()

	app := authzAppWith(orgOwner, func(a *fiber.App) {
		a.Post("/resource/:id/edit", PostEditResource)
	})

	body := strings.NewReader("name=orgres-per2&type=pack&description=updated+by+owner&license=MIT")
	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/edit", orgRes.ID), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "org owner should be allowed to edit")
}

func TestPostEditResource_OrgNonMember_Forbidden(t *testing.T) {
	_, _, nonMember, _, orgRes, _, _, cleanup := authzSetup(t, "per3")
	defer cleanup()

	app := authzAppWith(nonMember, func(a *fiber.App) {
		a.Post("/resource/:id/edit", PostEditResource)
	})

	body := strings.NewReader("name=hacked&type=pack&description=hacked")
	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/edit", orgRes.ID), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 403, resp.StatusCode, "non-member must receive 403")
}

// --- DeleteResource org authorization tests ---

func TestDeleteResource_OrgOwner_Allowed(t *testing.T) {
	orgOwner, _, _, _, orgRes, _, _, cleanup := authzSetup(t, "dr1")
	defer cleanup()

	app := authzAppWith(orgOwner, func(a *fiber.App) {
		a.Delete("/resource/:id", DeleteResource)
	})

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/resource/%d", orgRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "org owner should be allowed to delete")

	var count int64
	database.DB.Model(&models.NomadResource{}).Where("id = ?", orgRes.ID).Count(&count)
	assert.Equal(t, int64(0), count, "resource must be deleted")
}

func TestDeleteResource_OrgMember_Forbidden(t *testing.T) {
	_, orgMember, _, _, orgRes, _, _, cleanup := authzSetup(t, "dr2")
	defer cleanup()

	app := authzAppWith(orgMember, func(a *fiber.App) {
		a.Delete("/resource/:id", DeleteResource)
	})

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/resource/%d", orgRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 403, resp.StatusCode, "org member must receive 403 for delete")

	var count int64
	database.DB.Model(&models.NomadResource{}).Unscoped().Where("id = ?", orgRes.ID).Count(&count)
	assert.Equal(t, int64(1), count, "resource must NOT be deleted")
}

func TestDeleteResource_OrgNonMember_Forbidden(t *testing.T) {
	_, _, nonMember, _, orgRes, _, _, cleanup := authzSetup(t, "dr3")
	defer cleanup()

	app := authzAppWith(nonMember, func(a *fiber.App) {
		a.Delete("/resource/:id", DeleteResource)
	})

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/resource/%d", orgRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 403, resp.StatusCode, "non-member must receive 403 for delete")
}

func TestDeleteResource_PersonalOwner_Allowed(t *testing.T) {
	_, _, _, _, _, personalOwner, personalRes, cleanup := authzSetup(t, "dr4")
	defer cleanup()

	app := authzAppWith(personalOwner, func(a *fiber.App) {
		a.Delete("/resource/:id", DeleteResource)
	})

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/resource/%d", personalRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "personal owner should be allowed to delete")

	var count int64
	database.DB.Model(&models.NomadResource{}).Where("id = ?", personalRes.ID).Count(&count)
	assert.Equal(t, int64(0), count, "personal resource must be deleted")
}

func TestDeleteResource_PersonalOtherUser_Forbidden(t *testing.T) {
	_, _, nonMember, _, _, _, personalRes, cleanup := authzSetup(t, "dr5")
	defer cleanup()

	app := authzAppWith(nonMember, func(a *fiber.App) {
		a.Delete("/resource/:id", DeleteResource)
	})

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/resource/%d", personalRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 403, resp.StatusCode, "other user must receive 403 for personal resource delete")
}

// --- PostRetryFetch org authorization tests ---

// setVersionFailed sets the latest version of a resource to failed state.
func setVersionFailed(t *testing.T, resourceID uint) {
	t.Helper()
	database.DB.Model(&models.ResourceVersion{}).
		Where("resource_id = ?", resourceID).
		Updates(map[string]interface{}{
			"fetch_status": models.FetchStatusFailed,
			"fetch_error":  "test failure",
		})
}

func TestPostRetryFetch_OrgMember_Allowed(t *testing.T) {
	_, orgMember, _, _, orgRes, _, _, cleanup := authzSetup(t, "prf1")
	defer cleanup()
	setVersionFailed(t, orgRes.ID)

	app := authzAppWith(orgMember, func(a *fiber.App) {
		a.Post("/resource/:id/retry-fetch", PostRetryFetch)
	})

	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/retry-fetch", orgRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "org member should be allowed to retry fetch")
}

func TestPostRetryFetch_OrgOwner_Allowed(t *testing.T) {
	orgOwner, _, _, _, orgRes, _, _, cleanup := authzSetup(t, "prf2")
	defer cleanup()
	setVersionFailed(t, orgRes.ID)

	app := authzAppWith(orgOwner, func(a *fiber.App) {
		a.Post("/resource/:id/retry-fetch", PostRetryFetch)
	})

	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/retry-fetch", orgRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "org owner should be allowed to retry fetch")
}

func TestPostRetryFetch_OrgNonMember_Forbidden(t *testing.T) {
	_, _, nonMember, _, orgRes, _, _, cleanup := authzSetup(t, "prf3")
	defer cleanup()
	setVersionFailed(t, orgRes.ID)

	app := authzAppWith(nonMember, func(a *fiber.App) {
		a.Post("/resource/:id/retry-fetch", PostRetryFetch)
	})

	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/retry-fetch", orgRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 403, resp.StatusCode, "non-member must receive 403 for retry fetch")
}

func TestPostRetryFetch_PersonalOwner_Allowed(t *testing.T) {
	_, _, _, _, _, personalOwner, personalRes, cleanup := authzSetup(t, "prf4")
	defer cleanup()
	setVersionFailed(t, personalRes.ID)

	app := authzAppWith(personalOwner, func(a *fiber.App) {
		a.Post("/resource/:id/retry-fetch", PostRetryFetch)
	})

	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/retry-fetch", personalRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "personal owner should be allowed to retry fetch")
}

func TestPostRetryFetch_PersonalOtherUser_Forbidden(t *testing.T) {
	_, _, nonMember, _, _, _, personalRes, cleanup := authzSetup(t, "prf5")
	defer cleanup()
	setVersionFailed(t, personalRes.ID)

	app := authzAppWith(nonMember, func(a *fiber.App) {
		a.Post("/resource/:id/retry-fetch", PostRetryFetch)
	})

	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/retry-fetch", personalRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 403, resp.StatusCode, "other user must receive 403 for personal resource retry fetch")
}

// --- PostResetWebhookSecret org authorization tests ---

func TestPostResetWebhookSecret_OrgMember_Allowed(t *testing.T) {
	_, orgMember, _, _, orgRes, _, _, cleanup := authzSetup(t, "prws1")
	defer cleanup()
	oldSecret := orgRes.WebhookSecret

	app := authzAppWith(orgMember, func(a *fiber.App) {
		a.Post("/resource/:id/reset-webhook", PostResetWebhookSecret)
	})

	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/reset-webhook", orgRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "org member should be allowed to reset webhook secret")

	var updated models.NomadResource
	database.DB.First(&updated, orgRes.ID)
	assert.NotEqual(t, oldSecret, updated.WebhookSecret, "webhook secret must be rotated")
	assert.NotEmpty(t, updated.WebhookSecret)
}

func TestPostResetWebhookSecret_OrgOwner_Allowed(t *testing.T) {
	orgOwner, _, _, _, orgRes, _, _, cleanup := authzSetup(t, "prws2")
	defer cleanup()
	oldSecret := orgRes.WebhookSecret

	app := authzAppWith(orgOwner, func(a *fiber.App) {
		a.Post("/resource/:id/reset-webhook", PostResetWebhookSecret)
	})

	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/reset-webhook", orgRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "org owner should be allowed to reset webhook secret")

	var updated models.NomadResource
	database.DB.First(&updated, orgRes.ID)
	assert.NotEqual(t, oldSecret, updated.WebhookSecret, "webhook secret must be rotated")
}

func TestPostResetWebhookSecret_OrgNonMember_Forbidden(t *testing.T) {
	_, _, nonMember, _, orgRes, _, _, cleanup := authzSetup(t, "prws3")
	defer cleanup()
	oldSecret := orgRes.WebhookSecret

	app := authzAppWith(nonMember, func(a *fiber.App) {
		a.Post("/resource/:id/reset-webhook", PostResetWebhookSecret)
	})

	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/reset-webhook", orgRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 403, resp.StatusCode, "non-member must receive 403")

	var updated models.NomadResource
	database.DB.First(&updated, orgRes.ID)
	assert.Equal(t, oldSecret, updated.WebhookSecret, "webhook secret must NOT be rotated for non-member")
}

func TestPostResetWebhookSecret_PersonalOwner_Allowed(t *testing.T) {
	_, _, _, _, _, personalOwner, personalRes, cleanup := authzSetup(t, "prws4")
	defer cleanup()
	oldSecret := personalRes.WebhookSecret

	app := authzAppWith(personalOwner, func(a *fiber.App) {
		a.Post("/resource/:id/reset-webhook", PostResetWebhookSecret)
	})

	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/reset-webhook", personalRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode, "personal owner should be allowed to reset webhook secret")

	var updated models.NomadResource
	database.DB.First(&updated, personalRes.ID)
	assert.NotEqual(t, oldSecret, updated.WebhookSecret, "webhook secret must be rotated")
}

func TestPostResetWebhookSecret_PersonalOtherUser_Forbidden(t *testing.T) {
	_, _, nonMember, _, _, _, personalRes, cleanup := authzSetup(t, "prws5")
	defer cleanup()
	oldSecret := personalRes.WebhookSecret

	app := authzAppWith(nonMember, func(a *fiber.App) {
		a.Post("/resource/:id/reset-webhook", PostResetWebhookSecret)
	})

	req := httptest.NewRequest("POST", fmt.Sprintf("/resource/%d/reset-webhook", personalRes.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 403, resp.StatusCode, "other user must receive 403")

	var updated models.NomadResource
	database.DB.First(&updated, personalRes.ID)
	assert.Equal(t, oldSecret, updated.WebhookSecret, "webhook secret must NOT be rotated")
}
