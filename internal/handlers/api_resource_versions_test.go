package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/apitoken"
)

// mkVersionResource inserts a resource whose fetch fails fast, without the
// network, with one completed version v1.0.0. orgID nil makes it personal.
func mkVersionResource(t *testing.T, u models.User, orgID *uint) models.NomadResource {
	t.Helper()
	r := models.NomadResource{
		Name: uniqueName(t, "ver"), Type: models.ResourceTypePack, UserID: u.ID, OrganizationID: orgID,
		RepositoryURL: failFastRepoURL, License: "MIT", WebhookSecret: "old-secret-value",
	}
	require.NoError(t, database.DB.Create(&r).Error)
	v := models.ResourceVersion{ResourceID: r.ID, Version: "v1.0.0", FetchStatus: models.FetchStatusCompleted}
	require.NoError(t, database.DB.Create(&v).Error)
	t.Cleanup(func() {
		// Let a background fetch finish before the rows go away.
		time.Sleep(50 * time.Millisecond)
		database.DB.Unscoped().Where("resource_id = ?", r.ID).Delete(&models.ResourceVersion{})
		database.DB.Unscoped().Delete(&r)
	})
	return r
}

func apiDo(t *testing.T, app *fiber.App, method, path, token, body string) (int, string) {
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

func vpath(r models.NomadResource, rest string) string {
	return fmt.Sprintf("/v1/resources/%d%s", r.ID, rest)
}

func versionRowsOf(t *testing.T, id uint, version string) []models.ResourceVersion {
	t.Helper()
	var rows []models.ResourceVersion
	require.NoError(t, database.DB.Where("resource_id = ? AND version = ?", id, version).Find(&rows).Error)
	return rows
}

func waitFetchSettled(t *testing.T, id uint, version string) {
	t.Helper()
	require.Eventually(t, func() bool {
		rows := versionRowsOf(t, id, version)
		return len(rows) == 1 && (rows[0].FetchStatus == models.FetchStatusFailed || rows[0].FetchStatus == models.FetchStatusCompleted)
	}, 10*time.Second, 20*time.Millisecond)
}

func TestListResourceVersionsAPI(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeRead)
	r := mkVersionResource(t, u, nil)
	base := time.Now().Add(-time.Hour)
	for i, v := range []string{"v1.1.0", "v1.2.0"} {
		row := models.ResourceVersion{ResourceID: r.ID, Version: v, FetchStatus: models.FetchStatusFailed, FetchError: "bad", Content: "BIG",
			CreatedAt: base.Add(time.Duration(i) * time.Minute)}
		require.NoError(t, database.DB.Create(&row).Error)
	}
	require.NoError(t, database.DB.Model(&models.ResourceVersion{}).Where("resource_id = ? AND version = ?", r.ID, "v1.0.0").
		Update("created_at", base.Add(-time.Hour)).Error)

	status, body := apiDo(t, app, "GET", vpath(r, "/versions"), plain, "")
	require.Equal(t, 200, status, body)
	var raw []map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &raw))
	require.Len(t, raw, 3)
	for _, m := range raw {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		assert.ElementsMatch(t, []string{"version", "fetch_status", "fetch_error", "created_at"}, keys)
	}
	assert.Equal(t, "v1.2.0", raw[0]["version"])
	assert.Equal(t, "v1.1.0", raw[1]["version"])
	assert.Equal(t, "v1.0.0", raw[2]["version"])
	assert.Equal(t, "failed", raw[0]["fetch_status"])
	assert.Equal(t, "bad", raw[0]["fetch_error"])
	assert.NotContains(t, body, "BIG")
}

func TestListResourceVersionsAPI_EmptyIsArray(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeRead)
	r := mkVersionResource(t, u, nil)
	require.NoError(t, database.DB.Unscoped().Where("resource_id = ?", r.ID).Delete(&models.ResourceVersion{}).Error)
	status, body := apiDo(t, app, "GET", vpath(r, "/versions"), plain, "")
	assert.Equal(t, 200, status)
	assert.JSONEq(t, "[]", body)
}

