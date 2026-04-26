package service

import (
	"context"
	"fmt"

	"github.com/kamui-project/kamui-cli/internal/api"
	"github.com/kamui-project/kamui-cli/internal/config"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

// projectService implements iface.ProjectService
type projectService struct {
	configManager *config.Manager
	authService   iface.AuthService
}

// NewProjectService creates a new project service
func NewProjectService(configManager *config.Manager, authService iface.AuthService) iface.ProjectService {
	return &projectService{
		configManager: configManager,
		authService:   authService,
	}
}

// getAPIClient creates an API client with the current credentials
func (s *projectService) getAPIClient(ctx context.Context) (*api.Client, error) {
	// Ensure we're authenticated (refresh token if needed)
	if err := s.authService.EnsureAuthenticated(ctx); err != nil {
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

// CreateProject creates a new project
func (s *projectService) CreateProject(ctx context.Context, input *iface.CreateProjectInput) error {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return err
	}

	req := &api.CreateProjectRequest{
		Name:        input.Name,
		Description: input.Description,
		PlanType:    input.PlanType,
		Region:      input.Region,
	}

	if err := client.CreateProject(ctx, req); err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	return nil
}

// DeleteProject deletes a project by ID
func (s *projectService) DeleteProject(ctx context.Context, id string) error {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return err
	}

	if err := client.DeleteProject(ctx, id); err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	return nil
}
