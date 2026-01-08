package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestUser creates a user for testing and returns it
func createTestUser(t *testing.T, username string) models.User {
	user := models.User{
		Username:      username,
		Email:         username + "@test.com",
		Name:          "Test " + username,
		EmailVerified: true,
	}
	err := database.DB.Create(&user).Error
	require.NoError(t, err)
	return user
}

// createTestPack creates a pack resource for testing
func createTestPack(t *testing.T, userID uint, name string) models.NomadResource {
	resource := models.NomadResource{
		Name:          name,
		Description:   "Test pack: " + name,
		Type:          models.ResourceTypePack,
		UserID:        userID,
		RepositoryURL: "https://github.com/test/" + name,
	}
	err := database.DB.Create(&resource).Error
	require.NoError(t, err)

	// Add a version
	version := models.ResourceVersion{
		ResourceID: resource.ID,
		Version:    "v1.0.0",
		Readme:     "# " + name,
	}
	err = database.DB.Create(&version).Error
	require.NoError(t, err)

	return resource
}

// createTestJob creates a job resource for testing
func createTestJob(t *testing.T, userID uint, name string) models.NomadResource {
	resource := models.NomadResource{
		Name:          name,
		Description:   "Test job: " + name,
		Type:          models.ResourceTypeJob,
		UserID:        userID,
		RepositoryURL: "https://github.com/test/" + name,
	}
	err := database.DB.Create(&resource).Error
	require.NoError(t, err)
	return resource
}

// cleanupTestData removes test data created during tests
func cleanupTestData(t *testing.T) {
	database.DB.Exec("DELETE FROM resource_versions")
	database.DB.Exec("DELETE FROM nomad_resources")
	database.DB.Exec("DELETE FROM users WHERE email LIKE '%@test.com'")
}

func TestContentNegotiation_UserProfile_JSON(t *testing.T) {
	defer cleanupTestData(t)

	// Create test user with packs
	user := createTestUser(t, "apiuser1")
	createTestPack(t, user.ID, "my-pack-1")
	createTestPack(t, user.ID, "my-pack-2")
	createTestJob(t, user.ID, "my-job-1") // Jobs should not appear in pack list

	app := setupTestApp()
	app.Get("/:username", GetUserProfile)

	// Test JSON response
	req := httptest.NewRequest("GET", "/apiuser1", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var result map[string][]PackSummary
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	packs := result["packs"]
	assert.Len(t, packs, 2, "Should return only packs, not jobs")

	// Verify pack names
	names := make(map[string]bool)
	for _, p := range packs {
		names[p.Name] = true
	}
	assert.True(t, names["my-pack-1"])
	assert.True(t, names["my-pack-2"])
}

func TestContentNegotiation_UserProfile_HTML(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "apiuser2")
	createTestPack(t, user.ID, "html-pack")

	app := setupTestApp()
	app.Get("/:username", GetUserProfile)

	// Test HTML response (default)
	req := httptest.NewRequest("GET", "/apiuser2", nil)
	req.Header.Set("Accept", "text/html")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	// HTML response should not be JSON
	assert.NotEqual(t, "application/json", resp.Header.Get("Content-Type"))
}

func TestContentNegotiation_Resource_JSON(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "apiuser3")
	createTestPack(t, user.ID, "detail-pack")

	app := setupTestApp()
	app.Get("/:username/:resourcename", GetResource)

	req := httptest.NewRequest("GET", "/apiuser3/detail-pack", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var result PackDetail
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	assert.Equal(t, "detail-pack", result.Name)
	assert.Contains(t, result.Description, "detail-pack")
	assert.Len(t, result.Versions, 1)
	assert.Equal(t, "v1.0.0", result.Versions[0].Version)
}

