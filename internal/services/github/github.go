package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

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
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", owner, repo)

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

	client := &http.Client{}
	resp, err := client.Do(req)
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
