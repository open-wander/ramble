package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/github"
	"rmbl/internal/services/logger"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/sync/errgroup"
)

// SyncConfig holds configuration for GitHub sync
type SyncConfig struct {
	Token string
	Owner string
	Repo  string
}

// SyncResult holds the results of a bidirectional sync
type SyncResult struct {
	CreatedRequests int      `json:"created_requests"`
	UpdatedRequests int      `json:"updated_requests"`
	UpdatedIssues   int      `json:"updated_issues"`
	Errors          []string `json:"errors"`
	SyncedAt        time.Time
}

// IssueWebhookPayload represents the GitHub webhook payload for issue events
type IssueWebhookPayload struct {
	Action string       `json:"action"`
	Issue  IssuePayload `json:"issue"`
}

// IssuePayload represents an issue in the webhook payload
type IssuePayload struct {
	Number  int            `json:"number"`
	Title   string         `json:"title"`
	Body    string         `json:"body"`
	State   string         `json:"state"`
	HTMLURL string         `json:"html_url"`
	Labels  []LabelPayload `json:"labels"`
}

// LabelPayload represents a label in the webhook payload
type LabelPayload struct {
	Name string `json:"name"`
}

// getGitHubSyncConfig retrieves sync configuration from env and DB
func getGitHubSyncConfig() (*SyncConfig, error) {
	token := os.Getenv("GITHUB_REQUESTS_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_REQUESTS_TOKEN not configured")
	}

	var repoSetting models.SiteSetting
	if err := database.DB.Where("key = ?", "github_requests_repo").First(&repoSetting).Error; err != nil {
		return nil, fmt.Errorf("github_requests_repo not configured in admin settings")
	}

	if repoSetting.Value == "" {
		return nil, fmt.Errorf("github_requests_repo is empty")
	}

	parts := strings.Split(repoSetting.Value, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s (expected owner/repo)", repoSetting.Value)
	}

	return &SyncConfig{
		Token: token,
		Owner: parts[0],
		Repo:  parts[1],
	}, nil
}

// mapAppStatusToGitHub converts app status to GitHub state and labels
func mapAppStatusToGitHub(status models.RequestStatus) (state string, label string) {
	switch status {
	case models.RequestStatusOpen:
		return "open", "status:open"
	case models.RequestStatusInProgress:
		return "open", "status:in-progress"
	case models.RequestStatusCompleted:
		return "closed", "status:completed"
	case models.RequestStatusClosed:
		return "closed", "status:closed"
	default:
		return "open", "status:open"
	}
}

// mapGitHubToAppStatus converts GitHub state and labels to app status
func mapGitHubToAppStatus(issue *github.Issue) models.RequestStatus {
	// Check labels first for granular status
	for _, label := range issue.Labels {
		switch label.Name {
		case "status:in-progress":
			return models.RequestStatusInProgress
		case "status:completed":
			return models.RequestStatusCompleted
		case "status:closed":
			return models.RequestStatusClosed
		case "status:open":
			return models.RequestStatusOpen
		}
	}
	// Fall back to issue state
	if issue.State == "closed" {
		return models.RequestStatusCompleted
	}
	return models.RequestStatusOpen
}

// getStatusLabels returns all status labels to manage
func getStatusLabels() []string {
	return []string{"status:open", "status:in-progress", "status:completed", "status:closed"}
}

// SyncRequestToGitHub syncs a single request's status to GitHub
func SyncRequestToGitHub(request models.PackRequest) error {
	if request.GitHubIssueNum == 0 {
		return nil // No GitHub issue linked
	}

	cfg, err := getGitHubSyncConfig()
	if err != nil {
		return err
	}

	state, newLabel := mapAppStatusToGitHub(request.Status)

	// Build labels: keep non-status labels, add new status label
	issue, err := github.GetIssue(cfg.Token, cfg.Owner, cfg.Repo, request.GitHubIssueNum)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}

	// Preserve non-status labels and add new status label
	var labels []string
	statusLabels := getStatusLabels()
	for _, label := range issue.Labels {
		isStatusLabel := false
		for _, sl := range statusLabels {
			if label.Name == sl {
				isStatusLabel = true
				break
			}
		}
		if !isStatusLabel {
			labels = append(labels, label.Name)
		}
	}
	labels = append(labels, newLabel)

	update := github.IssueUpdate{
		State:  &state,
		Labels: labels,
	}

	if err := github.UpdateIssue(cfg.Token, cfg.Owner, cfg.Repo, request.GitHubIssueNum, update); err != nil {
		return fmt.Errorf("failed to update issue: %w", err)
	}

	return nil
}

