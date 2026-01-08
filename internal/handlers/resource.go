package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

var tagRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

// generateWebhookSecret generates a cryptographically secure random webhook secret
func generateWebhookSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// This should never happen, but if it does, we panic as webhook secrets are critical
		panic("failed to generate webhook secret: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// Middleware to check if user is authenticated
func RequireAuth(c *fiber.Ctx) error {
	sess, err := Store.Get(c)
	if err != nil || sess.Get("user_id") == nil {
		return c.Redirect("/login")
	}
	return c.Next()
}

// RequireVerifiedEmail ensures the user has verified their email
func RequireVerifiedEmail(c *fiber.Ctx) error {
	userLoc := c.Locals("User")
	if userLoc == nil {
		return c.Redirect("/login")
	}

	user := userLoc.(models.User)

	// OAuth users are always verified
	if user.Provider != "" {
		return c.Next()
	}

	// Check if email is verified
	if !user.EmailVerified {
		SetFlash(c, "error", "Please verify your email address before performing this action. Check your inbox for the verification link.")
		return c.Redirect("/")
	}

	return c.Next()
}

func GetNewResource(c *fiber.Ctx) error {
	isLoggedIn := c.Locals("UserID") != nil
	var user models.User
	if isLoggedIn {
		userID := c.Locals("UserID").(uint)
		database.DB.Preload("Memberships.Organization").First(&user, userID)
	}
	var orgs []models.Organization
	for _, m := range user.Memberships {
		orgs = append(orgs, m.Organization)
	}
	return c.Render("new_resource", MergeContext(BaseContext(c), fiber.Map{
		"Provider":      user.Provider,
		"Organizations": orgs,
	}), "layouts/main")
}

type GitRepoInfo struct {
	Name        string `json:"name"`
	FullURL     string `json:"html_url"`
	Description string `json:"description"`
}

func GetMyRepos(c *fiber.Ctx) error {
	sess, err := Store.Get(c)
	if err != nil || sess.Get("user_id") == nil { return c.Status(401).SendString("Unauthorized") }
	userID := sess.Get("user_id").(uint)
	var user models.User
	database.DB.First(&user, userID)
	if user.AccessToken == "" { return c.SendString("<p class='text-sm text-red-500'>No access token found. Please logout and login again.</p>") }
	var repos []GitRepoInfo
	if user.Provider == "github" {
		agent := fiber.Get("https://api.github.com/user/repos?sort=updated&per_page=50")
		agent.Set("Authorization", "token "+user.AccessToken)
		agent.Set("User-Agent", "RMBL-Registry")
		statusCode, _, errs := agent.Struct(&repos)
		if len(errs) > 0 || statusCode != 200 { return c.SendString("<p class='text-sm text-red-500'>Failed to fetch GitHub repositories.</p>") }
	} else if user.Provider == "gitlab" {
		type GitLabProject struct { Name string `json:"name"`; WebURL string `json:"web_url"`; Description string `json:"description"` }
		var gitlabRepos []GitLabProject
		agent := fiber.Get("https://gitlab.com/api/v4/projects?membership=true&simple=true&per_page=50")
		agent.Set("Authorization", "Bearer "+user.AccessToken)
		statusCode, _, errs := agent.Struct(&gitlabRepos)
		if len(errs) > 0 || statusCode != 200 { return c.SendString("<p class='text-sm text-red-500'>Failed to fetch GitLab projects.</p>") }
		for _, r := range gitlabRepos { repos = append(repos, GitRepoInfo{Name: r.Name, FullURL: r.WebURL, Description: r.Description})
		}
	}
	return c.Render("partials/repo_importer_list", fiber.Map{"Repos": repos})
}

func FetchInfo(c *fiber.Ctx) error {
	repoURL := c.Query("repository_url"); currentType := c.Query("type"); existingTags := c.Query("tags")
	if repoURL == "" { return c.SendString("") }

	// Try to get token from session
	var token string
	sess, err := Store.Get(c)
	if err == nil {
		if uID := sess.Get("user_id"); uID != nil {
			var user models.User
			database.DB.First(&user, uID.(uint))
			if user.AccessToken != "" {
				token = user.AccessToken
			}
		}
	}

	name := ""; description := ""; license := ""; filePath := ""; version := "v1.0.0"; var tags []string
	if strings.Contains(repoURL, "github.com") {
		repo, err := fetchGitHubMetadata(repoURL, token)
		if err == nil {
			name = repo.Name; description = repo.Description; license = repo.License.SpdxID
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
			name = repo.Name; description = repo.Description
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
		if t != "" { tagMap[t] = true }
	}
	if existingTags != "" {
		for _, t := range strings.Split(existingTags, ",") {
			t = strings.ToLower(strings.TrimSpace(t))
			if t != "" { tagMap[t] = true }
		}
	}
	
	var mergedTags []string
	for t := range tagMap {
		mergedTags = append(mergedTags, t)
	}
	tagsString := strings.Join(mergedTags, ", ")

	if name == "" {
		trimmedURL := strings.TrimSuffix(repoURL, "/"); trimmedURL = strings.TrimSuffix(trimmedURL, ".git")
		parts := strings.Split(trimmedURL, "/"); if len(parts) > 0 { name = parts[len(parts)-1] }
	}
	if currentType == "job" && filePath == "" { filePath = name + ".nomad.hcl"
	} else if currentType == "pack" {
		if metaBody, err := downloadFile(repoURL, "metadata.hcl"); err == nil && metaBody != "" {
			if meta, err := parsePackMetadata(metaBody); err == nil {
				name = meta.Pack.Name; description = meta.Pack.Description
				return c.Render("partials/resource_form_fields", fiber.Map{"Name": name, "License": license, "Description": description, "Version": meta.Pack.Version, "Type": currentType, "FilePath": filePath, "Tags": tagsString})
			}
		}
	}
	return c.Render("partials/resource_form_fields", fiber.Map{"Name": name, "License": license, "Description": description, "Version": version, "Type": currentType, "FilePath": filePath, "Tags": tagsString})
}

// PostNewResource godoc
// @Summary Create a new resource
// @Description Register a new Nomad job or pack in the registry.
// @Tags resources
// @Accept x-www-form-urlencoded
// @Param name formData string true "Resource name"
// @Param type formData string true "Resource type (job, pack)"
// @Param owner formData string true "Namespace owner (user or org:ID)"
// @Param repository_url formData string true "Git repository URL"
// @Param version formData string true "Initial version"
// @Success 302 {string} string "Redirect to new resource"
// @Failure 400 {string} string "Bad Request"
// @Router /new [post]
func PostNewResource(c *fiber.Ctx) error {
	sess, err := Store.Get(c); if err != nil { return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized") }
	userID := sess.Get("user_id").(uint)
	type ResourceInput struct {
		Name string `form:"name"`; Type string `form:"type"`; Owner string `form:"owner"`
		Description string `form:"description"`; RepositoryURL string `form:"repository_url"`
		FilePath string `form:"file_path"`; Version string `form:"version"`; License string `form:"license"`; Tags string `form:"tags"`
	}
	var input ResourceInput
	if err := c.BodyParser(&input); err != nil { return c.Status(fiber.StatusBadRequest).SendString("Invalid input") }
	if input.Name == "" || input.Version == "" { return c.Status(fiber.StatusBadRequest).SendString("Name and Version are required") }
	var orgID *uint
	if strings.HasPrefix(input.Owner, "org:") {
		id, _ := strconv.ParseUint(strings.TrimPrefix(input.Owner, "org:"), 10, 32); val := uint(id); orgID = &val
	}
	var existing models.NomadResource
	dbQuery := database.DB.Where("name = ?", input.Name)
	if orgID != nil { dbQuery = dbQuery.Where("organization_id = ?", *orgID) } else { dbQuery = dbQuery.Where("user_id = ? AND organization_id IS NULL", userID) }
	if err := dbQuery.First(&existing).Error; err == nil { return c.Status(fiber.StatusBadRequest).SendString("A resource with this name already exists in this namespace") }
	license := input.License
	if license == "" {
		if licBody, err := downloadFile(input.RepositoryURL, "LICENSE"); err == nil && licBody != "" {
			lines := strings.Split(licBody, "\n"); if len(lines) > 0 { license = strings.TrimPrefix(strings.TrimSpace(lines[0]), "The "); if len(license) > 25 { license = license[:25] + "..." } }
		}
	}
	if license == "" { license = "Unknown" }
	var tags []models.Tag
	if input.Tags != "" {
		for _, tn := range strings.Split(input.Tags, ",") {
			tn = strings.ToLower(strings.TrimSpace(tn)); if len(tn) < 2 || len(tn) > 20 || !tagRegex.MatchString(tn) { continue }
			var tag models.Tag; database.DB.Where(models.Tag{Name: tn}).FirstOrCreate(&tag); tags = append(tags, tag)
		}
	}
	resource := models.NomadResource{
		Name: input.Name, Type: models.ResourceType(input.Type), Description: input.Description, License: license,
		RepositoryURL: input.RepositoryURL, FilePath: input.FilePath, WebhookSecret: generateWebhookSecret(),
		UserID: userID, OrganizationID: orgID, Tags: tags, Versions: []models.ResourceVersion{{Version: input.Version}},
	}
	if err := database.DB.Create(&resource).Error; err != nil { return c.Status(fiber.StatusInternalServerError).SendString("Could not create resource") }
	go func(resID uint, versionStr string, repoURL string, resType string, resName string, filePath string) {
		readme, _ := downloadFile(repoURL, "README.md"); var content string
		var variablesJSON string
		if resType == string(models.ResourceTypeJob) {
			fetchPath := filePath; if fetchPath == "" { fetchPath = resName; if !strings.HasSuffix(fetchPath, ".nomad.hcl") { fetchPath = fetchPath + ".nomad.hcl" } }
			content, _ = downloadFile(repoURL, fetchPath)
		} else if resType == string(models.ResourceTypePack) {
			if varsContent, err := downloadFile(repoURL, "variables.hcl"); err == nil && varsContent != "" {
				if vars, err := parsePackVariables(varsContent); err == nil {
					if b, err := json.Marshal(vars); err == nil {
						variablesJSON = string(b)
					}
				}
			}
			content, _ = downloadFile(repoURL, "metadata.hcl")
		}
		database.DB.Model(&models.ResourceVersion{}).Where("resource_id = ? AND version = ?", resID, versionStr).Updates(map[string]interface{}{"readme": readme, "content": content, "variables": variablesJSON})
	}(resource.ID, input.Version, input.RepositoryURL, string(resource.Type), resource.Name, resource.FilePath)
	redirectPath := "/"; var user models.User; database.DB.First(&user, userID)
	if orgID != nil { var org models.Organization; database.DB.First(&org, *orgID); redirectPath = "/" + org.Name + "/" + resource.Name } else { redirectPath = "/" + user.Username + "/" + resource.Name }
	SetFlash(c, "success", "Resource '"+resource.Name+"' created!"); c.Set("HX-Redirect", redirectPath); return c.SendStatus(fiber.StatusOK)
}

func GetNewVersion(c *fiber.Ctx) error {
	id := c.Params("id"); return c.Render("partials/new_version_modal", fiber.Map{"ResourceID": id, "CSRFToken": c.Locals("CSRFToken")})
}

func GetEditResource(c *fiber.Ctx) error {
	namespace := c.Params("username"); resourcename := c.Params("resourcename")
	sess, _ := Store.Get(c); currentUserID := sess.Get("user_id").(uint)
	var userID uint; var orgID *uint; var user models.User
	if err := database.DB.Where("username ILIKE ?", namespace).First(&user).Error; err == nil { userID = user.ID
	} else {
		var org models.Organization
		if err := database.DB.Where("name ILIKE ?", namespace).First(&org).Error; err == nil { orgID = &org.ID } else { return c.Status(404).SendString("Namespace not found") }
	}
	var isAllowed bool
	if orgID != nil {
		var m models.Membership
		if err := database.DB.Where("user_id = ? AND organization_id = ?", currentUserID, *orgID).First(&m).Error; err == nil { isAllowed = true }
	} else { isAllowed = currentUserID == userID }
	if !isAllowed { return c.Status(403).SendString("You don't have permission to edit this resource") }
	var resource models.NomadResource
	dbQuery := database.DB.Preload("User").Preload("Tags")
	if orgID != nil { dbQuery = dbQuery.Where("organization_id = ? AND name ILIKE ?", *orgID, resourcename)
	} else { dbQuery = dbQuery.Where("user_id = ? AND organization_id IS NULL AND name ILIKE ?", userID, resourcename) }
	if err := dbQuery.First(&resource).Error; err != nil { return c.Status(404).SendString("Resource not found") }
	var currentUser models.User; database.DB.Preload("Memberships.Organization").First(&currentUser, currentUserID)
	var orgs []models.Organization
	for _, m := range currentUser.Memberships { orgs = append(orgs, m.Organization) }
	var tagNames []string
	for _, t := range resource.Tags { tagNames = append(tagNames, t.Name) }
	return c.Render("edit_resource", MergeContext(BaseContext(c), fiber.Map{
		"Resource":      resource,
		"TagsString":    strings.Join(tagNames, ", "),
		"Organizations": orgs,
	}), "layouts/main")
}

// PostEditResource godoc
// @Summary Update resource details
// @Description Update the metadata, repository URL, or tags for an existing resource.
// @Tags resources
// @Accept x-www-form-urlencoded
// @Param id path string true "Resource ID"
// @Param name formData string true "New resource name"
// @Param type formData string true "New resource type"
// @Param description formData string false "New description"
// @Success 200 {string} string "OK"
// @Failure 403 {string} string "Unauthorized"
// @Router /resource/{id}/edit [post]
func PostEditResource(c *fiber.Ctx) error {
	idStr := c.Params("id"); sess, _ := Store.Get(c); currentUserID := sess.Get("user_id").(uint); id, _ := strconv.ParseUint(idStr, 10, 32)
	var resource models.NomadResource
	if err := database.DB.First(&resource, uint(id)).Error; err != nil { return c.Status(404).SendString("Resource not found") }
	var isAllowed bool
	if resource.OrganizationID != nil {
		var m models.Membership
		if err := database.DB.Where("user_id = ? AND organization_id = ?", currentUserID, *resource.OrganizationID).First(&m).Error; err == nil { isAllowed = true }
	} else { isAllowed = currentUserID == resource.UserID }
	if !isAllowed { return c.Status(403).SendString("Unauthorized") }
	type EditInput struct {
		Name string `form:"name"`; Type string `form:"type"`; Owner string `form:"owner"`; Description string `form:"description"`
		RepositoryURL string `form:"repository_url"`; FilePath string `form:"file_path"`; License string `form:"license"`; Tags string `form:"tags"`
	}
	var input EditInput; if err := c.BodyParser(&input); err != nil { return c.Status(400).SendString("Invalid input") }
	var newOrgID *uint
	if strings.HasPrefix(input.Owner, "org:") {
		oid, _ := strconv.ParseUint(strings.TrimPrefix(input.Owner, "org:"), 10, 32); val := uint(oid); newOrgID = &val
	}
	if input.Name != resource.Name || (resource.OrganizationID != newOrgID) {
		var count int64; collideQuery := database.DB.Model(&models.NomadResource{}).Where("name = ?", input.Name)
		if newOrgID != nil { collideQuery = collideQuery.Where("organization_id = ?", *newOrgID) } else { collideQuery = collideQuery.Where("user_id = ? AND organization_id IS NULL", resource.UserID) }
		collideQuery.Count(&count); if count > 0 { return c.Status(400).SendString("A resource with this name already exists in that namespace") }
	}
	resource.Name = input.Name; resource.Type = models.ResourceType(input.Type); resource.OrganizationID = newOrgID
	resource.Description = input.Description; resource.RepositoryURL = input.RepositoryURL; resource.FilePath = input.FilePath; resource.License = input.License
	var tags []models.Tag
	if input.Tags != "" {
		for _, tn := range strings.Split(input.Tags, ",") {
			tn = strings.ToLower(strings.TrimSpace(tn)); if len(tn) < 2 || len(tn) > 20 || !tagRegex.MatchString(tn) { continue }
			var tag models.Tag; database.DB.Where(models.Tag{Name: tn}).FirstOrCreate(&tag); tags = append(tags, tag)
		}
	}
	if err := database.DB.Model(&resource).Association("Tags").Replace(tags); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update tags")
	}
	if err := database.DB.Save(&resource).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save resource")
	}
	SetFlash(c, "success", "Resource updated successfully!")
	newNamespace := ""
	if resource.OrganizationID != nil {
		var o models.Organization; database.DB.First(&o, *resource.OrganizationID); newNamespace = o.Name
	} else {
		var u models.User; database.DB.First(&u, resource.UserID); newNamespace = u.Username
	}
	c.Set("HX-Redirect", "/"+newNamespace+"/"+resource.Name); return c.SendStatus(200)
}

func PostNewVersion(c *fiber.Ctx) error {
	idStr := c.Params("id"); versionStr := c.FormValue("version"); id, _ := strconv.ParseUint(idStr, 10, 32)
	if versionStr == "" { return c.Status(400).SendString("Version is required") }
	var resource models.NomadResource; if err := database.DB.First(&resource, uint(id)).Error; err != nil { return c.Status(404).SendString("Resource not found") }
	version := models.ResourceVersion{ResourceID: uint(id), Version: versionStr}
	if err := database.DB.Create(&version).Error; err != nil { return c.Status(500).SendString("Could not add version") }
	go func(resID uint, versionStr string, repoURL string, resType string, resName string, filePath string) {
		readme, _ := downloadFile(repoURL, "README.md"); var content string
		var variablesJSON string
		if resType == string(models.ResourceTypeJob) {
			fetchPath := filePath; if fetchPath == "" { fetchPath = resName; if !strings.HasSuffix(fetchPath, ".nomad.hcl") { fetchPath = fetchPath + ".nomad.hcl" } }
			content, _ = downloadFile(repoURL, fetchPath)
		} else if resType == string(models.ResourceTypePack) {
			if varsContent, err := downloadFile(repoURL, "variables.hcl"); err == nil && varsContent != "" {
				if vars, err := parsePackVariables(varsContent); err == nil {
					if b, err := json.Marshal(vars); err == nil {
						variablesJSON = string(b)
					}
				}
			}
			content, _ = downloadFile(repoURL, "metadata.hcl")
		}
		database.DB.Model(&models.ResourceVersion{}).Where("resource_id = ? AND version = ?", resID, versionStr).Updates(map[string]interface{}{"readme":  readme, "content": content, "variables": variablesJSON})
	}(resource.ID, versionStr, resource.RepositoryURL, string(resource.Type), resource.Name, resource.FilePath)
	SetFlash(c, "success", "Version "+version.Version+" added!"); c.Set("HX-Refresh", "true"); return c.SendStatus(200)
}

// DeleteResource godoc
// @Summary Delete a resource
// @Description Permantently remove a resource and all its versions from the registry.
// @Tags resources
// @Param id path string true "Resource ID"
// @Success 200 {string} string "OK"
// @Failure 403 {string} string "Unauthorized"
// @Router /resource/{id} [delete]
func DeleteResource(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, _ := Store.Get(c)
	userID := sess.Get("user_id").(uint)

	var resource models.NomadResource
	if err := database.DB.First(&resource, id).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}

	// Check authorization: user owns resource OR user is organization owner
	var isAuthorized bool
	if resource.OrganizationID != nil {
		// Resource belongs to an organization - check if user is owner
		var membership models.Membership
		if err := database.DB.Where("user_id = ? AND organization_id = ? AND role = ?", userID, *resource.OrganizationID, "owner").First(&membership).Error; err == nil {
			isAuthorized = true
		}
	} else {
		// Resource belongs to a user - check if it's the current user
		isAuthorized = resource.UserID == userID
	}

	if !isAuthorized {
		return c.Status(403).SendString("Unauthorized")
	}

	database.DB.Delete(&resource)
	SetFlash(c, "success", "Resource deleted successfully.")
	c.Set("HX-Redirect", "/")
	return c.SendStatus(200)
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
	secret := c.Query("secret")

	var resource models.NomadResource
	if err := database.DB.Preload("Versions", func(db *gorm.DB) *gorm.DB {
		return db.Order("resource_versions.created_at DESC")
	}).First(&resource, id).Error; err != nil {
		return c.SendStatus(404)
	}

	// Use constant-time comparison to prevent timing attacks
	if secret == "" || subtle.ConstantTimeCompare([]byte(resource.WebhookSecret), []byte(secret)) != 1 {
		resource.LastWebhookDelivery = time.Now()
		resource.LastWebhookStatus = "failure"
		resource.LastWebhookError = "Invalid or missing secret"
		database.DB.Save(&resource)
		return c.SendStatus(403)
	}

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
			
			// Background Fetch for the NEW version
			go func(resID uint, vStr string, repoURL string, resType string, resName string, filePath string) {
				readme, _ := downloadFile(repoURL, "README.md")
				var content string
				var variablesJSON string
				if resType == string(models.ResourceTypeJob) {
					fetchPath := filePath
					if fetchPath == "" {
						fetchPath = resName
						if !strings.HasSuffix(fetchPath, ".nomad.hcl") {
							fetchPath = fetchPath + ".nomad.hcl"
						}
					}
					content, _ = downloadFile(repoURL, fetchPath)
				} else if resType == string(models.ResourceTypePack) {
					if varsContent, err := downloadFile(repoURL, "variables.hcl"); err == nil && varsContent != "" {
						if vars, err := parsePackVariables(varsContent); err == nil {
							if b, err := json.Marshal(vars); err == nil {
								variablesJSON = string(b)
							}
						}
					}
					content, _ = downloadFile(repoURL, "metadata.hcl")
				}
				database.DB.Model(&models.ResourceVersion{}).
					Where("resource_id = ? AND version = ?", resID, vStr).
					Updates(map[string]interface{}{"readme": readme, "content": content, "variables": variablesJSON})
			}(resource.ID, newVersion, resource.RepositoryURL, string(resource.Type), resource.Name, resource.FilePath)
			
			return c.SendStatus(200)
		}
	}

	// Default behavior: Refresh latest version
	if len(resource.Versions) > 0 {
		latest := resource.Versions[0]
		go func(resID uint, versionStr string, repoURL string, resType string, resName string, filePath string) {
			readme, _ := downloadFile(repoURL, "README.md")
			var content string
			var variablesJSON string
			if resType == string(models.ResourceTypeJob) {
				fetchPath := filePath; if fetchPath == "" { fetchPath = resName; if !strings.HasSuffix(fetchPath, ".nomad.hcl") { fetchPath = fetchPath + ".nomad.hcl" } }
				content, _ = downloadFile(repoURL, fetchPath)
			} else if resType == string(models.ResourceTypePack) {
				if varsContent, err := downloadFile(repoURL, "variables.hcl"); err == nil && varsContent != "" {
					if vars, err := parsePackVariables(varsContent); err == nil {
						if b, err := json.Marshal(vars); err == nil {
							variablesJSON = string(b)
						}
					}
				}
				content, _ = downloadFile(repoURL, "metadata.hcl")
			}
			database.DB.Model(&models.ResourceVersion{}).Where("resource_id = ? AND version = ?", resID, versionStr).Updates(map[string]interface{}{"readme":  readme, "content": content, "variables": variablesJSON})
		}(resource.ID, latest.Version, resource.RepositoryURL, string(resource.Type), resource.Name, resource.FilePath)
	}

	return c.SendStatus(200)
}

