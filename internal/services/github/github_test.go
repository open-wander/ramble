package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient()

	if client.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %s, want %s", client.BaseURL, DefaultBaseURL)
	}
	if client.HTTPClient == nil {
		t.Error("HTTPClient is nil")
	}
}

func TestValidateWebhookSignature_Valid(t *testing.T) {
	secret := "my-webhook-secret"
	payload := []byte(`{"action":"opened","issue":{"number":1}}`)

	// Generate correct signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !ValidateWebhookSignature(payload, signature, secret) {
		t.Error("ValidateWebhookSignature() = false, want true for valid signature")
	}
}

func TestValidateWebhookSignature_Invalid(t *testing.T) {
	secret := "my-webhook-secret"
	payload := []byte(`{"action":"opened","issue":{"number":1}}`)

	// Use wrong secret to generate signature
	wrongMac := hmac.New(sha256.New, []byte("wrong-secret"))
	wrongMac.Write(payload)
	wrongSignature := "sha256=" + hex.EncodeToString(wrongMac.Sum(nil))

	if ValidateWebhookSignature(payload, wrongSignature, secret) {
		t.Error("ValidateWebhookSignature() = true, want false for invalid signature")
	}
}

func TestValidateWebhookSignature_MissingPrefix(t *testing.T) {
	secret := "my-webhook-secret"
	payload := []byte(`{"action":"opened"}`)

	// Generate signature without sha256= prefix
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	signatureNoPrefix := hex.EncodeToString(mac.Sum(nil))

	if ValidateWebhookSignature(payload, signatureNoPrefix, secret) {
		t.Error("ValidateWebhookSignature() = true, want false for signature without sha256= prefix")
	}
}

func TestValidateWebhookSignature_EmptySignature(t *testing.T) {
	secret := "my-webhook-secret"
	payload := []byte(`{"action":"opened"}`)

	if ValidateWebhookSignature(payload, "", secret) {
		t.Error("ValidateWebhookSignature() = true, want false for empty signature")
	}
}

func TestValidateWebhookSignature_EmptyPayload(t *testing.T) {
	secret := "my-webhook-secret"
	payload := []byte{}

	// Generate correct signature for empty payload
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !ValidateWebhookSignature(payload, signature, secret) {
		t.Error("ValidateWebhookSignature() = false, want true for empty payload with correct signature")
	}
}

func TestValidateWebhookSignature_TamperedPayload(t *testing.T) {
	secret := "my-webhook-secret"
	originalPayload := []byte(`{"action":"opened","issue":{"number":1}}`)
	tamperedPayload := []byte(`{"action":"opened","issue":{"number":2}}`)

	// Generate signature for original payload
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(originalPayload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Verify tampered payload fails
	if ValidateWebhookSignature(tamperedPayload, signature, secret) {
		t.Error("ValidateWebhookSignature() = true, want false for tampered payload")
	}
}

func TestClient_CreateIssue_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("Missing or invalid Authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("User-Agent") != "RMBL-Registry" {
			t.Errorf("User-Agent = %s, want RMBL-Registry", r.Header.Get("User-Agent"))
		}

		// Parse request body
		var req IssueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
		}
		if req.Title != "Test Issue" {
			t.Errorf("Title = %s, want Test Issue", req.Title)
		}

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(IssueResponse{
			Number:  42,
			HTMLURL: "https://github.com/owner/repo/issues/42",
		})
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{},
	}

	issue := IssueRequest{
		Title:  "Test Issue",
		Body:   "Issue body",
		Labels: []string{"bug"},
	}

	resp, err := client.CreateIssue("test-token", "owner", "repo", issue)
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	if resp.Number != 42 {
		t.Errorf("Number = %d, want 42", resp.Number)
	}
	if resp.HTMLURL != "https://github.com/owner/repo/issues/42" {
		t.Errorf("HTMLURL = %s, want https://github.com/owner/repo/issues/42", resp.HTMLURL)
	}
}

