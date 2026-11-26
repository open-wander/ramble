package apitoken

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"rmbl/internal/database"
	"rmbl/internal/models"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	var cleanup func()
	testDB, cleanup = database.SetupTestDB()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

var tokenRe = regexp.MustCompile(`^rmbl_[0-9A-Za-z]{40}$`)

func mkUser(t *testing.T, name string) models.User {
	t.Helper()
	u := models.User{Username: name + "-" + suffix(t), Email: name + suffix(t) + "@example.com"}
	require.NoError(t, testDB.Create(&u).Error)
	return u
}

// suffix returns a per-test unique string.
func suffix(t *testing.T) string {
	t.Helper()
	s, err := Generate()
	require.NoError(t, err)
	return strings.ToLower(s[5:15])
}

func mkOrg(t *testing.T, owner uint) models.Organization {
	t.Helper()
	o := models.Organization{Name: "org-" + suffix(t)}
	require.NoError(t, testDB.Create(&o).Error)
	require.NoError(t, testDB.Create(&models.Membership{UserID: owner, OrganizationID: o.ID, Role: "owner"}).Error)
	return o
}

func countTokens(t *testing.T, userID uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testDB.Model(&models.APIToken{}).Where("user_id = ?", userID).Count(&n).Error)
	return n
}

func TestGenerate(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		tok, err := Generate()
		require.NoError(t, err)
		require.Regexp(t, tokenRe, tok)
		require.False(t, seen[tok])
		seen[tok] = true
	}
}

func TestCreate_Validation(t *testing.T) {
	svc := New(testDB)
	u := mkUser(t, "alice")
	long := strings.Repeat("a", 65)
	tests := []struct {
		name    string
		tname   string
		scope   string
		days    int
		wantErr error
	}{
		{"ok", "ci", "read", 30, nil},
		{"ok write", "ci", "write", 1, nil},
		{"name 64 ok", strings.Repeat("a", 64), "read", 30, nil},
		{"empty name", "", "read", 30, ErrInvalidName},
		{"blank name", "   ", "read", 30, ErrInvalidName},
		{"long name", long, "read", 30, ErrInvalidName},
		{"name 64 multibyte ok", strings.Repeat("\u00e9", 64), "read", 30, nil},
		{"name 65 multibyte", strings.Repeat("\u00e9", 65), "read", 30, ErrInvalidName},
		{"bad scope", "ci", "admin", 30, ErrInvalidScope},
		{"empty scope", "ci", "", 30, ErrInvalidScope},
		{"expiry 365 ok", "ci", "read", 365, nil},
		{"expiry 366", "ci", "read", 366, ErrInvalidExpiry},
		{"expiry negative", "ci", "read", -1, ErrInvalidExpiry},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := countTokens(t, u.ID)
			plain, tok, err := svc.Create(u.ID, tt.tname, tt.scope, nil, tt.days)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, plain)
				assert.Equal(t, before, countTokens(t, u.ID))
				return
			}
			require.NoError(t, err)
			assert.Regexp(t, tokenRe, plain)
			assert.Equal(t, plain[:12], tok.Prefix)
			assert.Equal(t, HashToken(plain), tok.Hash)
			assert.Equal(t, before+1, countTokens(t, u.ID))
		})
	}
}

func TestCreate_ExpiryDefault(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := newWithClock(testDB, func() time.Time { return now })
	u := mkUser(t, "alice")
	_, tok, err := svc.Create(u.ID, "ci", "read", nil, 0)
	require.NoError(t, err)
	assert.WithinDuration(t, now.AddDate(0, 0, 90), tok.ExpiresAt, time.Second)
	_, tok, err = svc.Create(u.ID, "ci", "read", nil, 365)
	require.NoError(t, err)
	assert.WithinDuration(t, now.AddDate(0, 0, 365), tok.ExpiresAt, time.Second)
}