// syncRequestToGitHubWithRetry wraps SyncRequestToGitHub with exponential backoff retry
func syncRequestToGitHubWithRetry(request models.PackRequest) error {
	log := logger.Log.With().
		Str("operation", "sync_request_to_github").
		Uint("request_id", request.ID).
		Int("issue_num", request.GitHubIssueNum).
		Logger()

	var attempt int
	operation := func() error {
		attempt++
		log.Debug().Int("attempt", attempt).Msg("sync attempt")
		return SyncRequestToGitHub(request)
	}

	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 1 * time.Second
	b.Multiplier = 5.0
	b.MaxInterval = 15 * time.Second
	b.MaxElapsedTime = 30 * time.Second
	b.RandomizationFactor = 0.5

	if err := backoff.Retry(operation, backoff.WithMaxRetries(b, 3)); err != nil {
		log.Error().Err(err).Int("attempts", attempt).Msg("sync failed after retries")
		return err
	}

	if attempt > 1 {
		log.Info().Int("attempts", attempt).Msg("sync succeeded after retries")
	}
	return nil
}

// SyncGitHubToRequest syncs GitHub issue state to a request
func SyncGitHubToRequest(issueNum int) error {
	cfg, err := getGitHubSyncConfig()
	if err != nil {
		return err
	}

	issue, err := github.GetIssue(cfg.Token, cfg.Owner, cfg.Repo, issueNum)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}

	var request models.PackRequest
	if err := database.DB.Where("github_issue_num = ?", issueNum).First(&request).Error; err != nil {
		// Issue exists on GitHub but not in DB - check if it's app-created
		if strings.Contains(issue.Body, "automatically created from [RMBL]") {
			// Skip - this shouldn't happen normally
			return nil
		}
		// Could create a new request from external issue here if desired
		return nil
	}

	newStatus := mapGitHubToAppStatus(issue)
	if request.Status != newStatus {
		request.Status = newStatus
		database.DB.Save(&request)
	}

	return nil
}

