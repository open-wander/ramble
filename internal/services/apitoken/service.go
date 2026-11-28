// Package apitoken implements personal access tokens: generation, creation
// with limits, authentication by hash, listing, revocation and last-used
// tracking. The plaintext token exists only in the return value of Create.
package apitoken

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"
)

const (
	TokenPrefix       = "rmbl_"
	PrefixLen         = 12
	randomLen         = 40
	MaxActiveTokens   = 25
	DefaultExpiryDays = 90
	MaxExpiryDays     = 365
	MaxNameLen        = 64

	touchInterval = time.Minute
	base62        = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// Sentinel errors. Handlers map them with errors.Is.
var (
	ErrInvalidName   = errors.New("token name must be 1-64 characters")
	ErrInvalidScope  = errors.New("token scope must be read or write")
	ErrInvalidExpiry = errors.New("token expiry must be between 1 and 365 days")
	ErrNotOrgMember  = errors.New("not a member of that organization")
	ErrTooManyTokens = errors.New("too many active tokens")
	ErrInvalidToken  = errors.New("invalid token")
	ErrNotFound      = errors.New("token not found")
)

var tokenRegex = regexp.MustCompile(`^rmbl_[0-9A-Za-z]{40}$`)

// Service holds token business logic.
type Service struct {
	db  *gorm.DB
	now func() time.Time
}

// New returns a Service using the wall clock.
func New(db *gorm.DB) *Service { return newWithClock(db, time.Now) }

func newWithClock(db *gorm.DB, now func() time.Time) *Service {
	return &Service{db: db, now: now}
}

// WithContext returns a service whose queries run on ctx. It keeps the clock.
func (s *Service) WithContext(ctx context.Context) *Service {
	return &Service{db: s.db.WithContext(ctx), now: s.now}
}

// Svc is the package-level service, set by Init.
var Svc *Service

// Init initializes the package-level service.
func Init(db *gorm.DB) { Svc = New(db) }

// Generate returns a new token: "rmbl_" plus 40 base62 characters from
// crypto/rand, using rejection sampling to avoid modulo bias.
func Generate() (string, error) {
	var sb strings.Builder
	sb.WriteString(TokenPrefix)
	buf := make([]byte, 64)
	n := 0
	for n < randomLen {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate token: %w", err)
		}
		for _, b := range buf {
			if b >= 248 { // 248 = 62*4; reject to keep the distribution uniform
				continue
			}
			sb.WriteByte(base62[b%62])
			n++
			if n == randomLen {
				break
			}
		}
	}
	return sb.String(), nil
}

// HashToken returns the hex SHA-256 of a token.
func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// Create makes a token for userID. expiresInDays 0 means the default (90).
// The plaintext is returned once and never stored.
func (s *Service) Create(userID uint, name, scope string, orgID *uint, expiresInDays int) (string, *models.APIToken, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > MaxNameLen {
		return "", nil, ErrInvalidName
	}
	if scope != models.ScopeRead && scope != models.ScopeWrite {
		return "", nil, ErrInvalidScope
	}
	if expiresInDays == 0 {
		expiresInDays = DefaultExpiryDays
	}
	if expiresInDays < 0 || expiresInDays > MaxExpiryDays {
		return "", nil, ErrInvalidExpiry
	}

	plain, err := Generate()
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	tok := &models.APIToken{
		UserID:         userID,
		Name:           name,
		Prefix:         plain[:PrefixLen],
		Hash:           HashToken(plain),
		Scope:          scope,
		OrganizationID: orgID,
		ExpiresAt:      now.AddDate(0, 0, expiresInDays),
		CreatedAt:      now,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Serialize concurrent creates for this user so the cap holds.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").First(&models.User{}, userID).Error; err != nil {
			return fmt.Errorf("lock user: %w", err)
		}
		if orgID != nil {
			var org int64
			if err := tx.Model(&models.Organization{}).Where("id = ?", *orgID).Count(&org).Error; err != nil {
				return fmt.Errorf("check organization: %w", err)
			}
			var mem int64
			if err := tx.Model(&models.Membership{}).
				Where("user_id = ? AND organization_id = ?", userID, *orgID).Count(&mem).Error; err != nil {
				return fmt.Errorf("check membership: %w", err)
			}
			if org == 0 || mem == 0 {
				return ErrNotOrgMember
			}
		}
		var active int64
		if err := tx.Model(&models.APIToken{}).
			Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, now).
			Count(&active).Error; err != nil {
			return fmt.Errorf("count tokens: %w", err)
		}
		if active >= MaxActiveTokens {
			return ErrTooManyTokens
		}
		if err := tx.Create(tok).Error; err != nil {
			return fmt.Errorf("create token: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	logger.FromContext(s.db.Statement.Context).Info("token.created",
		"token_id", tok.ID, "prefix", tok.Prefix)
	return plain, tok, nil
}

// AuthResult is the outcome of a successful Authenticate.
type AuthResult struct {
	Token *models.APIToken
	User  *models.User
}