func TestCreate_Org(t *testing.T) {
	svc := New(testDB)
	owner := mkUser(t, "alice")
	outsider := mkUser(t, "bob")
	org := mkOrg(t, owner.ID)

	_, tok, err := svc.Create(owner.ID, "ci", "read", &org.ID, 10)
	require.NoError(t, err)
	require.NotNil(t, tok.OrganizationID)

	_, _, err = svc.Create(outsider.ID, "ci", "read", &org.ID, 10)
	require.ErrorIs(t, err, ErrNotOrgMember)
	assert.EqualValues(t, 0, countTokens(t, outsider.ID))

	missing := uint(999999)
	_, _, err = svc.Create(owner.ID, "ci2", "read", &missing, 10)
	require.ErrorIs(t, err, ErrNotOrgMember)

	// Soft-deleted org.
	require.NoError(t, testDB.Delete(&org).Error)
	before := countTokens(t, owner.ID)
	_, _, err = svc.Create(owner.ID, "ci3", "read", &org.ID, 10)
	require.ErrorIs(t, err, ErrNotOrgMember)
	assert.Equal(t, before, countTokens(t, owner.ID))
}

func TestCreate_Cap(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := newWithClock(testDB, func() time.Time { return now })
	u := mkUser(t, "alice")
	var first *models.APIToken
	for i := range MaxActiveTokens {
		_, tok, err := svc.Create(u.ID, "t", "read", nil, 10)
		require.NoError(t, err, "token %d", i)
		if first == nil {
			first = tok
		}
	}
	_, _, err := svc.Create(u.ID, "t", "read", nil, 10)
	require.ErrorIs(t, err, ErrTooManyTokens)
	assert.EqualValues(t, MaxActiveTokens, countTokens(t, u.ID))

	// Another user is unaffected.
	other := mkUser(t, "bob")
	_, _, err = svc.Create(other.ID, "t", "read", nil, 10)
	require.NoError(t, err)

	// Revoking frees a slot.
	_, err = svc.Revoke(u.ID, first.ID)
	require.NoError(t, err)
	_, _, err = svc.Create(u.ID, "t", "read", nil, 10)
	require.NoError(t, err)
	_, _, err = svc.Create(u.ID, "t", "read", nil, 10)
	require.ErrorIs(t, err, ErrTooManyTokens)

	// Expiring exactly one token frees exactly one slot.
	var victim models.APIToken
	require.NoError(t, testDB.Where("user_id = ? AND revoked_at IS NULL", u.ID).First(&victim).Error)
	require.NoError(t, testDB.Model(&victim).Update("expires_at", now.Add(-time.Hour)).Error)
	_, _, err = svc.Create(u.ID, "t", "read", nil, 10)
	require.NoError(t, err)
	_, _, err = svc.Create(u.ID, "t", "read", nil, 10)
	require.ErrorIs(t, err, ErrTooManyTokens)
}

func TestCreate_CapConcurrent(t *testing.T) {
	svc := New(testDB)
	u := mkUser(t, "alice")
	for range MaxActiveTokens - 1 {
		_, _, err := svc.Create(u.ID, "t", "read", nil, 10)
		require.NoError(t, err)
	}
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			_, _, err := svc.Create(u.ID, "t", "read", nil, 10)
			errs <- err
		}()
	}
	ok := 0
	for range 8 {
		if err := <-errs; err == nil {
			ok++
		} else {
			assert.True(t, errors.Is(err, ErrTooManyTokens), "unexpected: %v", err)
		}
	}
	assert.Equal(t, 1, ok)
	assert.EqualValues(t, MaxActiveTokens, countTokens(t, u.ID))
}

func TestAuthenticate_Valid(t *testing.T) {
	svc := New(testDB)
	u := mkUser(t, "alice")
	plain, tok, err := svc.Create(u.ID, "ci", "write", nil, 10)
	require.NoError(t, err)
	res, err := svc.Authenticate(plain)
	require.NoError(t, err)
	assert.Equal(t, tok.ID, res.Token.ID)
	assert.Equal(t, u.ID, res.User.ID)
}

