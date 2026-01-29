package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/github"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestMapAppStatusToGitHub(t *testing.T) {
	testCases := []struct {
		status        models.RequestStatus
		expectedState string
		expectedLabel string
	}{
		{models.RequestStatusOpen, "open", "status:open"},
		{models.RequestStatusInProgress, "open", "status:in-progress"},
		{models.RequestStatusCompleted, "closed", "status:completed"},
		{models.RequestStatusClosed, "closed", "status:closed"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.status), func(t *testing.T) {
			state, label := mapAppStatusToGitHub(tc.status)
			assert.Equal(t, tc.expectedState, state)
			assert.Equal(t, tc.expectedLabel, label)
		})
	}
}

func TestMapAppStatusToGitHub_Default(t *testing.T) {
	// Test with an invalid/unknown status
	state, label := mapAppStatusToGitHub(models.RequestStatus("unknown"))
	assert.Equal(t, "open", state)
	assert.Equal(t, "status:open", label)
}

func TestMapGitHubToAppStatus_ByLabel(t *testing.T) {
	testCases := []struct {
		labelName      string
		expectedStatus models.RequestStatus
	}{
		{"status:open", models.RequestStatusOpen},
		{"status:in-progress", models.RequestStatusInProgress},
		{"status:completed", models.RequestStatusCompleted},
		{"status:closed", models.RequestStatusClosed},
	}

	for _, tc := range testCases {
		t.Run(tc.labelName, func(t *testing.T) {
			issue := &github.Issue{
				State: "open",
				Labels: []github.Label{
					{Name: tc.labelName},
				},
			}
			status := mapGitHubToAppStatus(issue)
			assert.Equal(t, tc.expectedStatus, status)
		})
	}
}

func TestMapGitHubToAppStatus_ByState(t *testing.T) {
	// Test closed state without status label
	issue := &github.Issue{
		State:  "closed",
		Labels: []github.Label{},
	}
	status := mapGitHubToAppStatus(issue)
	assert.Equal(t, models.RequestStatusCompleted, status)

	// Test open state without status label
	issue2 := &github.Issue{
		State:  "open",
		Labels: []github.Label{},
	}
	status2 := mapGitHubToAppStatus(issue2)
	assert.Equal(t, models.RequestStatusOpen, status2)
}

func TestMapGitHubToAppStatus_LabelTakesPrecedence(t *testing.T) {
	// Even if state is closed, label should take precedence
	issue := &github.Issue{
		State: "closed",
		Labels: []github.Label{
			{Name: "status:in-progress"},
		},
	}
	status := mapGitHubToAppStatus(issue)
	assert.Equal(t, models.RequestStatusInProgress, status)
}

func TestGetStatusLabels(t *testing.T) {
	labels := getStatusLabels()
	assert.Len(t, labels, 4)
	assert.Contains(t, labels, "status:open")
	assert.Contains(t, labels, "status:in-progress")
	assert.Contains(t, labels, "status:completed")
	assert.Contains(t, labels, "status:closed")
}

func TestIssueWebhookPayload_Unmarshal(t *testing.T) {
	jsonData := `{
		"action": "opened",
		"issue": {
			"number": 42,
			"title": "Test Issue",
			"body": "Issue body",
			"state": "open",
			"html_url": "https://github.com/owner/repo/issues/42",
			"labels": [{"name": "pack-request"}, {"name": "status:open"}]
		}
	}`

	var payload IssueWebhookPayload
	err := json.Unmarshal([]byte(jsonData), &payload)
	assert.Nil(t, err)
	assert.Equal(t, "opened", payload.Action)
	assert.Equal(t, 42, payload.Issue.Number)
	assert.Equal(t, "Test Issue", payload.Issue.Title)
	assert.Equal(t, "open", payload.Issue.State)
	assert.Len(t, payload.Issue.Labels, 2)
}

func TestSyncConfig(t *testing.T) {
	cfg := SyncConfig{
		Token: "test-token",
		Owner: "test-owner",
		Repo:  "test-repo",
	}

	assert.Equal(t, "test-token", cfg.Token)
	assert.Equal(t, "test-owner", cfg.Owner)
	assert.Equal(t, "test-repo", cfg.Repo)
}

func TestSyncResult(t *testing.T) {
	result := SyncResult{
		CreatedRequests: 5,
		UpdatedRequests: 3,
		UpdatedIssues:   2,
		Errors:          []string{"error1", "error2"},
	}

	assert.Equal(t, 5, result.CreatedRequests)
	assert.Equal(t, 3, result.UpdatedRequests)
	assert.Equal(t, 2, result.UpdatedIssues)
	assert.Len(t, result.Errors, 2)
}

func TestHandleGitHubIssueWebhook_NoSecret(t *testing.T) {
	// Clear the webhook secret
	t.Setenv("GITHUB_WEBHOOK_SECRET", "")

	app := fiber.New()
	app.Post("/webhooks/github/issues", HandleGitHubIssueWebhook)

	req := httptest.NewRequest("POST", "/webhooks/github/issues", nil)
	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 500, resp.StatusCode)
}

