package resource

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
)

type updateEnv struct {
	svc      *ResourceService
	ownerA   models.User // owner of orgA
	memberA  models.User // plain member of orgA
	memberB  models.User // owner of orgB and plain member of orgA
	outsider models.User
	orgA     models.Organization
	orgB     models.Organization
}

func newUpdateEnv(t *testing.T, prefix string) updateEnv {
	t.Helper()
	e := updateEnv{svc: NewResourceService(database.DB)}
	e.ownerA = createTestUser(t, prefix+"-ownera")
	e.memberA = createTestUser(t, prefix+"-membera")
	e.memberB = createTestUser(t, prefix+"-ownerb")
	e.outsider = createTestUser(t, prefix+"-out")
	e.orgA = models.Organization{Name: prefix + "-orga"}
	e.orgB = models.Organization{Name: prefix + "-orgb"}
	require.NoError(t, database.DB.Create(&e.orgA).Error)
	require.NoError(t, database.DB.Create(&e.orgB).Error)
	for _, m := range []models.Membership{
		{UserID: e.ownerA.ID, OrganizationID: e.orgA.ID, Role: RoleOwner},
		{UserID: e.memberA.ID, OrganizationID: e.orgA.ID, Role: RoleMember},
		{UserID: e.memberB.ID, OrganizationID: e.orgA.ID, Role: RoleMember},
		{UserID: e.memberB.ID, OrganizationID: e.orgB.ID, Role: RoleOwner},
		{UserID: e.ownerA.ID, OrganizationID: e.orgB.ID, Role: RoleMember},
	} {
		require.NoError(t, database.DB.Create(&m).Error)
	}
	t.Cleanup(func() {
		ids := []uint{e.orgA.ID, e.orgB.ID}
		database.DB.Exec("DELETE FROM resource_tags WHERE nomad_resource_id IN (SELECT id FROM nomad_resources WHERE organization_id IN ? OR user_id IN ?)", ids, []uint{e.ownerA.ID, e.memberA.ID, e.memberB.ID, e.outsider.ID})
		database.DB.Exec("DELETE FROM resource_versions WHERE nomad_resource_id IN (SELECT id FROM nomad_resources WHERE organization_id IN ? OR user_id IN ?)", ids, []uint{e.ownerA.ID, e.memberA.ID, e.memberB.ID, e.outsider.ID})
		database.DB.Unscoped().Exec("DELETE FROM nomad_resources WHERE organization_id IN ? OR user_id IN ?", ids, []uint{e.ownerA.ID, e.memberA.ID, e.memberB.ID, e.outsider.ID})
		database.DB.Exec("DELETE FROM memberships WHERE organization_id IN ?", ids)
		database.DB.Exec("DELETE FROM organizations WHERE id IN ?", ids)
	})
	return e
}

func (e updateEnv) create(t *testing.T, name string, userID uint, orgID *uint) *models.NomadResource {
	t.Helper()
	res, err := e.svc.CreateResource(validInput(name), userID, orgID)
	require.NoError(t, err)
	return res
}

func updateInput(res *models.NomadResource, owner string) UpdateInput {
	return UpdateInput{
		Name:          res.Name,
		Type:          string(res.Type),
		Owner:         owner,
		Description:   res.Description,
		RepositoryURL: res.RepositoryURL,
		FilePath:      res.FilePath,
		License:       res.License,
	}
}

func reload(t *testing.T, id uint) models.NomadResource {
	t.Helper()
	var r models.NomadResource
	require.NoError(t, database.DB.First(&r, id).Error)
	return r
}

func orgOwner(id uint) string { return fmt.Sprintf("org:%d", id) }