// runBidirectionalSync performs full bidirectional sync
func runBidirectionalSync() (*SyncResult, error) {
	log := logger.Log.With().Str("operation", "github_sync").Logger()

	result := &SyncResult{
		SyncedAt: time.Now(),
	}

	log.Info().Msg("starting bidirectional sync")

	cfg, err := getGitHubSyncConfig()
	if err != nil {
		return nil, err
	}

	// Fetch all issues from GitHub
	issues, err := github.ListIssues(cfg.Token, cfg.Owner, cfg.Repo, "all")
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	log.Info().Int("issue_count", len(issues)).Msg("fetched issues from GitHub")

	// Build a map of issue numbers to issues
	issueMap := make(map[int]*github.Issue)
	for i := range issues {
		issueMap[issues[i].Number] = &issues[i]
	}

	// Get all requests with GitHub issues linked
	var requests []models.PackRequest
	database.DB.Where("github_issue_num > 0").Find(&requests)

	log.Info().Int("request_count", len(requests)).Msg("loaded linked requests")

	// Sync app -> GitHub: update issues based on request status
	g := new(errgroup.Group)
	g.SetLimit(5) // Limit concurrent API calls
	var syncErrors []string
	var mu sync.Mutex
	var updatedCount atomic.Int32

	for _, request := range requests {
		request := request
		issue, exists := issueMap[request.GitHubIssueNum]
		if !exists {
			mu.Lock()
			syncErrors = append(syncErrors, fmt.Sprintf("Issue #%d not found on GitHub", request.GitHubIssueNum))
			mu.Unlock()
			delete(issueMap, request.GitHubIssueNum)
			continue
		}

		// Check if sync is needed
		currentGHStatus := mapGitHubToAppStatus(issue)
		if currentGHStatus != request.Status {
			g.Go(func() error {
				if err := syncRequestToGitHubWithRetry(request); err != nil {
					mu.Lock()
					syncErrors = append(syncErrors, fmt.Sprintf("Failed to sync request %d to GitHub: %v", request.ID, err))
					mu.Unlock()
					log.Warn().Err(err).Uint("request_id", request.ID).Msg("sync request to GitHub failed")
					return nil // Don't fail entire sync
				}
				updatedCount.Add(1)
				return nil
			})
		}

		// Remove from map so we can identify unlinked issues
		delete(issueMap, request.GitHubIssueNum)
	}

	if err := g.Wait(); err != nil {
		syncErrors = append(syncErrors, err.Error())
	}
	result.UpdatedIssues = int(updatedCount.Load())
	result.Errors = syncErrors

	// Sync GitHub -> app: create requests for unlinked issues
	for _, issue := range issueMap {
		// Check if it's a pack/job request issue (has the label)
		isPackRequest := false
		isJobRequest := false
		for _, label := range issue.Labels {
			if label.Name == "pack-request" {
				isPackRequest = true
			}
			if label.Name == "job-request" {
				isJobRequest = true
			}
		}

		if !isPackRequest && !isJobRequest {
			continue // Not a request issue
		}

		// Check if already exists by issue number
		var existing models.PackRequest
		if err := database.DB.Where("github_issue_num = ?", issue.Number).First(&existing).Error; err == nil {
			continue // Already exists
		}

		// Create new request from GitHub issue
		reqType := models.ResourceTypePack
		if isJobRequest {
			reqType = models.ResourceTypeJob
		}

		// Parse title - remove prefix if present
		title := issue.Title
		title = strings.TrimPrefix(title, "[PACK Request] ")
		title = strings.TrimPrefix(title, "[JOB Request] ")

		// Get body without the footer
		body := issue.Body
		if idx := strings.LastIndex(body, "---\n*This request was automatically created from"); idx > 0 {
			body = strings.TrimSpace(body[:idx])
		}
		// Remove "**Submitted by:** username\n\n" prefix if present
		if strings.HasPrefix(body, "**Submitted by:**") {
			if idx := strings.Index(body, "\n\n"); idx > 0 {
				body = strings.TrimSpace(body[idx+2:])
			}
		}

		// Find an admin user to own the imported request
		var adminUser models.User
		if err := database.DB.Where("is_admin = ?", true).First(&adminUser).Error; err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to create request from issue #%d: no admin user found", issue.Number))
			continue
		}

		newRequest := models.PackRequest{
			Title:          title,
			Description:    body,
			Type:           reqType,
			Status:         mapGitHubToAppStatus(issue),
			UserID:         adminUser.ID,
			GitHubIssueURL: issue.HTMLURL,
			GitHubIssueNum: issue.Number,
		}

		if err := database.DB.Create(&newRequest).Error; err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to create request from issue #%d: %v", issue.Number, err))
		} else {
			result.CreatedRequests++
		}
	}

	log.Info().
		Int("created", result.CreatedRequests).
		Int("updated_issues", result.UpdatedIssues).
		Int("errors", len(result.Errors)).
		Dur("duration", time.Since(result.SyncedAt)).
		Msg("sync completed")

	return result, nil
}

// PostSyncGitHubRequests performs a manual bidirectional sync
func PostSyncGitHubRequests(c *fiber.Ctx) error {
	result, err := runBidirectionalSync()
	if err != nil {
		return c.Render("partials/sync_result", fiber.Map{
			"Error": err.Error(),
		})
	}

	AuditLog(c, "admin.sync_github_requests", "system", 0, "", map[string]interface{}{
		"created": result.CreatedRequests,
		"updated": result.UpdatedRequests,
		"synced":  result.UpdatedIssues,
		"errors":  len(result.Errors),
	})

	return c.Render("partials/sync_result", fiber.Map{
		"Result": result,
	})
}

