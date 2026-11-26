package resource

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"rmbl/internal/database"
	"rmbl/internal/models"
)

type purgeEnv struct {
	svc   *ResourceService
	alice models.User
	bob   models.User
	org   models.Organization
}

func newPurgeEnv(t *testing.T, prefix string) purgeEnv {
	t.Helper()
	alice := createTestUser(t, prefix+"-alice")
	bob := createTestUser(t, prefix+"-bob")
	org := models.Organization{Name: prefix + "-org"}
	require.NoError(t, database.DB.Create(&org).Error)
	t.Cleanup(func() {
		database.DB.Exec("DELETE FROM memberships WHERE organization_id = ?", org.ID)
		database.DB.Exec("DELETE FROM resource_versions WHERE resource_id IN (SELECT id FROM nomad_resources WHERE user_id IN (?,?))", alice.ID, bob.ID)
		database.DB.Exec("DELETE FROM resource_tags WHERE nomad_resource_id IN (SELECT id FROM nomad_resources WHERE user_id IN (?,?))", alice.ID, bob.ID)
		database.DB.Exec("DELETE FROM user_stars WHERE nomad_resource_id IN (SELECT id FROM nomad_resources WHERE user_id IN (?,?))", alice.ID, bob.ID)
		database.DB.Exec("DELETE FROM nomad_resources WHERE user_id IN (?,?)", alice.ID, bob.ID)
		database.DB.Delete(&org)
		database.DB.Delete(&alice)
		database.DB.Delete(&bob)
	})
	require.NoError(t, database.DB.Create(&models.Membership{UserID: alice.ID, OrganizationID: org.ID, Role: RoleMember}).Error)
	return purgeEnv{svc: NewResourceService(database.DB), alice: alice, bob: bob, org: org}
}

// deadResource creates a resource with a version, a tag and a star, then
// soft-deletes it.
func (e purgeEnv) deadResource(t *testing.T, name string, orgID *uint) models.NomadResource {
	t.Helper()
	in := validInput(name)
	in.Tags = "dead-tag"
	res, err := e.svc.CreateResource(in, e.alice.ID, orgID)
	require.NoError(t, err)
	require.NoError(t, database.DB.Model(res).Association("StarredBy").Append(&models.User{ID: e.bob.ID}))
	require.NoError(t, database.DB.Delete(&models.NomadResource{}, res.ID).Error)
	return *res
}

func rowsFor(t *testing.T, id uint) (resources, versions, tags, stars int64) {
	t.Helper()
	require.NoError(t, database.DB.Unscoped().Model(&models.NomadResource{}).Where("id = ?", id).Count(&resources).Error)
	require.NoError(t, database.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", id).Count(&versions).Error)
	require.NoError(t, database.DB.Table("resource_tags").Where("nomad_resource_id = ?", id).Count(&tags).Error)
	require.NoError(t, database.DB.Table("user_stars").Where("nomad_resource_id = ?", id).Count(&stars).Error)
	return
}

func hardDelete(t *testing.T, id uint) {
	t.Helper()
	require.NoError(t, database.DB.Exec("DELETE FROM resource_versions WHERE resource_id = ?", id).Error)
	require.NoError(t, database.DB.Exec("DELETE FROM resource_tags WHERE nomad_resource_id = ?", id).Error)
	require.NoError(t, database.DB.Exec("DELETE FROM nomad_resources WHERE id = ?", id).Error)
}

func assertPurged(t *testing.T, id uint) {
	t.Helper()
	r, v, tg, s := rowsFor(t, id)
	assert.EqualValues(t, []int64{0, 0, 0, 0}, []int64{r, v, tg, s})
}

func assertIntact(t *testing.T, id uint) {
	t.Helper()
	r, v, tg, s := rowsFor(t, id)
	assert.EqualValues(t, []int64{1, 1, 1, 1}, []int64{r, v, tg, s})
}

func TestCreateResource_PurgesSoftDeletedName(t *testing.T) {
	e := newPurgeEnv(t, "purge")
	tests := []struct {
		name    string
		deadAs  string
		newName string
		org     bool
	}{
		{"personal same name", "x", "x", false},
		{"personal case variant", "x", "X", false},
		{"org same name", "x", "x", true},
		{"org case variant", "x", "X", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var orgID *uint
			if tt.org {
				orgID = &e.org.ID
			}
			dead := e.deadResource(t, tt.deadAs, orgID)
			assertIntact(t, dead.ID)

			res, err := e.svc.CreateResource(validInput(tt.newName), e.alice.ID, orgID)
			require.NoError(t, err)
			assertPurged(t, dead.ID)
			r, _, _, _ := rowsFor(t, res.ID)
			assert.EqualValues(t, 1, r)
			hardDelete(t, res.ID)
		})
	}
}

func TestCreateResource_PurgeIsNamespaceScoped(t *testing.T) {
	e := newPurgeEnv(t, "purgens")
	bobDead := e.deadResource(t, "x", nil)
	require.NoError(t, database.DB.Exec("UPDATE nomad_resources SET user_id = ? WHERE id = ?", e.bob.ID, bobDead.ID).Error)
	orgDead := e.deadResource(t, "x", &e.org.ID)
	aliceDead := e.deadResource(t, "y", nil)

	// Creating personal "x" for alice must not touch bob's or the org's.
	res, err := e.svc.CreateResource(validInput("x"), e.alice.ID, nil)
	require.NoError(t, err)
	assertIntact(t, bobDead.ID)
	assertIntact(t, orgDead.ID)
	assertIntact(t, aliceDead.ID)

	// Creating org "x" must not touch alice's personal rows.
	res2, err := e.svc.CreateResource(validInput("x"), e.alice.ID, &e.org.ID)
	require.NoError(t, err)
	assertPurged(t, orgDead.ID)
	assertIntact(t, bobDead.ID)
	assertIntact(t, aliceDead.ID)
	hardDelete(t, res.ID)
	hardDelete(t, res2.ID)
}