func TestUpdateResource_UnchangedKeepsOrganization(t *testing.T) {
	e := newUpdateEnv(t, "upd-keep")
	res := e.create(t, "upd-keep-res", e.memberA.ID, &e.orgA.ID)

	for _, owner := range []string{"", orgOwner(e.orgA.ID)} {
		t.Run("owner="+owner, func(t *testing.T) {
			result, err := e.svc.UpdateResource(res.ID, updateInput(res, owner), e.memberA.ID)
			require.NoError(t, err)
			assert.False(t, result.Transferred)
			got := reload(t, res.ID)
			require.NotNil(t, got.OrganizationID)
			assert.Equal(t, e.orgA.ID, *got.OrganizationID)
			assert.Equal(t, e.memberA.ID, got.UserID)
		})
	}
}

func TestUpdateResource_PersonalUnchanged(t *testing.T) {
	e := newUpdateEnv(t, "upd-pers")
	res := e.create(t, "upd-pers-res", e.outsider.ID, nil)

	for _, owner := range []string{"", OwnerPersonal} {
		result, err := e.svc.UpdateResource(res.ID, updateInput(res, owner), e.outsider.ID)
		require.NoError(t, err, owner)
		assert.False(t, result.Transferred, owner)
		got := reload(t, res.ID)
		assert.Nil(t, got.OrganizationID)
		assert.Equal(t, e.outsider.ID, got.UserID)
	}
}

func TestUpdateResource_MoveOutRequiresOwner(t *testing.T) {
	e := newUpdateEnv(t, "upd-out")
	res := e.create(t, "upd-out-res", e.memberA.ID, &e.orgA.ID)

	tests := []struct {
		name   string
		actor  uint
		target string
	}{
		{"member to personal", e.memberA.ID, OwnerPersonal},
		{"member to other org", e.memberB.ID, orgOwner(e.orgB.ID)},
		{"member creator to other org", e.memberA.ID, orgOwner(e.orgB.ID)},
		{"outsider to personal", e.outsider.ID, OwnerPersonal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := updateInput(res, tt.target)
			in.Tags = "should-not-land"
			_, err := e.svc.UpdateResource(res.ID, in, tt.actor)
			assert.ErrorIs(t, err, ErrUnauthorized)
			got := reload(t, res.ID)
			require.NotNil(t, got.OrganizationID)
			assert.Equal(t, e.orgA.ID, *got.OrganizationID)
			assert.Equal(t, e.memberA.ID, got.UserID)
			assert.NotContains(t, tagNames(t, res.ID), "should-not-land")
		})
	}
}

func TestUpdateResource_OwnerMovesOutToOwnNamespace(t *testing.T) {
	e := newUpdateEnv(t, "upd-own")
	// Created by a plain member; an owner moves it out.
	res := e.create(t, "upd-own-res", e.memberA.ID, &e.orgA.ID)

	result, err := e.svc.UpdateResource(res.ID, updateInput(res, OwnerPersonal), e.ownerA.ID)
	require.NoError(t, err)
	assert.True(t, result.Transferred)
	require.NotNil(t, result.FromOrgID)
	assert.Equal(t, e.orgA.ID, *result.FromOrgID)
	assert.Nil(t, result.ToOrgID)
	assert.Equal(t, e.memberA.ID, result.FromUserID)
	assert.Equal(t, e.ownerA.ID, result.ToUserID)

	got := reload(t, res.ID)
	assert.Nil(t, got.OrganizationID)
	assert.Equal(t, e.ownerA.ID, got.UserID)
}

func TestUpdateResource_OwnerMovesOrgToOrg(t *testing.T) {
	e := newUpdateEnv(t, "upd-o2o")
	res := e.create(t, "upd-o2o-res", e.memberA.ID, &e.orgA.ID)

	// ownerA owns orgA and is a member of orgB.
	result, err := e.svc.UpdateResource(res.ID, updateInput(res, orgOwner(e.orgB.ID)), e.ownerA.ID)
	require.NoError(t, err)
	assert.True(t, result.Transferred)
	got := reload(t, res.ID)
	require.NotNil(t, got.OrganizationID)
	assert.Equal(t, e.orgB.ID, *got.OrganizationID)
	assert.Equal(t, e.memberA.ID, got.UserID, "org to org leaves the creator alone")
}

