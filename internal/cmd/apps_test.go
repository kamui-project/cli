package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/kamui-project/kamui-cli/internal/di"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

// MockAppService is a mock implementation of iface.AppService
type MockAppService struct {
	GetInstallationsFunc func(ctx context.Context) ([]iface.Installation, error)
	GetBranchesFunc      func(ctx context.Context, owner, repo string) ([]iface.Branch, error)
	CreateAppFunc        func(ctx context.Context, input *iface.CreateAppInput) (*iface.CreateAppOutput, error)
	ListAppsFunc         func(ctx context.Context, projectID string) ([]iface.App, error)
	GetAppFunc           func(ctx context.Context, appID string) (*iface.AppDetail, error)
	DeleteAppFunc        func(ctx context.Context, appID string) error
}

func (m *MockAppService) GetInstallations(ctx context.Context) ([]iface.Installation, error) {
	if m.GetInstallationsFunc != nil {
		return m.GetInstallationsFunc(ctx)
	}
	return nil, nil
}

func (m *MockAppService) GetBranches(ctx context.Context, owner, repo string) ([]iface.Branch, error) {
	if m.GetBranchesFunc != nil {
		return m.GetBranchesFunc(ctx, owner, repo)
	}
	return nil, nil
}

func (m *MockAppService) CreateApp(ctx context.Context, input *iface.CreateAppInput) (*iface.CreateAppOutput, error) {
	if m.CreateAppFunc != nil {
		return m.CreateAppFunc(ctx, input)
	}
	return &iface.CreateAppOutput{ID: "test-app-id", Name: "test-app"}, nil
}

func (m *MockAppService) ListApps(ctx context.Context, projectID string) ([]iface.App, error) {
	if m.ListAppsFunc != nil {
		return m.ListAppsFunc(ctx, projectID)
	}
	return nil, nil
}

func (m *MockAppService) GetApp(ctx context.Context, appID string) (*iface.AppDetail, error) {
	if m.GetAppFunc != nil {
		return m.GetAppFunc(ctx, appID)
	}
	return &iface.AppDetail{
		ID:          appID,
		DisplayName: "Test App",
		AppType:     "dynamic",
		URL:         "https://test.example.com",
	}, nil
}

func (m *MockAppService) DeleteApp(ctx context.Context, appID string) error {
	if m.DeleteAppFunc != nil {
		return m.DeleteAppFunc(ctx, appID)
	}
	return nil
}

func TestAppsListCommand_Run(t *testing.T) {
	tests := []struct {
		name          string
		projectFlag   string
		mockProjects  []iface.Project
		mockAppDetail *iface.AppDetail
		mockError     error
		wantOutput    []string
		wantNotOutput []string
		wantErr       bool
		wantErrMsg    string
	}{
		{
			name:        "successfully lists apps by project name",
			projectFlag: "my-project",
			mockProjects: []iface.Project{
				{
					ID:   "proj-123",
					Name: "my-project",
					Apps: []iface.App{
						{ID: "app-1", Name: "web-app"},
						{ID: "app-2", Name: "api-app"},
					},
				},
			},
			mockAppDetail: &iface.AppDetail{
				DisplayName: "Web App",
				AppType:     "dynamic",
				URL:         "https://web.example.com",
				Status:      &iface.ProjectStatus{StatusRunning: 1},
			},
			wantOutput: []string{"my-project", "proj-123", "Web App", "app-1", "running"},
			wantErr:    false,
		},
		{
			name:        "successfully lists apps by project ID",
			projectFlag: "proj-456",
			mockProjects: []iface.Project{
				{
					ID:   "proj-456",
					Name: "another-project",
					Apps: []iface.App{
						{ID: "app-3", Name: "service"},
					},
				},
			},
			mockAppDetail: &iface.AppDetail{
				DisplayName: "Service",
				AppType:     "dynamic",
			},
			wantOutput: []string{"another-project", "proj-456", "Service"},
			wantErr:    false,
		},
		{
			name:        "shows empty message when no apps",
			projectFlag: "empty-project",
			mockProjects: []iface.Project{
				{
					ID:   "proj-empty",
					Name: "empty-project",
					Apps: []iface.App{},
				},
			},
			wantOutput: []string{"No apps found"},
			wantErr:    false,
		},
		{
			name:         "returns error when project not found",
			projectFlag:  "nonexistent",
			mockProjects: []iface.Project{},
			wantErr:      true,
			wantErrMsg:   "project not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock services
			mockAuth := &MockAuthService{}
			mockProject := &MockProjectService{
				ListProjectsFunc: func(ctx context.Context) ([]iface.Project, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockProjects, nil
				},
			}
			mockApp := &MockAppService{
				GetAppFunc: func(ctx context.Context, appID string) (*iface.AppDetail, error) {
					if tt.mockAppDetail != nil {
						return tt.mockAppDetail, nil
					}
					return &iface.AppDetail{DisplayName: "Default", AppType: "dynamic"}, nil
				},
			}

			// Create container with mocks
			container := di.NewContainerWithAllServices(mockAuth, mockProject, mockApp)

			// Create command hierarchy
			root := NewRootCommand()
			root.SetContainer(container)

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Execute command
			args := []string{"apps", "list", "-p", tt.projectFlag}
			root.Command().SetArgs(args)

			err := root.Command().Execute()

			// Restore stdout and read output
			w.Close()
			os.Stdout = oldStdout
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrMsg != "" {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("Error should contain %q, got: %v", tt.wantErrMsg, err)
				}
				return
			}

			// Check output contains expected strings
			for _, want := range tt.wantOutput {
				if !strings.Contains(output, want) {
					t.Errorf("Output should contain %q, got: %s", want, output)
				}
			}

			// Check output does not contain unwanted strings
			for _, notWant := range tt.wantNotOutput {
				if strings.Contains(output, notWant) {
					t.Errorf("Output should not contain %q, got: %s", notWant, output)
				}
			}
		})
	}
}

