package pack

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PackSummary represents a pack in list responses
type PackSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PackVersion represents a version with download URL
type PackVersion struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// PackDetail represents detailed pack information
type PackDetail struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Versions    []PackVersion `json:"versions"`
}

// RegistryListResponse wraps the registry list API response
type RegistryListResponse struct {
	Registries []string `json:"registries"`
}

// PackListResponse wraps the pack list API response
type PackListResponse struct {
	Packs []PackSummary `json:"packs"`
}

// JobSummary represents a job in list responses
type JobSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// JobDetail represents detailed job information
type JobDetail struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Versions    []PackVersion `json:"versions"`
}

// JobListResponse wraps the job list API response
type JobListResponse struct {
	Jobs []JobSummary `json:"jobs"`
}

// Client is an HTTP client for the Ramble registry API
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a new Ramble registry client
func NewClient(baseURL string) *Client {
	// Ensure URL has scheme
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ListRegistries returns all namespaces that have published packs
func (c *Client) ListRegistries() ([]string, error) {
	resp, err := c.get("/v1/registries")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result RegistryListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Registries, nil
}

// ListAllPacks returns all packs across all namespaces
func (c *Client) ListAllPacks() ([]PackSummary, error) {
	resp, err := c.get("/v1/packs")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result PackListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Packs, nil
}

// ListPacks returns packs for a specific namespace
func (c *Client) ListPacks(namespace string) ([]PackSummary, error) {
	resp, err := c.getJSON("/" + url.PathEscape(namespace))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("namespace not found: %s", namespace)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result PackListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Packs, nil
}

// GetPack returns detailed information about a specific pack
func (c *Client) GetPack(namespace, name string) (*PackDetail, error) {
	path := "/" + url.PathEscape(namespace) + "/" + url.PathEscape(name)
	resp, err := c.getJSON(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("pack not found: %s/%s", namespace, name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result PackDetail
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// SearchPacks searches for packs by name or description
func (c *Client) SearchPacks(query string) ([]PackSummary, error) {
	resp, err := c.get("/v1/packs/search?q=" + url.QueryEscape(query))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result PackListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Packs, nil
}

// ListAllJobs returns all jobs across all namespaces
func (c *Client) ListAllJobs() ([]JobSummary, error) {
	resp, err := c.get("/v1/jobs")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result JobListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Jobs, nil
}

// GetJob returns detailed information about a specific job
func (c *Client) GetJob(namespace, name string) (*JobDetail, error) {
	path := "/" + url.PathEscape(namespace) + "/" + url.PathEscape(name)
	resp, err := c.getJSON(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("job not found: %s/%s", namespace, name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result JobDetail
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// SearchJobs searches for jobs by name or description
func (c *Client) SearchJobs(query string) ([]JobSummary, error) {
	resp, err := c.get("/v1/jobs/search?q=" + url.QueryEscape(query))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result JobListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Jobs, nil
}

// GetRawContent fetches raw pack content (HCL)
func (c *Client) GetRawContent(namespace, name, version string) (string, error) {
	var path string
	if version != "" {
		path = fmt.Sprintf("/%s/%s/v/%s/raw", url.PathEscape(namespace), url.PathEscape(name), url.PathEscape(version))
	} else {
		path = fmt.Sprintf("/%s/%s/raw", url.PathEscape(namespace), url.PathEscape(name))
	}

	resp, err := c.get(path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("pack not found: %s/%s", namespace, name)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(body), nil
}

// get performs a GET request to the specified path
func (c *Client) get(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	return c.HTTPClient.Do(req)
}

// getJSON performs a GET request with Accept: application/json header
func (c *Client) getJSON(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	return c.HTTPClient.Do(req)
}
