// Package api provides HTTP client for communicating with the Kamui API.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Client is an HTTP client for the Kamui API
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

// NewClient creates a new API client
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token: token,
	}
}

// SetToken updates the authentication token
func (c *Client) SetToken(token string) {
	c.token = token
}

// Request performs an HTTP request to the API
func (c *Client) Request(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error status codes
	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Message != "" {
			return &APIError{
				StatusCode: resp.StatusCode,
				Message:    errResp.Message,
			}
		}
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("request failed with status %d", resp.StatusCode),
		}
	}

	// Parse response if result is provided
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// Get performs a GET request
func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
	return c.Request(ctx, http.MethodGet, path, nil, result)
}

// Post performs a POST request
func (c *Client) Post(ctx context.Context, path string, body interface{}, result interface{}) error {
	return c.Request(ctx, http.MethodPost, path, body, result)
}

// Put performs a PUT request
func (c *Client) Put(ctx context.Context, path string, body interface{}, result interface{}) error {
	return c.Request(ctx, http.MethodPut, path, body, result)
}

// Delete performs a DELETE request
func (c *Client) Delete(ctx context.Context, path string, result interface{}) error {
	return c.Request(ctx, http.MethodDelete, path, nil, result)
}

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	Message string `json:"message"`
}

// APIError represents an error returned by the API
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error (status %d): %s", e.StatusCode, e.Message)
}

// IsUnauthorized checks if the error is an unauthorized error
func (e *APIError) IsUnauthorized() bool {
	return e.StatusCode == http.StatusUnauthorized
}

// IsNotFound checks if the error is a not found error
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

// Installation represents a GitHub App installation with repositories
type Installation struct {
	ID        int64  `json:"id"`
	Repository string `json:"repository"`
	Owner     string `json:"owner"`
	OwnerType string `json:"owner_type"`
}

// InstallationsResponse represents the response from /api/installations
type InstallationsResponse struct {
	Installations []Installation `json:"installations"`
}

// Branch represents a GitHub branch
type Branch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

// BranchListResponse represents the response from branches endpoint
type BranchListResponse struct {
	Branches []Branch `json:"branches"`
}

// CreateAppRequest represents the request body for creating an app
type CreateAppRequest struct {
	ProjectID           string            `json:"project_id"`
	AppName             string            `json:"app_name"`
	AppDisplayName      string            `json:"app_display_name,omitempty"`
	Replicas            int               `json:"replicas"`
	EnvVars             map[string]string `json:"env_vars"`
	PreCommand          string            `json:"pre_command"`
	StartCommand        string            `json:"start_command"`
	SetupCommand        string            `json:"setup_command"`
	HealthCheckEndpoint string            `json:"health_check_endpoint,omitempty"`
	DeployType          string            `json:"deploy_type"`
	AppType             string            `json:"app_type"`
	LanguageType        string            `json:"language_type"`
	OrganizationName    string            `json:"organization_name,omitempty"`
	OwnerType           string            `json:"owner_type,omitempty"`
	RepositoryName      string            `json:"repository_name,omitempty"`
	RepositoryBranch    string            `json:"repository_branch,omitempty"`
	Directory           string            `json:"directory,omitempty"`
	DatabaseID          string            `json:"database_id,omitempty"`
	AppSpecType         string            `json:"app_spec_type,omitempty"`
	Status              *ProjectStatus    `json:"status"`
}

// ProjectStatus represents the status of a project/app
type ProjectStatus struct {
	StatusRunning int `json:"status_running"`
	StatusStopped int `json:"status_stopped"`
	StatusError   int `json:"status_error"`
	StatusUnknown int `json:"status_unknown"`
}

// AppCreateResponse represents the response from creating an app
type AppCreateResponse struct {
	Message string `json:"message"`
	AppID   string `json:"app_id"`
}

// GetInstallations fetches all GitHub App installations for the user
func (c *Client) GetInstallations(ctx context.Context) ([]Installation, error) {
	var resp InstallationsResponse
	if err := c.Get(ctx, "/api/installations", &resp); err != nil {
		return nil, err
	}
	return resp.Installations, nil
}

// GetBranches fetches branches for a repository
func (c *Client) GetBranches(ctx context.Context, owner, repo string) ([]Branch, error) {
	path := fmt.Sprintf("/api/repositories/%s/%s/branches", owner, repo)
	var resp BranchListResponse
	if err := c.Get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Branches, nil
}

