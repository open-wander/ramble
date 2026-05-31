package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"rmbl/internal/crypto"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	"rmbl/internal/services/resource"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// validateGitHubSignature validates GitHub webhook signature (X-Hub-Signature-256)
func validateGitHubSignature(payload []byte, secret, signature string) bool {
	if secret == "" || signature == "" {
		return false
	}

	// GitHub signature format: sha256=<hex>
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	expectedSig := strings.TrimPrefix(signature, "sha256=")
	expectedBytes, err := hex.DecodeString(expectedSig)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	actualBytes := mac.Sum(nil)

	return hmac.Equal(expectedBytes, actualBytes)
}

type GitRepoInfo struct {
	Name        string `json:"name"`
	FullURL     string `json:"html_url"`
	Description string `json:"description"`
}

func GetMyRepos(c *fiber.Ctx) error {
	sess, err := Store.Get(c)
	if err != nil || sess.Get("user_id") == nil {
		return c.Status(401).SendString("Unauthorized")
	}
	userID := sess.Get("user_id").(uint)
	var user models.User
	database.DB.First(&user, userID)
	if user.AccessToken == "" {
		return c.SendString("<p class='text-sm text-red-500'>No access token found. Please logout and login again.</p>")
	}

	// Decrypt the access token
	accessToken, err := crypto.DecryptToken(user.AccessToken)
	if err != nil {
		return c.SendString("<p class='text-sm text-red-500'>Failed to decrypt access token.</p>")
	}

	var repos []GitRepoInfo
	if user.Provider == "github" {
		agent := fiber.Get("https://api.github.com/user/repos?sort=updated&per_page=50")
		agent.Set("Authorization", "token "+accessToken)
		agent.Set("User-Agent", "RMBL-Registry")
		statusCode, _, errs := agent.Struct(&repos)
		if len(errs) > 0 || statusCode != 200 {
			return c.SendString("<p class='text-sm text-red-500'>Failed to fetch GitHub repositories.</p>")
		}
	} else if user.Provider == "gitlab" {
		type GitLabProject struct {
			Name        string `json:"name"`
			WebURL      string `json:"web_url"`
			Description string `json:"description"`
		}
		var gitlabRepos []GitLabProject
		agent := fiber.Get("https://gitlab.com/api/v4/projects?membership=true&simple=true&per_page=50")
		agent.Set("Authorization", "Bearer "+accessToken)
		statusCode, _, errs := agent.Struct(&gitlabRepos)
		if len(errs) > 0 || statusCode != 200 {
			return c.SendString("<p class='text-sm text-red-500'>Failed to fetch GitLab projects.</p>")
		}
		for _, r := range gitlabRepos {
			repos = append(repos, GitRepoInfo{Name: r.Name, FullURL: r.WebURL, Description: r.Description})
		}
	}
	return c.Render("partials/repo_importer_list", fiber.Map{"Repos": repos})
}

