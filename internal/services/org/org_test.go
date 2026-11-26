package org

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"rmbl/internal/database"
	"rmbl/internal/models"
)

func TestMain(m *testing.M) {
	_, cleanup := database.SetupTestDB()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func mkUser(t *testing.T, username string) models.User {
	t.Helper()
	u := models.User{Username: username, Email: username + "@test.example.com"}
	require.NoError(t, database.DB.Create(&u).Error)
	return u
}

func mkOrg(t *testing.T, name string, owner models.User) models.Organization {
	t.Helper()
	o, err := CreateOrg(database.DB, owner.ID, name, "")
	require.NoError(t, err)
	return o
}

func liveMembers(t *testing.T, orgID uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, database.DB.Model(&models.Membership{}).Where("organization_id = ?", orgID).Count(&n).Error)
	return n
}

func TestCreateOrg(t *testing.T) {
	owner := mkUser(t, "co-owner")
	mkUser(t, "CoTaken")
	mkOrg(t, "CoExisting", owner)

	tests := []struct {
		name string
		in   string
		want error
	}{
		{"empty", "", ErrNameRequired},
		{"whitespace", "   ", ErrNameRequired},
		{"reserved", "admin", ErrNameReserved},
		{"reserved mixed case", "  Admin ", ErrNameReserved},
		{"user clash other case", "cotaken", ErrNameTakenByUser},
		{"org clash other case", "coexisting", ErrNameTaken},
		{"ok trimmed", "  co-fresh ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org, err := CreateOrg(database.DB, owner.ID, tt.in, "d")
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "co-fresh", org.Name)
			var ms []models.Membership
			require.NoError(t, database.DB.Where("organization_id = ?", org.ID).Find(&ms).Error)
			require.Len(t, ms, 1)
			assert.Equal(t, RoleOwner, ms[0].Role)
			assert.Equal(t, owner.ID, ms[0].UserID)
		})
	}
}

func TestCreateOrg_MembershipFailureRollsBack(t *testing.T) {
	owner := mkUser(t, "rb-owner")
	sqlDB, err := database.DB.DB()
	require.NoError(t, err)
	fdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, fdb.Callback().Create().Before("gorm:create").Register("test:fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "memberships" {
			_ = tx.AddError(errors.New("forced failure"))
		}
	}))

	_, err = CreateOrg(fdb, owner.ID, "rb-org", "")
	require.Error(t, err)
	assert.False(t, IsInputError(err))

	var n int64
	require.NoError(t, database.DB.Unscoped().Model(&models.Organization{}).Where("name = ?", "rb-org").Count(&n).Error)
	assert.Zero(t, n)
}

