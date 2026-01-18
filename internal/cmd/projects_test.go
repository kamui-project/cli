package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/kamui-project/kamui-cli/internal/di"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

// MockAuthService is a mock implementation of iface.AuthService
type MockAuthService struct {
	LoginFunc               func(ctx context.Context) error
	LogoutFunc              func(ctx context.Context) error
	IsLoggedInFunc          func() bool
	GetAccessTokenFunc      func(ctx context.Context) (string, error)
	EnsureAuthenticatedFunc func(ctx context.Context) error
}

func (m *MockAuthService) Login(ctx context.Context) error {
	if m.LoginFunc != nil {
		return m.LoginFunc(ctx)
	}
	return nil
}

func (m *MockAuthService) Logout(ctx context.Context) error {
	if m.LogoutFunc != nil {
		return m.LogoutFunc(ctx)
	}
	return nil
}

func (m *MockAuthService) IsLoggedIn() bool {
	if m.IsLoggedInFunc != nil {
		return m.IsLoggedInFunc()
	}
	return true
}

func (m *MockAuthService) GetAccessToken(ctx context.Context) (string, error) {
	if m.GetAccessTokenFunc != nil {
		return m.GetAccessTokenFunc(ctx)
	}
	return "test-token", nil
}

func (m *MockAuthService) EnsureAuthenticated(ctx context.Context) error {
	if m.EnsureAuthenticatedFunc != nil {
		return m.EnsureAuthenticatedFunc(ctx)
	}
	return nil
}

// MockProjectService is a mock implementation of iface.ProjectService
type MockProjectService struct {
	ListProjectsFunc func(ctx context.Context) ([]iface.Project, error)
	GetProjectFunc   func(ctx context.Context, id string) (*iface.Project, error)
}

func (m *MockProjectService) ListProjects(ctx context.Context) ([]iface.Project, error) {
	if m.ListProjectsFunc != nil {
		return m.ListProjectsFunc(ctx)
	}
	return nil, nil
}

func (m *MockProjectService) GetProject(ctx context.Context, id string) (*iface.Project, error) {
	if m.GetProjectFunc != nil {
		return m.GetProjectFunc(ctx, id)
	}
	return nil, nil
}

