package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
	"rmbl/internal/services/logger"
	"rmbl/internal/services/resource"
)

// resourceAPIApp mounts the resource routes the way registerRoutes does, minus
// the limiter. Logs go to a goroutine-safe buffer because the background
// version fetch logs after the response.
func resourceAPIApp(t *testing.T) (*fiber.App, *lockedBuffer) {
	t.Helper()
	apitoken.Init(database.DB)
	buf := &lockedBuffer{}
	app := fiber.New(fiber.Config{ErrorHandler: APIErrorHandler})
	app.Use(requestid.New())
	app.Use(logger.RequestLogger(logger.NewJSON(buf)))
	app.Post("/v1/resources", RequireAPIToken(models.ScopeWrite), CreateResourceAPI)
	app.Get("/v1/resources/:id<int>", RequireAPIToken(models.ScopeRead), GetResourceAPI)
	app.Patch("/v1/resources/:id<int>", RequireAPIToken(models.ScopeWrite), UpdateResourceAPI)
	app.Delete("/v1/resources/:id<int>", RequireAPIToken(models.ScopeWrite), DeleteResourceAPI)
	app.Get("/v1/namespaces/:namespace/resources/:name", RequireAPIToken(models.ScopeRead), GetResourceByNameAPI)
	app.Get("/v1/resources/:id<int>/versions", RequireAPIToken(models.ScopeRead), ListResourceVersionsAPI)
	app.Post("/v1/resources/:id<int>/versions", RequireAPIToken(models.ScopeWrite), CreateResourceVersionAPI)
	app.Post("/v1/resources/:id<int>/versions/:version/retry", RequireAPIToken(models.ScopeWrite), RetryResourceVersionAPI)
	app.Post("/v1/resources/:id<int>/webhook-secret", RequireAPIToken(models.ScopeWrite), RotateWebhookSecretAPI)
	return app, buf
}