// Authenticate resolves a plaintext token. Every failure, including database
// errors, returns ErrInvalidToken so callers cannot probe token existence.
// EmailVerified and IsAdmin are left to the caller.
func (s *Service) Authenticate(plain string) (*AuthResult, error) {
	if !tokenRegex.MatchString(plain) {
		return nil, ErrInvalidToken
	}
	hash := HashToken(plain)
	var tok models.APIToken
	if err := s.db.Where("hash = ?", hash).First(&tok).Error; err != nil {
		return nil, ErrInvalidToken
	}
	if subtle.ConstantTimeCompare([]byte(tok.Hash), []byte(hash)) != 1 {
		return nil, ErrInvalidToken
	}
	if tok.RevokedAt != nil || !s.now().Before(tok.ExpiresAt) {
		return nil, ErrInvalidToken
	}
	// Users and organizations are soft deleted, so the FK cascade never fires.
	// Default-scoped lookups exclude deleted rows.
	var user models.User
	if err := s.db.Where("id = ?", tok.UserID).First(&user).Error; err != nil {
		return nil, ErrInvalidToken
	}
	if tok.OrganizationID != nil {
		var org, mem int64
		if err := s.db.Model(&models.Organization{}).Where("id = ?", *tok.OrganizationID).Count(&org).Error; err != nil || org == 0 {
			return nil, ErrInvalidToken
		}
		if err := s.db.Model(&models.Membership{}).
			Where("user_id = ? AND organization_id = ?", tok.UserID, *tok.OrganizationID).Count(&mem).Error; err != nil || mem == 0 {
			return nil, ErrInvalidToken
		}
	}
	return &AuthResult{Token: &tok, User: &user}, nil
}

// TokenView is a token for display. It never carries the hash.
type TokenView struct {
	ID               uint
	Name             string
	Prefix           string
	Scope            string
	OrganizationID   *uint
	OrganizationName string
	ExpiresAt        time.Time
	LastUsedAt       *time.Time
	CreatedAt        time.Time
	RevokedAt        *time.Time
	Expired          bool
	Revoked          bool
}

// List returns all of a user's tokens, newest first, including revoked and
// expired ones with status flags.
func (s *Service) List(userID uint) ([]TokenView, error) {
	var rows []models.APIToken
	if err := s.db.Preload("Organization").Where("user_id = ?", userID).
		Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	now := s.now()
	views := make([]TokenView, 0, len(rows))
	for _, r := range rows {
		v := TokenView{
			ID: r.ID, Name: r.Name, Prefix: r.Prefix, Scope: r.Scope,
			OrganizationID: r.OrganizationID, ExpiresAt: r.ExpiresAt,
			LastUsedAt: r.LastUsedAt, CreatedAt: r.CreatedAt, RevokedAt: r.RevokedAt,
			Revoked: r.RevokedAt != nil,
			Expired: !now.Before(r.ExpiresAt),
		}
		if r.Organization != nil {
			v.OrganizationName = r.Organization.Name
		}
		views = append(views, v)
	}
	return views, nil
}

// CountActive returns the number of unrevoked, unexpired tokens of a user.
func (s *Service) CountActive(userID uint) (int64, error) {
	var n int64
	err := s.db.Model(&models.APIToken{}).
		Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, s.now()).
		Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("count tokens: %w", err)
	}
	return n, nil
}

// Revoke revokes one of userID's tokens. Another user's or an unknown token
// gives ErrNotFound. Revoking twice is a no-op.
func (s *Service) Revoke(userID, tokenID uint) (*models.APIToken, error) {
	var tok models.APIToken
	err := s.db.Where("id = ? AND user_id = ?", tokenID, userID).First(&tok).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find token: %w", err)
	}
	if tok.RevokedAt != nil {
		return &tok, nil
	}
	now := s.now()
	if err := s.db.Model(&models.APIToken{}).
		Where("id = ? AND revoked_at IS NULL", tok.ID).
		Update("revoked_at", now).Error; err != nil {
		return nil, fmt.Errorf("revoke token: %w", err)
	}
	tok.RevokedAt = &now
	return &tok, nil
}

// RevokeAllForUser revokes every token of userID that is not yet revoked, with
// one conditional UPDATE, and returns how many it revoked. Tokens already
// revoked keep their original revoked_at.
func (s *Service) RevokeAllForUser(userID uint) (int64, error) {
	res := s.db.Model(&models.APIToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", s.now())
	if res.Error != nil {
		return 0, fmt.Errorf("revoke tokens: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// TouchLastUsed sets last_used_at, at most once per minute per token, with a
// single conditional UPDATE so it also holds across replicas.
func (s *Service) TouchLastUsed(tokenID uint) error {
	now := s.now()
	err := s.db.Model(&models.APIToken{}).
		Where("id = ? AND (last_used_at IS NULL OR last_used_at <= ?)", tokenID, now.Add(-touchInterval)).
		Update("last_used_at", now).Error
	if err != nil {
		return fmt.Errorf("touch token: %w", err)
	}
	return nil
}
