package cmd

import (
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
	"github.com/spf13/cobra"
)

// AppsCommand represents the apps command group
type AppsCommand struct {
	root *RootCommand
	cmd  *cobra.Command

	// Subcommands
	createCmd *AppsCreateCommand
	listCmd   *AppsListCommand
	deleteCmd *AppsDeleteCommand
}

// NewAppsCommand creates a new apps command
func NewAppsCommand(root *RootCommand) *AppsCommand {
	a := &AppsCommand{
		root: root,
	}

	a.cmd = &cobra.Command{
		Use:   "apps",
		Short: "Manage Kamui applications",
		Long: `Manage your Kamui applications.

Applications are deployable units within a project. They can be web servers,
APIs, or any other containerized applications.`,
	}

	// Initialize subcommands
	a.createCmd = NewAppsCreateCommand(a)
	a.listCmd = NewAppsListCommand(a)
	a.deleteCmd = NewAppsDeleteCommand(a)

	// Add subcommands
	a.cmd.AddCommand(a.createCmd.Command())
	a.cmd.AddCommand(a.listCmd.Command())
	a.cmd.AddCommand(a.deleteCmd.Command())

	return a
}

// Command returns the underlying cobra command
func (a *AppsCommand) Command() *cobra.Command {
	return a.cmd
}

// Root returns the parent root command
func (a *AppsCommand) Root() *RootCommand {
	return a.root
}

// AppsCreateCommand represents the apps create command
type AppsCreateCommand struct {
	parent *AppsCommand
	cmd    *cobra.Command
}

// NewAppsCreateCommand creates a new apps create command
func NewAppsCreateCommand(parent *AppsCommand) *AppsCreateCommand {
	c := &AppsCreateCommand{
		parent: parent,
	}

	c.cmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new application",
		Long: `Create a new application with an interactive wizard.

This command will guide you through the process of creating a new application,
including selecting a project, configuring the deployment source, and setting
up the build and start commands.

You can specify the project by name or ID using the --project flag.

Examples:
  kamui apps create
  kamui apps create --project my-project
  kamui apps create -p 5f809f2f-0787-40ca-9a43-a3a59edb5400`,
		RunE: c.Run,
	}

	c.cmd.Flags().StringP("project", "p", "", "Project name or ID")

	return c
}

// Command returns the underlying cobra command
func (c *AppsCreateCommand) Command() *cobra.Command {
	return c.cmd
}

