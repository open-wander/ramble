package handlers

import (
	"fmt"
	"log"
	"os"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/github"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// GetRequests lists pack/job requests with filtering and sorting
func GetRequests(c *fiber.Ctx) error {
	// Validate status parameter
	status := c.Query("status", "open")
	validStatuses := map[string]bool{
		"all": true, "open": true, "in_progress": true,
		"completed": true, "closed": true,
	}
	if !validStatuses[status] {
		status = "open"
	}

	// Validate type parameter
	reqType := c.Query("type", "")
	validTypes := map[string]bool{"": true, "pack": true, "job": true}
	if !validTypes[reqType] {
		reqType = ""
	}

	// Validate sort parameter
	sort := c.Query("sort", "votes")
	validSorts := map[string]bool{"votes": true, "newest": true, "oldest": true}
	if !validSorts[sort] {
		sort = "votes"
	}

	// Validate page parameter
	pageStr := c.Query("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 || page > 1000 {
		page = 1
	}
	pageSize := 12

	var requests []models.PackRequest
	dbQuery := database.DB.Model(&models.PackRequest{}).
		Preload("User").
		Preload("Voters")

	if status != "all" {
		dbQuery = dbQuery.Where("status = ?", status)
	}
	if reqType != "" {
		dbQuery = dbQuery.Where("type = ?", reqType)
	}

	switch sort {
	case "votes":
		dbQuery = dbQuery.Order("vote_count desc, created_at desc")
	case "newest":
		dbQuery = dbQuery.Order("created_at desc")
	case "oldest":
		dbQuery = dbQuery.Order("created_at asc")
	default:
		dbQuery = dbQuery.Order("vote_count desc, created_at desc")
	}

	offset := (page - 1) * pageSize
	dbQuery.Limit(pageSize).Offset(offset).Find(&requests)

	nextPage := 0
	if len(requests) == pageSize {
		nextPage = page + 1
	}

	// Check if user has voted on each request
	var votedRequestIDs []uint
	if c.Locals("UserID") != nil {
		userID := c.Locals("UserID").(uint)
		database.DB.Table("request_votes").
			Where("user_id = ?", userID).
			Pluck("pack_request_id", &votedRequestIDs)
	}

	// Convert to map for easy lookup in templates
	votedMap := make(map[uint]bool)
	for _, id := range votedRequestIDs {
		votedMap[id] = true
	}

	if c.Get("HX-Request") == "true" {
		return c.Render("partials/request_list", fiber.Map{
			"Requests":  requests,
			"VotedMap":  votedMap,
			"NextPage":  nextPage,
			"Status":    status,
			"Type":      reqType,
			"Sort":      sort,
		})
	}

	return c.Render("requests", MergeContext(BaseContext(c), fiber.Map{
		"Page":     "requests",
		"Requests": requests,
		"VotedMap": votedMap,
		"Status":   status,
		"Type":     reqType,
		"Sort":     sort,
		"NextPage": nextPage,
	}), "layouts/main")
}

// GetNewRequest shows the request submission form
func GetNewRequest(c *fiber.Ctx) error {
	return c.Render("new_request", MergeContext(BaseContext(c), fiber.Map{
		"Page": "requests",
	}), "layouts/main")
}

// PostNewRequest creates a new pack/job request
func PostNewRequest(c *fiber.Ctx) error {
	sess, _ := Store.Get(c)
	userID := sess.Get("user_id").(uint)

	type RequestInput struct {
		Title       string `form:"title"`
		Description string `form:"description"`
		Type        string `form:"type"`
	}
	var input RequestInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid input")
	}

	// Trim whitespace
	title := strings.TrimSpace(input.Title)
	description := strings.TrimSpace(input.Description)

	// Validate title
	if title == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Title is required")
	}
	if len(title) < 3 {
		return c.Status(fiber.StatusBadRequest).SendString("Title must be at least 3 characters")
	}
	if len(title) > 200 {
		return c.Status(fiber.StatusBadRequest).SendString("Title cannot exceed 200 characters")
	}

	// Validate description length
	if len(description) > 10000 {
		return c.Status(fiber.StatusBadRequest).SendString("Description cannot exceed 10,000 characters")
	}

	// Validate type
	reqType := models.ResourceTypePack
	if input.Type == "job" {
		reqType = models.ResourceTypeJob
	} else if input.Type != "" && input.Type != "pack" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid type")
	}

	request := models.PackRequest{
		Title:       title,
		Description: description,
		Type:        reqType,
		UserID:      userID,
		Status:      models.RequestStatusOpen,
	}

	if err := database.DB.Create(&request).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Could not create request")
	}

	AuditLog(c, "request.create", "request", request.ID, request.Title, nil)

	// Create GitHub issue in background
	go createGitHubIssueForRequest(request.ID)

	SetFlash(c, "success", "Request submitted successfully!")
	c.Set("HX-Redirect", "/requests/"+strconv.Itoa(int(request.ID)))
	return c.SendStatus(fiber.StatusOK)
}