func TestClient_CreateIssue_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Bad credentials"}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{},
	}

	issue := IssueRequest{Title: "Test", Body: "Body"}
	_, err := client.CreateIssue("invalid-token", "owner", "repo", issue)
	if err == nil {
		t.Error("CreateIssue() should return error for 401 status")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("Error should contain status code: %v", err)
	}
}

func TestClient_ListIssues_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Method = %s, want GET", r.Method)
		}

		// Check query params
		if !strings.Contains(r.URL.RawQuery, "state=open") {
			t.Errorf("Query = %s, should contain state=open", r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]Issue{
			{
				Number:  1,
				Title:   "First Issue",
				State:   "open",
				HTMLURL: "https://github.com/owner/repo/issues/1",
				Labels:  []Label{{Name: "bug", Color: "d73a4a"}},
			},
			{
				Number:  2,
				Title:   "Second Issue",
				State:   "open",
				HTMLURL: "https://github.com/owner/repo/issues/2",
			},
		})
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{},
	}

	issues, err := client.ListIssues("test-token", "owner", "repo", "open")
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}

	if len(issues) != 2 {
		t.Errorf("len(issues) = %d, want 2", len(issues))
	}
	if issues[0].Title != "First Issue" {
		t.Errorf("issues[0].Title = %s, want First Issue", issues[0].Title)
	}
	if len(issues[0].Labels) != 1 || issues[0].Labels[0].Name != "bug" {
		t.Errorf("issues[0].Labels = %v, want [bug]", issues[0].Labels)
	}
}

func TestClient_ListIssues_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{},
	}

	_, err := client.ListIssues("test-token", "owner", "nonexistent", "open")
	if err == nil {
		t.Error("ListIssues() should return error for 404 status")
	}
}

func TestClient_GetIssue_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Method = %s, want GET", r.Method)
		}

		// Check path contains issue number
		if !strings.Contains(r.URL.Path, "/123") {
			t.Errorf("Path = %s, should contain /123", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Issue{
			Number:    123,
			Title:     "Specific Issue",
			Body:      "Issue details",
			State:     "open",
			HTMLURL:   "https://github.com/owner/repo/issues/123",
			CreatedAt: "2024-01-15T10:00:00Z",
			UpdatedAt: "2024-01-16T12:00:00Z",
		})
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{},
	}

	issue, err := client.GetIssue("test-token", "owner", "repo", 123)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}

	if issue.Number != 123 {
		t.Errorf("Number = %d, want 123", issue.Number)
	}
	if issue.Title != "Specific Issue" {
		t.Errorf("Title = %s, want Specific Issue", issue.Title)
	}
}

func TestClient_GetIssue_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Not Found"}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{},
	}

	_, err := client.GetIssue("test-token", "owner", "repo", 99999)
	if err == nil {
		t.Error("GetIssue() should return error for 404 status")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Error should contain status code: %v", err)
	}
}

func TestClient_UpdateIssue_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("Method = %s, want PATCH", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", r.Header.Get("Content-Type"))
		}

		var update IssueUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			t.Errorf("Failed to decode request: %v", err)
		}

		if update.State == nil || *update.State != "closed" {
			t.Errorf("State = %v, want 'closed'", update.State)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Issue{
			Number: 1,
			State:  "closed",
		})
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{},
	}

	state := "closed"
	update := IssueUpdate{
		State:  &state,
		Labels: []string{"wontfix"},
	}

	err := client.UpdateIssue("test-token", "owner", "repo", 1, update)
	if err != nil {
		t.Fatalf("UpdateIssue() error = %v", err)
	}
}

func TestClient_UpdateIssue_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "Forbidden"}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{},
	}

	title := "New Title"
	update := IssueUpdate{Title: &title}

	err := client.UpdateIssue("test-token", "owner", "repo", 1, update)
	if err == nil {
		t.Error("UpdateIssue() should return error for 403 status")
	}
}

// Test JSON serialization of types

