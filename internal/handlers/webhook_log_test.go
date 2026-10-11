package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmbl/internal/database"
	"rmbl/internal/models"
)

// logLines decodes the JSON log lines in buf.
func logLines(buf *bytes.Buffer) []map[string]any {
	var out []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var m map[string]any
		if json.Unmarshal(line, &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func requireLineWithRequestID(t *testing.T, buf *bytes.Buffer, msg, rid string) {
	t.Helper()
	for _, m := range logLines(buf) {
		if m[zerolog.MessageFieldName] == msg {
			assert.Equal(t, rid, m["request_id"])
			return
		}
	}
	t.Fatalf("%s not logged via request logger: %s", msg, buf.String())
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
			app := newRequestLogApp(zerolog.New(&buf))
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
	app := newRequestLogApp(zerolog.New(&buf))
	app.Post("/webhook/:id", HandleWebhook)

	req := httptest.NewRequest("POST", "/webhook/"+toString(resource.ID)+"?secret=wrong-secret", nil)
	req.Header.Set("X-Request-ID", "rid-72-wh")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
	requireLineWithRequestID(t, &buf, "webhook.validation_failed", "rid-72-wh")
}
