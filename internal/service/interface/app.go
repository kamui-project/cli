package iface

import (
	"context"
)

// Installation represents a GitHub App installation
type Installation struct {
	ID         int64  `json:"id"`
	Repository string `json:"repository"`
	Owner      string `json:"owner"`
	OwnerType  string `json:"owner_type"`
}

// Branch represents a GitHub branch
type Branch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

// CreateAppInput represents the input for creating an app
type CreateAppInput struct {
	ProjectID        string
	AppName          string
	DisplayName      string
	Language         string
	DeployType       string
	Owner            string
	OwnerType        string
	Repository       string
	Branch           string
	Directory        string
	StartCommand     string
	SetupCommand     string
	PreCommand       string
	Replicas         int
	EnvVars          map[string]string
	HealthCheckPath  string
	DatabaseID       string
}

// CreateAppOutput represents the result of creating an app
type CreateAppOutput struct {
	ID   string
	Name string
}

// AppDetail represents detailed app information from GET /api/apps/{id}
type AppDetail struct {
	ID            string
	DisplayName   string
	AppType       string
	LanguageType  string
	URL           string
	CustomDomain  string
	GithubOrgRepo string
	GithubBranch  string
	Status        *ProjectStatus
}

// AppService defines the interface for app operations
type AppService interface {
	// GetInstallations returns all GitHub App installations for the user
	GetInstallations(ctx context.Context) ([]Installation, error)

	// GetBranches returns branches for a repository
	GetBranches(ctx context.Context, owner, repo string) ([]Branch, error)

	// CreateApp creates a new application
	CreateApp(ctx context.Context, input *CreateAppInput) (*CreateAppOutput, error)

	// ListApps returns all apps for a project
	ListApps(ctx context.Context, projectID string) ([]App, error)

	// GetApp returns detailed app information by ID
	GetApp(ctx context.Context, appID string) (*AppDetail, error)

	// DeleteApp deletes an app by ID
	DeleteApp(ctx context.Context, appID string) error
}

