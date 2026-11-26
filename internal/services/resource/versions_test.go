package resource

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"

	"rmbl/internal/database"
	"rmbl/internal/models"
)

func mkResourceFor(t *testing.T, owner models.User, name string) *models.NomadResource {
	t.Helper()
	res, err := NewResourceService(database.DB).CreateResource(validInput(name), owner.ID, nil)
	require.NoError(t, err)
	cleanupResource(t, res)
	return res
}

func versionRows(t *testing.T, resourceID uint, version string) []models.ResourceVersion {
	t.Helper()
	var rows []models.ResourceVersion
	require.NoError(t, database.DB.Where("resource_id = ? AND version = ?", resourceID, version).Find(&rows).Error)
	return rows
}

func setStatus(t *testing.T, resourceID uint, version string, status models.FetchStatus) {
	t.Helper()
	require.NoError(t, database.DB.Model(&models.ResourceVersion{}).
		Where("resource_id = ? AND version = ?", resourceID, version).
		Updates(map[string]any{"fetch_status": status, "fetch_error": "boom"}).Error)
}

func TestAddVersion(t *testing.T) {
	owner := createTestUser(t, "addver-owner")
	other := createTestUser(t, "addver-other")
	t.Cleanup(func() { database.DB.Delete(&owner); database.DB.Delete(&other) })
	res := mkResourceFor(t, owner, "addver-res")
	svc := NewResourceService(database.DB)

	tests := []struct {
		name    string
		user    uint
		version string
		want    error
		stored  string
	}{
		{"ok trimmed", owner.ID, "  v1.1.0 ", nil, "v1.1.0"},
		{"duplicate", owner.ID, "v1.0.0", ErrDuplicateVersion, ""},
		{"duplicate after trim", owner.ID, " v1.1.0", ErrDuplicateVersion, ""},
		{"empty", owner.ID, "", ErrInvalidVersion, ""},
		{"blank", owner.ID, "   ", ErrInvalidVersion, ""},
		{"too long", owner.ID, strings.Repeat("v", 65), ErrInvalidVersion, ""},
		{"max length", owner.ID, strings.Repeat("v", 64), nil, strings.Repeat("v", 64)},
		{"not a member", other.ID, "v9.9.9", ErrUnauthorized, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := svc.AddVersion(res.ID, tt.user, tt.version)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
				assert.Nil(t, v)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.stored, v.Version)
			assert.Equal(t, models.FetchStatusPending, v.FetchStatus)
			assert.Len(t, versionRows(t, res.ID, tt.stored), 1)
		})
	}
	assert.Empty(t, versionRows(t, res.ID, "v9.9.9"))

	_, err := svc.AddVersion(999999999, owner.ID, "v1")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestAddVersion_ConcurrentDuplicate(t *testing.T) {
	owner := createTestUser(t, "addver-conc")
	t.Cleanup(func() { database.DB.Delete(&owner) })
	res := mkResourceFor(t, owner, "addver-conc-res")
	svc := NewResourceService(database.DB)

	const n = 32
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() { _, errs[i] = svc.AddVersion(res.ID, owner.ID, "v2.0.0") })
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else {
			assert.ErrorIs(t, err, ErrDuplicateVersion)
		}
	}
	assert.Equal(t, 1, ok)
	assert.Len(t, versionRows(t, res.ID, "v2.0.0"), 1)
}

func TestListVersions_NewestFirstWithoutContent(t *testing.T) {
	owner := createTestUser(t, "listver-owner")
	t.Cleanup(func() { database.DB.Delete(&owner) })
	res := mkResourceFor(t, owner, "listver-res")
	base := time.Now().Add(-time.Hour)
	for i, v := range []string{"va", "vb", "vc"} {
		row := models.ResourceVersion{ResourceID: res.ID, Version: v, Content: "BIG", Readme: "BIG", Variables: "BIG",
			CreatedAt: base.Add(time.Duration(i) * time.Minute)}
		require.NoError(t, database.DB.Create(&row).Error)
	}
	// Same timestamp as vc: the higher id comes first.
	tie := models.ResourceVersion{ResourceID: res.ID, Version: "vd",
		CreatedAt: base.Add(2 * time.Minute)}
	require.NoError(t, database.DB.Create(&tie).Error)

	got, err := NewResourceService(database.DB).ListVersions(res.ID)
	require.NoError(t, err)
	var names []string
	for _, v := range got {
		names = append(names, v.Version)
		assert.Empty(t, v.Content)
		assert.Empty(t, v.Readme)
		assert.Empty(t, v.Variables)
	}
	// v1.0.0 was created by CreateResource just now, so it is the newest.
	assert.Equal(t, []string{"v1.0.0", "vd", "vc", "vb", "va"}, names)
}