// createGitHubIssueForRequest creates a GitHub issue for the request
func createGitHubIssueForRequest(requestID uint) {
	// Get token from environment variable (like other GitHub/GitLab secrets)
	token := os.Getenv("GITHUB_REQUESTS_TOKEN")
	if token == "" {
		log.Printf("GITHUB_REQUESTS_TOKEN not configured")
		return
	}

	var request models.PackRequest
	if err := database.DB.Preload("User").First(&request, requestID).Error; err != nil {
		log.Printf("Failed to load request %d: %v", requestID, err)
		return
	}

	// Get repo from database settings (not a secret, can be configured in admin)
	var repoSetting models.SiteSetting
	if err := database.DB.Where("key = ?", "github_requests_repo").First(&repoSetting).Error; err != nil {
		log.Printf("GitHub requests repo not configured in admin settings")
		return
	}

	if repoSetting.Value == "" {
		return
	}

	parts := strings.Split(repoSetting.Value, "/")
	if len(parts) != 2 {
		log.Printf("Invalid repo format: %s (expected owner/repo)", repoSetting.Value)
		return
	}

	typeLabel := "pack-request"
	if request.Type == models.ResourceTypeJob {
		typeLabel = "job-request"
	}

	issue := github.IssueRequest{
		Title: fmt.Sprintf("[%s Request] %s", strings.ToUpper(string(request.Type)), request.Title),
		Body: fmt.Sprintf("**Submitted by:** %s\n\n%s\n\n---\n*This request was automatically created from [RMBL](https://rmbl.dev)*",
			request.User.Username, request.Description),
		Labels: []string{typeLabel},
	}

	resp, err := github.CreateIssue(token, parts[0], parts[1], issue)
	if err != nil {
		log.Printf("Failed to create GitHub issue: %v", err)
		return
	}

	database.DB.Model(&request).Updates(map[string]interface{}{
		"github_issue_url": resp.HTMLURL,
		"github_issue_num": resp.Number,
	})
}

// GetRequest shows a single request detail
func GetRequest(c *fiber.Ctx) error {
	id := c.Params("id")

	var request models.PackRequest
	if err := database.DB.Preload("User").Preload("Voters").First(&request, id).Error; err != nil {
		return c.Status(404).SendString("Request not found")
	}

	hasVoted := false
	if c.Locals("UserID") != nil {
		userID := c.Locals("UserID").(uint)
		for _, voter := range request.Voters {
			if voter.ID == userID {
				hasVoted = true
				break
			}
		}
	}

	return c.Render("request_detail", MergeContext(BaseContext(c), fiber.Map{
		"Page":     "requests",
		"Request":  request,
		"HasVoted": hasVoted,
	}), "layouts/main")
}