func TestAuthenticate_Invalid(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := now
	svc := newWithClock(testDB, func() time.Time { return clock })

	tests := []struct {
		name  string
		setup func(t *testing.T) string // returns the plaintext to present
	}{
		{"wrong prefix", func(t *testing.T) string {
			u := mkUser(t, "alice")
			p, _, err := svc.Create(u.ID, "t", "read", nil, 10)
			require.NoError(t, err)
			return "rmbx_" + p[5:]
		}},
		{"too short", func(t *testing.T) string {
			u := mkUser(t, "alice")
			p, _, err := svc.Create(u.ID, "t", "read", nil, 10)
			require.NoError(t, err)
			return p[:len(p)-1]
		}},
		{"too long", func(t *testing.T) string {
			u := mkUser(t, "alice")
			p, _, err := svc.Create(u.ID, "t", "read", nil, 10)
			require.NoError(t, err)
			return p + "a"
		}},
		{"bad char", func(t *testing.T) string {
			u := mkUser(t, "alice")
			p, _, err := svc.Create(u.ID, "t", "read", nil, 10)
			require.NoError(t, err)
			return p[:len(p)-1] + "-"
		}},
		{"empty", func(t *testing.T) string { return "" }},
		{"unknown", func(t *testing.T) string {
			p, err := Generate()
			require.NoError(t, err)
			return p
		}},
		{"revoked", func(t *testing.T) string {
			u := mkUser(t, "alice")
			p, tok, err := svc.Create(u.ID, "t", "read", nil, 10)
			require.NoError(t, err)
			_, err = svc.Revoke(u.ID, tok.ID)
			require.NoError(t, err)
			return p
		}},
		{"expired", func(t *testing.T) string {
			u := mkUser(t, "alice")
			p, tok, err := svc.Create(u.ID, "t", "read", nil, 10)
			require.NoError(t, err)
			require.NoError(t, testDB.Model(tok).Update("expires_at", now.Add(-time.Second)).Error)
			return p
		}},
		{"expires exactly now", func(t *testing.T) string {
			u := mkUser(t, "alice")
			p, tok, err := svc.Create(u.ID, "t", "read", nil, 10)
			require.NoError(t, err)
			require.NoError(t, testDB.Model(tok).Update("expires_at", now).Error)
			return p
		}},
		{"user soft-deleted", func(t *testing.T) string {
			u := mkUser(t, "alice")
			p, _, err := svc.Create(u.ID, "t", "read", nil, 10)
			require.NoError(t, err)
			require.NoError(t, testDB.Delete(&u).Error)
			return p
		}},
		{"org soft-deleted", func(t *testing.T) string {
			u := mkUser(t, "alice")
			org := mkOrg(t, u.ID)
			p, _, err := svc.Create(u.ID, "t", "read", &org.ID, 10)
			require.NoError(t, err)
			require.NoError(t, testDB.Delete(&org).Error)
			return p
		}},
		{"membership removed", func(t *testing.T) string {
			u := mkUser(t, "alice")
			org := mkOrg(t, u.ID)
			p, _, err := svc.Create(u.ID, "t", "read", &org.ID, 10)
			require.NoError(t, err)
			require.NoError(t, testDB.Where("user_id = ? AND organization_id = ?", u.ID, org.ID).Delete(&models.Membership{}).Error)
			return p
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.setup(t)
			res, err := svc.Authenticate(p)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidToken))
			assert.Equal(t, ErrInvalidToken, err)
			assert.Nil(t, res)
		})
	}

	// Control: org-restricted token with live membership authenticates.
	u := mkUser(t, "alice")
	org := mkOrg(t, u.ID)
	p, _, err := svc.Create(u.ID, "t", "read", &org.ID, 10)
	require.NoError(t, err)
	_, err = svc.Authenticate(p)
	require.NoError(t, err)

	// Injected clock survives WithContext: advance past expiry.
	svc2 := svc.WithContext(context.Background())
	_, err = svc2.Authenticate(p)
	require.NoError(t, err)
	clock = now.AddDate(0, 0, 11)
	_, err = svc2.Authenticate(p)
	assert.Equal(t, ErrInvalidToken, err)
}

