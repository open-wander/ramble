package handlers

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
	"rmbl/internal/services/logger"
)

// tokenOrgOption is one organisation the user may restrict a token to.
type tokenOrgOption struct {
	ID   uint
	Name string
}

// tokenForm echoes submitted form values back after a validation error.
type tokenForm struct {
	Name           string
	Scope          string
	OrganizationID string
	ExpiresInDays  string
}

func defaultTokenForm() tokenForm {
	return tokenForm{Scope: models.ScopeRead, ExpiresInDays: "90"}
}

// tokenOrgOptions lists the organisations the user belongs to. Memberships of
// soft-deleted organisations are skipped.
func tokenOrgOptions(c *fiber.Ctx, uid uint) ([]tokenOrgOption, error) {
	var user models.User
	if err := reqDB(c).Preload("Memberships.Organization").First(&user, uid).Error; err != nil {
		return nil, err
	}
	var opts []tokenOrgOption
	for _, m := range user.Memberships {
		if m.Organization.ID == 0 {
			continue
		}
		opts = append(opts, tokenOrgOption{ID: m.Organization.ID, Name: m.Organization.Name})
	}
	return opts, nil
}

// renderTokenPage renders the settings page with the given status. extra may
// carry NewToken, FormError and Form.
func renderTokenPage(c *fiber.Ctx, status int, extra fiber.Map) error {
	uid := c.Locals("UserID").(uint)
	tokens, err := apitoken.Svc.WithContext(c.UserContext()).List(uid)
	if err != nil {
		logger.FromContext(c.UserContext()).Error("token.list_failed", logger.Err(err))
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}
	orgs, err := tokenOrgOptions(c, uid)
	if err != nil {
		logger.FromContext(c.UserContext()).Error("token.orgs_failed", logger.Err(err))
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}
	ctx := MergeContext(BaseContext(c), fiber.Map{
		"Tokens":     tokens,
		"Orgs":       orgs,
		"Form":       defaultTokenForm(),
		"MaxTokens":  apitoken.MaxActiveTokens,
		"Unverified": !userVerified(c),
		"PageTitle":  "API tokens",
		"NewToken":   "",
		"FormError":  "",
	})
	return c.Status(status).Render("settings_tokens", MergeContext(ctx, extra), "layouts/main")
}

func userVerified(c *fiber.Ctx) bool {
	u, ok := c.Locals("User").(models.User)
	return !ok || u.EmailVerified
}

// GetTokenSettings lists the user's API tokens and the create form.
func GetTokenSettings(c *fiber.Ctx) error {
	return renderTokenPage(c, fiber.StatusOK, nil)
}

// parseTokenOrg returns the requested organisation. ok is false when the
// value is malformed; callers treat that like a non-member organisation.
func parseTokenOrg(raw string) (orgID *uint, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "all" {
		return nil, true
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 {
		return nil, false
	}
	id := uint(n)
	return &id, true
}

// PostCreateToken creates a token and renders the page with the plaintext,
// which is shown exactly once. The plaintext is never stored in the session,
// a flash, a header or the audit log.
func PostCreateToken(c *fiber.Ctx) error {
	uid := c.Locals("UserID").(uint)
	form := tokenForm{
		Name:           c.FormValue("name"),
		Scope:          c.FormValue("scope"),
		OrganizationID: c.FormValue("organization_id"),
		ExpiresInDays:  strings.TrimSpace(c.FormValue("expires_in_days")),
	}
	fail := func(status int, msg string) error {
		return renderTokenPage(c, status, fiber.Map{"FormError": msg, "Form": form})
	}

	orgID, ok := parseTokenOrg(form.OrganizationID)
	if !ok {
		return fail(fiber.StatusForbidden, "You are not a member of that organisation.")
	}

	days := 0
	if form.ExpiresInDays != "" {
		n, err := strconv.Atoi(form.ExpiresInDays)
		if err != nil || n <= 0 {
			return fail(fiber.StatusUnprocessableEntity, apitoken.ErrInvalidExpiry.Error())
		}
		days = n
	}

	plain, tok, err := apitoken.Svc.WithContext(c.UserContext()).Create(uid, form.Name, form.Scope, orgID, days)
	switch {
	case errors.Is(err, apitoken.ErrInvalidName), errors.Is(err, apitoken.ErrInvalidScope), errors.Is(err, apitoken.ErrInvalidExpiry):
		return fail(fiber.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, apitoken.ErrTooManyTokens):
		return fail(fiber.StatusUnprocessableEntity, fmt.Sprintf("You have reached the limit of %d active tokens. Revoke one to create another.", apitoken.MaxActiveTokens))
	case errors.Is(err, apitoken.ErrNotOrgMember):
		return fail(fiber.StatusForbidden, "You are not a member of that organisation.")
	case err != nil:
		logger.FromContext(c.UserContext()).Error("token.create_failed", logger.Err(err))
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	AuditLog(c, "token.create", "token", tok.ID, fmt.Sprintf("%s (%s)", tok.Name, tok.Prefix), map[string]any{
		"scope":           tok.Scope,
		"organization_id": tok.OrganizationID,
		"expires_at":      tok.ExpiresAt,
	})

	c.Set("Cache-Control", "no-store")
	return renderTokenPage(c, fiber.StatusOK, fiber.Map{"NewToken": plain})
}

// DeleteToken revokes one of the user's tokens and returns the updated row.
func DeleteToken(c *fiber.Ctx) error {
	uid := c.Locals("UserID").(uint)
	// Errors must not be swapped into the table row (main.html swaps 4xx/5xx).
	fail := func(status int, msg string) error {
		c.Set("HX-Reswap", "none")
		return c.Status(status).SendString(msg)
	}
	id, err := paramID(c, "id")
	if err != nil {
		return fail(fiber.StatusNotFound, "Token not found")
	}
	svc := apitoken.Svc.WithContext(c.UserContext())
	start := time.Now()
	tok, err := svc.Revoke(uid, id)
	if errors.Is(err, apitoken.ErrNotFound) {
		return fail(fiber.StatusNotFound, "Token not found")
	}
	if err != nil {
		logger.FromContext(c.UserContext()).Error("token.revoke_failed", logger.Err(err))
		return fail(fiber.StatusInternalServerError, "Internal Server Error")
	}

	// Revoke is idempotent; only the request that revoked the token audits it.
	if tok.RevokedAt != nil && !tok.RevokedAt.Before(start) {
		AuditLog(c, "token.revoke", "token", tok.ID, fmt.Sprintf("%s (%s)", tok.Name, tok.Prefix), nil)
	}

	views, err := svc.List(uid)
	if err != nil {
		logger.FromContext(c.UserContext()).Error("token.list_failed", logger.Err(err))
		return fail(fiber.StatusInternalServerError, "Internal Server Error")
	}
	for _, v := range views {
		if v.ID == tok.ID {
			return c.Render("partials/token_row", v)
		}
	}
	return fail(fiber.StatusNotFound, "Token not found")
}
