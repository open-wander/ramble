package github

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DefaultBaseURL is the default GitHub API base URL
const DefaultBaseURL = "https://api.github.com"

// Client is a GitHub API client
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a new GitHub API client with the default base URL
func NewClient() *Client {
	return &Client{
		BaseURL:    DefaultBaseURL,
		HTTPClient: &http.Client{},
	}
}

// defaultClient is used by package-level functions
var defaultClient = NewClient()

// IssueRequest represents a GitHub issue creation request
type IssueRequest struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels,omitempty"`
}

// IssueResponse represents the GitHub API response for issue creation
type IssueResponse struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
}

// CreateIssue creates a GitHub issue in the specified repository
func CreateIssue(token, owner, repo string, issue IssueRequest) (*IssueResponse, error) {
	return defaultClient.CreateIssue(token, owner, repo, issue)
}

// CreateIssue creates a GitHub issue in the specified repository
func (c *Client) CreateIssue(token, owner, repo string, issue IssueRequest) (*IssueResponse, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues", c.BaseURL, owner, repo)

	body, err := json.Marshal(issue)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal issue: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "RMBL-Registry")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTPClient.Do(req) //nolint:gosec // G704 -- URL constructed from hardcoded github.com API base
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var issueResp IssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&issueResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &issueResp, nil
}

// Label represents a GitHub issue label
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Issue represents a GitHub issue with full details
type Issue struct {
	Number    int     `json:"number"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	State     string  `json:"state"` // "open" or "closed"
	HTMLURL   string  `json:"html_url"`
	Labels    []Label `json:"labels"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// IssueUpdate represents fields that can be updated on an issue
type IssueUpdate struct {
	Title  *string  `json:"title,omitempty"`
	Body   *string  `json:"body,omitempty"`
	State  *string  `json:"state,omitempty"` // "open" or "closed"
	Labels []string `json:"labels,omitempty"`
}

// ListIssues fetches issues from a repository with optional state filter
func ListIssues(token, owner, repo, state string) ([]Issue, error) {
	return defaultClient.ListIssues(token, owner, repo, state)
}

// ListIssues fetches issues from a repository with optional state filter
func (c *Client) ListIssues(token, owner, repo, state string) ([]Issue, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues?state=%s&per_page=100", c.BaseURL, owner, repo, state)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "RMBL-Registry")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTPClient.Do(req) //nolint:gosec // G704 -- URL constructed from hardcoded github.com API base
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var issues []Issue
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return issues, nil
}

// GetIssue fetches a single issue by number
func GetIssue(token, owner, repo string, issueNum int) (*Issue, error) {
	return defaultClient.GetIssue(token, owner, repo, issueNum)
}

// GetIssue fetches a single issue by number
func (c *Client) GetIssue(token, owner, repo string, issueNum int) (*Issue, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d", c.BaseURL, owner, repo, issueNum)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "RMBL-Registry")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTPClient.Do(req) //nolint:gosec // G704 -- URL constructed from hardcoded github.com API base
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var issue Issue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &issue, nil
}

// UpdateIssue updates an issue's title, body, state, and/or labels
func UpdateIssue(token, owner, repo string, issueNum int, update IssueUpdate) error {
	return defaultClient.UpdateIssue(token, owner, repo, issueNum, update)
}

// UpdateIssue updates an issue's title, body, state, and/or labels
func (c *Client) UpdateIssue(token, owner, repo string, issueNum int, update IssueUpdate) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d", c.BaseURL, owner, repo, issueNum)

	body, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("failed to marshal update: %w", err)
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "RMBL-Registry")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTPClient.Do(req) //nolint:gosec // G704 -- URL constructed from hardcoded github.com API base
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	return nil
}

// ValidateWebhookSignature validates the HMAC-SHA256 signature from GitHub webhooks
func ValidateWebhookSignature(payload []byte, signature, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(signature), []byte(expectedSig)) == 1
}