func TestList(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := newWithClock(testDB, func() time.Time { return now })
	u := mkUser(t, "alice")
	other := mkUser(t, "bob")
	org := mkOrg(t, u.ID)
	_, a, err := svc.Create(u.ID, "a", "read", &org.ID, 10)
	require.NoError(t, err)
	_, b, err := svc.Create(u.ID, "b", "write", nil, 10)
	require.NoError(t, err)
	_, c, err := svc.Create(u.ID, "c", "read", nil, 10)
	require.NoError(t, err)
	_, _, err = svc.Create(other.ID, "x", "read", nil, 10)
	require.NoError(t, err)
	_, err = svc.Revoke(u.ID, b.ID)
	require.NoError(t, err)
	require.NoError(t, testDB.Model(c).Update("expires_at", now.Add(-time.Hour)).Error)

	views, err := svc.List(u.ID)
	require.NoError(t, err)
	require.Len(t, views, 3)
	byID := map[uint]TokenView{}
	for _, v := range views {
		byID[v.ID] = v
	}
	assert.Equal(t, org.Name, byID[a.ID].OrganizationName)
	assert.False(t, byID[a.ID].Revoked || byID[a.ID].Expired)
	assert.True(t, byID[b.ID].Revoked)
	assert.True(t, byID[c.ID].Expired)
	assert.True(t, views[0].CreatedAt.After(views[2].CreatedAt) || views[0].CreatedAt.Equal(views[2].CreatedAt))
	assert.Equal(t, c.ID, views[0].ID)
}

func TestRevoke(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := now
	svc := newWithClock(testDB, func() time.Time { return clock })
	u := mkUser(t, "alice")
	other := mkUser(t, "bob")
	plain, tok, err := svc.Create(u.ID, "t", "read", nil, 10)
	require.NoError(t, err)

	// Other user: not found, token stays active.
	got, err := svc.Revoke(other.ID, tok.ID)
	require.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, got)
	var row models.APIToken
	require.NoError(t, testDB.First(&row, tok.ID).Error)
	assert.Nil(t, row.RevokedAt)
	_, err = svc.Authenticate(plain)
	require.NoError(t, err)

	// Unknown id.
	_, err = svc.Revoke(u.ID, 999999)
	require.ErrorIs(t, err, ErrNotFound)

	// Owner revokes.
	got, err = svc.Revoke(u.ID, tok.ID)
	require.NoError(t, err)
	assert.Equal(t, tok.Prefix, got.Prefix)
	require.NoError(t, testDB.First(&row, tok.ID).Error)
	require.NotNil(t, row.RevokedAt)
	first := row.RevokedAt.Truncate(time.Microsecond)

	// Idempotent, RevokedAt unchanged.
	clock = now.Add(time.Hour)
	_, err = svc.Revoke(u.ID, tok.ID)
	require.NoError(t, err)
	require.NoError(t, testDB.First(&row, tok.ID).Error)
	assert.True(t, first.Equal(row.RevokedAt.Truncate(time.Microsecond)))
}

func TestTouchLastUsed(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := t0
	svc := newWithClock(testDB, func() time.Time { return clock })
	u := mkUser(t, "alice")
	_, tok, err := svc.Create(u.ID, "t", "read", nil, 10)
	require.NoError(t, err)

	last := func() time.Time {
		var row models.APIToken
		require.NoError(t, testDB.First(&row, tok.ID).Error)
		require.NotNil(t, row.LastUsedAt)
		return row.LastUsedAt.UTC().Truncate(time.Microsecond)
	}

	require.NoError(t, svc.TouchLastUsed(tok.ID))
	assert.True(t, last().Equal(t0))

	clock = t0.Add(30 * time.Second)
	require.NoError(t, svc.TouchLastUsed(tok.ID))
	assert.True(t, last().Equal(t0), "within a minute must not write")

	clock = t0.Add(61 * time.Second)
	require.NoError(t, svc.TouchLastUsed(tok.ID))
	assert.True(t, last().Equal(clock))
}

// TestGenerate_Uniform fails if the base62 mapping becomes biased, for
// example by narrowing the rejection of bytes >= 248 (62*4). Pearson's
// chi-square over 800,000 characters, 61 degrees of freedom: a uniform
// source stays far below 140 (p < 1e-7); the modulo-bias mutation scores
// in the thousands.
func TestGenerate_Uniform(t *testing.T) {
	counts := make([]int, len(base62))
	total := 0
	for range 20000 {
		tok, err := Generate()
		require.NoError(t, err)
		for i := len(TokenPrefix); i < len(tok); i++ {
			idx := strings.IndexByte(base62, tok[i])
			require.GreaterOrEqual(t, idx, 0)
			counts[idx]++
			total++
		}
	}
	expected := float64(total) / float64(len(base62))
	chi := 0.0
	for _, c := range counts {
		d := float64(c) - expected
		chi += d * d / expected
	}
	assert.Less(t, chi, 140.0, "chi-square %.1f over %d chars", chi, total)
}
