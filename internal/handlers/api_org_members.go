package handlers

import (
	"errors"

	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	orgsvc "rmbl/internal/services/org"

	"github.com/gofiber/fiber/v2"
)

// OrgMemberResponse describes one member of an organization.
type OrgMemberResponse struct {
	Username string `json:"username" validate:"required" example:"alice"`
	Role     string `json:"role" validate:"required" enums:"owner,member" example:"owner"`
}

// PutOrgMemberRequest is the body of PUT /v1/orgs/{name}/members/{username}.
type PutOrgMemberRequest struct {
	Role string `json:"role" validate:"required" enums:"owner,member" example:"member"`
}

// ListOrgMembersAPI godoc
// @Summary List the members of an organization
// @Description List the live members of an organization as a JSON array of username and role, ordered by username ignoring case. Only a member of the organization may list its members; anyone else gets 403. Needs a read token.
// @Tags orgs
// @Produce json
// @Security bearerAuth
// @Param name path string true "Organization name"
// @Success 200 {array} OrgMemberResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID listOrgMembers
// @Router /v1/orgs/{name}/members [get]
func ListOrgMembersAPI(c *fiber.Ctx) error {
	u, uok := c.Locals("User").(models.User)
	_, tokOK := APITokenFromCtx(c)
	if !uok || !tokOK {
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	org, ok, err := loadOrgForToken(c)
	if !ok {
		return err
	}

	if _, err := orgsvc.MemberRole(reqDB(c), org.ID, u.ID); err != nil {
		if errors.Is(err, orgsvc.ErrNotMember) {
			return APIError(c, fiber.StatusForbidden, codeForbidden, "only members can list members")
		}
		logger.FromContext(c.UserContext()).Error("org.membership_load_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	members, err := orgsvc.ListMembers(reqDB(c), org.ID)
	if err != nil {
		logger.FromContext(c.UserContext()).Error("org.members_list_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	out := make([]OrgMemberResponse, 0, len(members))
	for _, m := range members {
		out = append(out, OrgMemberResponse{Username: m.Username, Role: m.Role})
	}
	return c.JSON(out)
}

// PutOrgMemberAPI godoc
// @Summary Add a member or change a member's role
// @Description Set the role of a user in an organization: the user is added when not a member (a removed membership is restored) and the role is changed when already a member. The role must be owner or member, otherwise 422 invalid_role. Only an owner may call this. Demoting the last owner is answered with 409 last_owner. An unknown username is 404. Setting the role a member already has changes nothing. Audited as org.add_member or org.change_member_role. Needs a write token.
// @Tags orgs
// @Accept json
// @Produce json
// @Security bearerAuth
// @Param name path string true "Organization name"
// @Param username path string true "Username"
// @Param body body PutOrgMemberRequest true "Role"
// @Success 200 {object} OrgMemberResponse
// @Failure 400 {object} APIErrorResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 409 {object} APIErrorResponse "last_owner"
// @Failure 422 {object} APIErrorResponse "invalid_role"
// @Failure 429 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID putOrgMember
// @Router /v1/orgs/{name}/members/{username} [put]
func PutOrgMemberAPI(c *fiber.Ctx) error {
	u, uok := c.Locals("User").(models.User)
	_, tokOK := APITokenFromCtx(c)
	if !uok || !tokOK {
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	var req PutOrgMemberRequest
	if err := decodeStrictJSON(c.Body(), &req); err != nil {
		return APIError(c, fiber.StatusBadRequest, codeBadRequest, "invalid JSON body")
	}

	org, ok, err := loadOrgForToken(c)
	if !ok {
		return err
	}

	res, err := orgsvc.SetMemberRole(reqDB(c), org.ID, u.ID, c.Params("username"), req.Role)
	switch {
	case err == nil:
	case errors.Is(err, orgsvc.ErrNotFound):
		return APIError(c, fiber.StatusNotFound, codeNotFound, "organization not found")
	case errors.Is(err, orgsvc.ErrNotOwner):
		return APIError(c, fiber.StatusForbidden, codeForbidden, "only an owner can change members")
	case errors.Is(err, orgsvc.ErrInvalidRole):
		return APIError(c, fiber.StatusUnprocessableEntity, codeInvalidRole, "role must be owner or member")
	case errors.Is(err, orgsvc.ErrUserNotFound):
		return APIError(c, fiber.StatusNotFound, codeNotFound, "user not found")
	case errors.Is(err, orgsvc.ErrLastOwner):
		return APIError(c, fiber.StatusConflict, codeLastOwner, "cannot demote the last owner")
	default:
		logger.FromContext(c.UserContext()).Error("org.member_set_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	switch {
	case res.Created:
		AuditLog(c, "org.add_member", "organization", org.ID, org.Name, map[string]any{
			"member": res.Username,
			"role":   res.Role,
		})
	case res.OldRole != res.Role:
		AuditLog(c, "org.change_member_role", "organization", org.ID, org.Name, map[string]any{
			"member":   res.Username,
			"old_role": res.OldRole,
			"new_role": res.Role,
		})
	}

	return c.JSON(OrgMemberResponse{Username: res.Username, Role: res.Role})
}

// RemoveOrgMemberAPI godoc
// @Summary Remove a member from an organization
// @Description Remove a member. Only an owner may call this. Removing the last owner is answered with 409 last_owner. An unknown user, or a user who is not a member, is 404 (the web form ignores that case). Audited as org.remove_member. Needs a write token.
// @Tags orgs
// @Produce json
// @Security bearerAuth
// @Param name path string true "Organization name"
// @Param username path string true "Username"
// @Success 204 "Removed"
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 409 {object} APIErrorResponse "last_owner"
// @Failure 429 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID removeOrgMember
// @Router /v1/orgs/{name}/members/{username} [delete]
func RemoveOrgMemberAPI(c *fiber.Ctx) error {
	u, uok := c.Locals("User").(models.User)
	_, tokOK := APITokenFromCtx(c)
	if !uok || !tokOK {
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	org, ok, err := loadOrgForToken(c)
	if !ok {
		return err
	}

	if err := orgsvc.RequireOwner(reqDB(c), org.ID, u.ID); err != nil {
		if errors.Is(err, orgsvc.ErrNotOwner) {
			return APIError(c, fiber.StatusForbidden, codeForbidden, "only an owner can remove members")
		}
		logger.FromContext(c.UserContext()).Error("org.membership_load_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	target, err := orgsvc.FindUser(reqDB(c), c.Params("username"))
	if err != nil {
		if errors.Is(err, orgsvc.ErrUserNotFound) {
			return APIError(c, fiber.StatusNotFound, codeNotFound, "user not found")
		}
		logger.FromContext(c.UserContext()).Error("org.member_remove_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	err = orgsvc.RemoveMember(reqDB(c), org.ID, target.ID)
	switch {
	case err == nil:
	case errors.Is(err, orgsvc.ErrNotMember):
		return APIError(c, fiber.StatusNotFound, codeNotFound, "member not found")
	case errors.Is(err, orgsvc.ErrNotFound):
		return APIError(c, fiber.StatusNotFound, codeNotFound, "organization not found")
	case errors.Is(err, orgsvc.ErrLastOwner):
		return APIError(c, fiber.StatusConflict, codeLastOwner, "cannot remove the last owner")
	default:
		logger.FromContext(c.UserContext()).Error("org.member_remove_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	AuditLog(c, "org.remove_member", "organization", org.ID, org.Name, map[string]any{
		"member_id": target.ID,
		"member":    target.Username,
	})
	return c.SendStatus(fiber.StatusNoContent)
}