func FetchInfo(c *fiber.Ctx) error {
	repoURL := c.Query("repository_url")
	currentType := c.Query("type")
	existingTags := c.Query("tags")
	if repoURL == "" {
		return c.SendString("")
	}

	// Try to get token from session
	var token string
	sess, err := Store.Get(c)
	if err == nil {
		if uID := sess.Get("user_id"); uID != nil {
			var user models.User
			database.DB.First(&user, uID.(uint))
			if user.AccessToken != "" {
				decryptedToken, _ := crypto.DecryptToken(user.AccessToken)
				token = decryptedToken
			}
		}
	}

	name := ""
	description := ""
	license := ""
	filePath := ""
	version := "v1.0.0"
	var tags []string
	if strings.Contains(repoURL, "github.com") {
		repo, err := fetchGitHubMetadata(repoURL, token)
		if err == nil {
			name = repo.Name
			description = repo.Description
			license = repo.License.SpdxID
			tags = append(tags, repo.Topics...)
		}

		if currentType == "job" {
			if v, err := fetchGitHubLatestTag(repoURL, token); err == nil {
				version = v
			}
			if f, err := fetchGitHubJobFile(repoURL, token); err == nil {
				filePath = f
			}
		}
	} else if strings.Contains(repoURL, "gitlab.com") {
		repo, err := fetchGitLabMetadata(repoURL, token)
		if err == nil {
			name = repo.Name
			description = repo.Description
			tags = append(tags, repo.TagList...)
		}

		if currentType == "job" {
			if v, err := fetchGitLabLatestTag(repoURL, token); err == nil {
				version = v
			}
			if f, err := fetchGitLabJobFile(repoURL, token); err == nil {
				filePath = f
			}
		}
	}

	// Merge with existing tags from the form
	tagMap := make(map[string]bool)
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			tagMap[t] = true
		}
	}
	if existingTags != "" {
		for _, t := range strings.Split(existingTags, ",") {
			t = strings.ToLower(strings.TrimSpace(t))
			if t != "" {
				tagMap[t] = true
			}
		}
	}

	var mergedTags []string
	for t := range tagMap {
		mergedTags = append(mergedTags, t)
	}
	tagsString := strings.Join(mergedTags, ", ")

	if name == "" {
		trimmedURL := strings.TrimSuffix(repoURL, "/")
		trimmedURL = strings.TrimSuffix(trimmedURL, ".git")
		parts := strings.Split(trimmedURL, "/")
		if len(parts) > 0 {
			name = parts[len(parts)-1]
		}
	}
	if currentType == "job" && filePath == "" {
		filePath = name + ".nomad.hcl"
	} else if currentType == "pack" {
		if metaBody, err := downloadFile(repoURL, "metadata.hcl"); err == nil && metaBody != "" {
			if meta, err := parsePackMetadata(metaBody); err == nil {
				name = meta.Pack.Name
				description = meta.Pack.Description
				return c.Render("partials/resource_form_fields", fiber.Map{"Name": name, "License": license, "Description": description, "Version": meta.Pack.Version, "Type": currentType, "FilePath": filePath, "Tags": tagsString})
			}
		}
	}
	return c.Render("partials/resource_form_fields", fiber.Map{"Name": name, "License": license, "Description": description, "Version": version, "Type": currentType, "FilePath": filePath, "Tags": tagsString})
}

