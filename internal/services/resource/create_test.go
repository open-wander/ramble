package resource

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
)

const validRepoURL = "https://github.com/x/y"

type rowCounts struct{ resources, versions, tags int64 }

func countRows(t *testing.T) rowCounts {
	t.Helper()
	var c rowCounts
	require.NoError(t, database.DB.Model(&models.NomadResource{}).Count(&c.resources).Error)
	require.NoError(t, database.DB.Model(&models.ResourceVersion{}).Count(&c.versions).Error)
	require.NoError(t, database.DB.Model(&models.Tag{}).Count(&c.tags).Error)
	return c
}

func validInput(name string) CreateInput {
	return CreateInput{Name: name, Type: "pack", RepositoryURL: validRepoURL, Version: "v1.0.0", License: "MIT"}
}

func cleanupResource(t *testing.T, res *models.NomadResource) {
	t.Helper()
	t.Cleanup(func() { database.DB.Unscoped().Delete(res) })
}

func TestCreateResource_InputValidation(t *testing.T) {
	user := createTestUser(t, "createval")
	t.Cleanup(func() { database.DB.Delete(&user) })
	svc := NewResourceService(database.DB)

	tests := []struct {
		name  string
		mod   func(*CreateInput)
		valid bool
		want  error
	}{
		{"name simple", func(i *CreateInput) { i.Name = "cv-mysql" }, true, nil},
		{"name punctuation", func(i *CreateInput) { i.Name = "my-pack_1.2" }, true, nil},
		{"name 64 chars", func(i *CreateInput) { i.Name = strings.Repeat("a", 64) }, true, nil},
		{"name empty", func(i *CreateInput) { i.Name = "" }, false, ErrInvalidName},
		{"name leading dash", func(i *CreateInput) { i.Name = "-x" }, false, ErrInvalidName},
		{"name slash", func(i *CreateInput) { i.Name = "a/b" }, false, ErrInvalidName},
		{"name space", func(i *CreateInput) { i.Name = "a b" }, false, ErrInvalidName},
		{"name 65 chars", func(i *CreateInput) { i.Name = strings.Repeat("a", 65) }, false, ErrInvalidName},
		{"name dotdot", func(i *CreateInput) { i.Name = ".." }, false, ErrInvalidName},
		{"type job", func(i *CreateInput) { i.Name = "cv-job"; i.Type = "job" }, true, nil},
		{"type foo", func(i *CreateInput) { i.Type = "foo" }, false, ErrInvalidType},
		{"type empty", func(i *CreateInput) { i.Type = "" }, false, ErrInvalidType},
		{"url http", func(i *CreateInput) { i.RepositoryURL = "http://github.com/x/y" }, false, ErrInvalidRepositoryURL},
		{"url foreign host", func(i *CreateInput) { i.RepositoryURL = "https://evil.example/x" }, false, ErrInvalidRepositoryURL},
		{"url file", func(i *CreateInput) { i.RepositoryURL = "file:///etc/passwd" }, false, ErrInvalidRepositoryURL},
		{"url suffix trick", func(i *CreateInput) { i.RepositoryURL = "https://github.com.evil.example/x" }, false, ErrInvalidRepositoryURL},
		{"url userinfo host trick", func(i *CreateInput) { i.RepositoryURL = "https://github.com@evil.example/x" }, false, ErrInvalidRepositoryURL},
		{"url credentials", func(i *CreateInput) { i.RepositoryURL = "https://user:pw@github.com/x" }, false, ErrInvalidRepositoryURL},
		{"url empty", func(i *CreateInput) { i.RepositoryURL = "" }, false, ErrInvalidRepositoryURL},
		{"version empty", func(i *CreateInput) { i.Version = "" }, false, ErrInvalidVersion},
		{"version blank", func(i *CreateInput) { i.Version = "   " }, false, ErrInvalidVersion},
		{"version 64", func(i *CreateInput) { i.Name = "cv-v64"; i.Version = strings.Repeat("1", 64) }, true, nil},
		{"version traversal", func(i *CreateInput) { i.Version = "v1.tar.gz#/../../.." }, false, ErrInvalidVersion},
		{"version slash", func(i *CreateInput) { i.Version = "a/b" }, false, ErrInvalidVersion},
		{"version rc", func(i *CreateInput) { i.Name = "cv-rc"; i.Version = "v1.0.0-rc.1" }, true, nil},
		{"version 65", func(i *CreateInput) { i.Version = strings.Repeat("1", 65) }, false, ErrInvalidVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput("cv-base")
			tt.mod(&in)
			before := countRows(t)
			res, err := svc.CreateResource(in, user.ID, nil)
			if tt.valid {
				require.NoError(t, err)
				cleanupResource(t, res)
				return
			}
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.want), "got %v want %v", err, tt.want)
			assert.Nil(t, res)
			assert.Equal(t, before, countRows(t), "rejection must not persist anything")
		})
	}
}

