package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/kamui-project/kamui-cli/internal/storage"
)

func (c *Client) storageRequest(ctx context.Context, method, path string, input, result interface{}) error {
	// Control-plane credentials must never follow redirects to another endpoint.
	clone := *c
	httpClient := *c.httpClient
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return fmt.Errorf("storage API redirects are not allowed") }
	clone.httpClient = &httpClient
	return clone.Request(ctx, method, path, input, result)
}
func storagePath(project string) string {
	return "/api/projects/" + url.PathEscape(project) + "/storages"
}
func (c *Client) ListStorages(ctx context.Context, project string) (*storage.Overview, error) {
	var result storage.Overview
	err := c.storageRequest(ctx, http.MethodGet, storagePath(project), nil, &result)
	return &result, err
}
func (c *Client) CreateStorage(ctx context.Context, project, name string) (*storage.Item, error) {
	var result storage.Item
	err := c.storageRequest(ctx, http.MethodPost, storagePath(project), map[string]string{"name": name}, &result)
	return &result, err
}
func (c *Client) DeleteStorage(ctx context.Context, project, id, name string) error {
	return c.storageRequest(ctx, http.MethodDelete, storagePath(project)+"/"+url.PathEscape(id), map[string]interface{}{"confirmation_name": name, "empty_only": true}, nil)
}
func (c *Client) GetStoragePermit(ctx context.Context, project, id, role string) (*storage.Permit, error) {
	var result storage.Permit
	err := c.storageRequest(ctx, http.MethodPost, storagePath(project)+"/"+url.PathEscape(id)+"/access-permit", map[string]string{"role": role}, &result)
	return &result, err
}

// decodeStorageError also handles errors returned through the storage Worker.
func decodeStorageError(data []byte) string {
	var result struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &result) == nil {
		return result.Error.Message
	}
	return ""
}

func (c *Client) StorageProjects(ctx context.Context) ([]storage.ProjectReference, error) {
	var result []storage.ProjectReference
	err := c.storageRequest(ctx, http.MethodGet, "/api/projects", nil, &result)
	return result, err
}