func TestGetByName(t *testing.T) {
	owner := mkUser(t, "gn-owner")
	o := mkOrg(t, "GnOrg", owner)

	got, err := GetByName(database.DB, "gnorg")
	require.NoError(t, err)
	assert.Equal(t, o.ID, got.ID)

	_, err = GetByName(database.DB, "gn-missing")
	require.ErrorIs(t, err, ErrNotFound)

	got, err = GetByID(database.DB, o.ID)
	require.NoError(t, err)
	assert.Equal(t, "GnOrg", got.Name)
	_, err = GetByID(database.DB, o.ID+100000)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRequireOwnerAndMemberRole(t *testing.T) {
	owner := mkUser(t, "ro-owner")
	member := mkUser(t, "ro-member")
	outsider := mkUser(t, "ro-outsider")
	o := mkOrg(t, "ro-org", owner)
	_, err := AddMember(database.DB, o.ID, "ro-member", RoleMember)
	require.NoError(t, err)

	require.NoError(t, RequireOwner(database.DB, o.ID, owner.ID))
	require.ErrorIs(t, RequireOwner(database.DB, o.ID, member.ID), ErrNotOwner)
	require.ErrorIs(t, RequireOwner(database.DB, o.ID, outsider.ID), ErrNotOwner)

	role, err := MemberRole(database.DB, o.ID, member.ID)
	require.NoError(t, err)
	assert.Equal(t, RoleMember, role)
	_, err = MemberRole(database.DB, o.ID, outsider.ID)
	require.ErrorIs(t, err, ErrNotMember)
}

func TestUpdateDescription(t *testing.T) {
	owner := mkUser(t, "ud-owner")
	o := mkOrg(t, "ud-org", owner)
	require.NoError(t, UpdateDescription(database.DB, &o, "new text"))
	got, err := GetByID(database.DB, o.ID)
	require.NoError(t, err)
	assert.Equal(t, "new text", got.Description)
	assert.Equal(t, "ud-org", got.Name)
}

func TestParseRole(t *testing.T) {
	for _, ok := range []string{"owner", "member"} {
		got, err := ParseRole(ok)
		require.NoError(t, err)
		assert.Equal(t, ok, got)
	}
	for _, bad := range []string{"", "admin", "Owner", "viewer"} {
		_, err := ParseRole(bad)
		require.ErrorIs(t, err, ErrInvalidRole, bad)
	}
}

func TestAddMember(t *testing.T) {
	owner := mkUser(t, "am-owner")
	target := mkUser(t, "Am-Target")
	o := mkOrg(t, "am-org", owner)

	t.Run("invalid role writes nothing", func(t *testing.T) {
		_, err := AddMember(database.DB, o.ID, "Am-Target", "admin")
		require.ErrorIs(t, err, ErrInvalidRole)
		assert.EqualValues(t, 1, liveMembers(t, o.ID))
	})
	t.Run("unknown user", func(t *testing.T) {
		_, err := AddMember(database.DB, o.ID, "am-nobody", RoleMember)
		require.ErrorIs(t, err, ErrUserNotFound)
	})
	t.Run("case-insensitive username", func(t *testing.T) {
		m, err := AddMember(database.DB, o.ID, "am-target", RoleMember)
		require.NoError(t, err)
		assert.Equal(t, target.ID, m.UserID)
	})
	t.Run("already member", func(t *testing.T) {
		_, err := AddMember(database.DB, o.ID, "Am-Target", RoleOwner)
		require.ErrorIs(t, err, ErrAlreadyMember)
	})
	t.Run("restore soft-deleted keeps one row", func(t *testing.T) {
		require.NoError(t, RemoveMember(database.DB, o.ID, target.ID))
		m, err := AddMember(database.DB, o.ID, "Am-Target", RoleOwner)
		require.NoError(t, err)
		assert.Equal(t, RoleOwner, m.Role)

		var rows []models.Membership
		require.NoError(t, database.DB.Unscoped().Where("user_id = ? AND organization_id = ?", target.ID, o.ID).Find(&rows).Error)
		require.Len(t, rows, 1)
		assert.False(t, rows[0].DeletedAt.Valid)
		assert.Equal(t, RoleOwner, rows[0].Role)
	})
}

func TestRemoveMember(t *testing.T) {
	owner := mkUser(t, "rm-owner")
	second := mkUser(t, "rm-second")
	plain := mkUser(t, "rm-plain")
	outsider := mkUser(t, "rm-outsider")
	o := mkOrg(t, "rm-org", owner)

	require.ErrorIs(t, RemoveMember(database.DB, o.ID, owner.ID), ErrLastOwner)
	require.ErrorIs(t, RemoveMember(database.DB, o.ID, outsider.ID), ErrNotMember)

	_, err := AddMember(database.DB, o.ID, "rm-plain", RoleMember)
	require.NoError(t, err)
	require.NoError(t, RemoveMember(database.DB, o.ID, plain.ID))

	_, err = AddMember(database.DB, o.ID, "rm-second", RoleOwner)
	require.NoError(t, err)
	require.NoError(t, RemoveMember(database.DB, o.ID, second.ID))
	require.ErrorIs(t, RemoveMember(database.DB, o.ID, owner.ID), ErrLastOwner)
	assert.EqualValues(t, 1, liveMembers(t, o.ID))
}

func TestIsInputError(t *testing.T) {
	yes := []error{ErrNameRequired, ErrNameInvalid, ErrNameReserved, ErrNameTakenByUser, ErrNameTaken, ErrInvalidRole, ErrAlreadyMember}
	no := []error{ErrNotFound, ErrNotOwner, ErrNotMember, ErrUserNotFound, ErrLastOwner, errors.New("x"), nil}
	for _, e := range yes {
		assert.True(t, IsInputError(e), "%v", e)
	}
	for _, e := range no {
		assert.False(t, IsInputError(e), "%v", e)
	}
}

func TestCreateOrg_InvalidNames(t *testing.T) {
	owner := mkUser(t, "inv-owner")
	for _, in := range []string{"-x", "a b", strings.Repeat("x", 65), "ü", ".dot", "a/b"} {
		t.Run(in, func(t *testing.T) {
			_, err := CreateOrg(database.DB, owner.ID, in, "")
			require.ErrorIs(t, err, ErrNameInvalid)
			assert.True(t, IsInputError(err))
			var n int64
			require.NoError(t, database.DB.Model(&models.Organization{}).Where("name = ?", in).Count(&n).Error)
			assert.Zero(t, n)
		})
	}
}

func TestMemberCount(t *testing.T) {
	owner := mkUser(t, "mc-owner")
	other := mkUser(t, "mc-other")
	o := mkOrg(t, "mc-org", owner)

	count := func() int64 {
		n, err := MemberCount(database.DB, o.ID)
		require.NoError(t, err)
		return n
	}
	assert.EqualValues(t, 1, count())
	_, err := AddMember(database.DB, o.ID, other.Username, RoleMember)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count())
	require.NoError(t, RemoveMember(database.DB, o.ID, other.ID))
	assert.EqualValues(t, 1, count())
}