func TestAppsDeleteCommand_Run(t *testing.T) {
	tests := []struct {
		name          string
		appArg        string
		yesFlag       bool
		mockProjects  []iface.Project
		mockAppDetail *iface.AppDetail
		mockDelError  error
		wantOutput    []string
		wantErr       bool
		wantErrMsg    string
	}{
		{
			name:    "successfully deletes app by ID with --yes flag",
			appArg:  "app-123",
			yesFlag: true,
			mockProjects: []iface.Project{
				{
					ID:   "proj-1",
					Name: "project-1",
					Apps: []iface.App{
						{ID: "app-123", Name: "my-app"},
					},
				},
			},
			mockAppDetail: &iface.AppDetail{
				ID:          "app-123",
				DisplayName: "My App",
				AppType:     "dynamic",
				URL:         "https://myapp.example.com",
			},
			wantOutput: []string{"deleted successfully"},
			wantErr:    false,
		},
		{
			name:    "successfully deletes app by name prefix with --yes flag",
			appArg:  "my-app",
			yesFlag: true,
			mockProjects: []iface.Project{
				{
					ID:   "proj-1",
					Name: "project-1",
					Apps: []iface.App{
						{ID: "app-456", Name: "my-app-abc123"},
					},
				},
			},
			mockAppDetail: &iface.AppDetail{
				ID:          "app-456",
				DisplayName: "My App",
				AppType:     "dynamic",
			},
			wantOutput: []string{"deleted successfully"},
			wantErr:    false,
		},
		{
			name:         "returns error when app not found",
			appArg:       "nonexistent",
			yesFlag:      true,
			mockProjects: []iface.Project{},
			wantErr:      true,
			wantErrMsg:   "app not found",
		},
		{
			name:    "shows multiple matches error for name prefix",
			appArg:  "duplicate",
			yesFlag: true,
			mockProjects: []iface.Project{
				{
					ID:   "proj-1",
					Name: "project-1",
					Apps: []iface.App{
						{ID: "app-1", Name: "duplicate-abc"},
						{ID: "app-2", Name: "duplicate-xyz"},
					},
				},
			},
			mockAppDetail: &iface.AppDetail{
				DisplayName: "Duplicate App",
				AppType:     "dynamic",
			},
			wantErr:    true,
			wantErrMsg: "please specify the app by ID",
		},
		{
			name:    "shows multiple matches error for duplicate display names across projects",
			appArg:  "My App",
			yesFlag: true,
			mockProjects: []iface.Project{
				{
					ID:   "proj-1",
					Name: "project-1",
					Apps: []iface.App{
						{ID: "app-1", Name: "myapp-abc123"},
					},
				},
				{
					ID:   "proj-2",
					Name: "project-2",
					Apps: []iface.App{
						{ID: "app-2", Name: "myapp-xyz456"},
					},
				},
			},
			mockAppDetail: &iface.AppDetail{
				DisplayName: "My App",
				AppType:     "dynamic",
			},
			wantErr:    true,
			wantErrMsg: "please specify the app by ID",
		},
		{
			name:    "returns error when delete fails",
			appArg:  "app-789",
			yesFlag: true,
			mockProjects: []iface.Project{
				{
					ID:   "proj-1",
					Name: "project-1",
					Apps: []iface.App{
						{ID: "app-789", Name: "failing-app"},
					},
				},
			},
			mockAppDetail: &iface.AppDetail{
				ID:          "app-789",
				DisplayName: "Failing App",
			},
			mockDelError: errors.New("delete failed"),
			wantErr:      true,
			wantErrMsg:   "delete failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock services
			mockAuth := &MockAuthService{}
			mockProject := &MockProjectService{
				ListProjectsFunc: func(ctx context.Context) ([]iface.Project, error) {
					return tt.mockProjects, nil
				},
			}
			mockApp := &MockAppService{
				GetAppFunc: func(ctx context.Context, appID string) (*iface.AppDetail, error) {
					if tt.mockAppDetail != nil {
						detail := *tt.mockAppDetail
						detail.ID = appID
						return &detail, nil
					}
					return &iface.AppDetail{ID: appID, DisplayName: "Test"}, nil
				},
				DeleteAppFunc: func(ctx context.Context, appID string) error {
					return tt.mockDelError
				},
			}

			// Create container with mocks
			container := di.NewContainerWithAllServices(mockAuth, mockProject, mockApp)

			// Create command hierarchy
			root := NewRootCommand()
			root.SetContainer(container)

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Execute command
			args := []string{"apps", "delete", tt.appArg}
			if tt.yesFlag {
				args = append(args, "--yes")
			}
			root.Command().SetArgs(args)

			err := root.Command().Execute()

			// Restore stdout and read output
			w.Close()
			os.Stdout = oldStdout
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrMsg != "" {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("Error should contain %q, got: %v", tt.wantErrMsg, err)
				}
				return
			}

			// Check output contains expected strings
			for _, want := range tt.wantOutput {
				if !strings.Contains(output, want) {
					t.Errorf("Output should contain %q, got: %s", want, output)
				}
			}
		})
	}
}