func TestRetryVersion(t *testing.T) {
	owner := createTestUser(t, "retry-owner")
	other := createTestUser(t, "retry-other")
	t.Cleanup(func() { database.DB.Delete(&owner); database.DB.Delete(&other) })
	res := mkResourceFor(t, owner, "retry-res")
	svc := NewResourceService(database.DB)

	for _, st := range []models.FetchStatus{models.FetchStatusPending, models.FetchStatusFetching, models.FetchStatusCompleted} {
		t.Run("not failed "+string(st), func(t *testing.T) {
			setStatus(t, res.ID, "v1.0.0", st)
			_, err := svc.RetryVersion(res.ID, owner.ID, "v1.0.0")
			require.ErrorIs(t, err, ErrVersionNotFailed)
			assert.Equal(t, st, versionRows(t, res.ID, "v1.0.0")[0].FetchStatus)
		})
	}

	t.Run("unknown version", func(t *testing.T) {
		_, err := svc.RetryVersion(res.ID, owner.ID, "v404")
		assert.ErrorIs(t, err, ErrVersionNotFound)
	})
	t.Run("unauthorized", func(t *testing.T) {
		setStatus(t, res.ID, "v1.0.0", models.FetchStatusFailed)
		_, err := svc.RetryVersion(res.ID, other.ID, "v1.0.0")
		require.ErrorIs(t, err, ErrUnauthorized)
		assert.Equal(t, models.FetchStatusFailed, versionRows(t, res.ID, "v1.0.0")[0].FetchStatus)
	})
	t.Run("failed then second retry sees not failed", func(t *testing.T) {
		setStatus(t, res.ID, "v1.0.0", models.FetchStatusFailed)
		v, err := svc.RetryVersion(res.ID, owner.ID, "v1.0.0")
		require.NoError(t, err)
		assert.Equal(t, "v1.0.0", v.Version)
		row := versionRows(t, res.ID, "v1.0.0")[0]
		assert.Equal(t, models.FetchStatusPending, row.FetchStatus)
		assert.Empty(t, row.FetchError)
		assert.Nil(t, row.FetchCompletedAt)

		_, err = svc.RetryVersion(res.ID, owner.ID, "v1.0.0")
		assert.ErrorIs(t, err, ErrVersionNotFailed)
	})
	t.Run("concurrent retries start one", func(t *testing.T) {
		setStatus(t, res.ID, "v1.0.0", models.FetchStatusFailed)
		const n = 8
		var wg sync.WaitGroup
		errs := make([]error, n)
		for i := range n {
			wg.Go(func() { _, errs[i] = svc.RetryVersion(res.ID, owner.ID, "v1.0.0") })
		}
		wg.Wait()
		ok := 0
		for _, err := range errs {
			if err == nil {
				ok++
			} else {
				assert.ErrorIs(t, err, ErrVersionNotFailed)
			}
		}
		assert.Equal(t, 1, ok)
	})
}

func TestResetWebhookSecret_ChangesOnlySecret(t *testing.T) {
	owner := createTestUser(t, "reset-owner")
	other := createTestUser(t, "reset-other")
	t.Cleanup(func() { database.DB.Delete(&owner); database.DB.Delete(&other) })
	in := validInput("reset-res")
	in.Description = "desc"
	in.Tags = "aa,bb"
	svc := NewResourceService(database.DB)
	res, err := svc.CreateResource(in, owner.ID, nil)
	require.NoError(t, err)
	cleanupResource(t, res)

	load := func() map[string]any {
		var m map[string]any
		require.NoError(t, database.DB.Table("nomad_resources").Where("id = ?", res.ID).Take(&m).Error)
		return m
	}
	before := load()

	_, err = svc.ResetWebhookSecret(res.ID, other.ID)
	require.ErrorIs(t, err, ErrUnauthorized)
	assert.Equal(t, before, load())

	secret, err := svc.ResetWebhookSecret(res.ID, owner.ID)
	require.NoError(t, err)
	require.NotEmpty(t, secret)
	assert.NotEqual(t, res.WebhookSecret, secret)

	after := load()
	assert.Equal(t, secret, after["webhook_secret"])
	for k, v := range before {
		if k == "webhook_secret" || k == "updated_at" {
			continue
		}
		assert.Equal(t, v, after[k], k)
	}

	_, err = svc.ResetWebhookSecret(999999999, owner.ID)
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

// TestAddVersion_WaitsForResourceRowLock proves the duplicate check runs under
// the resource row lock: with the row held (NO KEY UPDATE, which does not block
// the foreign key check of the insert) by another transaction, AddVersion
// does not complete until that transaction ends.
func TestAddVersion_WaitsForResourceRowLock(t *testing.T) {
	owner := createTestUser(t, "addver-lock")
	t.Cleanup(func() { database.DB.Delete(&owner) })
	res := mkResourceFor(t, owner, "addver-lock-res")
	svc := NewResourceService(database.DB)

	holder := database.DB.Begin()
	require.NoError(t, holder.Error)
	require.NoError(t, holder.Clauses(clause.Locking{Strength: "NO KEY UPDATE"}).First(&models.NomadResource{}, res.ID).Error)

	done := make(chan error, 1)
	go func() {
		_, err := svc.AddVersion(res.ID, owner.ID, "v7.0.0")
		done <- err
	}()
	select {
	case err := <-done:
		holder.Rollback()
		t.Fatalf("AddVersion finished while the resource row was locked: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	require.NoError(t, holder.Commit().Error)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("AddVersion did not finish after the lock was released")
	}
	assert.Len(t, versionRows(t, res.ID, "v7.0.0"), 1)
}