// HandleWebhook godoc
// @Summary Receive Git webhooks
// @Description Endpoint for receiving push and tag events from GitHub or GitLab. Automatically refreshes documentation or creates new versions.
// @Tags webhooks
// @Accept json
// @Param id path string true "Resource ID"
// @Param secret query string true "Webhook secret"
// @Success 200 {string} string "OK"
// @Failure 403 {string} string "Forbidden"
// @Router /resource/{id}/webhook [post]
func HandleWebhook(c *fiber.Ctx) error {
	id := c.Params("id")

	var resource models.NomadResource
	if err := database.DB.Preload("Versions", func(db *gorm.DB) *gorm.DB {
		return db.Order("resource_versions.created_at DESC")
	}).First(&resource, id).Error; err != nil {
		return c.SendStatus(404)
	}

	// Create webhook logger
	webhookLog := logger.Log.With().
		Str("operation", "webhook").
		Str("resource_id", id).
		Logger()

	// Extract authentication headers and query params
	signature := c.Get("X-Hub-Signature-256")
	timestamp := c.Get("X-Webhook-Timestamp")
	querySecret := c.Query("secret")

	// Validate webhook request
	valid, isLegacy, reason := ValidateWebhookRequest(
		c.Body(), signature, timestamp, querySecret, resource.WebhookSecret,
	)

	if !valid {
		// Log validation failure with reason (for debugging)
		webhookLog.Warn().Str("reason", reason).Msg("webhook validation failed")

		// Record failure in database
		resource.LastWebhookDelivery = time.Now()
		resource.LastWebhookStatus = "failure"
		resource.LastWebhookError = "Invalid or missing secret"
		database.DB.Save(&resource)

		// Return generic error to client (401 with no details)
		return c.Status(401).SendString("Invalid webhook signature")
	}

	// Handle legacy auth deprecation warning
	if isLegacy {
		c.Set("X-Deprecation-Warning", "Query parameter authentication is deprecated. Migrate to X-Hub-Signature-256 headers.")
		webhookLog.Warn().Msg("resource using deprecated query parameter webhook authentication")
	}

	webhookLog.Info().Bool("legacy_auth", isLegacy).Msg("webhook validated")

	// Initial success state (might be updated later if fetch fails)
	resource.LastWebhookDelivery = time.Now()
	resource.LastWebhookStatus = "success"
	resource.LastWebhookError = ""
	database.DB.Save(&resource)

	type GitHubPayload struct {
		Ref     string `json:"ref"`
		RefType string `json:"ref_type"` // "tag"
	}
	type GitLabPayload struct {
		ObjectKind string `json:"object_kind"` // "tag_push"
		Ref        string `json:"ref"`         // "refs/tags/v1.0.0"
	}

	var ghPayload GitHubPayload
	var glPayload GitLabPayload

	newVersion := ""
	isTagEvent := false

	// Try parsing payloads
	if c.Get("X-GitHub-Event") == "create" {
		if err := c.BodyParser(&ghPayload); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid GitHub payload")
		}
		if ghPayload.RefType == "tag" {
			newVersion = ghPayload.Ref
			isTagEvent = true
		}
	} else if c.Get("X-Gitlab-Event") == "Tag Push Hook" {
		if err := c.BodyParser(&glPayload); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid GitLab payload")
		}
		newVersion = strings.TrimPrefix(glPayload.Ref, "refs/tags/")
		isTagEvent = true
	}

	if isTagEvent && newVersion != "" {
		// Check if version already exists
		var exists int64
		database.DB.Model(&models.ResourceVersion{}).Where("resource_id = ? AND version = ?", resource.ID, newVersion).Count(&exists)
		if exists == 0 {
			// Create new version
			version := models.ResourceVersion{
				ResourceID: resource.ID,
				Version:    newVersion,
			}
			database.DB.Create(&version)

			webhookLog.Info().Str("version", newVersion).Msg("new version from webhook")

			// Background Fetch for the NEW version with error propagation to webhook status
			go func(resID uint, vStr, repoURL, resType, resName, filePath string) {
				fetchVersionContent(context.Background(), resID, vStr, repoURL, resType, resName, filePath)

				// Check if the fetch failed and update webhook error status
				var version models.ResourceVersion
				if err := database.DB.Where("resource_id = ? AND version = ?", resID, vStr).First(&version).Error; err == nil {
					if version.FetchStatus == models.FetchStatusFailed {
						database.DB.Model(&models.NomadResource{}).
							Where("id = ?", resID).
							Updates(map[string]interface{}{
								"last_webhook_status": "failure",
								"last_webhook_error":  "Fetch failed: " + version.FetchError,
							})
						webhookLog.Warn().
							Str("version", vStr).
							Str("fetch_error", version.FetchError).
							Msg("webhook fetch failed, updated resource error status")
					}
				}
			}(resource.ID, newVersion, resource.RepositoryURL, string(resource.Type), resource.Name, resource.FilePath)

			return c.SendStatus(200)
		}
	}

	// Default behavior: Refresh latest version
	if len(resource.Versions) > 0 {
		latest := resource.Versions[0]

		webhookLog.Info().Str("version", latest.Version).Msg("refreshing latest version from webhook")

		// Background Fetch for refresh with error propagation to webhook status
		go func(resID uint, vStr, repoURL, resType, resName, filePath string) {
			fetchVersionContent(context.Background(), resID, vStr, repoURL, resType, resName, filePath)

			// Check if the fetch failed and update webhook error status
			var version models.ResourceVersion
			if err := database.DB.Where("resource_id = ? AND version = ?", resID, vStr).First(&version).Error; err == nil {
				if version.FetchStatus == models.FetchStatusFailed {
					database.DB.Model(&models.NomadResource{}).
						Where("id = ?", resID).
						Updates(map[string]interface{}{
							"last_webhook_status": "failure",
							"last_webhook_error":  "Fetch failed: " + version.FetchError,
						})
					webhookLog.Warn().
						Str("version", vStr).
						Str("fetch_error", version.FetchError).
						Msg("webhook fetch failed, updated resource error status")
				}
			}
		}(resource.ID, latest.Version, resource.RepositoryURL, string(resource.Type), resource.Name, resource.FilePath)
	}

	return c.SendStatus(200)
}

func PostResetWebhookSecret(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, _ := Store.Get(c)
	currentUserID := sess.Get("user_id").(uint)

	// Parse resource ID
	resourceIDUint64, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return c.Status(404).SendString("Resource not found")
	}
	resourceID := uint(resourceIDUint64)

	// Reset webhook secret via service
	if err := resource.Service.ResetWebhookSecret(resourceID, currentUserID); err != nil {
		if errors.Is(err, resource.ErrUnauthorized) {
			return c.Status(403).SendString("Unauthorized")
		}
		if errors.Is(err, resource.ErrResourceNotFound) {
			return c.Status(404).SendString("Resource not found")
		}
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}

	AuditLog(c, "resource.reset_webhook", "resource", resourceID, "", nil)
	SetFlash(c, "success", "Webhook secret has been rotated. Please update your repository settings.")
	c.Set("HX-Refresh", "true")
	return c.SendStatus(200)
}