func TestProjectsDeleteCommand_Run(t *testing.T) {
	tests := []struct {
		name         string
		projectArg   string
		yesFlag      bool
		mockProjects []iface.Project
		mockProject  *iface.Project
		mockDelError error
		wantOutput   []string
		wantErr      bool
		wantErrMsg   string
	}{
		{
			name:       "successfully deletes project by name with --yes flag",
			projectArg: "my-project",
			yesFlag:    true,
			mockProjects: []iface.Project{
				{
					ID:       "proj-123",
					Name:     "my-project",
					PlanType: "free",
					Region:   "tokyo",
				},
			},
			wantOutput: []string{"deleted successfully"},
			wantErr:    false,
		},
		{
			name:       "successfully deletes project by ID with --yes flag",
			projectArg: "proj-456",
			yesFlag:    true,
			mockProjects: []iface.Project{
				{
					ID:       "proj-456",
					Name:     "another-project",
					PlanType: "pro",
					Region:   "tokyo",
				},
			},
			wantOutput: []string{"deleted successfully"},
			wantErr:    false,
		},
		{
			name:         "returns error when project not found",
			projectArg:   "nonexistent",
			yesFlag:      true,
			mockProjects: []iface.Project{},
			wantErr:      true,
			wantErrMsg:   "project not found",
		},
		{
			name:       "returns error when delete fails",
			projectArg: "failing-project",
			yesFlag:    true,
			mockProjects: []iface.Project{
				{
					ID:   "proj-fail",
					Name: "failing-project",
				},
			},
			mockDelError: errors.New("delete failed"),
			wantErr:      true,
			wantErrMsg:   "delete failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock services
			mockAuth := &MockAuthService{}
			mockProject := &MockProjectService{
				ListProjectsFunc: func(ctx context.Context) ([]iface.Project, error) {
					return tt.mockProjects, nil
				},
				DeleteProjectFunc: func(ctx context.Context, id string) error {
					return tt.mockDelError
				},
			}

			// Create container with mocks
			container := di.NewContainerWithServices(mockAuth, mockProject)

			// Create command hierarchy
			root := NewRootCommand()
			root.SetContainer(container)

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Execute command
			args := []string{"projects", "delete", tt.projectArg}
			if tt.yesFlag {
				args = append(args, "--yes")
			}
			root.Command().SetArgs(args)

			err := root.Command().Execute()

			// Restore stdout and read output
			w.Close()
			os.Stdout = oldStdout
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrMsg != "" {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("Error should contain %q, got: %v", tt.wantErrMsg, err)
				}
				return
			}

			// Check output contains expected strings
			for _, want := range tt.wantOutput {
				if !strings.Contains(output, want) {
					t.Errorf("Output should contain %q, got: %s", want, output)
				}
			}
		})
	}
}