func postResource(t *testing.T, app *fiber.App, token, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/resources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// validBody is a create body that the service accepts; override fields per case.
func validBody(t *testing.T, ns, name string, override map[string]any) string {
	t.Helper()
	m := map[string]any{
		"name": name, "type": "pack", "namespace": ns, "description": "d",
		"repository_url": "https://github.com/x/y", "file_path": "packs/mysql",
		"version": "v1.0.0", "license": "MIT", "tags": []string{"db"},
	}
	for k, v := range override {
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return string(b)
}

func countResources(t *testing.T, name string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, database.DB.Model(&models.NomadResource{}).Where("name = ?", name).Count(&n).Error)
	return n
}

func uniqueName(t *testing.T, prefix string) string {
	t.Helper()
	sfx, err := apitoken.Generate()
	require.NoError(t, err)
	return prefix + "-" + strings.ToLower(sfx[5:13])
}

func mkOrgWithMember(t *testing.T, u models.User, role string) models.Organization {
	t.Helper()
	org := models.Organization{Name: uniqueName(t, "apiorg")}
	require.NoError(t, database.DB.Create(&org).Error)
	if role != "" {
		require.NoError(t, database.DB.Create(&models.Membership{UserID: u.ID, OrganizationID: org.ID, Role: role}).Error)
	}
	return org
}

func apiErrCode(t *testing.T, body string) string {
	t.Helper()
	var e APIErrorResponse
	require.NoError(t, json.Unmarshal([]byte(body), &e), body)
	return e.Error.Code
}

func TestCreateResourceAPI_Success(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	name := uniqueName(t, "mysql")

	status, body := postResource(t, app, plain, validBody(t, u.Username, name, nil))
	require.Equal(t, 201, status, body)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	assert.ElementsMatch(t, []string{"id", "name", "type", "namespace", "description", "repository_url",
		"file_path", "license", "tags", "webhook_url", "webhook_secret", "created_at", "updated_at", "latest_version"}, keys(got))
	assert.Equal(t, name, got["name"])
	assert.Equal(t, "pack", got["type"])
	assert.Equal(t, u.Username, got["namespace"])
	assert.Equal(t, "d", got["description"])
	assert.Equal(t, "https://github.com/x/y", got["repository_url"])
	assert.Equal(t, "packs/mysql", got["file_path"])
	assert.Equal(t, "MIT", got["license"])
	assert.Equal(t, []any{"db"}, got["tags"])
	assert.Equal(t, "v1.0.0", got["latest_version"])
	assert.NotEmpty(t, got["webhook_secret"])
	assert.NotEmpty(t, got["created_at"])
	assert.NotEmpty(t, got["updated_at"])
	id := fmt.Sprintf("%.0f", got["id"])
	assert.True(t, strings.HasSuffix(got["webhook_url"].(string), "/resource/"+id+"/webhook"), got["webhook_url"])

	var row models.NomadResource
	require.NoError(t, database.DB.Where("name = ? AND user_id = ?", name, u.ID).First(&row).Error)
	assert.Nil(t, row.OrganizationID)
	assert.Equal(t, got["webhook_secret"], row.WebhookSecret)
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestCreateResourceAPI_EmptyTagsIsArray(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	status, body := postResource(t, app, plain, validBody(t, u.Username, uniqueName(t, "nt"), map[string]any{"tags": nil}))
	require.Equal(t, 201, status, body)
	assert.Contains(t, body, `"tags":[]`)
}

func TestCreateResourceAPI_ResponseTagsAreStoredTags(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	status, body := postResource(t, app, plain, validBody(t, u.Username, uniqueName(t, "tg"), map[string]any{"tags": []string{"db", "ci-cd"}}))
	require.Equal(t, 201, status, body)
	var got ResourceResponse
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	assert.Equal(t, []string{"ci-cd", "db"}, got.Tags)
}

func TestCreateResourceAPI_Namespaces(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	other := mkAPIUser(t, "bob", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	memberOrg := mkOrgWithMember(t, u, "member")
	foreignOrg := mkOrgWithMember(t, other, "owner")
	deletedOrg := mkOrgWithMember(t, u, "owner")
	require.NoError(t, database.DB.Delete(&deletedOrg).Error)

	tests := []struct {
		name       string
		ns         string
		wantStatus int
		wantCode   string
		wantNS     string
	}{
		{"own username", u.Username, 201, "", u.Username},
		{"own username other case", strings.ToUpper(u.Username), 201, "", u.Username},
		{"member org", memberOrg.Name, 201, "", memberOrg.Name},
		{"member org other case", strings.ToUpper(memberOrg.Name), 201, "", memberOrg.Name},
		{"org non-member", foreignOrg.Name, 403, "forbidden", ""},
		{"another user", other.Username, 404, "not_found", ""},
		{"unknown", "no-such-namespace", 404, "not_found", ""},
		{"deleted org", deletedOrg.Name, 404, "not_found", ""},
		{"empty", "", 404, "not_found", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := uniqueName(t, "ns")
			status, body := postResource(t, app, plain, validBody(t, tt.ns, name, nil))
			require.Equal(t, tt.wantStatus, status, body)
			if tt.wantStatus == 201 {
				var got ResourceResponse
				require.NoError(t, json.Unmarshal([]byte(body), &got))
				assert.Equal(t, tt.wantNS, got.Namespace)
				assert.EqualValues(t, 1, countResources(t, name))
				return
			}
			assert.Equal(t, tt.wantCode, apiErrCode(t, body))
			assert.EqualValues(t, 0, countResources(t, name))
		})
	}
}

func TestCreateResourceAPI_NamespaceNotFoundIndistinguishable(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	other := mkAPIUser(t, "bob", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	name := uniqueName(t, "enum")

	s1, b1 := postResource(t, app, plain, validBody(t, other.Username, name, nil))
	s2, b2 := postResource(t, app, plain, validBody(t, "no-such-ns-xyz", name, nil))
	assert.Equal(t, 404, s1)
	assert.Equal(t, s1, s2)
	assert.Equal(t, b1, b2)
	assert.Contains(t, b1, "namespace not found")
}

func TestCreateResourceAPI_ValidationErrors(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)

	tests := []struct {
		name     string
		override map[string]any
		code     string
	}{
		{"bad name", map[string]any{"name": "-x"}, "invalid_name"},
		{"bad type", map[string]any{"type": "chart"}, "invalid_type"},
		{"http url", map[string]any{"repository_url": "http://github.com/x/y"}, "invalid_repository_url"},
		{"disallowed host", map[string]any{"repository_url": "https://evil.example.com/x/y"}, "invalid_repository_url"},
		{"empty version", map[string]any{"version": ""}, "invalid_version"},
		{"blank version", map[string]any{"version": "   "}, "invalid_version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := uniqueName(t, "val")
			ov := map[string]any{}
			maps.Copy(ov, tt.override)
			if _, ok := ov["name"]; !ok {
				ov["name"] = name
			}
			before := countResources(t, name)
			status, body := postResource(t, app, plain, validBody(t, u.Username, name, ov))
			assert.Equal(t, 422, status, body)
			assert.Equal(t, tt.code, apiErrCode(t, body))
			assert.Equal(t, before, countResources(t, name))
			var total int64
			database.DB.Model(&models.NomadResource{}).Where("user_id = ?", u.ID).Count(&total)
			assert.EqualValues(t, 0, total)
		})
	}
}

func TestCreateResourceAPI_Duplicate(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	name := uniqueName(t, "dup")

	status, body := postResource(t, app, plain, validBody(t, u.Username, name, nil))
	require.Equal(t, 201, status, body)
	status, body = postResource(t, app, plain, validBody(t, u.Username, strings.ToUpper(name), nil))
	assert.Equal(t, 409, status, body)
	assert.Equal(t, "conflict", apiErrCode(t, body))
	assert.EqualValues(t, 1, countResources(t, name))
}

func TestCreateResourceAPI_BadBodies(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	name := uniqueName(t, "bad")
	valid := validBody(t, u.Username, name, nil)

	tests := []struct {
		name, body string
	}{
		{"unknown field", validBody(t, u.Username, name, map[string]any{"webhook_secret": "x"})},
		{"malformed", `{"name":`},
		{"empty body", ``},
		{"not an object", `[1,2]`},
		{"wrong field type", validBody(t, u.Username, name, map[string]any{"tags": "db"})},
		{"trailing object", valid + valid},
		{"trailing garbage", valid + " x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := postResource(t, app, plain, tt.body)
			assert.Equal(t, 400, status, body)
			assert.Equal(t, "bad_request", apiErrCode(t, body))
			assert.NotContains(t, body, "unknown field")
			assert.EqualValues(t, 0, countResources(t, name))
		})
	}

	// A JSON null decodes to an empty request; there is no namespace to resolve.
	status, body := postResource(t, app, plain, `null`)
	assert.Equal(t, 404, status, body)
	assert.Equal(t, "not_found", apiErrCode(t, body))
}

