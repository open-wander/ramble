package org

import (
	"errors"
	"os"
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

func TestCreateOrg_SoftDeletedNameIsNameTaken(t *testing.T) {
	owner := mkUser(t, "sd-owner")
	o := mkOrg(t, "sd-org", owner)
	require.NoError(t, database.DB.Delete(&o).Error)
	_, err := CreateOrg(database.DB, owner.ID, "sd-org", "")
	require.ErrorIs(t, err, ErrNameTaken)
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
	yes := []error{ErrNameRequired, ErrNameReserved, ErrNameTakenByUser, ErrNameTaken, ErrInvalidRole, ErrAlreadyMember}
	no := []error{ErrNotFound, ErrNotOwner, ErrNotMember, ErrUserNotFound, ErrLastOwner, errors.New("x"), nil}
	for _, e := range yes {
		assert.True(t, IsInputError(e), "%v", e)
	}
	for _, e := range no {
		assert.False(t, IsInputError(e), "%v", e)
	}
}