func TestCreateResource_DuplicateNamespaces(t *testing.T) {
	alice := createTestUser(t, "dupns-alice")
	bob := createTestUser(t, "dupns-bob")
	org := models.Organization{Name: "dupns-org"}
	require.NoError(t, database.DB.Create(&org).Error)
	t.Cleanup(func() {
		database.DB.Exec("DELETE FROM memberships WHERE organization_id = ?", org.ID)
		database.DB.Exec("DELETE FROM nomad_resources WHERE user_id IN (?,?)", alice.ID, bob.ID)
		database.DB.Delete(&org)
		database.DB.Delete(&alice)
		database.DB.Delete(&bob)
	})
	require.NoError(t, database.DB.Create(&models.Membership{UserID: alice.ID, OrganizationID: org.ID, Role: RoleMember}).Error)
	svc := NewResourceService(database.DB)

	_, err := svc.CreateResource(validInput("shared"), alice.ID, nil)
	require.NoError(t, err)

	before := countRows(t)
	res, err := svc.CreateResource(validInput("shared"), alice.ID, nil)
	assert.ErrorIs(t, err, ErrDuplicateName)
	assert.Nil(t, res)
	assert.Equal(t, before, countRows(t))

	// Same name in another user's namespace and in an org namespace is fine.
	_, err = svc.CreateResource(validInput("shared"), bob.ID, nil)
	require.NoError(t, err)
	_, err = svc.CreateResource(validInput("shared"), alice.ID, &org.ID)
	require.NoError(t, err)

	// Duplicate inside the org namespace.
	before = countRows(t)
	_, err = svc.CreateResource(validInput("shared"), alice.ID, &org.ID)
	assert.ErrorIs(t, err, ErrDuplicateName)
	assert.Equal(t, before, countRows(t))

	// Reverse order: a name taken in the org first is still free in the
	// personal namespace.
	_, err = svc.CreateResource(validInput("org-first"), alice.ID, &org.ID)
	require.NoError(t, err)
	_, err = svc.CreateResource(validInput("org-first"), alice.ID, nil)
	require.NoError(t, err)

	// Names differing only in case collide: resource URLs resolve
	// case-insensitively.
	before = countRows(t)
	_, err = svc.CreateResource(validInput("SHARED"), alice.ID, nil)
	assert.ErrorIs(t, err, ErrDuplicateName)
	assert.Equal(t, before, countRows(t))
}

func TestCreateResource_TrimsVersion(t *testing.T) {
	alice := createTestUser(t, "trimver-alice")
	svc := NewResourceService(database.DB)
	in := validInput("trimmed")
	in.Version = "  v1.2.3 "
	res, err := svc.CreateResource(in, alice.ID, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cleanupResource(t, res); database.DB.Delete(&alice) })
	require.Len(t, res.Versions, 1)
	assert.Equal(t, "v1.2.3", res.Versions[0].Version)
}

func TestIsInputError(t *testing.T) {
	for _, e := range []error{ErrInvalidName, ErrInvalidType, ErrInvalidRepositoryURL, ErrInvalidVersion, ErrDuplicateName} {
		assert.True(t, IsInputError(fmt.Errorf("wrapped: %w", e)), e.Error())
	}
	for _, e := range []error{ErrNotOrgMember, ErrOrgNotFound, errors.New("could not create resource: db down")} {
		assert.False(t, IsInputError(e), e.Error())
	}
}

func TestCreateResource_OrgAuthorization(t *testing.T) {
	owner := createTestUser(t, "orgauth-owner")
	member := createTestUser(t, "orgauth-member")
	outsider := createTestUser(t, "orgauth-outsider")
	pending := createTestUser(t, "orgauth-pending")
	org := models.Organization{Name: "orgauth-org"}
	require.NoError(t, database.DB.Create(&org).Error)
	t.Cleanup(func() {
		database.DB.Exec("DELETE FROM memberships WHERE organization_id = ?", org.ID)
		database.DB.Exec("DELETE FROM nomad_resources WHERE organization_id = ?", org.ID)
		database.DB.Delete(&org)
		for _, u := range []models.User{owner, member, outsider, pending} {
			database.DB.Delete(&u)
		}
	})
	require.NoError(t, database.DB.Create(&models.Membership{UserID: owner.ID, OrganizationID: org.ID, Role: RoleOwner}).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: member.ID, OrganizationID: org.ID, Role: RoleMember}).Error)
	require.NoError(t, database.DB.Create(&models.Membership{UserID: pending.ID, OrganizationID: org.ID, Role: "invited"}).Error)
	svc := NewResourceService(database.DB)

	missing := uint(999999)
	rejects := []struct {
		name  string
		user  uint
		orgID uint
		want  error
	}{
		{"outsider", outsider.ID, org.ID, ErrNotOrgMember},
		{"non-member role", pending.ID, org.ID, ErrNotOrgMember},
		{"nonexistent org", owner.ID, missing, ErrOrgNotFound},
		{"org id zero", owner.ID, 0, ErrOrgNotFound},
	}
	for _, tt := range rejects {
		t.Run(tt.name, func(t *testing.T) {
			before := countRows(t)
			res, err := svc.CreateResource(validInput("orgauth-res"), tt.user, &tt.orgID)
			assert.ErrorIs(t, err, tt.want)
			assert.Nil(t, res)
			assert.Equal(t, before, countRows(t), "authorization must run before persist")
		})
	}

	t.Run("outsider cannot probe existing names", func(t *testing.T) {
		_, err := svc.CreateResource(validInput("probe"), owner.ID, &org.ID)
		require.NoError(t, err)
		_, err = svc.CreateResource(validInput("probe"), outsider.ID, &org.ID)
		assert.ErrorIs(t, err, ErrNotOrgMember)
		assert.NotErrorIs(t, err, ErrDuplicateName)
	})

	for name, uid := range map[string]uint{"owner": owner.ID, "member": member.ID} {
		t.Run(name+" allowed", func(t *testing.T) {
			res, err := svc.CreateResource(validInput("orgauth-"+name), uid, &org.ID)
			require.NoError(t, err)
			assert.Equal(t, "orgauth-org", res.Organization.Name)
			assert.NotEmpty(t, res.User.Username)
		})
	}
}