// mkResource inserts a resource of the organisation with a version, a tag link
// and a star, optionally soft-deleted.
func mkResource(t *testing.T, name string, owner models.User, orgID uint, dead bool) models.NomadResource {
	t.Helper()
	r := models.NomadResource{
		Name: name, Type: models.ResourceTypePack, UserID: owner.ID, OrganizationID: &orgID,
		Versions: []models.ResourceVersion{{Version: "v1"}},
		Tags:     []models.Tag{{Name: "tag-" + name}},
	}
	require.NoError(t, database.DB.Create(&r).Error)
	require.NoError(t, database.DB.Model(&r).Association("StarredBy").Append(&models.User{ID: owner.ID}))
	if dead {
		require.NoError(t, database.DB.Delete(&models.NomadResource{}, r.ID).Error)
	}
	return r
}

func resourceRows(t *testing.T, id uint) []int64 {
	t.Helper()
	var r, v, tg, st int64
	require.NoError(t, database.DB.Unscoped().Model(&models.NomadResource{}).Where("id = ?", id).Count(&r).Error)
	require.NoError(t, database.DB.Unscoped().Model(&models.ResourceVersion{}).Where("resource_id = ?", id).Count(&v).Error)
	require.NoError(t, database.DB.Table("resource_tags").Where("nomad_resource_id = ?", id).Count(&tg).Error)
	require.NoError(t, database.DB.Table("user_stars").Where("nomad_resource_id = ?", id).Count(&st).Error)
	return []int64{r, v, tg, st}
}

func orgRows(t *testing.T, id uint) (orgs, members int64) {
	t.Helper()
	require.NoError(t, database.DB.Unscoped().Model(&models.Organization{}).Where("id = ?", id).Count(&orgs).Error)
	require.NoError(t, database.DB.Unscoped().Model(&models.Membership{}).Where("organization_id = ?", id).Count(&members).Error)
	return
}

func TestDeleteOrg_Empty(t *testing.T) {
	owner := mkUser(t, "do-owner")
	second := mkUser(t, "do-second")
	o := mkOrg(t, "do-org", owner)
	_, err := AddMember(database.DB, o.ID, second.Username, RoleOwner)
	require.NoError(t, err)

	require.NoError(t, DeleteOrg(database.DB, o.ID, owner.ID))

	_, err = GetByID(database.DB, o.ID)
	require.ErrorIs(t, err, ErrNotFound)
	assert.Zero(t, liveMembers(t, o.ID))
	var soft int64
	require.NoError(t, database.DB.Unscoped().Model(&models.Membership{}).
		Where("organization_id = ? AND deleted_at IS NOT NULL", o.ID).Count(&soft).Error)
	assert.EqualValues(t, 2, soft)
	var deleted int64
	require.NoError(t, database.DB.Unscoped().Model(&models.Organization{}).
		Where("id = ? AND deleted_at IS NOT NULL", o.ID).Count(&deleted).Error)
	assert.EqualValues(t, 1, deleted)
}

