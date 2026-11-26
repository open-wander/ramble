package handlers

import (
	"errors"
	"net/url"
	"time"

	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	"rmbl/internal/services/resource"

	"github.com/gofiber/fiber/v2"
)

// ResourceVersionResponse describes one version of a resource and the state of
// its background content fetch.
type ResourceVersionResponse struct {
	Version     string    `json:"version" validate:"required" example:"v1.1.0"`
	FetchStatus string    `json:"fetch_status" validate:"required" enums:"pending,fetching,completed,failed"`
	FetchError  string    `json:"fetch_error" validate:"required"`
	CreatedAt   time.Time `json:"created_at" validate:"required" format:"date-time"`
}

// CreateVersionRequest is the body of POST /v1/resources/{id}/versions.
type CreateVersionRequest struct {
	Version string `json:"version" validate:"required" example:"v1.1.0"`
}

// WebhookSecretResponse is the body of POST /v1/resources/{id}/webhook-secret.
type WebhookSecretResponse struct {
	WebhookSecret string `json:"webhook_secret" validate:"required"`
}

func newResourceVersionResponse(v models.ResourceVersion) ResourceVersionResponse {
	return ResourceVersionResponse{
		Version:     v.Version,
		FetchStatus: string(v.FetchStatus),
		FetchError:  v.FetchError,
		CreatedAt:   v.CreatedAt.UTC(),
	}
}