func TestContentNegotiation_Resource_NotFound(t *testing.T) {
	defer cleanupTestData(t)

	createTestUser(t, "apiuser4")

	app := setupTestApp()
	app.Get("/:username/:resourcename", GetResource)

	req := httptest.NewRequest("GET", "/apiuser4/nonexistent", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGlobalAPI_ListAllPacks(t *testing.T) {
	defer cleanupTestData(t)

	user1 := createTestUser(t, "globaluser1")
	user2 := createTestUser(t, "globaluser2")
	createTestPack(t, user1.ID, "global-pack-1")
	createTestPack(t, user2.ID, "global-pack-2")
	createTestJob(t, user1.ID, "global-job-1") // Should not appear

	app := setupTestApp()
	app.Get("/v1/packs", ListAllPacksAPI)

	req := httptest.NewRequest("GET", "/v1/packs", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string][]PackSummary
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	packs := result["packs"]
	assert.GreaterOrEqual(t, len(packs), 2, "Should have at least 2 packs")

	// Verify our test packs are included
	names := make(map[string]bool)
	for _, p := range packs {
		names[p.Name] = true
	}
	assert.True(t, names["global-pack-1"])
	assert.True(t, names["global-pack-2"])
}

func TestGlobalAPI_SearchPacks(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "searchuser")
	createTestPack(t, user.ID, "traefik-pack")
	createTestPack(t, user.ID, "redis-pack")

	app := setupTestApp()
	app.Get("/v1/packs/search", SearchPacksAPI)

	// Search for traefik
	req := httptest.NewRequest("GET", "/v1/packs/search?q=traefik", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string][]PackSummary
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	packs := result["packs"]
	assert.Len(t, packs, 1)
	assert.Equal(t, "traefik-pack", packs[0].Name)
}

func TestGlobalAPI_SearchPacks_Empty(t *testing.T) {
	app := setupTestApp()
	app.Get("/v1/packs/search", SearchPacksAPI)

	// Empty search should return empty array
	req := httptest.NewRequest("GET", "/v1/packs/search?q=", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string][]PackSummary
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	assert.Empty(t, result["packs"])
}

func TestGlobalAPI_ListRegistries(t *testing.T) {
	defer cleanupTestData(t)

	user1 := createTestUser(t, "reguser1")
	user2 := createTestUser(t, "reguser2")
	createTestPack(t, user1.ID, "reg-pack-1")
	createTestPack(t, user2.ID, "reg-pack-2")

	app := setupTestApp()
	app.Get("/v1/registries", ListUserRegistriesAPI)

	req := httptest.NewRequest("GET", "/v1/registries", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string][]string
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	registries := result["registries"]
	assert.GreaterOrEqual(t, len(registries), 2)

	// Check that our test users are in the registries list
	regMap := make(map[string]bool)
	for _, r := range registries {
		regMap[r] = true
	}
	assert.True(t, regMap["reguser1"])
	assert.True(t, regMap["reguser2"])
}

func TestListPacksAPI_UserNamespace(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "packlistuser")
	createTestPack(t, user.ID, "user-pack-1")
	createTestPack(t, user.ID, "user-pack-2")
	createTestJob(t, user.ID, "user-job-1") // Should not appear

	app := setupTestApp()
	app.Get("/:username/v1/packs", ListPacksAPI)

	req := httptest.NewRequest("GET", "/packlistuser/v1/packs", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string][]PackSummary
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	packs := result["packs"]
	assert.Len(t, packs, 2)

	names := make(map[string]bool)
	for _, p := range packs {
		names[p.Name] = true
	}
	assert.True(t, names["user-pack-1"])
	assert.True(t, names["user-pack-2"])
}

