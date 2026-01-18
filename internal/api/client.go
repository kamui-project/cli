// Package api provides HTTP client for communicating with the Kamui API.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