// CreateApp creates a new application
func (c *Client) CreateApp(ctx context.Context, req *CreateAppRequest) (*AppCreateResponse, error) {
	var resp AppCreateResponse
	if err := c.Post(ctx, "/api/apps", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateProjectRequest represents the request body for creating a project
type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	PlanType    string `json:"plan_type"`
	Region      string `json:"region"`
}

// BasicSuccessResponse represents a simple success response from the API
type BasicSuccessResponse struct {
	Message string `json:"message"`
}

// CreateProject creates a new project
func (c *Client) CreateProject(ctx context.Context, req *CreateProjectRequest) error {
	var resp BasicSuccessResponse
	if err := c.Post(ctx, "/api/projects", req, &resp); err != nil {
		return err
	}
	return nil
}

// DeleteProject deletes a project by ID
func (c *Client) DeleteProject(ctx context.Context, projectID string) error {
	path := fmt.Sprintf("/api/projects/%s", projectID)
	return c.Delete(ctx, path, nil)
}

// DeleteApp deletes an app by ID
func (c *Client) DeleteApp(ctx context.Context, appID string) error {
	path := fmt.Sprintf("/api/apps/%s", appID)
	return c.Delete(ctx, path, nil)
}

// AppDetailResponse represents the response from GET /api/apps/{id}
type AppDetailResponse struct {
	DisplayName   string         `json:"display_name"`
	PodStatus     *ProjectStatus `json:"pod_status"`
	LanguageType  string         `json:"language_type"`
	AppSpec       string         `json:"app_spec"`
	AppType       string         `json:"app_type"`
	GithubOrgRepo string         `json:"github_org_repo,omitempty"`
	GithubBranch  string         `json:"github_branch,omitempty"`
	URL           string         `json:"url"`
	CustomDomain  string         `json:"custom_domain,omitempty"`
}

// GetApp fetches app details by ID
func (c *Client) GetApp(ctx context.Context, appID string) (*AppDetailResponse, error) {
	path := fmt.Sprintf("/api/apps/%s", appID)
	var resp AppDetailResponse
	if err := c.Get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateStaticAppRequest represents the request body for creating a static app via GitHub
type CreateStaticAppRequest struct {
	AppName          string `json:"app_name"`
	ProjectID        string `json:"project_id"`
	Replicas         int    `json:"replicas"`
	AppSpecType      string `json:"app_spec_type"`
	DeployType       string `json:"deploy_type"`
	OrganizationName string `json:"organization_name"`
	OwnerType        string `json:"owner_type"`
	RepositoryName   string `json:"repository_name"`
	RepositoryBranch string `json:"repository_branch"`
	Directory        string `json:"directory,omitempty"`
}

// CreateStaticApp creates a new static app via GitHub repository
func (c *Client) CreateStaticApp(ctx context.Context, req *CreateStaticAppRequest) (*AppCreateResponse, error) {
	var resp AppCreateResponse
	if err := c.Post(ctx, "/api/static-apps", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateStaticAppUploadRequest represents the parameters for creating a static app via file upload
type CreateStaticAppUploadRequest struct {
	ProjectID   string
	AppName     string
	Replicas    int
	AppSpecType string
	FilePath    string // local path to the ZIP file
}

// CreateStaticAppUpload creates a new static app by uploading a ZIP file
func (c *Client) CreateStaticAppUpload(ctx context.Context, req *CreateStaticAppUploadRequest) (*AppCreateResponse, error) {
	// Open the file
	file, err := os.Open(req.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create a buffer and multipart writer
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add form fields
	if err := writer.WriteField("project_id", req.ProjectID); err != nil {
		return nil, fmt.Errorf("failed to write project_id field: %w", err)
	}
	if err := writer.WriteField("app_name", req.AppName); err != nil {
		return nil, fmt.Errorf("failed to write app_name field: %w", err)
	}
	if err := writer.WriteField("replicas", fmt.Sprintf("%d", req.Replicas)); err != nil {
		return nil, fmt.Errorf("failed to write replicas field: %w", err)
	}
	if err := writer.WriteField("app_spec_type", req.AppSpecType); err != nil {
		return nil, fmt.Errorf("failed to write app_spec_type field: %w", err)
	}

	// Add the file
	part, err := writer.CreateFormFile("file", filepath.Base(req.FilePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file content: %w", err)
	}

	// Close the writer to finalize the multipart form
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create the request
	url := c.baseURL + "/api/static-apps/upload"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	// Send the request
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error status codes
	if httpResp.StatusCode >= 400 {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Message != "" {
			return nil, &APIError{
				StatusCode: httpResp.StatusCode,
				Message:    errResp.Message,
			}
		}
		return nil, &APIError{
			StatusCode: httpResp.StatusCode,
			Message:    fmt.Sprintf("request failed with status %d", httpResp.StatusCode),
		}
	}

	// Parse response
	var resp AppCreateResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