// HandleGitHubIssueWebhook godoc
// @Summary Receive GitHub issue webhooks
// @Description Webhook endpoint for GitHub to send issue events. Used to sync community pack/job requests with GitHub issues. Requires valid X-Hub-Signature-256 header.
// @Tags webhooks
// @Accept json
// @Param X-Hub-Signature-256 header string true "GitHub webhook signature"
// @Param X-GitHub-Event header string true "GitHub event type"
// @Success 200 {string} string "OK"
// @Failure 401 {string} string "Invalid signature"
// @Failure 500 {string} string "Webhook secret not configured"
// @Router /webhooks/github/issues [post]
func HandleGitHubIssueWebhook(c *fiber.Ctx) error {
	// Validate signature
	secret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if secret == "" {
		logger.Log.Error().Msg("GITHUB_WEBHOOK_SECRET not configured")
		return c.SendStatus(500)
	}

	signature := c.Get("X-Hub-Signature-256")
	timestamp := c.Get("X-Webhook-Timestamp")

	var isValid bool
	if timestamp != "" {
		// New format: timestamp + signature with replay protection
		valid, _, reason := ValidateWebhookRequest(c.Body(), signature, timestamp, "", secret)
		if !valid {
			logger.Log.Warn().Str("operation", "github_issue_webhook").Str("reason", reason).Msg("webhook validation failed")
			return c.Status(401).SendString("Invalid webhook signature")
		}
		isValid = valid
	} else if signature != "" {
		// GitHub format: signature only (GitHub doesn't send X-Webhook-Timestamp yet)
		// This maintains backward compatibility with GitHub's current webhook format
		isValid = github.ValidateWebhookSignature(c.Body(), signature, secret)
		if !isValid {
			logger.Log.Warn().Str("operation", "github_issue_webhook").Msg("signature validation failed")
			return c.Status(401).SendString("Invalid webhook signature")
		}
	} else {
		logger.Log.Warn().Str("operation", "github_issue_webhook").Msg("missing signature header")
		return c.Status(401).SendString("Invalid webhook signature")
	}

	// Check event type
	event := c.Get("X-GitHub-Event")
	if event != "issues" {
		return c.SendStatus(200) // Ignore non-issue events
	}

	// Parse payload
	var payload IssueWebhookPayload
	if err := json.Unmarshal(c.Body(), &payload); err != nil {
		logger.Log.Error().Err(err).Str("operation", "github_issue_webhook").Msg("failed to parse payload")
		return c.Status(400).SendString("Invalid payload")
	}

	// Handle based on action
	switch payload.Action {
	case "opened":
		// Check if this is an externally created issue we should track
		if err := handleNewIssue(payload.Issue); err != nil {
			logger.Log.Error().Err(err).Str("operation", "github_issue_webhook").Int("issue_number", payload.Issue.Number).Msg("failed to handle new issue")
		}
	case "closed", "reopened", "labeled", "unlabeled":
		// Update existing request status
		if err := handleIssueStateChange(payload.Issue); err != nil {
			logger.Log.Error().Err(err).Str("operation", "github_issue_webhook").Int("issue_number", payload.Issue.Number).Msg("failed to handle issue state change")
		}
	}

	return c.SendStatus(200)
}

// handleNewIssue processes a newly opened issue
func handleNewIssue(issue IssuePayload) error {
	// Skip if it was created by our app
	if strings.Contains(issue.Body, "automatically created from [RMBL]") {
		return nil
	}

	// Check if it has pack/job request labels
	isPackRequest := false
	isJobRequest := false
	for _, label := range issue.Labels {
		if label.Name == "pack-request" {
			isPackRequest = true
		}
		if label.Name == "job-request" {
			isJobRequest = true
		}
	}

	if !isPackRequest && !isJobRequest {
		return nil // Not a request issue
	}

	// Check if already exists
	var existing models.PackRequest
	if err := database.DB.Where("github_issue_num = ?", issue.Number).First(&existing).Error; err == nil {
		return nil // Already tracked
	}

	reqType := models.ResourceTypePack
	if isJobRequest {
		reqType = models.ResourceTypeJob
	}

	// Find an admin user to own the imported request
	var adminUser models.User
	if err := database.DB.Where("is_admin = ?", true).First(&adminUser).Error; err != nil {
		return fmt.Errorf("no admin user found to own imported request")
	}

	newRequest := models.PackRequest{
		Title:          issue.Title,
		Description:    issue.Body,
		Type:           reqType,
		Status:         models.RequestStatusOpen,
		UserID:         adminUser.ID,
		GitHubIssueURL: issue.HTMLURL,
		GitHubIssueNum: issue.Number,
	}

	if err := database.DB.Create(&newRequest).Error; err != nil {
		return err
	}

	logger.Log.Info().
		Str("operation", "github_issue_webhook").
		Int("issue_number", issue.Number).
		Str("type", string(reqType)).
		Msg("created request from external issue")

	return nil
}

// handleIssueStateChange processes issue state/label changes
func handleIssueStateChange(issue IssuePayload) error {
	var request models.PackRequest
	if err := database.DB.Where("github_issue_num = ?", issue.Number).First(&request).Error; err != nil {
		return nil // Not tracking this issue
	}

	// Convert payload to github.Issue for mapping
	ghIssue := &github.Issue{
		Number: issue.Number,
		State:  issue.State,
		Labels: make([]github.Label, len(issue.Labels)),
	}
	for i, l := range issue.Labels {
		ghIssue.Labels[i] = github.Label{Name: l.Name}
	}

	newStatus := mapGitHubToAppStatus(ghIssue)
	if request.Status != newStatus {
		request.Status = newStatus
		database.DB.Save(&request)

		logger.Log.Info().
			Str("operation", "github_issue_webhook").
			Int("issue_number", issue.Number).
			Str("new_status", string(newStatus)).
			Msg("updated request status from webhook")
	}

	return nil
}
