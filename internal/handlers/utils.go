package handlers

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/hcl/v2/hclsimple"
)

type Flash struct {
	Type    string
	Message string
}

// wantsJSON checks if the client prefers JSON response based on Accept header
func wantsJSON(c *fiber.Ctx) bool {
	accept := c.Get("Accept")
	// Check for explicit JSON preference
	if strings.Contains(accept, "application/json") {
		return true
	}
	// If no Accept header or accepts anything, default to HTML for browsers
	return false
}

// escapeLikeString escapes special wildcard characters in LIKE/ILIKE queries
// to prevent SQL injection via wildcard abuse
func escapeLikeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

func SetFlash(c *fiber.Ctx, flashType, message string) {
	sess, err := Store.Get(c)
	if err != nil {
		return
	}
	sess.Set("flash_type", flashType)
	sess.Set("flash_message", message)
	if err := sess.Save(); err != nil {
		fmt.Printf("Error saving session in SetFlash: %v\n", err)
	}
}

type PackMetadata struct {
	Pack struct {
		Name        string `hcl:"name"`
		Description string `hcl:"description"`
		Version     string `hcl:"version"`
	} `hcl:"pack,block"`
}

func parsePackMetadata(content string) (*PackMetadata, error) {
	var metadata PackMetadata
	err := hclsimple.Decode("metadata.hcl", []byte(content), nil, &metadata)
	if err != nil {
		return nil, err
	}
	return &metadata, nil
}

type PackVariable struct {
	Name        string `hcl:"name,label" json:"name"`
	Description string `hcl:"description,optional" json:"description"`
}

type VariablesConfig struct {
	Variables []PackVariable `hcl:"variable,block"`
}

func parsePackVariables(content string) ([]PackVariable, error) {
	var config VariablesConfig
	err := hclsimple.Decode("variables.hcl", []byte(content), nil, &config)
	if err != nil {
		return nil, err
	}
	return config.Variables, nil
}

func downloadFile(repoURL string, fileName string) (string, error) {
	if repoURL == "" || fileName == "" {
		return "", nil
	}

	repoURL = strings.TrimSuffix(repoURL, ".git")
	repoURL = strings.TrimSuffix(repoURL, "/")

	// GitHub Logic
	if strings.Contains(repoURL, "github.com") {
		baseURL := strings.Replace(repoURL, "github.com", "raw.githubusercontent.com", 1)
		for _, branch := range []string{"main", "master"} {
			tempURL := fmt.Sprintf("%s/%s/%s", baseURL, branch, fileName)
			agent := fiber.Get(tempURL)
			statusCode, body, errs := agent.Bytes()
			if len(errs) == 0 && statusCode == 200 {
				return string(body), nil
			}
		}
	}

	// GitLab Logic
	if strings.Contains(repoURL, "gitlab.com") {
		// Extract path: https://gitlab.com/owner/repo -> owner/repo
		parts := strings.Split(repoURL, "gitlab.com/")
		if len(parts) < 2 {
			return "", fmt.Errorf("invalid gitlab url")
		}
		projectPath := strings.ReplaceAll(parts[1], "/", "%2F")
		fileNameEncoded := strings.ReplaceAll(fileName, "/", "%2F")

		for _, branch := range []string{"main", "master"} {
			tempURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/repository/files/%s/raw?ref=%s", projectPath, fileNameEncoded, branch)
			agent := fiber.Get(tempURL)
			statusCode, body, errs := agent.Bytes()
			if len(errs) == 0 && statusCode == 200 {
				return string(body), nil
			}
		}
	}

	return "", fmt.Errorf("unsupported repository host or file not found")
}

type GitLabProject struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TagList     []string `json:"tag_list"`
}

type GitHubRepo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Topics      []string `json:"topics"`
	License     struct {
		SpdxID string `json:"spdx_id"`
	} `json:"license"`
}

func fetchGitHubMetadata(repoURL string, token string) (*GitHubRepo, error) {
	// Extract owner/repo from https://github.com/owner/repo
	repoURL = strings.TrimSuffix(repoURL, ".git")
	parts := strings.Split(repoURL, "github.com/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid github url")
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s", parts[1])

	agent := fiber.Get(apiURL)
	agent.Set("User-Agent", "RMBL-Registry")
	if token != "" {
		agent.Set("Authorization", "token "+token)
	}

	var repo GitHubRepo
	statusCode, _, errs := agent.Struct(&repo)
	if len(errs) > 0 || statusCode != 200 {
		return nil, fmt.Errorf("failed to fetch github metadata")
	}
	return &repo, nil
}

