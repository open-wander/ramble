package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
)

func sendJSON(t *testing.T, app *fiber.App, method, token, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// createViaAPI creates a resource in ns and returns its decoded response.
func createViaAPI(t *testing.T, app *fiber.App, token, ns string, override map[string]any) ResourceResponse {
	t.Helper()
	status, body := postResource(t, app, token, validBody(t, ns, uniqueName(t, "pt"), override))
	require.Equal(t, 201, status, body)
	var r ResourceResponse
	require.NoError(t, json.Unmarshal([]byte(body), &r))
	return r
}

func resPath(id uint) string { return fmt.Sprintf("/v1/resources/%d", id) }

func loadRow(t *testing.T, id uint) models.NomadResource {
	t.Helper()
	var r models.NomadResource
	require.NoError(t, database.DB.Preload("Tags").First(&r, id).Error)
	return r
}

func TestUpdateResourceAPI_PartialUpdate(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	created := createViaAPI(t, app, plain, u.Username, map[string]any{"tags": []string{"a1", "b2"}})
	path := resPath(created.ID)

	status, body := sendJSON(t, app, "PATCH", plain, path, `{"description":"new"}`)
	require.Equal(t, 200, status, body)
	var got ResourceResponse
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	assert.Equal(t, "new", got.Description)
	assert.Equal(t, created.RepositoryURL, got.RepositoryURL)
	assert.Equal(t, created.FilePath, got.FilePath)
	assert.Equal(t, created.License, got.License)
	assert.Equal(t, created.Tags, got.Tags)
	assert.Equal(t, created.Name, got.Name)
	assert.NotContains(t, body, "webhook_secret")

	row := loadRow(t, created.ID)
	assert.Equal(t, "new", row.Description)
	assert.Equal(t, created.RepositoryURL, row.RepositoryURL)

	gs, gb := getJSON(t, app, plain, path)
	require.Equal(t, 200, gs, gb)
	var again ResourceResponse
	require.NoError(t, json.Unmarshal([]byte(gb), &again))
	assert.Equal(t, again, got)

	// Empty object and null fields change nothing.
	status, body = sendJSON(t, app, "PATCH", plain, path, `{"description":null,"tags":null}`)
	require.Equal(t, 200, status, body)
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	assert.Equal(t, "new", got.Description)
	assert.Equal(t, created.Tags, got.Tags)
	status, body = sendJSON(t, app, "PATCH", plain, path, `{}`)
	assert.Equal(t, 200, status, body)
}

func TestUpdateResourceAPI_FieldSemantics(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	created := createViaAPI(t, app, plain, u.Username, map[string]any{"tags": []string{"a1"}})
	path := resPath(created.ID)

	tests := []struct {
		name  string
		body  string
		check func(t *testing.T, r ResourceResponse)
	}{
		{"tags replace", `{"tags":["a1","b2"]}`, func(t *testing.T, r ResourceResponse) { assert.Equal(t, []string{"a1", "b2"}, r.Tags) }},
		{"tags replaced again", `{"tags":["c3"]}`, func(t *testing.T, r ResourceResponse) {
			assert.Equal(t, []string{"c3"}, r.Tags)
			assert.Equal(t, "d", r.Description)
		}},
		{"tags omitted keeps", `{"description":"x"}`, func(t *testing.T, r ResourceResponse) { assert.Equal(t, []string{"c3"}, r.Tags) }},
		{"tags empty clears", `{"tags":[]}`, func(t *testing.T, r ResourceResponse) { assert.Equal(t, []string{}, r.Tags) }},
		{"empty strings clear", `{"description":"","file_path":""}`, func(t *testing.T, r ResourceResponse) {
			assert.Empty(t, r.Description)
			assert.Empty(t, r.FilePath)
		}},
		{"empty license is Unknown", `{"license":""}`, func(t *testing.T, r ResourceResponse) { assert.Equal(t, "Unknown", r.License) }},
		{"valid url stored", `{"repository_url":"https://github.com/a/b"}`, func(t *testing.T, r ResourceResponse) {
			assert.Equal(t, "https://github.com/a/b", r.RepositoryURL)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := sendJSON(t, app, "PATCH", plain, path, tt.body)
			require.Equal(t, 200, status, body)
			var r ResourceResponse
			require.NoError(t, json.Unmarshal([]byte(body), &r))
			tt.check(t, r)
		})
	}
}

func TestUpdateResourceAPI_Rejections(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	created := createViaAPI(t, app, plain, u.Username, nil)
	path := resPath(created.ID)

	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"name", `{"name":"other","description":"x"}`, 422, "immutable_field"},
		{"name null", `{"name":null,"description":"x"}`, 422, "immutable_field"},
		{"name wrong case", `{"Name":"other","description":"x"}`, 422, "immutable_field"},
		{"type", `{"type":"job","description":"x"}`, 422, "immutable_field"},
		{"namespace", `{"namespace":"bob","description":"x"}`, 422, "immutable_field"},
		{"unknown field", `{"bogus":1,"description":"x"}`, 400, "bad_request"},
		{"malformed", `{"description":`, 400, "bad_request"},
		{"trailing data", `{"description":"x"} {}`, 400, "bad_request"},
		{"wrong type", `{"description":5}`, 400, "bad_request"},
		{"http url", `{"description":"x","repository_url":"http://github.com/x/y"}`, 422, "invalid_repository_url"},
		{"foreign host", `{"description":"x","repository_url":"https://evil.example/x"}`, 422, "invalid_repository_url"},
		{"empty url", `{"description":"x","repository_url":""}`, 422, "invalid_repository_url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := sendJSON(t, app, "PATCH", plain, path, tt.body)
			assert.Equal(t, tt.status, status, body)
			assert.Equal(t, tt.code, apiErrCode(t, body))
			row := loadRow(t, created.ID)
			assert.Equal(t, "d", row.Description)
			assert.Equal(t, created.RepositoryURL, row.RepositoryURL)
			assert.Equal(t, created.Name, row.Name)
			assert.Equal(t, models.ResourceType("pack"), row.Type)
		})
	}
}

