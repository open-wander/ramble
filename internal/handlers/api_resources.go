package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	"rmbl/internal/services/resource"

	"github.com/gofiber/fiber/v2"
)

// CreateResourceRequest is the body of POST /v1/resources.
type CreateResourceRequest struct {
	Name          string   `json:"name" validate:"required" example:"mysql"`
	Type          string   `json:"type" validate:"required" enums:"pack,job" example:"pack"`
	Namespace     string   `json:"namespace" validate:"required" example:"alice"`
	Description   string   `json:"description" example:"MySQL on Nomad"`
	RepositoryURL string   `json:"repository_url" validate:"required" example:"https://github.com/alice/packs"`
	FilePath      string   `json:"file_path" example:"packs/mysql"`
	Version       string   `json:"version" validate:"required" example:"v1.0.0"`
	License       string   `json:"license" example:"MIT"`
	Tags          []string `json:"tags" example:"database,sql"`
}

// UpdateResourceRequest is the body of PATCH /v1/resources/{id}. Every field
// is optional: an omitted or null field is left unchanged, an empty string
// clears description and file_path, an empty license becomes "Unknown" and an
// empty tags list removes all tags. name, type and namespace cannot change;
// they are declared only to answer 422 instead of 400 when a client sends them.
type UpdateResourceRequest struct {
	Description   *string   `json:"description" example:"MySQL on Nomad"`
	RepositoryURL *string   `json:"repository_url" example:"https://github.com/alice/packs"`
	FilePath      *string   `json:"file_path" example:"packs/mysql"`
	License       *string   `json:"license" example:"MIT"`
	Tags          *[]string `json:"tags" example:"database,sql"`

	Name      json.RawMessage `json:"name" swaggerignore:"true"`
	Type      json.RawMessage `json:"type" swaggerignore:"true"`
	Namespace json.RawMessage `json:"namespace" swaggerignore:"true"`
}

// immutableField names the first of name, type and namespace present in the body.
func (r UpdateResourceRequest) immutableField() string {
	switch {
	case len(r.Name) > 0:
		return "name"
	case len(r.Type) > 0:
		return "type"
	case len(r.Namespace) > 0:
		return "namespace"
	}
	return ""
}

// ResourceResponse describes a pack or job. The webhook secret is not part of
// it; only the create response carries the secret.
type ResourceResponse struct {
	ID            uint      `json:"id" validate:"required" example:"42"`
	Name          string    `json:"name" validate:"required" example:"mysql"`
	Type          string    `json:"type" validate:"required" enums:"pack,job" example:"pack"`
	Namespace     string    `json:"namespace" validate:"required" example:"alice"`
	Description   string    `json:"description" validate:"required" example:"MySQL on Nomad"`
	RepositoryURL string    `json:"repository_url" validate:"required" example:"https://github.com/alice/packs"`
	FilePath      string    `json:"file_path" validate:"required" example:"packs/mysql"`
	License       string    `json:"license" validate:"required" example:"MIT"`
	Tags          []string  `json:"tags" validate:"required" example:"database,sql"`
	WebhookURL    string    `json:"webhook_url" validate:"required" example:"https://ramble.openwander.org/resource/42/webhook"`
	CreatedAt     time.Time `json:"created_at" validate:"required" format:"date-time"`
	UpdatedAt     time.Time `json:"updated_at" validate:"required" format:"date-time"`
	LatestVersion string    `json:"latest_version" validate:"required" example:"v1.0.0"`
}

// CreateResourceResponse is the body of the 201 from POST /v1/resources: the
// resource plus its webhook secret, which no other response returns.
type CreateResourceResponse struct {
	ResourceResponse
	WebhookSecret string `json:"webhook_secret" validate:"required"`
}

// newResourceResponse builds the API view of r. r needs User, Organization
// (when OrganizationID is set), Tags and Versions loaded; callers must preload
// Versions ordered by created_at DESC, as LatestVersion is Versions[0].
func newResourceResponse(c *fiber.Ctx, r models.NomadResource) ResourceResponse {
	tags := make([]string, 0, len(r.Tags))
	for _, t := range r.Tags {
		tags = append(tags, t.Name)
	}
	latest := ""
	if len(r.Versions) > 0 {
		latest = r.Versions[0].Version
	}
	return ResourceResponse{
		ID:            r.ID,
		Name:          r.Name,
		Type:          string(r.Type),
		Namespace:     getResourceNamespace(r),
		Description:   r.Description,
		RepositoryURL: r.RepositoryURL,
		FilePath:      r.FilePath,
		License:       r.License,
		Tags:          tags,
		WebhookURL:    GetBaseURL(c) + "/resource/" + strconv.FormatUint(uint64(r.ID), 10) + "/webhook",
		CreatedAt:     r.CreatedAt.UTC(),
		UpdatedAt:     r.UpdatedAt.UTC(),
		LatestVersion: latest,
	}
}