func fetchGitHubLatestTag(repoURL string, token string) (string, error) {
	repoURL = strings.TrimSuffix(repoURL, ".git")
	parts := strings.Split(repoURL, "github.com/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid github url")
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/tags", parts[1])

	agent := fiber.Get(apiURL)
	agent.Set("User-Agent", "RMBL-Registry")
	if token != "" {
		agent.Set("Authorization", "token "+token)
	}

	type GitHubTag struct {
		Name string `json:"name"`
	}
	var tags []GitHubTag
	statusCode, _, errs := agent.Struct(&tags)
	if len(errs) > 0 || statusCode != 200 || len(tags) == 0 {
		return "", fmt.Errorf("no tags found")
	}
	return tags[0].Name, nil
}

func fetchGitHubJobFile(repoURL string, token string) (string, error) {
	repoURL = strings.TrimSuffix(repoURL, ".git")
	parts := strings.Split(repoURL, "github.com/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid github url")
	}
	// We'll check the default branch tree
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/contents", parts[1])

	agent := fiber.Get(apiURL)
	agent.Set("User-Agent", "RMBL-Registry")
	if token != "" {
		agent.Set("Authorization", "token "+token)
	}

	type GitHubContent struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	var contents []GitHubContent
	statusCode, _, errs := agent.Struct(&contents)
	if len(errs) > 0 || statusCode != 200 {
		return "", fmt.Errorf("failed to fetch contents")
	}

	for _, item := range contents {
		if item.Type == "file" && (strings.HasSuffix(item.Name, ".nomad.hcl") || strings.HasSuffix(item.Name, ".nomad")) {
			return item.Name, nil
		}
	}
	return "", fmt.Errorf("no nomad file found")
}

func fetchGitLabMetadata(repoURL string, token string) (*GitLabProject, error) {
	parts := strings.Split(repoURL, "gitlab.com/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid gitlab url")
	}
	projectPath := strings.ReplaceAll(strings.TrimSuffix(parts[1], ".git"), "/", "%2F")
	apiURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s", projectPath)

	agent := fiber.Get(apiURL)
	if token != "" {
		agent.Set("Authorization", "Bearer "+token)
	}

	var project GitLabProject
	statusCode, _, errs := agent.Struct(&project)
	if len(errs) > 0 || statusCode != 200 {
		return nil, fmt.Errorf("failed to fetch gitlab metadata")
	}
	return &project, nil
}

func fetchGitLabLatestTag(repoURL string, token string) (string, error) {
	parts := strings.Split(repoURL, "gitlab.com/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid gitlab url")
	}
	projectPath := strings.ReplaceAll(strings.TrimSuffix(parts[1], ".git"), "/", "%2F")
	apiURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/repository/tags", projectPath)

	agent := fiber.Get(apiURL)
	if token != "" {
		agent.Set("Authorization", "Bearer "+token)
	}

	type GitLabTag struct {
		Name string `json:"name"`
	}
	var tags []GitLabTag
	statusCode, _, errs := agent.Struct(&tags)
	if len(errs) > 0 || statusCode != 200 || len(tags) == 0 {
		return "", fmt.Errorf("no tags found")
	}
	return tags[0].Name, nil
}

func fetchGitLabJobFile(repoURL string, token string) (string, error) {
	parts := strings.Split(repoURL, "gitlab.com/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid gitlab url")
	}
	projectPath := strings.ReplaceAll(strings.TrimSuffix(parts[1], ".git"), "/", "%2F")
	apiURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/repository/tree", projectPath)

	agent := fiber.Get(apiURL)
	if token != "" {
		agent.Set("Authorization", "Bearer "+token)
	}

	type GitLabTreeItem struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	var tree []GitLabTreeItem
	statusCode, _, errs := agent.Struct(&tree)
	if len(errs) > 0 || statusCode != 200 {
		return "", fmt.Errorf("failed to fetch tree")
	}

	for _, item := range tree {
		if item.Type == "blob" && (strings.HasSuffix(item.Name, ".nomad.hcl") || strings.HasSuffix(item.Name, ".nomad")) {
			return item.Name, nil
		}
	}
	return "", fmt.Errorf("no nomad file found")
}