func TestUpdateResourceAPI_NotFoundAndAuth(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	read, _ := mkAPIToken(t, u, models.ScopeRead)
	created := createViaAPI(t, app, plain, u.Username, nil)

	status, body := sendJSON(t, app, "PATCH", plain, "/v1/resources/999999", `{"description":"x"}`)
	assert.Equal(t, 404, status)
	assert.Equal(t, "not_found", apiErrCode(t, body))
	status, body = sendJSON(t, app, "DELETE", plain, "/v1/resources/999999", "")
	assert.Equal(t, 404, status)
	assert.Equal(t, "not_found", apiErrCode(t, body))

	for _, m := range []string{"PATCH", "DELETE"} {
		status, body = sendJSON(t, app, m, read, resPath(created.ID), `{"description":"x"}`)
		assert.Equal(t, 403, status, m)
		assert.Equal(t, "insufficient_scope", apiErrCode(t, body))
		status, body = sendJSON(t, app, m, "", resPath(created.ID), `{"description":"x"}`)
		assert.Equal(t, 401, status, m)
		assert.Equal(t, "unauthorized", apiErrCode(t, body))
	}
	assert.Equal(t, "d", loadRow(t, created.ID).Description)

	// Another user's token on a personal resource.
	bob := mkAPIUser(t, "bob", true, "")
	bobTok, _ := mkAPIToken(t, bob, models.ScopeWrite)
	for _, m := range []string{"PATCH", "DELETE"} {
		status, body = sendJSON(t, app, m, bobTok, resPath(created.ID), `{"description":"x"}`)
		assert.Equal(t, 403, status, m)
		assert.Equal(t, "forbidden", apiErrCode(t, body))
	}
	row := loadRow(t, created.ID)
	assert.Equal(t, "d", row.Description)
	assert.False(t, row.DeletedAt.Valid)
}