func TestCreateResource_LiveDuplicateDoesNotPurge(t *testing.T) {
	e := newPurgeEnv(t, "purgedup")
	dead := e.deadResource(t, "x", nil)
	// A live resource whose name differs only in case: 'x' dead, 'X' live is
	// created by purging, so build the live row directly.
	live := models.NomadResource{Name: "X", Type: models.ResourceTypePack, UserID: e.alice.ID}
	require.NoError(t, database.DB.Exec("UPDATE nomad_resources SET name = 'x-old' WHERE id = ?", dead.ID).Error)
	require.NoError(t, database.DB.Create(&live).Error)
	require.NoError(t, database.DB.Exec("UPDATE nomad_resources SET name = 'x' WHERE id = ?", dead.ID).Error)
	// Now dead 'x' and live 'X' coexist; creating 'x' must fail and purge nothing.
	_, err := e.svc.CreateResource(validInput("x"), e.alice.ID, nil)
	assert.ErrorIs(t, err, ErrDuplicateName)
	assertIntact(t, dead.ID)
}

func TestCreateResource_RejectedCreateDoesNotPurge(t *testing.T) {
	e := newPurgeEnv(t, "purgerej")
	deadOrg := e.deadResource(t, "x", &e.org.ID)
	deadPersonal := e.deadResource(t, "x", nil)

	bad := validInput("x")
	bad.Type = "bogus"
	_, err := e.svc.CreateResource(bad, e.alice.ID, nil)
	assert.ErrorIs(t, err, ErrInvalidType)
	assertIntact(t, deadPersonal.ID)

	// An unauthorised caller (bob is not in the org) with otherwise valid input.
	_, err = e.svc.CreateResource(validInput("x"), e.bob.ID, &e.org.ID)
	assert.ErrorIs(t, err, ErrNotOrgMember)
	assertIntact(t, deadOrg.ID)
}

func TestCreateResource_InsertFailureRollsBackPurge(t *testing.T) {
	e := newPurgeEnv(t, "purgerb")
	dead := e.deadResource(t, "x", nil)

	boom := errors.New("forced insert failure")
	failing := database.DB.Session(&gorm.Session{NewDB: true})
	require.NoError(t, failing.Callback().Create().Before("gorm:create").Register("test:fail_resource_insert", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "nomad_resources" {
			_ = tx.AddError(boom)
		}
	}))
	t.Cleanup(func() { _ = database.DB.Callback().Create().Remove("test:fail_resource_insert") })

	_, err := NewResourceService(failing).CreateResource(validInput("x"), e.alice.ID, nil)
	require.ErrorIs(t, err, boom)
	assertIntact(t, dead.ID)
}

func TestPurgeDeletedInOrgs(t *testing.T) {
	e := newPurgeEnv(t, "purgeorg")
	dead := e.deadResource(t, "dead", &e.org.ID)
	live, err := e.svc.CreateResource(validInput("live"), e.alice.ID, &e.org.ID)
	require.NoError(t, err)
	personalDead := e.deadResource(t, "pdead", nil)

	require.NoError(t, database.DB.Transaction(func(tx *gorm.DB) error {
		return PurgeDeletedInOrgs(tx, []uint{e.org.ID})
	}))
	assertPurged(t, dead.ID)
	r, _, _, _ := rowsFor(t, live.ID)
	assert.EqualValues(t, 1, r)
	assertIntact(t, personalDead.ID)
	hardDelete(t, live.ID)

	require.NoError(t, database.DB.Transaction(func(tx *gorm.DB) error {
		return PurgeDeletedInOrgs(tx, nil)
	}))
}

// A create that waits on the organisation row while it is being deleted must
// fail with ErrOrgNotFound once the delete commits.
func TestCreateResource_OrgDeletedConcurrently(t *testing.T) {
	e := newPurgeEnv(t, "racedel")

	locked := make(chan struct{})
	release := make(chan struct{})
	deleted := make(chan error, 1)
	go func() {
		deleted <- database.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&models.Organization{}, e.org.ID).Error; err != nil {
				return err
			}
			if err := tx.Delete(&models.Organization{}, e.org.ID).Error; err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case err := <-deleted:
		t.Fatalf("delete finished early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out taking the org lock")
	}

	// The pre-check outside the transaction still sees the live row, so the
	// create reaches the locking re-check and blocks there.
	created := make(chan error, 1)
	go func() {
		_, err := e.svc.CreateResource(validInput("racing"), e.alice.ID, &e.org.ID)
		created <- err
	}()
	select {
	case err := <-created:
		t.Fatalf("create did not wait for the delete: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-created:
		require.ErrorIs(t, err, ErrOrgNotFound)
	case <-time.After(10 * time.Second):
		t.Fatal("create did not finish after the delete committed")
	}
	require.NoError(t, <-deleted)
	var n int64
	require.NoError(t, database.DB.Unscoped().Model(&models.NomadResource{}).Where("organization_id = ?", e.org.ID).Count(&n).Error)
	assert.Zero(t, n)
}
