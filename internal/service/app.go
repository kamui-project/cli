package service

import (
	"context"
	"fmt"

	"github.com/kamui-project/kamui-cli/internal/api"
	"github.com/kamui-project/kamui-cli/internal/auth"
	"github.com/kamui-project/kamui-cli/internal/config"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

// appService implements iface.AppService
type appService struct {
	configManager *config.Manager
}

// NewAppService creates a new app service
func NewAppService(configManager *config.Manager) iface.AppService {
	return &appService{
		configManager: configManager,
	}
}

// ensureAuthenticated checks if the user is logged in and refreshes token if needed
func (s *appService) ensureAuthenticated(ctx context.Context) error {
	cfg, err := s.configManager.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.AccessToken == "" && cfg.RefreshToken == "" {
		return fmt.Errorf("not logged in. Please run 'kamui login' first")
	}

	if s.configManager.IsLoggedIn() {
		return nil
	}

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

	if err := s.configManager.SaveTokens(result.AccessToken, result.RefreshToken, result.ExpiresIn); err != nil {
		return fmt.Errorf("failed to save refreshed tokens: %w", err)
	}

	return nil
}

// getAPIClient creates an API client with the current credentials
func (s *appService) getAPIClient(ctx context.Context) (*api.Client, error) {
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

// GetInstallations returns all GitHub App installations for the user
func (s *appService) GetInstallations(ctx context.Context) ([]iface.Installation, error) {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return nil, err
	}

	installations, err := client.GetInstallations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch installations: %w", err)
	}

	// Convert to interface type
	result := make([]iface.Installation, len(installations))
	for i, inst := range installations {
		result[i] = iface.Installation{
			ID:         inst.ID,
			Repository: inst.Repository,
			Owner:      inst.Owner,
			OwnerType:  inst.OwnerType,
		}
	}

	return result, nil
}

// GetBranches returns branches for a repository
func (s *appService) GetBranches(ctx context.Context, owner, repo string) ([]iface.Branch, error) {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return nil, err
	}

	branches, err := client.GetBranches(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch branches: %w", err)
	}

	// Convert to interface type
	result := make([]iface.Branch, len(branches))
	for i, b := range branches {
		result[i] = iface.Branch{
			Name:      b.Name,
			Protected: b.Protected,
		}
	}

	return result, nil
}

// CreateApp creates a new application
func (s *appService) CreateApp(ctx context.Context, input *iface.CreateAppInput) (*iface.CreateAppOutput, error) {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return nil, err
	}

	// Build the request
	req := &api.CreateAppRequest{
		ProjectID:           input.ProjectID,
		AppName:             input.AppName,
		AppDisplayName:      input.DisplayName,
		Replicas:            input.Replicas,
		EnvVars:             input.EnvVars,
		PreCommand:          input.PreCommand,
		StartCommand:        input.StartCommand,
		SetupCommand:        input.SetupCommand,
		HealthCheckEndpoint: input.HealthCheckPath,
		DeployType:          input.DeployType,
		AppType:             "dynamic",
		LanguageType:        input.Language,
		OrganizationName:    input.Owner,
		OwnerType:           input.OwnerType,
		RepositoryName:      input.Repository,
		RepositoryBranch:    input.Branch,
		Directory:           input.Directory,
		DatabaseID:          input.DatabaseID,
		Status: &api.ProjectStatus{
			StatusRunning: 0,
			StatusStopped: 0,
			StatusError:   0,
			StatusUnknown: 0,
		},
	}

	// Set defaults
	if req.Replicas == 0 {
		req.Replicas = 1
	}
	if req.EnvVars == nil {
		req.EnvVars = make(map[string]string)
	}
	if req.HealthCheckEndpoint == "" {
		req.HealthCheckEndpoint = "/health"
	}

	resp, err := client.CreateApp(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create app: %w", err)
	}

	return &iface.CreateAppOutput{
		ID:   resp.AppID,
		Name: input.AppName, // Use input name since API only returns app_id
	}, nil
}

// ListApps returns all apps for a project
func (s *appService) ListApps(ctx context.Context, projectID string) ([]iface.App, error) {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch project to get apps
	var project iface.Project
	if err := client.Get(ctx, fmt.Sprintf("/api/projects/%s", projectID), &project); err != nil {
		return nil, fmt.Errorf("failed to fetch project: %w", err)
	}

	return project.Apps, nil
}

// GetApp returns detailed app information by ID
func (s *appService) GetApp(ctx context.Context, appID string) (*iface.AppDetail, error) {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := client.GetApp(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch app: %w", err)
	}

	return &iface.AppDetail{
		ID:            appID,
		DisplayName:   resp.DisplayName,
		AppType:       resp.AppType,
		LanguageType:  resp.LanguageType,
		URL:           resp.URL,
		CustomDomain:  resp.CustomDomain,
		GithubOrgRepo: resp.GithubOrgRepo,
		GithubBranch:  resp.GithubBranch,
		Status:        (*iface.ProjectStatus)(resp.PodStatus),
	}, nil
}

// DeleteApp deletes an app by ID
func (s *appService) DeleteApp(ctx context.Context, appID string) error {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return err
	}

	if err := client.DeleteApp(ctx, appID); err != nil {
		return fmt.Errorf("failed to delete app: %w", err)
	}

	return nil
}

// CreateStaticApp creates a new static app via GitHub repository
func (s *appService) CreateStaticApp(ctx context.Context, input *iface.CreateStaticAppInput) (*iface.CreateAppOutput, error) {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return nil, err
	}

	// Build the request
	req := &api.CreateStaticAppRequest{
		AppName:          input.AppName,
		ProjectID:        input.ProjectID,
		Replicas:         input.Replicas,
		AppSpecType:      input.AppSpecType,
		DeployType:       input.DeployType,
		OrganizationName: input.OrganizationName,
		OwnerType:        input.OwnerType,
		RepositoryName:   input.RepositoryName,
		RepositoryBranch: input.RepositoryBranch,
		Directory:        input.Directory,
	}

	// Set defaults
	if req.Replicas == 0 {
		req.Replicas = 1
	}
	if req.AppSpecType == "" {
		req.AppSpecType = "nano"
	}
	if req.DeployType == "" {
		req.DeployType = "github"
	}

	resp, err := client.CreateStaticApp(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create static app: %w", err)
	}

	return &iface.CreateAppOutput{
		ID:   resp.AppID,
		Name: input.AppName,
	}, nil
}

// CreateStaticAppUpload creates a new static app via file upload
func (s *appService) CreateStaticAppUpload(ctx context.Context, input *iface.CreateStaticAppUploadInput) (*iface.CreateAppOutput, error) {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return nil, err
	}

	// Build the request
	req := &api.CreateStaticAppUploadRequest{
		ProjectID:   input.ProjectID,
		AppName:     input.AppName,
		Replicas:    input.Replicas,
		AppSpecType: input.AppSpecType,
		FilePath:    input.FilePath,
	}

	// Set defaults
	if req.Replicas == 0 {
		req.Replicas = 1
	}
	if req.AppSpecType == "" {
		req.AppSpecType = "nano"
	}

	resp, err := client.CreateStaticAppUpload(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create static app: %w", err)
	}

	return &iface.CreateAppOutput{
		ID:   resp.AppID,
		Name: input.AppName,
	}, nil
}