func TestListResourceVersionsAPI_NotFoundAndOrg(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	orgA := mkOrgWithMember(t, u, "member")
	orgB := mkOrgWithMember(t, u, "member")
	plain, _ := mkAPIToken(t, u, models.ScopeRead)
	restricted, _, err := apitoken.Svc.Create(u.ID, "t", models.ScopeRead, &orgA.ID, 30)
	require.NoError(t, err)

	status, body := apiDo(t, app, "GET", "/v1/resources/999999999/versions", plain, "")
	assert.Equal(t, 404, status)
	assert.Equal(t, "not_found", apiErrCode(t, body))

	gone := mkVersionResource(t, u, nil)
	require.NoError(t, database.DB.Delete(&gone).Error)
	status, _ = apiDo(t, app, "GET", vpath(gone, "/versions"), plain, "")
	assert.Equal(t, 404, status)

	own := mkVersionResource(t, u, &orgA.ID)
	other := mkVersionResource(t, u, &orgB.ID)
	status, _ = apiDo(t, app, "GET", vpath(own, "/versions"), restricted, "")
	assert.Equal(t, 200, status)
	status, body = apiDo(t, app, "GET", vpath(other, "/versions"), restricted, "")
	assert.Equal(t, 403, status)
	assert.Equal(t, "forbidden", apiErrCode(t, body))
}

func TestCreateResourceVersionAPI(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	outsider := mkAPIUser(t, "bob", true, "")
	org := mkOrgWithMember(t, u, "member")
	plain, tok := mkAPIToken(t, u, models.ScopeWrite)
	readTok, _ := mkAPIToken(t, u, models.ScopeRead)
	outTok, _ := mkAPIToken(t, outsider, models.ScopeWrite)
	r := mkVersionResource(t, u, nil)
	orgRes := mkVersionResource(t, u, &org.ID)

	t.Run("accepted", func(t *testing.T) {
		status, body := apiDo(t, app, "POST", vpath(r, "/versions"), plain, `{"version":"v1.1.0"}`)
		require.Equal(t, 202, status, body)
		var v ResourceVersionResponse
		require.NoError(t, json.Unmarshal([]byte(body), &v))
		assert.Equal(t, "v1.1.0", v.Version)
		assert.Equal(t, "pending", v.FetchStatus)
		assert.Empty(t, v.FetchError)
		assert.False(t, v.CreatedAt.IsZero())
		require.Len(t, versionRowsOf(t, r.ID, "v1.1.0"), 1)
		waitFetchSettled(t, r.ID, "v1.1.0")

		var row models.AuditLog
		require.NoError(t, database.DB.Where("action = ? AND target_id = ?", "resource.version_create", r.ID).First(&row).Error)
		var d map[string]any
		require.NoError(t, json.Unmarshal([]byte(row.Details), &d))
		assert.Equal(t, "token", d["via"])
		assert.Equal(t, float64(tok.ID), d["token_id"])
		assert.Equal(t, "v1.1.0", d["version"])
	})
	t.Run("duplicate", func(t *testing.T) {
		status, body := apiDo(t, app, "POST", vpath(r, "/versions"), plain, `{"version":"v1.1.0"}`)
		assert.Equal(t, 409, status, body)
		assert.Equal(t, "conflict", apiErrCode(t, body))
		assert.Len(t, versionRowsOf(t, r.ID, "v1.1.0"), 1)
	})
	for name, ver := range map[string]string{"empty": "", "too long": strings.Repeat("v", 65), "traversal": "v1.tar.gz#/../../..", "slash": "a/b", "dotdot": "../x"} {
		t.Run("invalid "+name, func(t *testing.T) {
			b, _ := json.Marshal(map[string]string{"version": ver})
			status, body := apiDo(t, app, "POST", vpath(r, "/versions"), plain, string(b))
			assert.Equal(t, 422, status, body)
			assert.Equal(t, "invalid_version", apiErrCode(t, body))
			assert.Empty(t, versionRowsOf(t, r.ID, ver))
		})
	}
	t.Run("bad json", func(t *testing.T) {
		for _, b := range []string{`{`, `{"version":"v9","x":1}`, ``} {
			status, body := apiDo(t, app, "POST", vpath(r, "/versions"), plain, b)
			assert.Equal(t, 400, status, body)
			assert.Equal(t, "bad_request", apiErrCode(t, body))
		}
	})
	t.Run("org member allowed", func(t *testing.T) {
		status, body := apiDo(t, app, "POST", vpath(orgRes, "/versions"), plain, `{"version":"v2.0.0"}`)
		assert.Equal(t, 202, status, body)
		waitFetchSettled(t, orgRes.ID, "v2.0.0")
	})
	t.Run("non member forbidden", func(t *testing.T) {
		status, body := apiDo(t, app, "POST", vpath(orgRes, "/versions"), outTok, `{"version":"v3.0.0"}`)
		assert.Equal(t, 403, status, body)
		assert.Equal(t, "forbidden", apiErrCode(t, body))
		assert.Empty(t, versionRowsOf(t, orgRes.ID, "v3.0.0"))
	})
	t.Run("unknown resource", func(t *testing.T) {
		status, body := apiDo(t, app, "POST", "/v1/resources/999999999/versions", plain, `{"version":"v1"}`)
		assert.Equal(t, 404, status, body)
	})
	t.Run("read token", func(t *testing.T) {
		status, body := apiDo(t, app, "POST", vpath(r, "/versions"), readTok, `{"version":"v4.0.0"}`)
		assert.Equal(t, 403, status, body)
		assert.Equal(t, "insufficient_scope", apiErrCode(t, body))
		assert.Empty(t, versionRowsOf(t, r.ID, "v4.0.0"))
	})
	t.Run("no token", func(t *testing.T) {
		status, _ := apiDo(t, app, "POST", vpath(r, "/versions"), "", `{"version":"v5.0.0"}`)
		assert.Equal(t, 401, status)
	})
	t.Run("org restricted token", func(t *testing.T) {
		other := mkOrgWithMember(t, u, "member")
		restricted, _, err := apitoken.Svc.Create(u.ID, "t", models.ScopeWrite, &other.ID, 30)
		require.NoError(t, err)
		status, body := apiDo(t, app, "POST", vpath(orgRes, "/versions"), restricted, `{"version":"v6.0.0"}`)
		assert.Equal(t, 403, status, body)
		assert.Empty(t, versionRowsOf(t, orgRes.ID, "v6.0.0"))
	})
}