func TestCreateResource_ReturnsUser(t *testing.T) {
	user := createTestUser(t, "retuser")
	t.Cleanup(func() { database.DB.Delete(&user) })
	res, err := NewResourceService(database.DB).CreateResource(validInput("ret-res"), user.ID, nil)
	require.NoError(t, err)
	cleanupResource(t, res)
	assert.Equal(t, "retuser", res.User.Username)
	assert.Nil(t, res.OrganizationID)
}

func TestCreateResource_LicenseDetection(t *testing.T) {
	user := createTestUser(t, "licuser")
	t.Cleanup(func() { database.DB.Delete(&user) })

	tests := []struct {
		name      string
		license   string
		fetcher   func(calls *[]string) FileFetcher
		want      string
		wantCalls int
	}{
		{"mit", "", func(c *[]string) FileFetcher {
			return func(u, f string) (string, error) {
				*c = append(*c, u+" "+f)
				return "The MIT License\n\nCopyright", nil
			}
		}, "MIT License", 1},
		{"download error", "", func(c *[]string) FileFetcher {
			return func(u, f string) (string, error) { *c = append(*c, f); return "", errors.New("boom") }
		}, "Unknown", 1},
		{"empty body", "", func(c *[]string) FileFetcher {
			return func(u, f string) (string, error) { *c = append(*c, f); return "", nil }
		}, "Unknown", 1},
		{"blank first line", "", func(c *[]string) FileFetcher {
			return func(u, f string) (string, error) { *c = append(*c, f); return "\n\nfoo", nil }
		}, "Unknown", 1},
		{"long first line truncated", "", func(c *[]string) FileFetcher {
			return func(u, f string) (string, error) { *c = append(*c, f); return strings.Repeat("a", 40), nil }
		}, strings.Repeat("a", 25) + "...", 1},
		{"explicit license skips downloader", "Apache-2.0", func(c *[]string) FileFetcher {
			return func(u, f string) (string, error) { *c = append(*c, f); return "The MIT License", nil }
		}, "Apache-2.0", 0},
		{"nil fetcher", "", func(*[]string) FileFetcher { return nil }, "Unknown", 0},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			svc := NewResourceService(database.DB).WithFileFetcher(tt.fetcher(&calls))
			in := validInput("lic-" + string(rune('a'+i)))
			in.License = tt.license
			res, err := svc.CreateResource(in, user.ID, nil)
			require.NoError(t, err)
			cleanupResource(t, res)
			assert.Equal(t, tt.want, res.License)
			assert.Len(t, calls, tt.wantCalls)
			if tt.name == "mit" {
				assert.Equal(t, []string{validRepoURL + " LICENSE"}, calls)
			}
		})
	}
}

func TestCreateResource_RejectedInputNeverFetches(t *testing.T) {
	user := createTestUser(t, "nofetch")
	t.Cleanup(func() { database.DB.Delete(&user) })
	calls := 0
	svc := NewResourceService(database.DB).WithFileFetcher(func(string, string) (string, error) { calls++; return "", nil })
	in := validInput("nofetch-res")
	in.License = ""
	in.RepositoryURL = "https://evil.example/x"
	_, err := svc.CreateResource(in, user.ID, nil)
	assert.ErrorIs(t, err, ErrInvalidRepositoryURL)
	assert.Zero(t, calls)
}

func TestWithContext_KeepsFileFetcher(t *testing.T) {
	called := false
	svc := NewResourceService(database.DB).WithFileFetcher(func(string, string) (string, error) { called = true; return "", nil })
	bound := svc.WithContext(t.Context())
	require.NotNil(t, bound.fetchFile)
	_, _ = bound.fetchFile("", "")
	assert.True(t, called)
}

func TestDetectLicense(t *testing.T) {
	tests := []struct{ in, want string }{
		{"The MIT License\nmore", "MIT License"},
		{"  Apache License  \r\nx", "Apache License"},
		{"", "Unknown"},
		{"\n", "Unknown"},
		{strings.Repeat("b", 26), strings.Repeat("b", 25) + "..."},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, detectLicense(tt.in), "%q", tt.in)
	}
}