func TestHandleGitHubIssueWebhook_InvalidSignature(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "test-secret")

	app := fiber.New()
	app.Post("/webhooks/github/issues", HandleGitHubIssueWebhook)

	payload := `{"action":"opened"}`
	req := httptest.NewRequest("POST", "/webhooks/github/issues", strings.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalidsignature")
	req.Header.Set("X-GitHub-Event", "issues")

	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestHandleGitHubIssueWebhook_NonIssueEvent(t *testing.T) {
	secret := "test-secret"
	t.Setenv("GITHUB_WEBHOOK_SECRET", secret)

	app := fiber.New()
	app.Post("/webhooks/github/issues", HandleGitHubIssueWebhook)

	payload := []byte(`{"action":"push"}`)

	// Generate valid signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhooks/github/issues", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Event", "push") // Not an issues event

	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode) // Should return 200 but ignore
}

func TestHandleGitHubIssueWebhook_ValidIssueOpened(t *testing.T) {
	secret := "test-secret"
	t.Setenv("GITHUB_WEBHOOK_SECRET", secret)

	app := fiber.New()
	app.Post("/webhooks/github/issues", HandleGitHubIssueWebhook)

	payload := IssueWebhookPayload{
		Action: "opened",
		Issue: IssuePayload{
			Number:  123,
			Title:   "Test Issue",
			Body:    "Test body",
			State:   "open",
			HTMLURL: "https://github.com/owner/repo/issues/123",
			Labels:  []LabelPayload{{Name: "pack-request"}},
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	// Generate valid signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payloadBytes)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhooks/github/issues", strings.NewReader(string(payloadBytes)))
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Event", "issues")

	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestHandleGitHubIssueWebhook_InvalidPayload(t *testing.T) {
	secret := "test-secret"
	t.Setenv("GITHUB_WEBHOOK_SECRET", secret)

	app := fiber.New()
	app.Post("/webhooks/github/issues", HandleGitHubIssueWebhook)

	payload := []byte(`invalid json`)

	// Generate valid signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhooks/github/issues", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Event", "issues")

	resp, err := app.Test(req)

	assert.Nil(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestHandleNewIssue_SkipsAppCreated(t *testing.T) {
	issue := IssuePayload{
		Number:  999,
		Title:   "Test Issue",
		Body:    "This was automatically created from [RMBL](https://rmbl.dev)",
		State:   "open",
		HTMLURL: "https://github.com/owner/repo/issues/999",
		Labels:  []LabelPayload{{Name: "pack-request"}},
	}

	err := handleNewIssue(issue)
	assert.Nil(t, err)

	// Verify no request was created
	var count int64
	database.DB.Model(&models.PackRequest{}).Where("github_issue_num = ?", 999).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestHandleNewIssue_SkipsNonRequestIssue(t *testing.T) {
	issue := IssuePayload{
		Number:  998,
		Title:   "Regular Issue",
		Body:    "This is a regular issue",
		State:   "open",
		HTMLURL: "https://github.com/owner/repo/issues/998",
		Labels:  []LabelPayload{{Name: "bug"}}, // No pack-request or job-request label
	}

	err := handleNewIssue(issue)
	assert.Nil(t, err)

	// Verify no request was created
	var count int64
	database.DB.Model(&models.PackRequest{}).Where("github_issue_num = ?", 998).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestHandleIssueStateChange_NotTracked(t *testing.T) {
	issue := IssuePayload{
		Number: 12345, // Non-existent issue
		State:  "closed",
		Labels: []LabelPayload{{Name: "status:completed"}},
	}

	err := handleIssueStateChange(issue)
	assert.Nil(t, err) // Should not error for untracked issues
}

func TestHandleIssueStateChange_TrackedRequest(t *testing.T) {
	// Use unique identifiers to avoid conflicts
	uniqueID := fmt.Sprintf("%d", time.Now().UnixNano())
	issueNum := int(time.Now().UnixNano() % 1000000) // Unique issue number

	// Create a user for the request
	user := models.User{Username: "syncuser" + uniqueID, Email: "syncuser" + uniqueID + "@test.com"}
	result := database.DB.Create(&user)
	if result.Error != nil {
		t.Fatalf("Failed to create user: %v", result.Error)
	}
	defer database.DB.Unscoped().Delete(&user)

	// Create a request linked to a GitHub issue
	request := models.PackRequest{
		Title:          "Synced Request " + uniqueID,
		UserID:         user.ID,
		Status:         models.RequestStatusOpen,
		GitHubIssueNum: issueNum,
		GitHubIssueURL: fmt.Sprintf("https://github.com/owner/repo/issues/%d", issueNum),
	}
	result = database.DB.Create(&request)
	if result.Error != nil {
		t.Fatalf("Failed to create request: %v", result.Error)
	}
	defer database.DB.Unscoped().Delete(&request)

	// Simulate state change
	issue := IssuePayload{
		Number: issueNum,
		State:  "closed",
		Labels: []LabelPayload{{Name: "status:completed"}},
	}

	err := handleIssueStateChange(issue)
	assert.Nil(t, err)

	// Verify status was updated
	var updated models.PackRequest
	database.DB.First(&updated, request.ID)
	assert.Equal(t, models.RequestStatusCompleted, updated.Status)
}
