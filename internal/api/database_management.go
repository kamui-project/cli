package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// DatabaseMetadata deliberately excludes connection information and passwords.
type DatabaseMetadata struct {
	ID       string `json:"id"`
	Name     string `json:"db_name"`
	Status   string `json:"database_status"`
	SpecType string `json:"db_spec_type"`
}

type DatabaseProject struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Databases []DatabaseMetadata `json:"database"`
}

type CreateDatabaseRequest struct {
	ProjectID   string `json:"project_id"`
	Name        string `json:"db_name"`
	SpecType    string `json:"db_spec_type"`
	RoutingMode string `json:"db_routing_mode"`
}

type CreateDatabaseResponse struct {
	DatabaseID string `json:"database_id"`
}

func (c *Client) databaseManagementRequest(ctx context.Context, method, path string, input, result interface{}) error {
	clone := *c
	transport := *c.httpClient
	transport.CheckRedirect = func(*http.Request, []*http.Request) error {
		return fmt.Errorf("database API redirects are not allowed")
	}
	clone.httpClient = &transport
	return clone.Request(ctx, method, path, input, result)
}

func (c *Client) ListDatabaseProjects(ctx context.Context) ([]DatabaseProject, error) {
	var result []DatabaseProject
	err := c.databaseManagementRequest(ctx, http.MethodGet, "/api/projects", nil, &result)
	return result, err
}

func (c *Client) GetDatabaseProject(ctx context.Context, id string) (*DatabaseProject, error) {
	var result DatabaseProject
	err := c.databaseManagementRequest(ctx, http.MethodGet, "/api/projects/"+url.PathEscape(id), nil, &result)
	return &result, err
}

func (c *Client) CreateDatabase(ctx context.Context, input CreateDatabaseRequest) (*CreateDatabaseResponse, error) {
	var result CreateDatabaseResponse
	err := c.databaseManagementRequest(ctx, http.MethodPost, "/api/databases", input, &result)
	return &result, err
}

func (c *Client) DeleteDatabase(ctx context.Context, id string) error {
	return c.databaseManagementRequest(ctx, http.MethodDelete, "/api/database/"+url.PathEscape(id), nil, nil)
}