func TestResourceAPI_OrgRoles(t *testing.T) {
	app, _ := resourceAPIApp(t)
	owner := mkAPIUser(t, "alice", true, "")
	member := mkAPIUser(t, "bob", true, "")
	outsider := mkAPIUser(t, "carol", true, "")
	org := mkOrgWithMember(t, owner, "owner")
	require.NoError(t, database.DB.Create(&models.Membership{UserID: member.ID, OrganizationID: org.ID, Role: "member"}).Error)
	ownerTok, _ := mkAPIToken(t, owner, models.ScopeWrite)
	memberTok, _ := mkAPIToken(t, member, models.ScopeWrite)
	outTok, _ := mkAPIToken(t, outsider, models.ScopeWrite)
	created := createViaAPI(t, app, ownerTok, org.Name, nil)
	path := resPath(created.ID)

	status, body := sendJSON(t, app, "PATCH", memberTok, path, `{"description":"by member"}`)
	assert.Equal(t, 200, status, body)
	assert.Equal(t, "by member", loadRow(t, created.ID).Description)

	status, body = sendJSON(t, app, "PATCH", outTok, path, `{"description":"by outsider"}`)
	assert.Equal(t, 403, status, body)
	assert.Equal(t, "forbidden", apiErrCode(t, body))
	assert.Equal(t, "by member", loadRow(t, created.ID).Description)

	status, body = sendJSON(t, app, "DELETE", memberTok, path, "")
	assert.Equal(t, 403, status, body)
	assert.Equal(t, "forbidden", apiErrCode(t, body))
	assert.False(t, loadRow(t, created.ID).DeletedAt.Valid)

	status, body = sendJSON(t, app, "DELETE", ownerTok, path, "")
	assert.Equal(t, 204, status, body)
	assert.Empty(t, body)
	var n int64
	require.NoError(t, database.DB.Unscoped().Model(&models.NomadResource{}).Where("id = ? AND deleted_at IS NOT NULL", created.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n, "soft deleted")

	for _, m := range []string{"DELETE", "PATCH"} {
		status, body = sendJSON(t, app, m, ownerTok, path, `{"description":"x"}`)
		assert.Equal(t, 404, status, m)
		assert.Equal(t, "not_found", apiErrCode(t, body))
	}
	gs, _ := getJSON(t, app, ownerTok, path)
	assert.Equal(t, 404, gs)
}

func TestDeleteResourceAPI_PersonalOwner(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	created := createViaAPI(t, app, plain, u.Username, nil)
	status, body := sendJSON(t, app, "DELETE", plain, resPath(created.ID), "")
	assert.Equal(t, 204, status, body)
	assert.EqualValues(t, 0, countResources(t, created.Name))
}

func TestResourceAPI_WriteAudit(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, tok := mkAPIToken(t, u, models.ScopeWrite)
	created := createViaAPI(t, app, plain, u.Username, nil)
	const secretDesc = "audit-marker-description"

	status, body := sendJSON(t, app, "PATCH", plain, resPath(created.ID), `{"description":"`+secretDesc+`"}`)
	require.Equal(t, 200, status, body)
	status, body = sendJSON(t, app, "DELETE", plain, resPath(created.ID), "")
	require.Equal(t, 204, status, body)

	for _, action := range []string{"resource.edit", "resource.delete"} {
		var row models.AuditLog
		require.NoError(t, database.DB.Where("action = ? AND target_id = ?", action, created.ID).First(&row).Error, action)
		assert.Equal(t, u.ID, row.ActorID)
		var d map[string]any
		require.NoError(t, json.Unmarshal([]byte(row.Details), &d))
		assert.Equal(t, "token", d["via"])
		assert.Equal(t, float64(tok.ID), d["token_id"])
		assert.NotContains(t, row.Details, secretDesc)
	}
}

func TestUpdateResourceAPI_WithoutLocalsIs500(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: APIErrorHandler})
	app.Patch("/v1/resources/:id<int>", UpdateResourceAPI)
	app.Delete("/v1/resources/:id<int>", DeleteResourceAPI)
	for _, m := range []string{"PATCH", "DELETE"} {
		status, body := sendJSON(t, app, m, "", "/v1/resources/1", `{}`)
		assert.Equal(t, 500, status, m)
		assert.Equal(t, "internal_error", apiErrCode(t, body))
	}
}