// resolveCreateNamespace maps a namespace to the owning organisation ID (nil
// for the caller's personal namespace). A foreign username and an unknown
// name are indistinguishable: both return ok=false and no user is looked up.
func resolveCreateNamespace(c *fiber.Ctx, u models.User, ns string) (orgID *uint, ok bool, err error) {
	if strings.EqualFold(ns, u.Username) {
		return nil, true, nil
	}
	var org models.Organization
	if err := reqDB(c).Where(iEq("name"), ns).Limit(1).Find(&org).Error; err != nil {
		return nil, false, err
	}
	if org.ID == 0 {
		return nil, false, nil
	}
	return &org.ID, true, nil
}

// decodeStrictJSON decodes body into v, rejecting unknown fields and trailing data.
func decodeStrictJSON(body []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
}

// CreateResourceAPI godoc
// @Summary Create a resource
// @Description Create a pack or job in the caller's personal namespace or in an organization the caller belongs to. Needs a write token. The response is the only one that includes webhook_secret.
// @Tags resources
// @Accept json
// @Produce json
// @Security bearerAuth
// @Param body body CreateResourceRequest true "Resource"
// @Success 201 {object} CreateResourceResponse
// @Failure 400 {object} APIErrorResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 409 {object} APIErrorResponse
// @Failure 422 {object} APIErrorResponse "invalid_name, invalid_type, invalid_repository_url, invalid_version or invalid_tags"
// @Failure 429 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID createResource
// @Router /v1/resources [post]
func CreateResourceAPI(c *fiber.Ctx) error {
	u, uok := c.Locals("User").(models.User)
	_, tokOK := APITokenFromCtx(c)
	if !uok || !tokOK {
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	var req CreateResourceRequest
	if err := decodeStrictJSON(c.Body(), &req); err != nil {
		return APIError(c, fiber.StatusBadRequest, codeBadRequest, "invalid JSON body")
	}

	if err := resource.ValidateCanonicalTags(req.Tags); err != nil {
		return APIError(c, fiber.StatusUnprocessableEntity, codeInvalidTags, err.Error())
	}

	orgID, found, err := resolveCreateNamespace(c, u, req.Namespace)
	if err != nil {
		logger.FromContext(c.UserContext()).Error("resource.create_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	if !found {
		return APIError(c, fiber.StatusNotFound, codeNotFound, "namespace not found")
	}
	if !TokenAllowsOrg(c, orgID) {
		return APIOrgForbidden(c)
	}

	res, err := resourceService(c).CreateResource(resource.CreateInput{
		Name:          req.Name,
		Type:          req.Type,
		Description:   req.Description,
		RepositoryURL: req.RepositoryURL,
		FilePath:      req.FilePath,
		Version:       req.Version,
		License:       req.License,
		Tags:          strings.Join(req.Tags, ","),
	}, u.ID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, resource.ErrInvalidName):
			return APIError(c, fiber.StatusUnprocessableEntity, codeInvalidName, err.Error())
		case errors.Is(err, resource.ErrInvalidType):
			return APIError(c, fiber.StatusUnprocessableEntity, codeInvalidType, err.Error())
		case errors.Is(err, resource.ErrInvalidRepositoryURL):
			return APIError(c, fiber.StatusUnprocessableEntity, codeInvalidRepositoryURL, err.Error())
		case errors.Is(err, resource.ErrInvalidVersion):
			return APIError(c, fiber.StatusUnprocessableEntity, codeInvalidVersion, err.Error())
		case errors.Is(err, resource.ErrDuplicateName):
			return APIError(c, fiber.StatusConflict, codeConflict, err.Error())
		case errors.Is(err, resource.ErrOrgNotFound), errors.Is(err, resource.ErrNotOrgMember):
			return APIError(c, fiber.StatusForbidden, codeForbidden, err.Error())
		}
		logger.FromContext(c.UserContext()).Error("resource.create_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	go fetchVersionContent(detachedLogCtx(c), res.ID, res.Versions[0].Version, res.RepositoryURL, string(res.Type), res.Name, res.FilePath)

	AuditLog(c, "resource.create", "resource", res.ID, res.Name, nil)

	// Render from the same read as GET so both agree on timestamp precision
	// and tag order. The row is committed, so a failed reload is not a failed
	// create: it is logged and the in-memory copy is returned, because this is
	// the only response that carries the webhook secret.
	stored, err := resourceService(c).GetResource(res.ID)
	if err != nil {
		logger.FromContext(c.UserContext()).Error("resource.reload_failed", logger.Err(err))
		stored = res
	}
	return c.Status(fiber.StatusCreated).JSON(CreateResourceResponse{
		ResourceResponse: newResourceResponse(c, *stored),
		WebhookSecret:    res.WebhookSecret,
	})
}

// GetResourceAPI godoc
// @Summary Get a resource by ID
// @Description Fetch a pack or job by its numeric ID. Needs a read token. The response never includes webhook_secret.
// @Tags resources
// @Produce json
// @Security bearerAuth
// @Param id path int true "Resource ID"
// @Success 200 {object} ResourceResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID getResource
// @Router /v1/resources/{id} [get]
func GetResourceAPI(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return APIError(c, fiber.StatusNotFound, codeNotFound, "resource not found")
	}
	return renderResourceAPI(c, func(s *resource.ResourceService) (*models.NomadResource, error) {
		return s.GetResource(id)
	})
}

// GetResourceByNameAPI godoc
// @Summary Get a resource by namespace and name
// @Description Fetch a pack or job by its namespace (a username or an organization name) and name, both case-insensitive. Needs a read token. The namespace/name pair is the import ID of the Terraform provider.
// @Tags resources
// @Produce json
// @Security bearerAuth
// @Param namespace path string true "Username or organization name"
// @Param name path string true "Resource name"
// @Success 200 {object} ResourceResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID getResourceByName
// @Router /v1/namespaces/{namespace}/resources/{name} [get]
func GetResourceByNameAPI(c *fiber.Ctx) error {
	namespace, name := c.Params("namespace"), c.Params("name")
	return renderResourceAPI(c, func(s *resource.ResourceService) (*models.NomadResource, error) {
		return s.GetResourceByNamespace(namespace, name)
	})
}

// renderResourceAPI runs a lookup and answers with the resource. The 404 comes
// before the token's organisation check: the organisation has to be loaded to
// apply the restriction, and resources are public anyway.
func renderResourceAPI(c *fiber.Ctx, load func(*resource.ResourceService) (*models.NomadResource, error)) error {
	r, err := load(resourceService(c))
	if err != nil {
		if errors.Is(err, resource.ErrResourceNotFound) {
			return APIError(c, fiber.StatusNotFound, codeNotFound, "resource not found")
		}
		logger.FromContext(c.UserContext()).Error("resource.get_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	if !TokenAllowsOrg(c, r.OrganizationID) {
		return APIOrgForbidden(c)
	}
	return c.JSON(newResourceResponse(c, *r))
}

// loadResourceForWrite loads the target of a write by the :id parameter and
// applies the token's organisation restriction, answering 404 before 403 as
// the read endpoints do. ok=false means the response has been written.
func loadResourceForWrite(c *fiber.Ctx) (r *models.NomadResource, ok bool, err error) {
	id, perr := paramID(c, "id")
	if perr != nil {
		return nil, false, APIError(c, fiber.StatusNotFound, codeNotFound, "resource not found")
	}
	r, gerr := resourceService(c).GetResource(id)
	if gerr != nil {
		if errors.Is(gerr, resource.ErrResourceNotFound) {
			return nil, false, APIError(c, fiber.StatusNotFound, codeNotFound, "resource not found")
		}
		logger.FromContext(c.UserContext()).Error("resource.get_failed", logger.Err(gerr))
		return nil, false, APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	if !TokenAllowsOrg(c, r.OrganizationID) {
		return nil, false, APIOrgForbidden(c)
	}
	return r, true, nil
}

// UpdateResourceAPI godoc
// @Summary Update a resource
// @Description Change description, repository_url, file_path, license or tags of a pack or job. Omitted and null fields stay unchanged; an empty string clears description and file_path, an empty license becomes "Unknown" and an empty tags list removes all tags. Tags must be lowercase, 2-20 characters of a-z, 0-9 and dash, without duplicates; anything else is answered with 422 invalid_tags. name, type and namespace cannot be changed and are answered with 422 immutable_field. Organization members may edit. Needs a write token.
// @Tags resources
// @Accept json
// @Produce json
// @Security bearerAuth
// @Param id path int true "Resource ID"
// @Param body body UpdateResourceRequest true "Fields to change"
// @Success 200 {object} ResourceResponse
// @Failure 400 {object} APIErrorResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 422 {object} APIErrorResponse "immutable_field, invalid_repository_url or invalid_tags"
// @Failure 429 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID updateResource
// @Router /v1/resources/{id} [patch]
func UpdateResourceAPI(c *fiber.Ctx) error {
	u, uok := c.Locals("User").(models.User)
	_, tokOK := APITokenFromCtx(c)
	if !uok || !tokOK {
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	if _, err := paramID(c, "id"); err != nil {
		return APIError(c, fiber.StatusNotFound, codeNotFound, "resource not found")
	}

	var req UpdateResourceRequest
	if err := decodeStrictJSON(c.Body(), &req); err != nil {
		return APIError(c, fiber.StatusBadRequest, codeBadRequest, "invalid JSON body")
	}
	if f := req.immutableField(); f != "" {
		return APIError(c, fiber.StatusUnprocessableEntity, codeImmutableField, f+" cannot be changed")
	}

	if req.Tags != nil {
		if err := resource.ValidateCanonicalTags(*req.Tags); err != nil {
			return APIError(c, fiber.StatusUnprocessableEntity, codeInvalidTags, err.Error())
		}
	}

	r, ok, err := loadResourceForWrite(c)
	if !ok {
		return err
	}

	svc := resourceService(c)
	if err := svc.PatchResource(r.ID, u.ID, resource.PatchInput{
		Description:   req.Description,
		RepositoryURL: req.RepositoryURL,
		FilePath:      req.FilePath,
		License:       req.License,
		Tags:          req.Tags,
	}); err != nil {
		switch {
		case errors.Is(err, resource.ErrResourceNotFound):
			return APIError(c, fiber.StatusNotFound, codeNotFound, "resource not found")
		case errors.Is(err, resource.ErrUnauthorized):
			return APIError(c, fiber.StatusForbidden, codeForbidden, "you may not edit this resource")
		case errors.Is(err, resource.ErrInvalidRepositoryURL):
			return APIError(c, fiber.StatusUnprocessableEntity, codeInvalidRepositoryURL, err.Error())
		}
		logger.FromContext(c.UserContext()).Error("resource.update_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}

	AuditLog(c, "resource.edit", "resource", r.ID, r.Name, nil)

	stored, err := svc.GetResource(r.ID)
	if err != nil {
		logger.FromContext(c.UserContext()).Error("resource.reload_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	return c.JSON(newResourceResponse(c, *stored))
}

// DeleteResourceAPI godoc
// @Summary Delete a resource
// @Description Delete a pack or job. Only the owner may delete: the owning user of a personal resource or an owner of the organization. The resource is gone afterwards: reads answer 404. Needs a write token.
// @Tags resources
// @Security bearerAuth
// @Param id path int true "Resource ID"
// @Success 204 "No Content"
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 429 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID deleteResource
// @Router /v1/resources/{id} [delete]
func DeleteResourceAPI(c *fiber.Ctx) error {
	u, uok := c.Locals("User").(models.User)
	_, tokOK := APITokenFromCtx(c)
	if !uok || !tokOK {
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	r, ok, err := loadResourceForWrite(c)
	if !ok {
		return err
	}
	if err := resourceService(c).DeleteResource(r.ID, u.ID); err != nil {
		switch {
		case errors.Is(err, resource.ErrResourceNotFound):
			return APIError(c, fiber.StatusNotFound, codeNotFound, "resource not found")
		case errors.Is(err, resource.ErrUnauthorized):
			return APIError(c, fiber.StatusForbidden, codeForbidden, "only the owner can delete this resource")
		}
		logger.FromContext(c.UserContext()).Error("resource.delete_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	AuditLog(c, "resource.delete", "resource", r.ID, r.Name, nil)
	return c.SendStatus(fiber.StatusNoContent)
}