func TestDeleteOrg_Rejections(t *testing.T) {
	owner := mkUser(t, "dr-owner")
	member := mkUser(t, "dr-member")
	outsider := mkUser(t, "dr-outsider")
	o := mkOrg(t, "dr-org", owner)
	_, err := AddMember(database.DB, o.ID, member.Username, RoleMember)
	require.NoError(t, err)

	tests := []struct {
		name string
		user uint
		org  uint
		want error
	}{
		{"member", member.ID, o.ID, ErrNotOwner},
		{"non-member", outsider.ID, o.ID, ErrNotOwner},
		{"unknown org", owner.ID, o.ID + 100000, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorIs(t, DeleteOrg(database.DB, tt.org, tt.user), tt.want)
			_, err := GetByID(database.DB, o.ID)
			require.NoError(t, err)
			assert.EqualValues(t, 2, liveMembers(t, o.ID))
		})
	}

	require.NoError(t, DeleteOrg(database.DB, o.ID, owner.ID))
	require.ErrorIs(t, DeleteOrg(database.DB, o.ID, owner.ID), ErrNotFound)
}

func TestDeleteOrg_NotEmpty(t *testing.T) {
	owner := mkUser(t, "ne-owner")
	o := mkOrg(t, "ne-org", owner)
	mkResource(t, "ne-live1", owner, o.ID, false)
	mkResource(t, "ne-live2", owner, o.ID, false)
	mkResource(t, "ne-dead", owner, o.ID, true)

	err := DeleteOrg(database.DB, o.ID, owner.ID)
	ne, ok := errors.AsType[*NotEmptyError](err)
	require.True(t, ok, "got %v", err)
	assert.EqualValues(t, 2, ne.Count)
	assert.False(t, IsInputError(err))

	_, err = GetByID(database.DB, o.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, liveMembers(t, o.ID))
}

func TestCreateOrg_ReusesDeletedName(t *testing.T) {
	owner := mkUser(t, "ru-owner")
	for _, again := range []string{"ru-org", "RU-ORG"} {
		o := mkOrg(t, "ru-org", owner)
		require.NoError(t, DeleteOrg(database.DB, o.ID, owner.ID))

		n, err := CreateOrg(database.DB, owner.ID, again, "")
		require.NoError(t, err)
		assert.NotEqual(t, o.ID, n.ID)
		assert.Equal(t, again, n.Name)
		orgs, members := orgRows(t, o.ID)
		assert.Zero(t, orgs)
		assert.Zero(t, members)
		require.NoError(t, DeleteOrg(database.DB, n.ID, owner.ID))
	}
}

func TestCreateOrg_PurgeIsScopedToTheDeletedOrg(t *testing.T) {
	owner := mkUser(t, "ps-owner")
	other := mkOrg(t, "ps-other", owner)
	otherDead := mkResource(t, "ps-other-dead", owner, other.ID, true)
	otherLive := mkResource(t, "ps-other-live", owner, other.ID, false)

	o := mkOrg(t, "ps-org", owner)
	dead := mkResource(t, "ps-dead", owner, o.ID, true)
	require.NoError(t, DeleteOrg(database.DB, o.ID, owner.ID))
	assert.EqualValues(t, []int64{1, 1, 1, 1}, resourceRows(t, dead.ID))

	n, err := CreateOrg(database.DB, owner.ID, "ps-org", "")
	require.NoError(t, err)
	assert.EqualValues(t, []int64{0, 0, 0, 0}, resourceRows(t, dead.ID))
	assert.EqualValues(t, []int64{1, 1, 1, 1}, resourceRows(t, otherDead.ID))
	assert.EqualValues(t, []int64{1, 1, 1, 1}, resourceRows(t, otherLive.ID))
	orgs, _ := orgRows(t, other.ID)
	assert.EqualValues(t, 1, orgs)
	assert.EqualValues(t, 1, liveMembers(t, n.ID))
}

func TestCreateOrg_AdminDeletedOrg(t *testing.T) {
	owner := mkUser(t, "ad-owner")

	t.Run("live resource keeps the name taken", func(t *testing.T) {
		o := mkOrg(t, "ad-live", owner)
		live := mkResource(t, "ad-live-res", owner, o.ID, false)
		dead := mkResource(t, "ad-live-dead", owner, o.ID, true)
		require.NoError(t, database.DB.Delete(&o).Error)

		_, err := CreateOrg(database.DB, owner.ID, "AD-LIVE", "")
		require.ErrorIs(t, err, ErrNameTaken)
		assert.EqualValues(t, []int64{1, 1, 1, 1}, resourceRows(t, live.ID))
		assert.EqualValues(t, []int64{1, 1, 1, 1}, resourceRows(t, dead.ID))
		orgs, members := orgRows(t, o.ID)
		assert.EqualValues(t, 1, orgs)
		assert.EqualValues(t, 1, members)
	})

	t.Run("live memberships and no resources are purged", func(t *testing.T) {
		o := mkOrg(t, "ad-free", owner)
		require.NoError(t, database.DB.Delete(&o).Error)
		assert.EqualValues(t, 1, liveMembers(t, o.ID))

		n, err := CreateOrg(database.DB, owner.ID, "ad-free", "")
		require.NoError(t, err)
		assert.NotEqual(t, o.ID, n.ID)
		orgs, members := orgRows(t, o.ID)
		assert.Zero(t, orgs)
		assert.Zero(t, members)
	})
}

