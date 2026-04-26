// Package di provides dependency injection for the Kamui CLI.
// It contains the service container and factory functions.
package di

import (
	"github.com/kamui-project/kamui-cli/internal/config"
	"github.com/kamui-project/kamui-cli/internal/service"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

// Container holds all service dependencies for the CLI.
// Services are accessed via interfaces to enable mocking in tests.
type Container struct {
	configManager  *config.Manager
	authService    iface.AuthService
	projectService iface.ProjectService
	appService     iface.AppService
	tokensService  iface.TokensService
}

// NewContainer creates a new dependency container with default implementations
func NewContainer() (*Container, error) {
	configManager, err := config.NewManager()
	if err != nil {
		return nil, err
	}

	authService := service.NewAuthService(configManager)
	return &Container{
		configManager:  configManager,
		authService:    authService,
		projectService: service.NewProjectService(configManager, authService),
		appService:     service.NewAppService(configManager, authService),
		tokensService:  service.NewTokensService(configManager, authService),
	}, nil
}

// NewContainerWithServices creates a container with custom service implementations.
// This is useful for testing with mock services.
func NewContainerWithServices(
	authService iface.AuthService,
	projectService iface.ProjectService,
) *Container {
	return &Container{
		authService:    authService,
		projectService: projectService,
	}
}

// NewContainerWithAllServices creates a container with all custom service implementations.
// This is useful for testing with mock services including app service.
func NewContainerWithAllServices(
	authService iface.AuthService,
	projectService iface.ProjectService,
	appService iface.AppService,
) *Container {
	return &Container{
		authService:    authService,
		projectService: projectService,
		appService:     appService,
	}
}

// AuthService returns the authentication service
func (c *Container) AuthService() iface.AuthService {
	return c.authService
}

// ProjectService returns the project service
func (c *Container) ProjectService() iface.ProjectService {
	return c.projectService
}

// AppService returns the app service
func (c *Container) AppService() iface.AppService {
	return c.appService
}

// TokensService returns the personal access token service
func (c *Container) TokensService() iface.TokensService {
	return c.tokensService
}

// ConfigManager returns the config manager
func (c *Container) ConfigManager() *config.Manager {
	return c.configManager
}