func TestIssueRequest_JSON(t *testing.T) {
	req := IssueRequest{
		Title:  "Test Issue",
		Body:   "This is the body",
		Labels: []string{"bug", "priority:high"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if parsed["title"] != "Test Issue" {
		t.Errorf("title = %v, want %v", parsed["title"], "Test Issue")
	}
	if parsed["body"] != "This is the body" {
		t.Errorf("body = %v, want %v", parsed["body"], "This is the body")
	}
}

func TestIssueRequest_OmitEmptyLabels(t *testing.T) {
	req := IssueRequest{
		Title: "Test Issue",
		Body:  "Body text",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if _, exists := parsed["labels"]; exists {
		t.Error("labels should be omitted when empty")
	}
}

func TestIssueUpdate_JSON(t *testing.T) {
	title := "Updated Title"
	state := "closed"

	update := IssueUpdate{
		Title:  &title,
		State:  &state,
		Labels: []string{"wontfix"},
	}

	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if parsed["title"] != "Updated Title" {
		t.Errorf("title = %v, want %v", parsed["title"], "Updated Title")
	}
	if parsed["state"] != "closed" {
		t.Errorf("state = %v, want %v", parsed["state"], "closed")
	}
}

func TestIssueUpdate_OmitNilFields(t *testing.T) {
	state := "open"
	update := IssueUpdate{
		State: &state,
	}

	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if _, exists := parsed["title"]; exists {
		t.Error("title should be omitted when nil")
	}
	if _, exists := parsed["body"]; exists {
		t.Error("body should be omitted when nil")
	}
	if parsed["state"] != "open" {
		t.Errorf("state = %v, want %v", parsed["state"], "open")
	}
}

func TestIssueResponse_Unmarshal(t *testing.T) {
	jsonData := `{"number": 42, "html_url": "https://github.com/owner/repo/issues/42"}`

	var resp IssueResponse
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if resp.Number != 42 {
		t.Errorf("Number = %d, want 42", resp.Number)
	}
	if resp.HTMLURL != "https://github.com/owner/repo/issues/42" {
		t.Errorf("HTMLURL = %s, want https://github.com/owner/repo/issues/42", resp.HTMLURL)
	}
}

func TestIssue_Unmarshal(t *testing.T) {
	jsonData := `{
		"number": 123,
		"title": "Bug Report",
		"body": "Details here",
		"state": "open",
		"html_url": "https://github.com/owner/repo/issues/123",
		"labels": [{"name": "bug", "color": "d73a4a"}],
		"created_at": "2024-01-15T10:00:00Z",
		"updated_at": "2024-01-16T12:00:00Z"
	}`

	var issue Issue
	if err := json.Unmarshal([]byte(jsonData), &issue); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if issue.Number != 123 {
		t.Errorf("Number = %d, want 123", issue.Number)
	}
	if issue.Title != "Bug Report" {
		t.Errorf("Title = %s, want Bug Report", issue.Title)
	}
	if issue.State != "open" {
		t.Errorf("State = %s, want open", issue.State)
	}
	if len(issue.Labels) != 1 || issue.Labels[0].Name != "bug" {
		t.Errorf("Labels = %v, want [bug]", issue.Labels)
	}
}

func TestLabel_Unmarshal(t *testing.T) {
	jsonData := `{"name": "enhancement", "color": "a2eeef"}`

	var label Label
	if err := json.Unmarshal([]byte(jsonData), &label); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if label.Name != "enhancement" {
		t.Errorf("Name = %s, want enhancement", label.Name)
	}
	if label.Color != "a2eeef" {
		t.Errorf("Color = %s, want a2eeef", label.Color)
	}
}

// Test package-level wrapper functions
func TestPackageLevelCreateIssue(t *testing.T) {
	// This test verifies the package-level function exists and delegates to the client
	// We can't easily mock this without modifying the default client,
	// so this is more of a compile-time check
	_ = CreateIssue
}

func TestPackageLevelListIssues(t *testing.T) {
	_ = ListIssues
}

func TestPackageLevelGetIssue(t *testing.T) {
	_ = GetIssue
}

func TestPackageLevelUpdateIssue(t *testing.T) {
	_ = UpdateIssue
}
