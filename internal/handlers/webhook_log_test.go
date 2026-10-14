package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	"rmbl/internal/testutil/logtest"
)

func requireLineWithRequestID(t *testing.T, buf *bytes.Buffer, msg, rid string) {
	t.Helper()
	m := logtest.Find(t, buf, msg)
	if m == nil {
		t.Fatalf("%s not logged via request logger: %s", msg, buf.String())
	}
	assert.Equal(t, rid, m["request_id"])
}

func TestHandleGitHubIssueWebhook_LogsCarryRequestID(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		headers map[string]string
		status  int
		msg     string
	}{
		{"secret missing", "", nil, 500, "github.webhook_secret_missing"},
		{"signature missing", "test-secret", nil, 401, "webhook.signature_missing"},
		{"signature invalid", "test-secret", map[string]string{"X-Hub-Signature-256": "sha256=invalid"}, 401, "webhook.signature_invalid"},
		{"timestamp with bad signature", "test-secret", map[string]string{"X-Hub-Signature-256": "sha256=invalid", "X-Webhook-Timestamp": "1"}, 401, "webhook.validation_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_WEBHOOK_SECRET", tt.secret)
			var buf bytes.Buffer
			app := newRequestLogApp(zerolog.New(&buf), testSlog(&buf))
			app.Post("/webhooks/github/issues", HandleGitHubIssueWebhook)

			req := httptest.NewRequest("POST", "/webhooks/github/issues", nil)
			req.Header.Set("X-Request-ID", "rid-72-gh")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.status, resp.StatusCode)
			requireLineWithRequestID(t, &buf, tt.msg, "rid-72-gh")
		})
	}
}

func TestHandleWebhook_LogsCarryRequestID(t *testing.T) {
	defer cleanupTestData(t)

	user := createTestUser(t, "webhooklogger")
	resource := models.NomadResource{
		Name:          "webhook-log-pack",
		Type:          models.ResourceTypePack,
		UserID:        user.ID,
		WebhookSecret: "correct-secret",
	}
	require.NoError(t, database.DB.Create(&resource).Error)

	var buf bytes.Buffer
	app := newRequestLogApp(zerolog.New(&buf), testSlog(&buf))
	app.Post("/webhook/:id", HandleWebhook)

	req := httptest.NewRequest("POST", "/webhook/"+toString(resource.ID)+"?secret=wrong-secret", nil)
	req.Header.Set("X-Request-ID", "rid-72-wh")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
	requireLineWithRequestID(t, &buf, "webhook.validation_failed", "rid-72-wh")
}

// TestHandleGitHubIssueWebhook_DBLinesCarryRequestID drives signed webhooks
// through the real handler and proves the request-created and status-updated
// lines carry the request_id of their own request, not the global logger's.
func TestHandleGitHubIssueWebhook_DBLinesCarryRequestID(t *testing.T) {
	const secret = "test-secret"
	t.Setenv("GITHUB_WEBHOOK_SECRET", secret)

	// Keep the global logger off the request buffer so a handler that falls
	// back to it is detected rather than masked.
	var globalBuf bytes.Buffer
	origLog := logger.Log
	logger.Log = zerolog.New(&globalBuf)
	t.Cleanup(func() { logger.Log = origLog })

	suffix := time.Now().UnixNano()
	admin := models.User{
		Username:      fmt.Sprintf("ghadmin%d", suffix),
		Email:         fmt.Sprintf("ghadmin%d@test.com", suffix),
		Name:          "GH Admin",
		EmailVerified: true,
		IsAdmin:       true,
	}
	require.NoError(t, database.DB.Create(&admin).Error)

	issueNum := int(suffix%1000000) + 500000
	database.DB.Unscoped().Where("github_issue_num = ?", issueNum).Delete(&models.PackRequest{})
	t.Cleanup(func() {
		database.DB.Unscoped().Where("github_issue_num = ?", issueNum).Delete(&models.PackRequest{})
		database.DB.Unscoped().Delete(&admin)
	})

	var buf bytes.Buffer
	app := newRequestLogApp(zerolog.New(&buf), testSlog(&buf))
	app.Post("/webhooks/github/issues", HandleGitHubIssueWebhook)

	send := func(action, state string, labels []LabelPayload) {
		t.Helper()
		buf.Reset()
		body, err := json.Marshal(IssueWebhookPayload{
			Action: action,
			Issue: IssuePayload{
				Number:  issueNum,
				Title:   "Please add a pack",
				Body:    "from a human",
				State:   state,
				HTMLURL: fmt.Sprintf("https://github.com/o/r/issues/%d", issueNum),
				Labels:  labels,
			},
		})
		require.NoError(t, err)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req := httptest.NewRequest("POST", "/webhooks/github/issues", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "issues")
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		resp, err := app.Test(req)
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
	}

	// lineFor returns the http.request line's request_id and the single line
	// for msg, asserting the latter exists exactly once.
	lineFor := func(msg string) (httpRID string, line map[string]any) {
		t.Helper()
		var n int
		for _, m := range logtest.Lines(t, &buf) {
			switch m[zerolog.MessageFieldName] {
			case "http.request":
				httpRID, _ = m["request_id"].(string)
			case msg:
				line = m
				n++
			}
		}
		require.NotEmpty(t, httpRID, "no http.request request_id: %s", buf.String())
		require.Equal(t, 1, n, "expected one %s line: %s", msg, buf.String())
		return httpRID, line
	}

	// Step 1: opened issue creates a request.
	send("opened", "open", []LabelPayload{{Name: "pack-request"}})
	var pr models.PackRequest
	require.NoError(t, database.DB.Where("github_issue_num = ?", issueNum).First(&pr).Error)
	assert.Equal(t, models.RequestStatusOpen, pr.Status)
	assert.Equal(t, models.ResourceTypePack, pr.Type)
	var owner models.User
	require.NoError(t, database.DB.First(&owner, pr.UserID).Error)
	assert.True(t, owner.IsAdmin)

	createdRID, created := lineFor("github.webhook_request_created")
	assert.Equal(t, createdRID, created["request_id"])
	assert.EqualValues(t, issueNum, created["issue_number"])

	// Step 2: closed issue moves it to completed.
	send("closed", "closed", nil)
	require.NoError(t, database.DB.Where("github_issue_num = ?", issueNum).First(&pr).Error)
	assert.Equal(t, models.RequestStatusCompleted, pr.Status)

	updatedRID, updated := lineFor("github.webhook_request_status_updated")
	assert.Equal(t, updatedRID, updated["request_id"])
	assert.NotEqual(t, createdRID, updatedRID, "each request has its own id")

	for _, m := range logtest.Lines(t, &globalBuf) {
		msg := m[zerolog.MessageFieldName]
		assert.NotEqual(t, "github.webhook_request_created", msg)
		assert.NotEqual(t, "github.webhook_request_status_updated", msg)
	}
}