func TestRetryResourceVersionAPI(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	outsider := mkAPIUser(t, "bob", true, "")
	plain, tok := mkAPIToken(t, u, models.ScopeWrite)
	outTok, _ := mkAPIToken(t, outsider, models.ScopeWrite)
	r := mkVersionResource(t, u, nil)
	for _, v := range []struct {
		name   string
		status models.FetchStatus
	}{
		{"v1.0.0+build", models.FetchStatusFailed},
		{"v+plus", models.FetchStatusFailed},
		{"vpending", models.FetchStatusPending},
		{"vdone", models.FetchStatusCompleted},
	} {
		require.NoError(t, database.DB.Create(&models.ResourceVersion{ResourceID: r.ID, Version: v.name, FetchStatus: v.status, FetchError: "old"}).Error)
	}

	t.Run("failed, escaped plus", func(t *testing.T) {
		status, body := apiDo(t, app, "POST", vpath(r, "/versions/v1.0.0%2Bbuild/retry"), plain, "")
		require.Equal(t, 202, status, body)
		var v ResourceVersionResponse
		require.NoError(t, json.Unmarshal([]byte(body), &v))
		assert.Equal(t, "v1.0.0+build", v.Version)
		assert.Equal(t, "pending", v.FetchStatus)
		assert.Empty(t, v.FetchError)
		waitFetchSettled(t, r.ID, "v1.0.0+build")

		var row models.AuditLog
		require.NoError(t, database.DB.Where("action = ? AND target_id = ?", "resource.retry_fetch", r.ID).First(&row).Error)
		assert.Contains(t, row.Details, `"via":"token"`)
		assert.Contains(t, row.Details, fmt.Sprintf(`"token_id":%d`, tok.ID))
	})
	t.Run("failed, literal plus", func(t *testing.T) {
		status, body := apiDo(t, app, "POST", vpath(r, "/versions/v+plus/retry"), plain, "")
		require.Equal(t, 202, status, body)
		waitFetchSettled(t, r.ID, "v+plus")
	})
	for _, name := range []string{"vpending", "vdone"} {
		t.Run("conflict "+name, func(t *testing.T) {
			before := versionRowsOf(t, r.ID, name)[0]
			status, body := apiDo(t, app, "POST", vpath(r, "/versions/"+name+"/retry"), plain, "")
			assert.Equal(t, 409, status, body)
			assert.Equal(t, "conflict", apiErrCode(t, body))
			after := versionRowsOf(t, r.ID, name)[0]
			assert.Equal(t, before.FetchStatus, after.FetchStatus)
			assert.Equal(t, "old", after.FetchError)
		})
	}
	t.Run("unknown version", func(t *testing.T) {
		status, body := apiDo(t, app, "POST", vpath(r, "/versions/nope/retry"), plain, "")
		assert.Equal(t, 404, status, body)
		assert.Equal(t, "not_found", apiErrCode(t, body))
	})
	t.Run("non member", func(t *testing.T) {
		status, body := apiDo(t, app, "POST", vpath(r, "/versions/vdone/retry"), outTok, "")
		assert.Equal(t, 403, status, body)
	})
}