func PostResetWebhookSecret(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, _ := Store.Get(c)
	currentUserID := sess.Get("user_id").(uint)

	var resource models.NomadResource
	if err := database.DB.First(&resource, id).Error; err != nil {
		return c.Status(404).SendString("Resource not found")
	}

	// Verify Permission
	var isAllowed bool
	if resource.OrganizationID != nil {
		var m models.Membership
		if err := database.DB.Where("user_id = ? AND organization_id = ?", currentUserID, *resource.OrganizationID).First(&m).Error; err == nil {
			isAllowed = true
		}
	} else {
		isAllowed = currentUserID == resource.UserID
	}

	if !isAllowed {
		return c.Status(403).SendString("Unauthorized")
	}

	// Generate New Secret
	resource.WebhookSecret = generateWebhookSecret()
	database.DB.Save(&resource)

	SetFlash(c, "success", "Webhook secret has been rotated. Please update your repository settings.")
	c.Set("HX-Refresh", "true")
	return c.SendStatus(200)
}

// ToggleStar godoc
// @Summary Toggle star status
// @Description Star or unstar a resource for the authenticated user. Returns updated star button HTML.
// @Tags resources
// @Produce html
// @Param id path string true "Resource ID"
// @Success 200 {string} string "HTML fragment"
// @Failure 401 {string} string "Unauthorized"
// @Router /resource/{id}/star [post]
func ToggleStar(c *fiber.Ctx) error {
	id := c.Params("id"); sess, _ := Store.Get(c); userID := sess.Get("user_id").(uint)
	var resource models.NomadResource
	if err := database.DB.Preload("StarredBy").First(&resource, id).Error; err != nil { return c.Status(404).SendString("Resource not found") }
	isStarred := false
	for _, u := range resource.StarredBy { if u.ID == userID { isStarred = true; break } }
	if isStarred {
		if err := database.DB.Model(&resource).Association("StarredBy").Delete(&models.User{Model: gorm.Model{ID: userID}}); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to unstar resource")
		}
		database.DB.Model(&resource).Update("star_count", gorm.Expr("star_count - ?", 1))
	} else {
		if err := database.DB.Model(&resource).Association("StarredBy").Append(&models.User{Model: gorm.Model{ID: userID}}); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to star resource")
		}
		database.DB.Model(&resource).Update("star_count", gorm.Expr("star_count + ?", 1))
	}

	// Refetch count
	var count int64
	database.DB.Table("user_stars").Where("nomad_resource_id = ?", resource.ID).Count(&count)

	// Return partial button
	return c.Render("partials/star_button", fiber.Map{
		"Resource":  resource,
		"IsStarred": !isStarred,
		"StarCount": count,
	})
}