func TestUpdateResource_PersonalIntoOrg(t *testing.T) {
	e := newUpdateEnv(t, "upd-in")
	res := e.create(t, "upd-in-res", e.memberA.ID, nil)

	_, err := e.svc.UpdateResource(res.ID, updateInput(res, orgOwner(e.orgB.ID)), e.memberA.ID)
	assert.ErrorIs(t, err, ErrUnauthorized, "not a member of the target")

	result, err := e.svc.UpdateResource(res.ID, updateInput(res, orgOwner(e.orgA.ID)), e.memberA.ID)
	require.NoError(t, err)
	assert.True(t, result.Transferred)
	got := reload(t, res.ID)
	require.NotNil(t, got.OrganizationID)
	assert.Equal(t, e.orgA.ID, *got.OrganizationID)
}

func TestUpdateResource_MissingAndForeignOrgIndistinguishable(t *testing.T) {
	e := newUpdateEnv(t, "upd-oracle")
	res := e.create(t, "upd-oracle-res", e.outsider.ID, nil)

	_, errForeign := e.svc.UpdateResource(res.ID, updateInput(res, orgOwner(e.orgA.ID)), e.outsider.ID)
	_, errMissing := e.svc.UpdateResource(res.ID, updateInput(res, orgOwner(999999999)), e.outsider.ID)
	require.Error(t, errForeign)
	require.Error(t, errMissing)
	assert.Equal(t, errForeign, errMissing)
	assert.ErrorIs(t, errMissing, ErrUnauthorized)
}

func TestUpdateResource_OtherUsersPersonalResource(t *testing.T) {
	e := newUpdateEnv(t, "upd-other")
	res := e.create(t, "upd-other-res", e.outsider.ID, nil)

	for _, owner := range []string{"", OwnerPersonal, orgOwner(e.orgA.ID)} {
		_, err := e.svc.UpdateResource(res.ID, updateInput(res, owner), e.memberA.ID)
		assert.ErrorIs(t, err, ErrUnauthorized, owner)
	}
	got := reload(t, res.ID)
	assert.Nil(t, got.OrganizationID)
	assert.Equal(t, e.outsider.ID, got.UserID)
}

func TestUpdateResource_CollisionInActorNamespace(t *testing.T) {
	e := newUpdateEnv(t, "upd-coll")
	// The actor already has a personal resource with the same name.
	e.create(t, "upd-coll-res", e.ownerA.ID, nil)
	res := e.create(t, "upd-coll-res", e.memberA.ID, &e.orgA.ID)

	_, err := e.svc.UpdateResource(res.ID, updateInput(res, OwnerPersonal), e.ownerA.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	got := reload(t, res.ID)
	require.NotNil(t, got.OrganizationID)

	// The creator's own namespace is not the target, so a collision there is irrelevant.
	e.create(t, "upd-coll-other", e.memberA.ID, nil)
	res2 := e.create(t, "upd-coll-other", e.memberB.ID, &e.orgA.ID)
	_, err = e.svc.UpdateResource(res2.ID, updateInput(res2, OwnerPersonal), e.ownerA.ID)
	require.NoError(t, err)
}

func TestUpdateResource_InvalidOwner(t *testing.T) {
	e := newUpdateEnv(t, "upd-bad")
	res := e.create(t, "upd-bad-res", e.memberA.ID, &e.orgA.ID)

	for _, owner := range []string{"garbage", "org:5x", "org:", "org:-1", "org: 5", "Personal", "org:0x5"} {
		t.Run(owner, func(t *testing.T) {
			_, err := e.svc.UpdateResource(res.ID, updateInput(res, owner), e.ownerA.ID)
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrUnauthorized)
			got := reload(t, res.ID)
			require.NotNil(t, got.OrganizationID)
			assert.Equal(t, e.orgA.ID, *got.OrganizationID)
		})
	}
}