// Run executes the apps create command with interactive wizard
func (c *AppsCreateCommand) Run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	projectService := c.parent.Root().Container().ProjectService()
	appService := c.parent.Root().Container().AppService()

	// Fetch all projects
	projects, err := projectService.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch projects: %w", err)
	}

	if len(projects) == 0 {
		return fmt.Errorf("no projects found. Create a project first with: kamui projects create")
	}

	// Step 1: Select project (by flag or interactive)
	var project iface.Project

	projectFlag, _ := cmd.Flags().GetString("project")
	if projectFlag != "" {
		// Find project by name or ID
		var found bool
		for _, p := range projects {
			if p.ID == projectFlag || p.Name == projectFlag {
				project = p
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("project not found: %s\n\nUse 'kamui projects list' to see available projects", projectFlag)
		}
		fmt.Printf("Using project: %s\n", project.Name)
	} else {
		// Interactive selection
		projectOptions := make([]string, len(projects))
		projectMap := make(map[string]iface.Project)
		for i, p := range projects {
			label := fmt.Sprintf("%s (%s)", p.Name, p.ID[:8])
			projectOptions[i] = label
			projectMap[label] = p
		}

		var selectedProject string
		if err := survey.AskOne(&survey.Select{
			Message: "Select project:",
			Options: projectOptions,
		}, &selectedProject); err != nil {
			return err
		}

		project = projectMap[selectedProject]
	}

	// Step 2: App name
	var appName string
	if err := survey.AskOne(&survey.Input{
		Message: "App name:",
	}, &appName, survey.WithValidator(survey.Required)); err != nil {
		return err
	}

	// Step 3: Language
	languages := []string{"Node.js", "Go", "Python"}
	languageMap := map[string]string{
		"Node.js": "node",
		"Go":      "go",
		"Python":  "python",
	}

	var selectedLanguage string
	if err := survey.AskOne(&survey.Select{
		Message: "Language:",
		Options: languages,
	}, &selectedLanguage); err != nil {
		return err
	}

	language := languageMap[selectedLanguage]

	// Step 4: Deploy type
	deployTypes := []string{"GitHub repository", "Docker Hub"}
	deployTypeMap := map[string]string{
		"GitHub repository": "github",
		"Docker Hub":        "docker_hub",
	}

	var selectedDeployType string
	if err := survey.AskOne(&survey.Select{
		Message: "Deploy from:",
		Options: deployTypes,
	}, &selectedDeployType); err != nil {
		return err
	}

	deployType := deployTypeMap[selectedDeployType]

	var owner, ownerType, repo, branch string

	if deployType == "github" {
		// Fetch GitHub installations
		fmt.Println("\nFetching GitHub repositories...")
		installations, err := appService.GetInstallations(ctx)
		if err != nil {
			return fmt.Errorf("failed to fetch GitHub repositories: %w", err)
		}

		if len(installations) == 0 {
			return fmt.Errorf("no GitHub repositories found. Please connect your GitHub account first")
		}

		// Build repository options
		repoOptions := make([]string, len(installations))
		repoMap := make(map[string]iface.Installation)
		for i, inst := range installations {
			label := fmt.Sprintf("%s/%s", inst.Owner, inst.Repository)
			repoOptions[i] = label
			repoMap[label] = inst
		}

		var selectedRepo string
		if err := survey.AskOne(&survey.Select{
			Message: "Select repository:",
			Options: repoOptions,
		}, &selectedRepo); err != nil {
			return err
		}

		installation := repoMap[selectedRepo]
		owner = installation.Owner
		ownerType = installation.OwnerType
		repo = installation.Repository

		// Fetch branches
		fmt.Println("\nFetching branches...")
		branches, err := appService.GetBranches(ctx, owner, repo)
		if err != nil {
			return fmt.Errorf("failed to fetch branches: %w", err)
		}

		if len(branches) == 0 {
			// Default to main if no branches found
			branch = "main"
		} else {
			branchOptions := make([]string, len(branches))
			for i, b := range branches {
				branchOptions[i] = b.Name
			}

			// Try to default to main or master
			defaultBranch := ""
			for _, b := range branchOptions {
				if b == "main" || b == "master" {
					defaultBranch = b
					break
				}
			}

			if err := survey.AskOne(&survey.Select{
				Message: "Select branch:",
				Options: branchOptions,
				Default: defaultBranch,
			}, &branch); err != nil {
				return err
			}
		}
	}

	// Step 5: Directory (for monorepos)
	var directory string
	if deployType == "github" {
		if err := survey.AskOne(&survey.Input{
			Message: "Directory (for monorepos, leave empty for root):",
			Default: "",
		}, &directory); err != nil {
			return err
		}
	}

	// Step 6: Commands
	var startCommand string
	if err := survey.AskOne(&survey.Input{
		Message: "Start command:",
	}, &startCommand, survey.WithValidator(survey.Required)); err != nil {
		return err
	}

	var setupCommand string
	if err := survey.AskOne(&survey.Input{
		Message: "Setup command:",
	}, &setupCommand); err != nil {
		return err
	}

	var preCommand string
	if err := survey.AskOne(&survey.Input{
		Message: "Pre-deploy command:",
	}, &preCommand); err != nil {
		return err
	}

	// Step 7: Health check endpoint
	var healthCheckPath string
	if err := survey.AskOne(&survey.Input{
		Message: "Health check endpoint:",
		Default: "/health",
	}, &healthCheckPath); err != nil {
		return err
	}

	// Step 8: Replicas
	var replicasStr string
	if err := survey.AskOne(&survey.Input{
		Message: "Replicas:",
		Default: "1",
	}, &replicasStr); err != nil {
		return err
	}

	var replicas int
	fmt.Sscanf(replicasStr, "%d", &replicas)
	if replicas < 1 {
		replicas = 1
	}

	// Step 9: Environment variables
	envVars := make(map[string]string)
	var addEnvVars bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Add environment variables?",
		Default: false,
	}, &addEnvVars); err != nil {
		return err
	}

	if addEnvVars {
		for {
			var envKey string
			if err := survey.AskOne(&survey.Input{
				Message: "Environment variable name (empty to finish):",
			}, &envKey); err != nil {
				return err
			}

			if envKey == "" {
				break
			}

			var envValue string
			if err := survey.AskOne(&survey.Input{
				Message: fmt.Sprintf("Value for %s:", envKey),
			}, &envValue); err != nil {
				return err
			}

			envVars[envKey] = envValue
		}
	}

	// Step 10: Database (if available in project)
	var databaseID string
	if len(project.Databases) > 0 {
		var useDatabase bool
		if err := survey.AskOne(&survey.Confirm{
			Message: "Connect to a database?",
			Default: false,
		}, &useDatabase); err != nil {
			return err
		}

		if useDatabase {
			dbOptions := make([]string, len(project.Databases)+1)
			dbOptions[0] = "(none)"
			dbMap := make(map[string]string)
			for i, db := range project.Databases {
				label := db.Name
				if label == "" {
					label = db.ID
				}
				dbOptions[i+1] = label
				dbMap[label] = db.ID
			}

			var selectedDB string
			if err := survey.AskOne(&survey.Select{
				Message: "Select database:",
				Options: dbOptions,
			}, &selectedDB); err != nil {
				return err
			}

			if selectedDB != "(none)" {
				databaseID = dbMap[selectedDB]
			}
		}
	}

	// Create the app
	fmt.Println("\nCreating application...")

	input := &iface.CreateAppInput{
		ProjectID:       project.ID,
		AppName:         appName,
		Language:        language,
		DeployType:      deployType,
		Owner:           owner,
		OwnerType:       ownerType,
		Repository:      repo,
		Branch:          branch,
		Directory:       directory,
		StartCommand:    startCommand,
		SetupCommand:    setupCommand,
		PreCommand:      preCommand,
		HealthCheckPath: healthCheckPath,
		Replicas:        replicas,
		EnvVars:         envVars,
		DatabaseID:      databaseID,
	}

	result, err := appService.CreateApp(ctx, input)
	if err != nil {
		return err
	}

	fmt.Printf("\n✓ App \"%s\" created successfully!\n", result.Name)
	fmt.Printf("  ID: %s\n", result.ID)
	fmt.Println("\n  Note: Deployment is in progress. Check status with:")
	fmt.Printf("  kamui apps list %s\n", project.ID)

	return nil
}

