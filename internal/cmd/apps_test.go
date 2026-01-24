package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamui-project/kamui-cli/internal/di"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

// MockAppService is a mock implementation of iface.AppService
type MockAppService struct {
	GetInstallationsFunc        func(ctx context.Context) ([]iface.Installation, error)
	GetBranchesFunc             func(ctx context.Context, owner, repo string) ([]iface.Branch, error)
	CreateAppFunc               func(ctx context.Context, input *iface.CreateAppInput) (*iface.CreateAppOutput, error)
	CreateStaticAppFunc         func(ctx context.Context, input *iface.CreateStaticAppInput) (*iface.CreateAppOutput, error)
	CreateStaticAppUploadFunc   func(ctx context.Context, input *iface.CreateStaticAppUploadInput) (*iface.CreateAppOutput, error)
	ListAppsFunc                func(ctx context.Context, projectID string) ([]iface.App, error)
	GetAppFunc                  func(ctx context.Context, appID string) (*iface.AppDetail, error)
	DeleteAppFunc               func(ctx context.Context, appID string) error
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

func (m *MockAppService) CreateStaticApp(ctx context.Context, input *iface.CreateStaticAppInput) (*iface.CreateAppOutput, error) {
	if m.CreateStaticAppFunc != nil {
		return m.CreateStaticAppFunc(ctx, input)
	}
	return &iface.CreateAppOutput{ID: "test-static-app-id", Name: input.AppName}, nil
}

func (m *MockAppService) CreateStaticAppUpload(ctx context.Context, input *iface.CreateStaticAppUploadInput) (*iface.CreateAppOutput, error) {
	if m.CreateStaticAppUploadFunc != nil {
		return m.CreateStaticAppUploadFunc(ctx, input)
	}
	return &iface.CreateAppOutput{ID: "test-static-upload-app-id", Name: input.AppName}, nil
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

func TestCreateZipFromDirectory(t *testing.T) {
	tests := []struct {
		name        string
		setupDir    func(dir string) error
		wantErr     bool
		wantErrMsg  string
		validateZip func(zipPath string) error
	}{
		{
			name: "successfully creates ZIP from directory with index.html",
			setupDir: func(dir string) error {
				// Create index.html
				if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0644); err != nil {
					return err
				}
				// Create style.css
				if err := os.WriteFile(filepath.Join(dir, "style.css"), []byte("body {}"), 0644); err != nil {
					return err
				}
				return nil
			},
			wantErr: false,
			validateZip: func(zipPath string) error {
				reader, err := zip.OpenReader(zipPath)
				if err != nil {
					return err
				}
				defer reader.Close()

				files := make(map[string]bool)
				for _, f := range reader.File {
					files[f.Name] = true
				}

				if !files["index.html"] {
					return errors.New("ZIP should contain index.html")
				}
				if !files["style.css"] {
					return errors.New("ZIP should contain style.css")
				}
				return nil
			},
		},
		{
			name: "successfully creates ZIP with subdirectories",
			setupDir: func(dir string) error {
				if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0644); err != nil {
					return err
				}
				// Create subdirectory
				subDir := filepath.Join(dir, "assets")
				if err := os.MkdirAll(subDir, 0755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(subDir, "script.js"), []byte("console.log('test')"), 0644); err != nil {
					return err
				}
				return nil
			},
			wantErr: false,
			validateZip: func(zipPath string) error {
				reader, err := zip.OpenReader(zipPath)
				if err != nil {
					return err
				}
				defer reader.Close()

				files := make(map[string]bool)
				for _, f := range reader.File {
					files[f.Name] = true
				}

				if !files["index.html"] {
					return errors.New("ZIP should contain index.html")
				}
				if !files["assets/script.js"] && !files["assets\\script.js"] {
					return errors.New("ZIP should contain assets/script.js")
				}
				return nil
			},
		},
		{
			name: "skips hidden files and directories",
			setupDir: func(dir string) error {
				if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0644); err != nil {
					return err
				}
				// Create hidden file
				if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden"), 0644); err != nil {
					return err
				}
				// Create hidden directory
				hiddenDir := filepath.Join(dir, ".git")
				if err := os.MkdirAll(hiddenDir, 0755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(hiddenDir, "config"), []byte("config"), 0644); err != nil {
					return err
				}
				return nil
			},
			wantErr: false,
			validateZip: func(zipPath string) error {
				reader, err := zip.OpenReader(zipPath)
				if err != nil {
					return err
				}
				defer reader.Close()

				for _, f := range reader.File {
					if strings.HasPrefix(filepath.Base(f.Name), ".") {
						return errors.New("ZIP should not contain hidden files: " + f.Name)
					}
					if strings.Contains(f.Name, ".git") {
						return errors.New("ZIP should not contain .git directory: " + f.Name)
					}
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tempDir, err := os.MkdirTemp("", "test-zip-dir-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Setup test directory
			if err := tt.setupDir(tempDir); err != nil {
				t.Fatalf("Failed to setup test dir: %v", err)
			}

			// Run the function
			zipPath, err := createZipFromDirectory(tempDir)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("createZipFromDirectory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("Error should contain %q, got: %v", tt.wantErrMsg, err)
				}
				return
			}

			// Clean up ZIP file
			defer os.Remove(zipPath)

			// Validate ZIP contents
			if tt.validateZip != nil {
				if err := tt.validateZip(zipPath); err != nil {
					t.Errorf("ZIP validation failed: %v", err)
				}
			}
		})
	}
}

func TestValidateZipContainsIndexHTML(t *testing.T) {
	tests := []struct {
		name       string
		setupZip   func(zipPath string) error
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "returns nil when index.html exists at root",
			setupZip: func(zipPath string) error {
				f, err := os.Create(zipPath)
				if err != nil {
					return err
				}
				defer f.Close()

				w := zip.NewWriter(f)
				defer w.Close()

				_, err = w.Create("index.html")
				return err
			},
			wantErr: false,
		},
		{
			name: "returns error when index.html is missing",
			setupZip: func(zipPath string) error {
				f, err := os.Create(zipPath)
				if err != nil {
					return err
				}
				defer f.Close()

				w := zip.NewWriter(f)
				defer w.Close()

				_, err = w.Create("style.css")
				return err
			},
			wantErr:    true,
			wantErrMsg: "index.html",
		},
		{
			name: "returns error when index.html is in subdirectory only",
			setupZip: func(zipPath string) error {
				f, err := os.Create(zipPath)
				if err != nil {
					return err
				}
				defer f.Close()

				w := zip.NewWriter(f)
				defer w.Close()

				_, err = w.Create("subdir/index.html")
				return err
			},
			wantErr:    true,
			wantErrMsg: "index.html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp ZIP file
			tempFile, err := os.CreateTemp("", "test-*.zip")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			tempPath := tempFile.Name()
			tempFile.Close()
			defer os.Remove(tempPath)

			// Setup ZIP file
			if err := tt.setupZip(tempPath); err != nil {
				t.Fatalf("Failed to setup ZIP: %v", err)
			}

			// Run the function
			err = validateZipContainsIndexHTML(tempPath)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("validateZipContainsIndexHTML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrMsg != "" {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("Error should contain %q, got: %v", tt.wantErrMsg, err)
				}
			}
		})
	}
}