func TestListPacksAPI_NamespaceNotFound(t *testing.T) {
	app := setupTestApp()
	app.Get("/:username/v1/packs", ListPacksAPI)

	req := httptest.NewRequest("GET", "/nonexistent-user/v1/packs", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestListPacksAPI_OrgNamespace(t *testing.T) {
	defer cleanupOrgTestData(t)

	user := createTestUser(t, "orgpackuser")
	org := models.Organization{Name: "testpackorg", Description: "Test"}
	database.DB.Create(&org)

	// Create pack under org
	resource := models.NomadResource{
		Name:           "org-pack-1",
		Description:    "Org pack",
		Type:           models.ResourceTypePack,
		UserID:         user.ID,
		OrganizationID: &org.ID,
		RepositoryURL:  "https://github.com/test/org-pack",
	}
	database.DB.Create(&resource)

	app := setupTestApp()
	app.Get("/:username/v1/packs", ListPacksAPI)

	req := httptest.NewRequest("GET", "/testpackorg/v1/packs", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string][]PackSummary
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	packs := result["packs"]
	assert.Len(t, packs, 1)
	assert.Equal(t, "org-pack-1", packs[0].Name)
}

func TestGetPackAPI_Success(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "getpackuser")
	createTestPack(t, user.ID, "detail-api-pack")

	app := setupTestApp()
	app.Get("/:username/v1/packs/:packname", GetPackAPI)

	req := httptest.NewRequest("GET", "/getpackuser/v1/packs/detail-api-pack", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result PackDetail
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	assert.Equal(t, "detail-api-pack", result.Name)
	assert.Len(t, result.Versions, 1)
	assert.Equal(t, "v1.0.0", result.Versions[0].Version)
}

func TestGetPackAPI_NotFound(t *testing.T) {
	defer cleanupTestData(t)

	createTestUser(t, "getpackuser2")

	app := setupTestApp()
	app.Get("/:username/v1/packs/:packname", GetPackAPI)

	req := httptest.NewRequest("GET", "/getpackuser2/v1/packs/nonexistent", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetPackAPI_NamespaceNotFound(t *testing.T) {
	app := setupTestApp()
	app.Get("/:username/v1/packs/:packname", GetPackAPI)

	req := httptest.NewRequest("GET", "/nonexistent/v1/packs/somepack", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestWantsJSON(t *testing.T) {
	app := fiber.New()

	tests := []struct {
		name     string
		accept   string
		expected bool
	}{
		{"JSON explicit", "application/json", true},
		{"JSON with charset", "application/json; charset=utf-8", true},
		{"HTML", "text/html", false},
		{"Empty", "", false},
		{"Wildcard", "*/*", false},
		{"HTML and JSON prefers HTML", "text/html, application/json", true}, // contains json
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app.Get("/test", func(c *fiber.Ctx) error {
				result := wantsJSON(c)
				if result {
					return c.SendString("json")
				}
				return c.SendString("html")
			})

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			resp, _ := app.Test(req)

			body := make([]byte, 4)
			resp.Body.Read(body)

			if tt.expected {
				assert.Equal(t, "json", string(body))
			} else {
				assert.Equal(t, "html", string(body))
			}
		})
	}
}

// ListUserRegistriesAPI tests

func TestListUserRegistriesAPI_Empty(t *testing.T) {
	app := fiber.New()

	app.Get("/v1/registries", ListUserRegistriesAPI)

	req := httptest.NewRequest("GET", "/v1/registries", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.NotNil(t, result["registries"])
}

func TestListUserRegistriesAPI_WithData(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "registryuser")

	// Create a pack resource
	resource := models.NomadResource{
		Name:   "testregistry",
		Type:   models.ResourceTypePack,
		UserID: user.ID,
	}
	database.DB.Create(&resource)

	app := fiber.New()

	app.Get("/v1/registries", ListUserRegistriesAPI)

	req := httptest.NewRequest("GET", "/v1/registries", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string][]string
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Contains(t, result["registries"], user.Username)
}

// FetchReadme tests

func TestFetchReadme_NotFound(t *testing.T) {
	app := fiber.New()

	app.Get("/resource/:id/readme", FetchReadme)

	req := httptest.NewRequest("GET", "/resource/99999/readme", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestFetchReadme_Success(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "readmeuser")

	resource := models.NomadResource{
		Name:   "readmeres",
		Type:   models.ResourceTypePack,
		UserID: user.ID,
	}
	database.DB.Create(&resource)

	version := models.ResourceVersion{
		ResourceID: resource.ID,
		Version:    "v1.0.0",
		Readme:     "# Test README",
	}
	database.DB.Create(&version)

	app := fiber.New()

	app.Get("/resource/:id/readme", FetchReadme)

	req := httptest.NewRequest("GET", "/resource/"+toString(resource.ID)+"/readme", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// GetMyRepos tests

func TestGetMyRepos_Unauthenticated(t *testing.T) {
	app := fiber.New()
	InitSession()

	app.Get("/my-repos", GetMyRepos)

	req := httptest.NewRequest("GET", "/my-repos", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestGetMyRepos_NoAccessToken(t *testing.T) {
	defer cleanupTestData(t)

	// User without access token
	user := models.User{
		Username:      "notokenuser",
		Email:         "notoken@test.com",
		Name:          "No Token User",
		EmailVerified: true,
		AccessToken:   "", // No token
	}
	database.DB.Create(&user)

	app := fiber.New()
	InitSession()

	app.Use(func(c *fiber.Ctx) error {
		sess, _ := Store.Get(c)
		sess.Set("user_id", user.ID)
		sess.Save()
		c.Locals("UserID", user.ID)
		c.Locals("User", user)
		return c.Next()
	})

	app.Get("/my-repos", GetMyRepos)

	req := httptest.NewRequest("GET", "/my-repos", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	// Should return error message about missing token
}

// FetchInfo tests

func TestFetchInfo_EmptyURL(t *testing.T) {
	app := fiber.New()
	InitSession()

	app.Get("/fetch-info", FetchInfo)

	req := httptest.NewRequest("GET", "/fetch-info", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	// Empty response for empty URL
}

// GetPopularTags tests

func TestGetPopularTags_Empty(t *testing.T) {
	tags := GetPopularTags()
	// Should return empty slice when no tags exist
	assert.NotNil(t, tags)
}

func TestGetPopularTags_WithData(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "taguser")

	// Create tag
	tag := models.Tag{Name: "popular-tag"}
	database.DB.Create(&tag)

	// Create resource with tag
	resource := models.NomadResource{
		Name:   "taggedres",
		Type:   models.ResourceTypePack,
		UserID: user.ID,
		Tags:   []models.Tag{tag},
	}
	database.DB.Create(&resource)

	tags := GetPopularTags()
	assert.NotNil(t, tags)
	// Should include our tag
	found := false
	for _, t := range tags {
		if t.Name == "popular-tag" {
			found = true
			break
		}
	}
	assert.True(t, found)
}

// ==================== Job API Tests ====================

func TestGlobalAPI_ListAllJobs(t *testing.T) {
	defer cleanupTestData(t)

	user1 := createTestUser(t, "jobuser1")
	user2 := createTestUser(t, "jobuser2")
	createTestJob(t, user1.ID, "global-job-1")
	createTestJob(t, user2.ID, "global-job-2")
	createTestPack(t, user1.ID, "global-pack-1") // Should not appear

	app := setupTestApp()
	app.Get("/v1/jobs", ListAllJobsAPI)

	req := httptest.NewRequest("GET", "/v1/jobs", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string][]JobSummary
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	jobs := result["jobs"]
	assert.GreaterOrEqual(t, len(jobs), 2, "Should have at least 2 jobs")

	// Verify our test jobs are included
	names := make(map[string]bool)
	for _, j := range jobs {
		names[j.Name] = true
	}
	assert.True(t, names["global-job-1"])
	assert.True(t, names["global-job-2"])
}

func TestGlobalAPI_SearchJobs(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "jobsearchuser")

	// Create job with unique name for search
	job := models.NomadResource{
		Name:          "postgres-db-job",
		Description:   "PostgreSQL database job",
		Type:          models.ResourceTypeJob,
		UserID:        user.ID,
		RepositoryURL: "https://github.com/test/postgres",
	}
	database.DB.Create(&job)

	createTestJob(t, user.ID, "redis-cache-job")

	app := setupTestApp()
	app.Get("/v1/jobs/search", SearchJobsAPI)

	// Search for postgres
	req := httptest.NewRequest("GET", "/v1/jobs/search?q=postgres", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string][]JobSummary
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	jobs := result["jobs"]
	assert.Len(t, jobs, 1)
	assert.Equal(t, "postgres-db-job", jobs[0].Name)
}

func TestGlobalAPI_SearchJobs_Empty(t *testing.T) {
	app := setupTestApp()
	app.Get("/v1/jobs/search", SearchJobsAPI)

	// Empty search should return empty array
	req := httptest.NewRequest("GET", "/v1/jobs/search?q=", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string][]JobSummary
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	assert.Empty(t, result["jobs"])
}

func TestGetJobAPI(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "jobdetailuser")

	// Create job with version
	job := models.NomadResource{
		Name:          "detail-job",
		Description:   "A test job for detail testing",
		Type:          models.ResourceTypeJob,
		UserID:        user.ID,
		RepositoryURL: "https://github.com/test/detail-job",
	}
	database.DB.Create(&job)

	version := models.ResourceVersion{
		ResourceID: job.ID,
		Version:    "v2.0.0",
		Readme:     "# Detail Job",
	}
	database.DB.Create(&version)

	app := setupTestApp()
	app.Get("/:username/v1/jobs/:jobname", GetJobAPI)

	req := httptest.NewRequest("GET", "/jobdetailuser/v1/jobs/detail-job", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result JobDetail
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	assert.Equal(t, "detail-job", result.Name)
	assert.Contains(t, result.Description, "test job")
	assert.Len(t, result.Versions, 1)
	assert.Equal(t, "v2.0.0", result.Versions[0].Version)
}

func TestGetJobAPI_NotFound(t *testing.T) {
	defer cleanupTestData(t)

	createTestUser(t, "jobnotfounduser")

	app := setupTestApp()
	app.Get("/:username/v1/jobs/:jobname", GetJobAPI)

	req := httptest.NewRequest("GET", "/jobnotfounduser/v1/jobs/nonexistent", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetJobAPI_NamespaceNotFound(t *testing.T) {
	app := setupTestApp()
	app.Get("/:username/v1/jobs/:jobname", GetJobAPI)

	req := httptest.NewRequest("GET", "/nonexistent/v1/jobs/somejob", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}
