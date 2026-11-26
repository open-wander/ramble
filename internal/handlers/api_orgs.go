package handlers

import (
	"encoding/json"
	"errors"
	"time"

	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	orgsvc "rmbl/internal/services/org"

	"github.com/gofiber/fiber/v2"
)

// CreateOrgRequest is the body of POST /v1/orgs.
type CreateOrgRequest struct {
	Name        string `json:"name" validate:"required" example:"myorg"`
	Description string `json:"description" example:"Our packs and jobs"`
}

// UpdateOrgRequest is the body of PATCH /v1/orgs/{name}. An omitted or null
// description is left unchanged and an empty string clears it. The name cannot
// change; it is declared only to answer 422 instead of 400 when a client sends
// it.
type UpdateOrgRequest struct {
	Description *string `json:"description" example:"Our packs and jobs"`

	Name json.RawMessage `json:"name" swaggerignore:"true"`
}

// OrgResponse describes an organization. member_count counts live memberships.
type OrgResponse struct {
	Name        string    `json:"name" validate:"required" example:"myorg"`
	Description string    `json:"description" validate:"required" example:"Our packs and jobs"`
	CreatedAt   time.Time `json:"created_at" validate:"required" format:"date-time"`
	MemberCount int64     `json:"member_count" validate:"required" example:"3"`
}

func newOrgResponse(org models.Organization, count int64) OrgResponse {
	return OrgResponse{
		Name:        org.Name,
		Description: org.Description,
		CreatedAt:   org.CreatedAt.UTC(),
		MemberCount: count,
	}
}

