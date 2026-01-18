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
}

// NewContainer creates a new dependency container with default implementations
func NewContainer() (*Container, error) {
	configManager, err := config.NewManager()
	if err != nil {
		return nil, err
	}

	return &Container{
		configManager:  configManager,
		authService:    service.NewAuthService(configManager),
		projectService: service.NewProjectService(configManager),
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

// AuthService returns the authentication service
func (c *Container) AuthService() iface.AuthService {
	return c.authService
}

// ProjectService returns the project service
func (c *Container) ProjectService() iface.ProjectService {
	return c.projectService
}

// ConfigManager returns the config manager
func (c *Container) ConfigManager() *config.Manager {
	return c.configManager
}
