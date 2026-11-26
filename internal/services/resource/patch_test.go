package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
)

func tagNames(t *testing.T, id uint) []string {
	t.Helper()
	var r models.NomadResource
	require.NoError(t, database.DB.Preload("Tags").First(&r, id).Error)
	var out []string
	for _, tg := range r.Tags {
		out = append(out, tg.Name)
	}
	return out
}

func TestPatchResource_PartialUpdate(t *testing.T) {
	owner := createTestUser(t, "patch-owner")
	svc := NewResourceService(database.DB)
	in := validInput("patch-partial")
	in.Description = "old"
	in.FilePath = "packs/x"
	in.Tags = "aa,bb"
	res, err := svc.CreateResource(in, owner.ID, nil)
	require.NoError(t, err)
	cleanupResource(t, res)

	tests := []struct {
		name  string
		patch PatchInput
		want  func(r models.NomadResource)
		tags  []string
	}{
		{"description only", PatchInput{Description: new("new")}, func(r models.NomadResource) {
			assert.Equal(t, "new", r.Description)
			assert.Equal(t, validRepoURL, r.RepositoryURL)
			assert.Equal(t, "packs/x", r.FilePath)
			assert.Equal(t, "MIT", r.License)
		}, []string{"aa", "bb"}},
		{"clear description and file path", PatchInput{Description: new(""), FilePath: new("")}, func(r models.NomadResource) {
			assert.Empty(t, r.Description)
			assert.Empty(t, r.FilePath)
			assert.Equal(t, "MIT", r.License)
		}, []string{"aa", "bb"}},
		{"empty license is Unknown", PatchInput{License: new("  ")}, func(r models.NomadResource) {
			assert.Equal(t, "Unknown", r.License)
		}, []string{"aa", "bb"}},
		{"valid repository url", PatchInput{RepositoryURL: new("https://github.com/a/b")}, func(r models.NomadResource) {
			assert.Equal(t, "https://github.com/a/b", r.RepositoryURL)
		}, []string{"aa", "bb"}},
		{"tags replaced", PatchInput{Tags: &[]string{"cc", "dd"}}, func(r models.NomadResource) {
			assert.Equal(t, "https://github.com/a/b", r.RepositoryURL)
		}, []string{"cc", "dd"}},
		{"empty tags clear", PatchInput{Tags: &[]string{}}, func(models.NomadResource) {}, nil},
		{"empty patch is a no-op", PatchInput{}, func(r models.NomadResource) {
			assert.Equal(t, "Unknown", r.License)
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, svc.PatchResource(res.ID, owner.ID, tt.patch))
			var r models.NomadResource
			require.NoError(t, database.DB.First(&r, res.ID).Error)
			assert.Equal(t, "patch-partial", r.Name)
			tt.want(r)
			assert.ElementsMatch(t, tt.tags, tagNames(t, res.ID))
		})
	}
}

func TestPatchResource_InvalidURLWritesNothing(t *testing.T) {
	owner := createTestUser(t, "patch-url")
	svc := NewResourceService(database.DB)
	in := validInput("patch-url-res")
	in.Description = "keep"
	in.Tags = "aa"
	res, err := svc.CreateResource(in, owner.ID, nil)
	require.NoError(t, err)
	cleanupResource(t, res)

	for _, u := range []string{"http://github.com/x/y", "https://evil.example/x", ""} {
		before := countRows(t)
		err := svc.PatchResource(res.ID, owner.ID, PatchInput{
			Description: new("changed"), RepositoryURL: new(u), Tags: &[]string{"brandnew"}})
		assert.ErrorIs(t, err, ErrInvalidRepositoryURL, u)
		assert.Equal(t, before, countRows(t), "no tag may be created")
		var r models.NomadResource
		require.NoError(t, database.DB.First(&r, res.ID).Error)
		assert.Equal(t, "keep", r.Description)
		assert.Equal(t, validRepoURL, r.RepositoryURL)
		assert.Equal(t, []string{"aa"}, tagNames(t, res.ID))
	}
}

func TestPatchResource_Authorization(t *testing.T) {
	owner := createTestUser(t, "patchauth-owner")
	member := createTestUser(t, "patchauth-member")
	outsider := createTestUser(t, "patchauth-out")
	org := models.Organization{Name: "patchauth-org"}
	require.NoError(t, database.DB.Create(&org).Error)
	t.Cleanup(func() {
		database.DB.Exec("DELETE FROM memberships WHERE organization_id = ?", org.ID)
		database.DB.Exec("DELETE FROM nomad_resources WHERE organization_id = ?", org.ID)
		database.DB.Delete(&org)
	})
	require.NoError(t, database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: RoleOwner}).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: member.ID, OrganizationID: org.ID, Role: RoleMember}).Error)
	svc := NewResourceService(database.DB)
	res, err := svc.CreateResource(validInput("patchauth-res"), owner.ID, &org.ID)
	require.NoError(t, err)

	assert.NoError(t, svc.PatchResource(res.ID, member.ID, PatchInput{Description: new("by member")}))
	var r models.NomadResource
	require.NoError(t, database.DB.First(&r, res.ID).Error)
	assert.Equal(t, "by member", r.Description)

	before := countRows(t)
	err = svc.PatchResource(res.ID, outsider.ID, PatchInput{Description: new("x"), Tags: &[]string{"zzz"}})
	assert.ErrorIs(t, err, ErrUnauthorized)
	assert.Equal(t, before, countRows(t))
	require.NoError(t, database.DB.First(&r, res.ID).Error)
	assert.Equal(t, "by member", r.Description)

	assert.ErrorIs(t, svc.PatchResource(999999, owner.ID, PatchInput{}), ErrResourceNotFound)
}
