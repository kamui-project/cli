package service

import (
	"context"
	"fmt"

	"github.com/kamui-project/kamui-cli/internal/api"
	"github.com/kamui-project/kamui-cli/internal/auth"
	"github.com/kamui-project/kamui-cli/internal/config"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

// projectService implements iface.ProjectService
type projectService struct {
	configManager *config.Manager
}

// NewProjectService creates a new project service
func NewProjectService(configManager *config.Manager) iface.ProjectService {
	return &projectService{
		configManager: configManager,
	}
}

// ensureAuthenticated checks if the user is logged in and refreshes token if needed
func (s *projectService) ensureAuthenticated(ctx context.Context) error {
	cfg, err := s.configManager.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if we have any tokens
	if cfg.AccessToken == "" && cfg.RefreshToken == "" {
		return fmt.Errorf("not logged in. Please run 'kamui login' first")
	}

	// Check if access token is still valid
	if s.configManager.IsLoggedIn() {
		return nil // Token is valid
	}

	// Token expired, try to refresh
	if cfg.RefreshToken == "" {
		return fmt.Errorf("session expired. Please run 'kamui login' again")
	}

	apiURL, err := s.configManager.GetAPIURL()
	if err != nil {
		return fmt.Errorf("failed to get API URL: %w", err)
	}

	oauthFlow := auth.NewOAuthFlow(apiURL)
	oauthFlow.SetClientCredentials(cfg.ClientID, cfg.ClientSecret)

	result, err := oauthFlow.RefreshTokens(ctx, cfg.RefreshToken)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w. Please run 'kamui login' again", err)
	}

	// Save new tokens
	if err := s.configManager.SaveTokens(result.AccessToken, result.RefreshToken, result.ExpiresIn); err != nil {
		return fmt.Errorf("failed to save refreshed tokens: %w", err)
	}

	return nil
}

// getAPIClient creates an API client with the current credentials
func (s *projectService) getAPIClient(ctx context.Context) (*api.Client, error) {
	// Ensure we're authenticated (refresh token if needed)
	if err := s.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	token, err := s.configManager.GetAccessToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	apiURL, err := s.configManager.GetAPIURL()
	if err != nil {
		return nil, fmt.Errorf("failed to get API URL: %w", err)
	}

	return api.NewClient(apiURL, token), nil
}

// ListProjects returns all projects for the authenticated user
func (s *projectService) ListProjects(ctx context.Context) ([]iface.Project, error) {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return nil, err
	}

	var projects []iface.Project
	if err := client.Get(ctx, "/api/projects", &projects); err != nil {
		return nil, fmt.Errorf("failed to fetch projects: %w", err)
	}

	return projects, nil
}

// GetProject returns a project by ID
func (s *projectService) GetProject(ctx context.Context, id string) (*iface.Project, error) {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return nil, err
	}

	var project iface.Project
	if err := client.Get(ctx, fmt.Sprintf("/api/projects/%s", id), &project); err != nil {
		return nil, fmt.Errorf("failed to fetch project: %w", err)
	}

	return &project, nil
}