// loadOrgForToken loads the organization named by the :name parameter and
// applies the token's organization restriction, answering 404 before 403 as
// the other read endpoints do. ok=false means the response has been written.
func loadOrgForToken(c *fiber.Ctx) (org models.Organization, ok bool, err error) {
	org, gerr := orgsvc.GetByName(reqDB(c), c.Params("name"))
	if gerr != nil {
		if errors.Is(gerr, orgsvc.ErrNotFound) {
			return org, false, APIError(c, fiber.StatusNotFound, codeNotFound, "organization not found")
		}
		logger.FromContext(c.UserContext()).Error("org.load_failed", logger.Err(gerr))
		return org, false, APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	if !TokenAllowsOrg(c, &org.ID) {
		return org, false, APIOrgForbidden(c)
	}
	return org, true, nil
}

// respondOrg answers 200 or 201 with the organization and its live member
// count.
func respondOrg(c *fiber.Ctx, status int, org models.Organization) error {
	count, err := orgsvc.MemberCount(reqDB(c), org.ID)
	if err != nil {
		logger.FromContext(c.UserContext()).Error("org.member_count_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	return c.Status(status).JSON(newOrgResponse(org, count))
}

// CreateOrgAPI godoc
// @Summary Create an organization
// @Description Create an organization; the caller becomes its owner. The name is 1-64 characters of letters, digits, dot, dash or underscore, starts with a letter or digit, is unique ignoring case and must not clash with a username or a reserved name. A token restricted to one organization cannot create organizations. Needs a write token.
// @Tags orgs
// @Accept json
// @Produce json
// @Security bearerAuth
// @Param body body CreateOrgRequest true "Organization"
// @Success 201 {object} OrgResponse
// @Failure 400 {object} APIErrorResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 409 {object} APIErrorResponse
// @Failure 422 {object} APIErrorResponse "invalid_name"
// @Failure 429 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID createOrg
// @Router /v1/orgs [post]
func CreateOrgAPI(c *fiber.Ctx) error {
	u, uok := c.Locals("User").(models.User)
	_, tokOK := APITokenFromCtx(c)
	if !uok || !tokOK {
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	// A restricted token never creates an organization, whatever the body.
	if !TokenAllowsOrg(c, nil) {
		return APIOrgForbidden(c)
	}

	var req CreateOrgRequest
	if err := decodeStrictJSON(c.Body(), &req); err != nil {
		return APIError(c, fiber.StatusBadRequest, codeBadRequest, "invalid JSON body")
	}

	org, err := orgsvc.CreateOrg(reqDB(c), u.ID, req.Name, req.Description)
	switch {
	case err == nil:
	case errors.Is(err, orgsvc.ErrNameRequired):
		return APIError(c, fiber.StatusUnprocessableEntity, codeInvalidName, "organization name is required")
	case errors.Is(err, orgsvc.ErrNameInvalid):
		return APIError(c, fiber.StatusUnprocessableEntity, codeInvalidName, "organization name must be 1-64 characters: letters, digits, dot, dash or underscore, starting with a letter or digit")
	case errors.Is(err, orgsvc.ErrNameReserved):
		return APIError(c, fiber.StatusUnprocessableEntity, codeInvalidName, "organization name is reserved")
	case errors.Is(err, orgsvc.ErrNameTakenByUser):
		return APIError(c, fiber.StatusConflict, codeConflict, "name is already taken by a user")
	case errors.Is(err, orgsvc.ErrNameTaken):
		return APIError(c, fiber.StatusConflict, codeConflict, "organization name is already taken")
	default:
		logger.FromContext(c.UserContext()).Error("org.create_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	AuditLog(c, "org.create", "organization", org.ID, org.Name, nil)

	return respondOrg(c, fiber.StatusCreated, org)
}

// GetOrgAPI godoc
// @Summary Get an organization
// @Description Fetch an organization by name, ignoring case. Name, description and member count are public, so any authenticated token may read any organization; a token restricted to one organization may read only that one. Needs a read token.
// @Tags orgs
// @Produce json
// @Security bearerAuth
// @Param name path string true "Organization name"
// @Success 200 {object} OrgResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID getOrg
// @Router /v1/orgs/{name} [get]
func GetOrgAPI(c *fiber.Ctx) error {
	org, ok, err := loadOrgForToken(c)
	if !ok {
		return err
	}
	return respondOrg(c, fiber.StatusOK, org)
}

// UpdateOrgAPI godoc
// @Summary Update an organization
// @Description Change the description of an organization. An omitted or null description stays unchanged and an empty string clears it. The name cannot be changed and is answered with 422 immutable_field. Only an owner may update; a change is audited as org.update, an empty or all-null body writes nothing. Needs a write token.
// @Tags orgs
// @Accept json
// @Produce json
// @Security bearerAuth
// @Param name path string true "Organization name"
// @Param body body UpdateOrgRequest true "Fields to change"
// @Success 200 {object} OrgResponse
// @Failure 400 {object} APIErrorResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 422 {object} APIErrorResponse "immutable_field"
// @Failure 429 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID updateOrg
// @Router /v1/orgs/{name} [patch]
func UpdateOrgAPI(c *fiber.Ctx) error {
	u, uok := c.Locals("User").(models.User)
	_, tokOK := APITokenFromCtx(c)
	if !uok || !tokOK {
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	var req UpdateOrgRequest
	if err := decodeStrictJSON(c.Body(), &req); err != nil {
		return APIError(c, fiber.StatusBadRequest, codeBadRequest, "invalid JSON body")
	}
	if len(req.Name) > 0 {
		return APIError(c, fiber.StatusUnprocessableEntity, codeImmutableField, "name cannot be changed")
	}

	org, ok, err := loadOrgForToken(c)
	if !ok {
		return err
	}

	if err := orgsvc.RequireOwner(reqDB(c), org.ID, u.ID); err != nil {
		if errors.Is(err, orgsvc.ErrNotOwner) {
			return APIError(c, fiber.StatusForbidden, codeForbidden, "only an owner can change this organization")
		}
		logger.FromContext(c.UserContext()).Error("org.membership_load_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	if req.Description != nil {
		if err := orgsvc.UpdateDescription(reqDB(c), &org, *req.Description); err != nil {
			logger.FromContext(c.UserContext()).Error("org.update_failed", logger.Err(err))
			return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
		}
		AuditLog(c, "org.update", "organization", org.ID, org.Name, nil)
	}

	return respondOrg(c, fiber.StatusOK, org)
}