// versionWriteContext returns the caller of a version write. ok=false means
// the 500 for a route registered without the token middleware was written.
func versionWriteContext(c *fiber.Ctx) (u models.User, ok bool, err error) {
	u, uok := c.Locals("User").(models.User)
	_, tokOK := APITokenFromCtx(c)
	if !uok || !tokOK {
		return u, false, APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	return u, true, nil
}

// versionWriteError maps the errors the version and secret writes share.
// handled=false means err is not a known rejection: the caller logs it and
// answers 500.
func versionWriteError(c *fiber.Ctx, err error) (handled bool, resp error) {
	switch {
	case errors.Is(err, resource.ErrResourceNotFound):
		return true, APIError(c, fiber.StatusNotFound, codeNotFound, "resource not found")
	case errors.Is(err, resource.ErrUnauthorized):
		return true, APIError(c, fiber.StatusForbidden, codeForbidden, "you may not edit this resource")
	case errors.Is(err, resource.ErrInvalidVersion):
		return true, APIError(c, fiber.StatusUnprocessableEntity, codeInvalidVersion, err.Error())
	case errors.Is(err, resource.ErrDuplicateVersion), errors.Is(err, resource.ErrVersionNotFailed):
		return true, APIError(c, fiber.StatusConflict, codeConflict, err.Error())
	case errors.Is(err, resource.ErrVersionNotFound):
		return true, APIError(c, fiber.StatusNotFound, codeNotFound, "version not found")
	}
	return false, nil
}

// ListResourceVersionsAPI godoc
// @Summary List the versions of a resource
// @Description List the versions of a pack or job, newest first, with the state of each background content fetch. Needs a read token.
// @Tags resources
// @Produce json
// @Security bearerAuth
// @Param id path int true "Resource ID"
// @Success 200 {array} ResourceVersionResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID listResourceVersions
// @Router /v1/resources/{id}/versions [get]
func ListResourceVersionsAPI(c *fiber.Ctx) error {
	r, ok, err := loadResourceForToken(c)
	if !ok {
		return err
	}
	versions, err := resourceService(c).ListVersions(r.ID)
	if err != nil {
		logger.FromContext(c.UserContext()).Error("resource.versions_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	out := make([]ResourceVersionResponse, 0, len(versions))
	for _, v := range versions {
		out = append(out, newResourceVersionResponse(v))
	}
	return c.JSON(out)
}

// CreateResourceVersionAPI godoc
// @Summary Add a version to a resource
// @Description Add a version to a pack or job. The version is created with fetch_status pending and its content is fetched from the repository in the background. Version must be 1-64 characters. Organization members may add versions. Needs a write token.
// @Tags resources
// @Accept json
// @Produce json
// @Security bearerAuth
// @Param id path int true "Resource ID"
// @Param body body CreateVersionRequest true "Version"
// @Success 202 {object} ResourceVersionResponse
// @Failure 400 {object} APIErrorResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 409 {object} APIErrorResponse
// @Failure 422 {object} APIErrorResponse "invalid_version"
// @Failure 429 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID createResourceVersion
// @Router /v1/resources/{id}/versions [post]
func CreateResourceVersionAPI(c *fiber.Ctx) error {
	u, ok, err := versionWriteContext(c)
	if !ok {
		return err
	}
	var req CreateVersionRequest
	if err := decodeStrictJSON(c.Body(), &req); err != nil {
		return APIError(c, fiber.StatusBadRequest, codeBadRequest, "invalid JSON body")
	}
	r, ok, err := loadResourceForToken(c)
	if !ok {
		return err
	}
	v, err := resourceService(c).AddVersion(r.ID, u.ID, req.Version)
	if err != nil {
		if handled, resp := versionWriteError(c, err); handled {
			return resp
		}
		logger.FromContext(c.UserContext()).Error("resource.version_create_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	return acceptVersionFetch(c, r, v, "resource.version_create")
}

// RetryResourceVersionAPI godoc
// @Summary Retry the fetch of a version
// @Description Fetch the content of a failed version again. The version is reset to pending and fetched in the background. A version that is not failed answers 409. Organization members may retry. Needs a write token.
// @Tags resources
// @Produce json
// @Security bearerAuth
// @Param id path int true "Resource ID"
// @Param version path string true "Version, URL-encoded"
// @Success 202 {object} ResourceVersionResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 409 {object} APIErrorResponse
// @Failure 429 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID retryResourceVersion
// @Router /v1/resources/{id}/versions/{version}/retry [post]
func RetryResourceVersionAPI(c *fiber.Ctx) error {
	u, ok, err := versionWriteContext(c)
	if !ok {
		return err
	}
	// Fiber hands out the raw path segment. PathUnescape keeps "+" as "+",
	// which QueryUnescape would turn into a space.
	version, uerr := url.PathUnescape(c.Params("version"))
	if uerr != nil {
		return APIError(c, fiber.StatusNotFound, codeNotFound, "version not found")
	}
	r, ok, err := loadResourceForToken(c)
	if !ok {
		return err
	}
	v, err := resourceService(c).RetryVersion(r.ID, u.ID, version)
	if err != nil {
		if handled, resp := versionWriteError(c, err); handled {
			return resp
		}
		logger.FromContext(c.UserContext()).Error("resource.retry_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	return acceptVersionFetch(c, r, v, "resource.retry_fetch")
}

// acceptVersionFetch starts the background fetch of v, records the audit event
// and answers 202. The body is built from v rather than reloaded, as the fetch
// may already have moved the row to fetching.
func acceptVersionFetch(c *fiber.Ctx, r *models.NomadResource, v *models.ResourceVersion, action string) error {
	resp := newResourceVersionResponse(*v)
	go fetchVersionContent(detachedLogCtx(c), r.ID, v.Version, r.RepositoryURL, string(r.Type), r.Name, r.FilePath)
	AuditLog(c, action, "resource", r.ID, r.Name, map[string]any{"version": v.Version})
	return c.Status(fiber.StatusAccepted).JSON(resp)
}

// RotateWebhookSecretAPI godoc
// @Summary Rotate the webhook secret
// @Description Replace the webhook secret of a pack or job. The new secret is returned in this response only; the old one stops working at once. Organization members may rotate. Needs a write token.
// @Tags resources
// @Produce json
// @Security bearerAuth
// @Param id path int true "Resource ID"
// @Success 200 {object} WebhookSecretResponse
// @Failure 401 {object} APIErrorResponse
// @Failure 403 {object} APIErrorResponse
// @Failure 404 {object} APIErrorResponse
// @Failure 429 {object} APIErrorResponse
// @Failure 500 {object} APIErrorResponse
// @ID rotateWebhookSecret
// @Router /v1/resources/{id}/webhook-secret [post]
func RotateWebhookSecretAPI(c *fiber.Ctx) error {
	u, ok, err := versionWriteContext(c)
	if !ok {
		return err
	}
	r, ok, err := loadResourceForToken(c)
	if !ok {
		return err
	}
	secret, err := resourceService(c).ResetWebhookSecret(r.ID, u.ID)
	if err != nil {
		if handled, resp := versionWriteError(c, err); handled {
			return resp
		}
		logger.FromContext(c.UserContext()).Error("resource.reset_webhook_failed", logger.Err(err))
		return APIError(c, fiber.StatusInternalServerError, codeInternal, "internal error")
	}
	AuditLog(c, "resource.reset_webhook", "resource", r.ID, r.Name, nil)
	return c.JSON(WebhookSecretResponse{WebhookSecret: secret})
}