func TestProjectsListCommand_Run(t *testing.T) {
	tests := []struct {
		name          string
		mockProjects  []iface.Project
		mockError     error
		outputFormat  string
		wantOutput    []string
		wantNotOutput []string
		wantErr       bool
	}{
		{
			name: "successfully lists projects in table format",
			mockProjects: []iface.Project{
				{
					ID:       "proj-123",
					Name:     "my-project",
					PlanType: "free",
					Region:   "tokyo",
					Apps: []iface.App{
						{ID: "app-1", Name: "web"},
					},
					Databases: []iface.Database{
						{ID: "db-1", Name: "postgres"},
					},
				},
				{
					ID:       "proj-456",
					Name:     "another-project",
					PlanType: "pro",
					Region:   "osaka",
				},
			},
			outputFormat: "text",
			wantOutput:   []string{"proj-123", "my-project", "free", "tokyo", "1", "1", "proj-456", "another-project", "pro", "osaka"},
			wantErr:      false,
		},
		{
			name:         "shows empty message when no projects",
			mockProjects: []iface.Project{},
			outputFormat: "text",
			wantOutput:   []string{"No projects found"},
			wantErr:      false,
		},
		{
			name: "outputs JSON format",
			mockProjects: []iface.Project{
				{
					ID:       "proj-789",
					Name:     "json-project",
					PlanType: "free",
					Region:   "tokyo",
				},
			},
			outputFormat: "json",
			wantOutput:   []string{`"id": "proj-789"`, `"name": "json-project"`},
			wantErr:      false,
		},
		{
			name:      "returns error when service fails",
			mockError: context.DeadlineExceeded,
			wantErr:   true,
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

			// Create container with mocks
			container := di.NewContainerWithServices(mockAuth, mockProject)

			// Create command hierarchy
			root := NewRootCommand()
			root.SetContainer(container)

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Set output format and execute
			args := []string{"projects", "list"}
			if tt.outputFormat == "json" {
				args = append(args, "-o", "json")
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

func TestProjectsGetCommand_Run(t *testing.T) {
	tests := []struct {
		name         string
		projectID    string
		mockProject  *iface.Project
		mockError    error
		outputFormat string
		wantOutput   []string
		wantErr      bool
		wantErrMsg   string
	}{
		{
			name:      "successfully gets project in detail format",
			projectID: "proj-123",
			mockProject: &iface.Project{
				ID:          "proj-123",
				Name:        "my-project",
				Description: "Test project description",
				PlanType:    "free",
				Region:      "tokyo",
				Apps: []iface.App{
					{ID: "app-1", Name: "web-app", AppType: "web", URL: "https://example.com"},
				},
				Databases: []iface.Database{
					{ID: "db-1", Name: "main-db", SpecType: "postgres", Status: "running"},
				},
			},
			outputFormat: "text",
			wantOutput:   []string{"Project: my-project", "ID:      proj-123", "Plan:    free", "Region:  tokyo", "Description: Test project description", "web-app", "main-db"},
			wantErr:      false,
		},
		{
			name:      "successfully gets project in JSON format",
			projectID: "proj-456",
			mockProject: &iface.Project{
				ID:       "proj-456",
				Name:     "json-project",
				PlanType: "pro",
				Region:   "osaka",
			},
			outputFormat: "json",
			wantOutput:   []string{`"id": "proj-456"`, `"name": "json-project"`, `"plan_type": "pro"`},
			wantErr:      false,
		},
		{
			name:      "shows no apps message when project has no apps",
			projectID: "proj-empty",
			mockProject: &iface.Project{
				ID:       "proj-empty",
				Name:     "empty-project",
				PlanType: "free",
				Region:   "tokyo",
				Apps:     []iface.App{},
			},
			outputFormat: "text",
			wantOutput:   []string{"No apps"},
			wantErr:      false,
		},
		{
			name:      "shows no databases message when project has no databases",
			projectID: "proj-no-db",
			mockProject: &iface.Project{
				ID:        "proj-no-db",
				Name:      "no-db-project",
				PlanType:  "free",
				Region:    "tokyo",
				Databases: []iface.Database{},
			},
			outputFormat: "text",
			wantOutput:   []string{"No databases"},
			wantErr:      false,
		},
		{
			name:       "returns error when service fails",
			projectID:  "proj-error",
			mockError:  context.DeadlineExceeded,
			wantErr:    true,
			wantErrMsg: "context deadline exceeded",
		},
		{
			name:       "returns error when project not found",
			projectID:  "non-existent",
			mockError:  context.Canceled,
			wantErr:    true,
			wantErrMsg: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock services
			mockAuth := &MockAuthService{}
			mockProject := &MockProjectService{
				GetProjectFunc: func(ctx context.Context, id string) (*iface.Project, error) {
					if id != tt.projectID {
						t.Errorf("GetProject called with wrong ID: got %s, want %s", id, tt.projectID)
					}
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockProject, nil
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

			// Set args and execute
			args := []string{"projects", "get", tt.projectID}
			if tt.outputFormat == "json" {
				args = append(args, "-o", "json")
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

func TestProjectsGetCommand_Args(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "fails without project ID",
			args:    []string{"projects", "get"},
			wantErr: true,
		},
		{
			name:    "fails with too many arguments",
			args:    []string{"projects", "get", "id1", "id2"},
			wantErr: true,
		},
		{
			name:    "succeeds with exactly one argument",
			args:    []string{"projects", "get", "valid-id"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAuth := &MockAuthService{}
			mockProject := &MockProjectService{
				GetProjectFunc: func(ctx context.Context, id string) (*iface.Project, error) {
					return &iface.Project{ID: id, Name: "test"}, nil
				},
			}

			container := di.NewContainerWithServices(mockAuth, mockProject)
			root := NewRootCommand()
			root.SetContainer(container)

			// Capture stdout to suppress output
			oldStdout := os.Stdout
			_, w, _ := os.Pipe()
			os.Stdout = w

			root.Command().SetArgs(tt.args)
			err := root.Command().Execute()

			w.Close()
			os.Stdout = oldStdout

			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