func TestRotateWebhookSecretAPI(t *testing.T) {
	app, buf := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	outsider := mkAPIUser(t, "bob", true, "")
	plain, tok := mkAPIToken(t, u, models.ScopeWrite)
	readTok, _ := mkAPIToken(t, u, models.ScopeRead)
	outTok, _ := mkAPIToken(t, outsider, models.ScopeWrite)
	r := mkVersionResource(t, u, nil)

	status, body := apiDo(t, app, "POST", vpath(r, "/webhook-secret"), outTok, "")
	assert.Equal(t, 403, status, body)
	status, body = apiDo(t, app, "POST", vpath(r, "/webhook-secret"), readTok, "")
	assert.Equal(t, 403, status, body)
	assert.Equal(t, "insufficient_scope", apiErrCode(t, body))
	var unchanged models.NomadResource
	require.NoError(t, database.DB.First(&unchanged, r.ID).Error)
	assert.Equal(t, "old-secret-value", unchanged.WebhookSecret)

	status, body = apiDo(t, app, "POST", vpath(r, "/webhook-secret"), plain, "")
	require.Equal(t, 200, status, body)
	var resp WebhookSecretResponse
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	require.NotEmpty(t, resp.WebhookSecret)
	assert.NotEqual(t, "old-secret-value", resp.WebhookSecret)

	var stored models.NomadResource
	require.NoError(t, database.DB.First(&stored, r.ID).Error)
	assert.Equal(t, resp.WebhookSecret, stored.WebhookSecret)

	// The secret is in this response only.
	status, body = apiDo(t, app, "GET", vpath(r, ""), plain, "")
	require.Equal(t, 200, status, body)
	assert.NotContains(t, body, "webhook_secret")
	assert.NotContains(t, body, resp.WebhookSecret)

	var row models.AuditLog
	require.NoError(t, database.DB.Where("action = ? AND target_id = ?", "resource.reset_webhook", r.ID).First(&row).Error)
	assert.Contains(t, row.Details, `"via":"token"`)
	assert.Contains(t, row.Details, fmt.Sprintf(`"token_id":%d`, tok.ID))
	assert.NotContains(t, row.Details, resp.WebhookSecret)
	assert.NotContains(t, buf.String(), resp.WebhookSecret)

	status, _ = apiDo(t, app, "POST", "/v1/resources/999999999/webhook-secret", plain, "")
	assert.Equal(t, 404, status)
}

func TestRetryResourceVersionAPI_ConcurrentOneAccepted(t *testing.T) {
	app, _ := resourceAPIApp(t)
	u := mkAPIUser(t, "alice", true, "")
	plain, _ := mkAPIToken(t, u, models.ScopeWrite)
	r := mkVersionResource(t, u, nil)
	require.NoError(t, database.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", r.ID).
		Update("fetch_status", models.FetchStatusFailed).Error)

	// Hold the started fetch in "fetching" so a late retry cannot see it fail again.
	release := make(chan struct{})
	fetchVersionHook = func() { <-release }
	t.Cleanup(func() { fetchVersionHook = nil })

	const n = 6
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { codes[i], _ = apiDo(t, app, "POST", vpath(r, "/versions/v1.0.0/retry"), plain, "") })
	}
	wg.Wait()
	accepted := 0
	for _, c := range codes {
		if c == 202 {
			accepted++
		} else {
			assert.Equal(t, 409, c)
		}
	}
	assert.Equal(t, 1, accepted)
	close(release)
	waitFetchSettled(t, r.ID, "v1.0.0")
}

func TestVersionAPIHandlersHaveNoAdminBypass(t *testing.T) {
	for _, f := range []string{"api_resources.go", "api_resource_versions.go"} {
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		assert.NotContains(t, string(src), "IsAdmin", f)
	}
}