// AppsListCommand represents the apps list command
type AppsListCommand struct {
	parent *AppsCommand
	cmd    *cobra.Command
}

// NewAppsListCommand creates a new apps list command
func NewAppsListCommand(parent *AppsCommand) *AppsListCommand {
	l := &AppsListCommand{
		parent: parent,
	}

	l.cmd = &cobra.Command{
		Use:   "list",
		Short: "List all applications in a project",
		Long: `List all applications in a project.

You can specify the project by name or ID using the --project flag.

Examples:
  kamui apps list --project my-project
  kamui apps list -p my-project`,
		RunE: l.Run,
	}

	l.cmd.Flags().StringP("project", "p", "", "Project name or ID (required)")
	l.cmd.MarkFlagRequired("project")

	return l
}

// Command returns the underlying cobra command
func (l *AppsListCommand) Command() *cobra.Command {
	return l.cmd
}

// Run executes the apps list command
func (l *AppsListCommand) Run(cmd *cobra.Command, args []string) error {
	nameOrID, _ := cmd.Flags().GetString("project")
	ctx := cmd.Context()

	projectService := l.parent.Root().Container().ProjectService()

	// Fetch all projects to find by name or ID
	projects, err := projectService.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch projects: %w", err)
	}

	// Find matching project
	var project *iface.Project
	for i := range projects {
		p := &projects[i]
		if p.ID == nameOrID || p.Name == nameOrID {
			project = p
			break
		}
	}

	if project == nil {
		return fmt.Errorf("project not found: %s\n\nUse 'kamui projects list' to see available projects", nameOrID)
	}

	apps := project.Apps

	if len(apps) == 0 {
		fmt.Printf("No apps found in project \"%s\".\n", project.Name)
		fmt.Println("\nCreate a new app with: kamui apps create")
		return nil
	}

	appService := l.parent.Root().Container().AppService()

	// Print apps
	fmt.Printf("Apps in project \"%s\" (%s):\n\n", project.Name, project.ID)
	for _, app := range apps {
		status := "unknown"
		if app.Status != nil {
			if app.Status.StatusRunning > 0 {
				status = "running"
			} else if app.Status.StatusError > 0 {
				status = "error"
			} else if app.Status.StatusStopped > 0 {
				status = "stopped"
			}
		}

		// Fetch app detail to get display name
		name := app.Name
		var url string
		appDetail, err := appService.GetApp(ctx, app.ID)
		if err == nil && appDetail.DisplayName != "" {
			name = appDetail.DisplayName
			url = appDetail.URL
			// Update status from detail if available
			if appDetail.Status != nil {
				if appDetail.Status.StatusRunning > 0 {
					status = "running"
				} else if appDetail.Status.StatusError > 0 {
					status = "error"
				} else if appDetail.Status.StatusStopped > 0 {
					status = "stopped"
				}
			}
		}
		if name == "" {
			name = "(unnamed)"
		}

		fmt.Printf("  • %s\n", name)
		fmt.Printf("    ID: %s\n", app.ID)
		fmt.Printf("    Status: %s\n", status)
		if url != "" {
			fmt.Printf("    URL: %s\n", url)
		}
		fmt.Println()
	}

	return nil
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Helper to check if a string contains a substring (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// AppsDeleteCommand represents the apps delete command
type AppsDeleteCommand struct {
	parent *AppsCommand
	cmd    *cobra.Command
}

// NewAppsDeleteCommand creates a new apps delete command
func NewAppsDeleteCommand(parent *AppsCommand) *AppsDeleteCommand {
	d := &AppsDeleteCommand{
		parent: parent,
	}

	d.cmd = &cobra.Command{
		Use:   "delete <app-name-or-id>",
		Short: "Delete an application",
		Long: `Delete an application and all its resources.

You can specify the app by name or ID. The command will search for
a matching app across all your projects.

WARNING: This action is irreversible. The application and all associated
Kubernetes resources will be permanently deleted.

Examples:
  kamui apps delete my-api
  kamui apps delete 5f809f2f-0787-40ca-9a43-a3a59edb5400`,
		Args: cobra.ExactArgs(1),
		RunE: d.Run,
	}

	d.cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	return d
}

// Command returns the underlying cobra command
func (d *AppsDeleteCommand) Command() *cobra.Command {
	return d.cmd
}

// appMatch represents a matched app for deletion
type appMatch struct {
	AppID       string
	ProjectName string
	ProjectID   string
	DisplayName string
	AppName     string
}

// Run executes the apps delete command
func (d *AppsDeleteCommand) Run(cmd *cobra.Command, args []string) error {
	nameOrID := args[0]
	ctx := cmd.Context()

	projectService := d.parent.Root().Container().ProjectService()
	appService := d.parent.Root().Container().AppService()

	// Fetch all projects to find the app by name or ID
	projects, err := projectService.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch projects: %w", err)
	}

	// First, check for exact ID match
	var exactIDMatch *appMatch
	for i := range projects {
		p := &projects[i]
		for j := range p.Apps {
			app := &p.Apps[j]
			if app.ID == nameOrID {
				exactIDMatch = &appMatch{
					AppID:       app.ID,
					ProjectName: p.Name,
					ProjectID:   p.ID,
					AppName:     app.Name,
				}
				break
			}
		}
		if exactIDMatch != nil {
			break
		}
	}

	var foundAppID string
	var foundProjectName string
	var appName string

	if exactIDMatch != nil {
		// Exact ID match - use it
		foundAppID = exactIDMatch.AppID
		foundProjectName = exactIDMatch.ProjectName
	} else {
		// Search by name - collect all matches
		var matches []appMatch

		for i := range projects {
			p := &projects[i]
			for j := range p.Apps {
				app := &p.Apps[j]
				// Check by app_name - exact or prefix match
				if app.Name == nameOrID || strings.HasPrefix(app.Name, nameOrID) {
					matches = append(matches, appMatch{
						AppID:       app.ID,
						ProjectName: p.Name,
						ProjectID:   p.ID,
						AppName:     app.Name,
					})
				}
			}
		}

		// Also check by display_name (need to fetch each app's detail)
		// Only do this if no matches found by app_name
		if len(matches) == 0 {
			for i := range projects {
				p := &projects[i]
				for j := range p.Apps {
					app := &p.Apps[j]
					detail, err := appService.GetApp(ctx, app.ID)
					if err == nil && detail.DisplayName == nameOrID {
						matches = append(matches, appMatch{
							AppID:       app.ID,
							ProjectName: p.Name,
							ProjectID:   p.ID,
							AppName:     app.Name,
							DisplayName: detail.DisplayName,
						})
					}
				}
			}
		}

		if len(matches) == 0 {
			return fmt.Errorf("app not found: %s\n\nUse 'kamui apps list -p <project>' to see available apps", nameOrID)
		}

		if len(matches) > 1 {
			// Multiple matches - show them and ask to specify by ID
			fmt.Printf("\nMultiple apps found matching \"%s\":\n\n", nameOrID)
			for _, m := range matches {
				displayName := m.DisplayName
				if displayName == "" {
					// Fetch display name
					detail, err := appService.GetApp(ctx, m.AppID)
					if err == nil && detail.DisplayName != "" {
						displayName = detail.DisplayName
					} else {
						displayName = m.AppName
					}
				}
				fmt.Printf("  • %s\n", displayName)
				fmt.Printf("    ID: %s\n", m.AppID)
				fmt.Printf("    Project: %s\n", m.ProjectName)
				fmt.Println()
			}
			return fmt.Errorf("please specify the app by ID to avoid ambiguity")
		}

		// Single match
		foundAppID = matches[0].AppID
		foundProjectName = matches[0].ProjectName
	}

	// Fetch full app details using the app API
	appDetail, err := appService.GetApp(ctx, foundAppID)
	if err != nil {
		return fmt.Errorf("failed to fetch app details: %w", err)
	}

	appName = appDetail.DisplayName
	if appName == "" {
		appName = foundAppID
	}

	// Check for --yes flag
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	if !skipConfirm {
		// Show warning
		fmt.Printf("\n⚠️  WARNING: You are about to delete the following app:\n\n")
		fmt.Printf("  Name:    %s\n", appName)
		fmt.Printf("  ID:      %s\n", foundAppID)
		fmt.Printf("  Type:    %s\n", appDetail.AppType)
		fmt.Printf("  Project: %s\n", foundProjectName)
		if appDetail.URL != "" {
			fmt.Printf("  URL:     %s\n", appDetail.URL)
		}
		fmt.Println("\n  This action is IRREVERSIBLE. The app will be permanently deleted.")

		// Confirmation prompt
		var confirm bool
		if err := survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("Are you sure you want to delete app \"%s\"?", appName),
			Default: false,
		}, &confirm); err != nil {
			return err
		}

		if !confirm {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	fmt.Println("\nDeleting app...")

	if err := appService.DeleteApp(ctx, foundAppID); err != nil {
		return err
	}

	fmt.Printf("\n✓ App \"%s\" deleted successfully.\n", appName)

	return nil
}