// ToggleRequestVote toggles the current user's vote on a request
func ToggleRequestVote(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, _ := Store.Get(c)
	userID := sess.Get("user_id").(uint)

	var request models.PackRequest
	if err := database.DB.Preload("Voters").First(&request, id).Error; err != nil {
		return c.Status(404).SendString("Request not found")
	}

	hasVoted := false
	for _, voter := range request.Voters {
		if voter.ID == userID {
			hasVoted = true
			break
		}
	}

	if hasVoted {
		database.DB.Model(&request).Association("Voters").Delete(&models.User{Model: gorm.Model{ID: userID}})
		database.DB.Model(&request).Update("vote_count", gorm.Expr("vote_count - ?", 1))
	} else {
		database.DB.Model(&request).Association("Voters").Append(&models.User{Model: gorm.Model{ID: userID}})
		database.DB.Model(&request).Update("vote_count", gorm.Expr("vote_count + ?", 1))
	}

	// Refetch count
	var count int64
	database.DB.Table("request_votes").Where("pack_request_id = ?", request.ID).Count(&count)

	return c.Render("partials/vote_button", fiber.Map{
		"Request":   request,
		"HasVoted":  !hasVoted,
		"VoteCount": count,
	})
}

// GetEditRequest shows the edit form for a request (owner only, open status only)
func GetEditRequest(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, _ := Store.Get(c)
	userID := sess.Get("user_id").(uint)

	var request models.PackRequest
	if err := database.DB.First(&request, id).Error; err != nil {
		return c.Status(404).SendString("Request not found")
	}

	// Check ownership
	if request.UserID != userID {
		return c.Status(403).SendString("You can only edit your own requests")
	}

	// Check status
	if request.Status != models.RequestStatusOpen {
		return c.Status(400).SendString("You can only edit requests that are still open")
	}

	return c.Render("edit_request", MergeContext(BaseContext(c), fiber.Map{
		"Page":    "requests",
		"Request": request,
	}), "layouts/main")
}

// PostEditRequest updates a request (owner only, open status only)
func PostEditRequest(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, _ := Store.Get(c)
	userID := sess.Get("user_id").(uint)

	var request models.PackRequest
	if err := database.DB.First(&request, id).Error; err != nil {
		return c.Status(404).SendString("Request not found")
	}

	// Check ownership
	if request.UserID != userID {
		return c.Status(403).SendString("You can only edit your own requests")
	}

	// Check status
	if request.Status != models.RequestStatusOpen {
		return c.Status(400).SendString("You can only edit requests that are still open")
	}

	type EditInput struct {
		Title       string `form:"title"`
		Description string `form:"description"`
		Type        string `form:"type"`
	}
	var input EditInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid input")
	}

	// Validate title
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Title is required")
	}
	if len(title) < 3 {
		return c.Status(fiber.StatusBadRequest).SendString("Title must be at least 3 characters")
	}
	if len(title) > 200 {
		return c.Status(fiber.StatusBadRequest).SendString("Title cannot exceed 200 characters")
	}

	// Validate description
	description := strings.TrimSpace(input.Description)
	if len(description) > 10000 {
		return c.Status(fiber.StatusBadRequest).SendString("Description cannot exceed 10,000 characters")
	}

	// Validate type
	reqType := models.ResourceTypePack
	if input.Type == "job" {
		reqType = models.ResourceTypeJob
	} else if input.Type != "" && input.Type != "pack" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid type")
	}

	request.Title = title
	request.Description = description
	request.Type = reqType

	if err := database.DB.Save(&request).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Could not update request")
	}

	AuditLog(c, "request.edit", "request", request.ID, request.Title, nil)

	SetFlash(c, "success", "Request updated successfully!")
	c.Set("HX-Redirect", "/requests/"+strconv.Itoa(int(request.ID)))
	return c.SendStatus(fiber.StatusOK)
}

// DeleteUserRequest deletes a request (owner only, open status only)
func DeleteUserRequest(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, _ := Store.Get(c)
	userID := sess.Get("user_id").(uint)

	var request models.PackRequest
	if err := database.DB.First(&request, id).Error; err != nil {
		return c.Status(404).SendString("Request not found")
	}

	// Check ownership
	if request.UserID != userID {
		return c.Status(403).SendString("You can only delete your own requests")
	}

	// Check status
	if request.Status != models.RequestStatusOpen {
		return c.Status(400).SendString("You can only delete requests that are still open")
	}

	AuditLog(c, "request.delete", "request", request.ID, request.Title, nil)

	database.DB.Delete(&request)

	SetFlash(c, "success", "Request deleted successfully.")
	c.Set("HX-Redirect", "/requests")
	return c.SendStatus(fiber.StatusOK)
}
