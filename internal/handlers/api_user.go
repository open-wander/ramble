package handlers

import (
	"time"

	"rmbl/internal/models"

	"github.com/gofiber/fiber/v2"
)

// CurrentUserResponse is the body of GET /v1/user.
type CurrentUserResponse struct {
	Username       string    `json:"username" validate:"required" example:"alice"`
	TokenScope     string    `json:"token_scope" validate:"required" enums:"read,write"`
	TokenOrg       *string   `json:"token_org" validate:"required" extensions:"x-nullable" example:"acme"`
	TokenExpiresAt time.Time `json:"token_expires_at" validate:"required" format:"date-time"`
}

// GetCurrentUserAPI godoc
// @Summary Current user and token
// @Description Identify the user a Bearer token belongs to and describe the token. Use it to check credentials.
// @Tags user
// @Produce json
// @Security bearerAuth
// @Success 200 {object} CurrentUserResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID getCurrentUser
// @Router /v1/user [get]
func GetCurrentUserAPI(c *fiber.Ctx) error {
	tok, ok := APITokenFromCtx(c)
	u, uok := c.Locals("User").(models.User)
	if !ok || !uok {
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	resp := CurrentUserResponse{
		Username:       u.Username,
		TokenScope:     tok.Scope,
		TokenExpiresAt: tok.ExpiresAt.UTC(),
	}
	if tok.OrganizationID != nil {
		found := false
		for _, m := range u.Memberships {
			if m.OrganizationID == *tok.OrganizationID {
				name := m.Organization.Name
				resp.TokenOrg = &name
				found = true
				break
			}
		}
		if !found {
			return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
		}
	}
	return c.JSON(resp)
}