func TestCreateResourceAPI_WithoutLocalsIs500(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: APIErrorHandler})
	app.Post("/v1/resources", CreateResourceAPI)
	status, body := postResource(t, app, "", `{}`)
	assert.Equal(t, 500, status)
	assert.Equal(t, "internal_error", apiErrCode(t, body))
}

func TestCreateResourceAPI_NoTokenIs401(t *testing.T) {
	app, _ := resourceAPIApp(t)
	name := uniqueName(t, "noauth")
	status, body := postResource(t, app, "", validBody(t, "alice", name, nil))
	assert.Equal(t, 401, status)
	assert.Equal(t, "unauthorized", apiErrCode(t, body))
	assert.EqualValues(t, 0, countResources(t, name))
}

func getJSON(t *testing.T, app *fiber.App, token, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestGetResourceAPI_EqualsCreateResponse(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	org := mkOrgWithMember(t, u, "member")
	wr, _ := mkAPIToken(t, u, models.ScopeWrite)
	rd, _ := mkAPIToken(t, u, models.ScopeRead)

	// Unsorted tags: the create response must already show the stored form,
	// not the request order.
	tags := []string{"zeta", "db", "alpha"}
	for _, tt := range []struct {
		name string
		ns   string
	}{{"personal", u.Username}, {"org", org.Name}} {
		t.Run(tt.name, func(t *testing.T) {
			name := uniqueName(t, "Eq")
			status, body := postResource(t, app, wr, validBody(t, tt.ns, name, map[string]any{"tags": tags}))
			require.Equal(t, 201, status, body)
			var created CreateResourceResponse
			require.NoError(t, json.Unmarshal([]byte(body), &created))
			assert.Equal(t, []string{"alpha", "db", "zeta"}, created.Tags)

			idPath := fmt.Sprintf("/v1/resources/%d", created.ID)
			// Case-insensitive on both path segments.
			namePath := "/v1/namespaces/" + strings.ToUpper(tt.ns) + "/resources/" + strings.ToLower(name)
			for _, path := range []string{idPath, namePath} {
				status, body := getJSON(t, app, rd, path)
				require.Equal(t, 200, status, body)
				var got ResourceResponse
				require.NoError(t, json.Unmarshal([]byte(body), &got))
				assert.Equal(t, created.ResourceResponse, got, path)
				assert.True(t, created.CreatedAt.Equal(got.CreatedAt), path)
				assert.Equal(t, created.Tags, got.Tags, path)

				var raw map[string]any
				require.NoError(t, json.Unmarshal([]byte(body), &raw))
				assert.NotContains(t, raw, "webhook_secret")
				assert.NotContains(t, body, created.WebhookSecret)
			}
		})
	}
}

func TestGetResourceAPI_LatestVersionIsNewest(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	wr, _ := mkAPIToken(t, u, models.ScopeWrite)
	rd, _ := mkAPIToken(t, u, models.ScopeRead)
	status, body := postResource(t, app, wr, validBody(t, u.Username, uniqueName(t, "lv"), nil))
	require.Equal(t, 201, status, body)
	var created ResourceResponse
	require.NoError(t, json.Unmarshal([]byte(body), &created))

	now := time.Now()
	// Newer row inserted first, older second.
	require.NoError(t, database.DB.Create(&models.ResourceVersion{ResourceID: created.ID, Version: "v3.0.0", CreatedAt: now.Add(time.Hour)}).Error)
	require.NoError(t, database.DB.Create(&models.ResourceVersion{ResourceID: created.ID, Version: "v0.1.0", CreatedAt: now.Add(-time.Hour)}).Error)

	status, body = getJSON(t, app, rd, fmt.Sprintf("/v1/resources/%d", created.ID))
	require.Equal(t, 200, status, body)
	var got ResourceResponse
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	assert.Equal(t, "v3.0.0", got.LatestVersion)
}

func TestGetResourceAPI_NotFound(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	wr, _ := mkAPIToken(t, u, models.ScopeWrite)
	rd, _ := mkAPIToken(t, u, models.ScopeRead)
	name := uniqueName(t, "gone")
	status, body := postResource(t, app, wr, validBody(t, u.Username, name, nil))
	require.Equal(t, 201, status, body)
	var created ResourceResponse
	require.NoError(t, json.Unmarshal([]byte(body), &created))

	// Each case is a request that would succeed if the resource were live.
	liveID := fmt.Sprintf("/v1/resources/%d", created.ID)
	liveName := "/v1/namespaces/" + u.Username + "/resources/" + name
	status, _ = getJSON(t, app, rd, liveID)
	require.Equal(t, 200, status)
	status, _ = getJSON(t, app, rd, liveName)
	require.Equal(t, 200, status)

	tests := []struct {
		name string
		path string
	}{
		{"unknown id", "/v1/resources/999999999"},
		{"unknown namespace", "/v1/namespaces/no-such-ns-zz/resources/" + name},
		{"unknown name", "/v1/namespaces/" + u.Username + "/resources/no-such-name-zz"},
		{"id zero", "/v1/resources/0"},
		{"negative id passes the route constraint", "/v1/resources/-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := getJSON(t, app, rd, tt.path)
			assert.Equal(t, 404, status, body)
			assert.Equal(t, "not_found", apiErrCode(t, body))
			assert.Contains(t, body, "resource not found")
			assert.NotContains(t, body, "no-such")
		})
	}

	require.NoError(t, database.DB.Delete(&models.NomadResource{}, created.ID).Error)
	for _, path := range []string{liveID, liveName} {
		status, body := getJSON(t, app, rd, path)
		assert.Equal(t, 404, status, path)
		assert.Equal(t, "not_found", apiErrCode(t, body))
	}
}

func TestGetResourceAPI_DatabaseFailureIs500(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	rd, _ := mkAPIToken(t, u, models.ScopeRead)
	saved := resource.Service
	t.Cleanup(func() { resource.Service = saved })
	resource.Service = resource.NewResourceService(database.DB.Session(&gorm.Session{}).Table("no_such_table"))

	status, body := getJSON(t, app, rd, "/v1/resources/1")
	assert.Equal(t, 500, status, body)
	assert.Equal(t, "internal_error", apiErrCode(t, body))
}