func TestDeleteOrg_FailureRollsBack(t *testing.T) {
	owner := mkUser(t, "dx-owner")
	o := mkOrg(t, "dx-org", owner)
	sqlDB, err := database.DB.DB()
	require.NoError(t, err)
	fdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, fdb.Callback().Delete().Before("gorm:delete").Register("test:fail-org-delete", func(tx *gorm.DB) {
		if tx.Statement.Table == "organizations" {
			_ = tx.AddError(errors.New("forced failure"))
		}
	}))

	require.Error(t, DeleteOrg(fdb, o.ID, owner.ID))

	_, err = GetByID(database.DB, o.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, liveMembers(t, o.ID))
}

func membershipRows(t *testing.T, orgID, userID uint) []models.Membership {
	t.Helper()
	var rows []models.Membership
	require.NoError(t, database.DB.Unscoped().Where("user_id = ? AND organization_id = ?", userID, orgID).Find(&rows).Error)
	return rows
}

func TestFindUser(t *testing.T) {
	u := mkUser(t, "Fu-Mixed")
	got, err := FindUser(database.DB, "fu-mixed")
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	_, err = FindUser(database.DB, "fu-nobody")
	require.ErrorIs(t, err, ErrUserNotFound)
}

func TestSetMemberRole(t *testing.T) {
	owner := mkUser(t, "sr-owner")
	other := mkUser(t, "sr-other")
	target := mkUser(t, "Sr-Target")
	plain := mkUser(t, "sr-plain")
	o := mkOrg(t, "sr-org", owner)
	_, err := AddMember(database.DB, o.ID, plain.Username, RoleMember)
	require.NoError(t, err)

	t.Run("rejections write nothing", func(t *testing.T) {
		tests := []struct {
			name     string
			actor    uint
			username string
			role     string
			want     error
		}{
			{"role admin", owner.ID, target.Username, "admin", ErrInvalidRole},
			{"role empty", owner.ID, target.Username, "", ErrInvalidRole},
			{"role wrong case", owner.ID, target.Username, "Owner", ErrInvalidRole},
			{"unknown user", owner.ID, "sr-nobody", RoleMember, ErrUserNotFound},
			{"actor is member", plain.ID, target.Username, RoleMember, ErrNotOwner},
			{"actor outsider", other.ID, target.Username, RoleMember, ErrNotOwner},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := SetMemberRole(database.DB, o.ID, tt.actor, tt.username, tt.role)
				require.ErrorIs(t, err, tt.want)
				assert.Empty(t, membershipRows(t, o.ID, target.ID))
			})
		}
	})

	t.Run("unknown org", func(t *testing.T) {
		_, err := SetMemberRole(database.DB, 0, owner.ID, target.Username, RoleMember)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("add returns stored username", func(t *testing.T) {
		res, err := SetMemberRole(database.DB, o.ID, owner.ID, "sr-target", RoleMember)
		require.NoError(t, err)
		assert.Equal(t, SetRoleResult{Username: "Sr-Target", Role: RoleMember, Created: true}, res)
	})
	t.Run("same role is a no-op", func(t *testing.T) {
		res, err := SetMemberRole(database.DB, o.ID, owner.ID, target.Username, RoleMember)
		require.NoError(t, err)
		assert.False(t, res.Created)
		assert.Equal(t, RoleMember, res.OldRole)
	})
	t.Run("promote", func(t *testing.T) {
		res, err := SetMemberRole(database.DB, o.ID, owner.ID, target.Username, RoleOwner)
		require.NoError(t, err)
		assert.Equal(t, RoleMember, res.OldRole)
		assert.Equal(t, RoleOwner, membershipRows(t, o.ID, target.ID)[0].Role)
	})
	t.Run("demote with two owners", func(t *testing.T) {
		res, err := SetMemberRole(database.DB, o.ID, owner.ID, target.Username, RoleMember)
		require.NoError(t, err)
		assert.Equal(t, RoleOwner, res.OldRole)
		assert.Equal(t, RoleMember, membershipRows(t, o.ID, target.ID)[0].Role)
	})
	t.Run("demote last owner refused", func(t *testing.T) {
		_, err := SetMemberRole(database.DB, o.ID, owner.ID, owner.Username, RoleMember)
		require.ErrorIs(t, err, ErrLastOwner)
		role, err := MemberRole(database.DB, o.ID, owner.ID)
		require.NoError(t, err)
		assert.Equal(t, RoleOwner, role)
	})
	t.Run("restore soft-deleted keeps one row", func(t *testing.T) {
		require.NoError(t, RemoveMember(database.DB, o.ID, target.ID))
		res, err := SetMemberRole(database.DB, o.ID, owner.ID, target.Username, RoleOwner)
		require.NoError(t, err)
		assert.True(t, res.Created)
		rows := membershipRows(t, o.ID, target.ID)
		require.Len(t, rows, 1)
		assert.False(t, rows[0].DeletedAt.Valid)
		assert.Equal(t, RoleOwner, rows[0].Role)
	})
}

func TestSetMemberRole_ConcurrentDemotionKeepsAnOwner(t *testing.T) {
	a := mkUser(t, "cd-a")
	b := mkUser(t, "cd-b")
	o := mkOrg(t, "cd-org", a)
	_, err := AddMember(database.DB, o.ID, b.Username, RoleOwner)
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, p := range []struct {
		actor  uint
		target string
	}{{a.ID, b.Username}, {b.ID, a.Username}} {
		wg.Go(func() {
			_, errs[i] = SetMemberRole(database.DB, o.ID, p.actor, p.target, RoleMember)
		})
	}
	wg.Wait()

	var owners int64
	require.NoError(t, database.DB.Model(&models.Membership{}).Where("organization_id = ? AND role = ?", o.ID, RoleOwner).Count(&owners).Error)
	assert.EqualValues(t, 1, owners)
	// The loser is refused either as a demoted actor or as the last owner.
	var refused int
	for _, e := range errs {
		if errors.Is(e, ErrNotOwner) || errors.Is(e, ErrLastOwner) {
			refused++
		}
	}
	assert.Equal(t, 1, refused, "%v", errs)
}

func TestListMembers(t *testing.T) {
	owner := mkUser(t, "lm-owner")
	bravo := mkUser(t, "lm-Bravo")
	alpha := mkUser(t, "lm-alpha")
	gone := mkUser(t, "lm-gone")
	deleted := mkUser(t, "lm-deleted")
	o := mkOrg(t, "lm-org", owner)
	for _, u := range []models.User{bravo, alpha, gone, deleted} {
		_, err := AddMember(database.DB, o.ID, u.Username, RoleMember)
		require.NoError(t, err)
	}
	require.NoError(t, RemoveMember(database.DB, o.ID, gone.ID))
	require.NoError(t, database.DB.Delete(&deleted).Error)

	got, err := ListMembers(database.DB, o.ID)
	require.NoError(t, err)
	assert.Equal(t, []Member{
		{"lm-alpha", RoleMember}, {"lm-Bravo", RoleMember}, {"lm-owner", RoleOwner},
	}, got)

	empty, err := ListMembers(database.DB, 0)
	require.NoError(t, err)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}

// An owner whose user account is soft-deleted does not keep the organisation
// owned: the last live owner can neither leave nor be demoted, while removing
// the deleted user's membership is still allowed.
func TestLastOwner_IgnoresDeletedUsers(t *testing.T) {
	owner := mkUser(t, "lo-owner")
	ghost := mkUser(t, "lo-ghost")
	o := mkOrg(t, "lo-org", owner)
	_, err := AddMember(database.DB, o.ID, "lo-ghost", RoleOwner)
	require.NoError(t, err)
	require.NoError(t, database.DB.Delete(&ghost).Error)

	require.ErrorIs(t, RemoveMember(database.DB, o.ID, owner.ID), ErrLastOwner)
	_, err = SetMemberRole(database.DB, o.ID, owner.ID, "lo-owner", RoleMember)
	require.ErrorIs(t, err, ErrLastOwner)
	require.NoError(t, RemoveMember(database.DB, o.ID, ghost.ID))
}
