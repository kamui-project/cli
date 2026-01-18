package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/AlecAivazis/survey/v2"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
	"github.com/spf13/cobra"
)

// ProjectsCommand represents the projects command group
type ProjectsCommand struct {
	root *RootCommand
	cmd  *cobra.Command

	// Subcommands
	listCmd   *ProjectsListCommand
	getCmd    *ProjectsGetCommand
	createCmd *ProjectsCreateCommand
	deleteCmd *ProjectsDeleteCommand
}

// NewProjectsCommand creates a new projects command
func NewProjectsCommand(root *RootCommand) *ProjectsCommand {
	p := &ProjectsCommand{
		root: root,
	}

	p.cmd = &cobra.Command{
		Use:   "projects",
		Short: "Manage Kamui projects",
		Long: `Manage your Kamui projects.

Projects are containers for your applications and databases.
Use subcommands to list, create, or manage your projects.`,
	}

	// Initialize subcommands
	p.listCmd = NewProjectsListCommand(p)
	p.getCmd = NewProjectsGetCommand(p)
	p.createCmd = NewProjectsCreateCommand(p)
	p.deleteCmd = NewProjectsDeleteCommand(p)

	// Add subcommands
	p.cmd.AddCommand(p.listCmd.Command())
	p.cmd.AddCommand(p.getCmd.Command())
	p.cmd.AddCommand(p.createCmd.Command())
	p.cmd.AddCommand(p.deleteCmd.Command())

	return p
}

// Command returns the underlying cobra command
func (p *ProjectsCommand) Command() *cobra.Command {
	return p.cmd
}

// Root returns the parent root command
func (p *ProjectsCommand) Root() *RootCommand {
	return p.root
}

// ProjectsListCommand represents the projects list command
type ProjectsListCommand struct {
	parent *ProjectsCommand
	cmd    *cobra.Command
}

// NewProjectsListCommand creates a new projects list command
func NewProjectsListCommand(parent *ProjectsCommand) *ProjectsListCommand {
	l := &ProjectsListCommand{
		parent: parent,
	}

	l.cmd = &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		Long: `List all projects associated with your Kamui account.

This command displays a table of your projects with their IDs, names, plans, and regions.

Examples:
  kamui projects list
  kamui projects list -o json`,
		RunE: l.Run,
	}

	return l
}

// Command returns the underlying cobra command
func (l *ProjectsListCommand) Command() *cobra.Command {
	return l.cmd
}

// Run executes the projects list command
func (l *ProjectsListCommand) Run(cmd *cobra.Command, args []string) error {
	// Get project service from DI container
	projectService := l.parent.Root().Container().ProjectService()

	// Fetch projects (service will ensure authentication)
	projects, err := projectService.ListProjects(cmd.Context())
	if err != nil {
		return err
	}

	// Get output format
	outputFormat, _ := cmd.Flags().GetString("output")
	if outputFormat == "" {
		outputFormat, _ = cmd.Parent().Parent().PersistentFlags().GetString("output")
	}

	// Output based on format
	switch outputFormat {
	case "json":
		return l.outputJSON(projects)
	default:
		return l.outputTable(projects)
	}
}

// outputJSON outputs projects in JSON format
func (l *ProjectsListCommand) outputJSON(projects []iface.Project) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(projects)
}

// outputTable outputs projects in table format
func (l *ProjectsListCommand) outputTable(projects []iface.Project) error {
	if len(projects) == 0 {
		fmt.Println("No projects found.")
		fmt.Println("\nCreate a new project with: kamui projects create")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tPLAN\tREGION\tAPPS\tDATABASES")
	fmt.Fprintln(w, "--\t----\t----\t------\t----\t---------")

	for _, p := range projects {
		appCount := len(p.Apps)
		dbCount := len(p.Databases)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\n",
			p.ID,
			p.Name,
			p.PlanType,
			p.Region,
			appCount,
			dbCount,
		)
	}

	return w.Flush()
}

// ProjectsGetCommand represents the projects get command
type ProjectsGetCommand struct {
	parent *ProjectsCommand
	cmd    *cobra.Command
}

// NewProjectsGetCommand creates a new projects get command
func NewProjectsGetCommand(parent *ProjectsCommand) *ProjectsGetCommand {
	g := &ProjectsGetCommand{
		parent: parent,
	}

	g.cmd = &cobra.Command{
		Use:   "get <project-id>",
		Short: "Get a project by ID",
		Long: `Get detailed information about a specific project.

This command displays the project details including its apps and databases.

Examples:
  kamui projects get 5f809f2f-0787-40ca-9a43-a3a59edb5400
  kamui projects get 5f809f2f-0787-40ca-9a43-a3a59edb5400 -o json`,
		Args: cobra.ExactArgs(1),
		RunE: g.Run,
	}

	return g
}

// Command returns the underlying cobra command
func (g *ProjectsGetCommand) Command() *cobra.Command {
	return g.cmd
}

// Run executes the projects get command
func (g *ProjectsGetCommand) Run(cmd *cobra.Command, args []string) error {
	projectID := args[0]

	// Get project service from DI container
	projectService := g.parent.Root().Container().ProjectService()

	// Fetch project (service will ensure authentication)
	project, err := projectService.GetProject(cmd.Context(), projectID)
	if err != nil {
		return err
	}

	// Get output format
	outputFormat, _ := cmd.Flags().GetString("output")
	if outputFormat == "" {
		outputFormat, _ = cmd.Parent().Parent().PersistentFlags().GetString("output")
	}

	// Output based on format
	switch outputFormat {
	case "json":
		return g.outputJSON(project)
	default:
		return g.outputDetail(project)
	}
}

// outputJSON outputs project in JSON format
func (g *ProjectsGetCommand) outputJSON(project *iface.Project) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(project)
}

// outputDetail outputs project details in human-readable format
func (g *ProjectsGetCommand) outputDetail(project *iface.Project) error {
	fmt.Printf("Project: %s\n", project.Name)
	fmt.Printf("ID:      %s\n", project.ID)
	fmt.Printf("Plan:    %s\n", project.PlanType)
	fmt.Printf("Region:  %s\n", project.Region)

	if project.Description != "" {
		fmt.Printf("Description: %s\n", project.Description)
	}

	fmt.Printf("Created: %s\n", project.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated: %s\n", project.UpdatedAt.Format("2006-01-02 15:04:05"))

	// Apps section
	fmt.Println("\nApps:")
	if len(project.Apps) == 0 {
		fmt.Println("  No apps")
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  ID\tNAME\tTYPE\tURL")
		fmt.Fprintln(w, "  --\t----\t----\t---")
		for _, app := range project.Apps {
			url := app.URL
			if url == "" {
				url = "-"
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n",
				app.ID,
				app.Name,
				app.AppType,
				url,
			)
		}
		w.Flush()
	}

	// Databases section
	fmt.Println("\nDatabases:")
	if len(project.Databases) == 0 {
		fmt.Println("  No databases")
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  ID\tNAME\tTYPE\tSTATUS")
		fmt.Fprintln(w, "  --\t----\t----\t------")
		for _, db := range project.Databases {
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n",
				db.ID,
				db.Name,
				db.SpecType,
				db.Status,
			)
		}
		w.Flush()
	}

	return nil
}

// ProjectsCreateCommand represents the projects create command
type ProjectsCreateCommand struct {
	parent *ProjectsCommand
	cmd    *cobra.Command
}

// NewProjectsCreateCommand creates a new projects create command
func NewProjectsCreateCommand(parent *ProjectsCommand) *ProjectsCreateCommand {
	c := &ProjectsCreateCommand{
		parent: parent,
	}

	c.cmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new project",
		Long: `Create a new project with an interactive wizard.

This command will guide you through the process of creating a new project,
including selecting the plan type and region.

Examples:
  kamui projects create`,
		RunE: c.Run,
	}

	return c
}

// Command returns the underlying cobra command
func (c *ProjectsCreateCommand) Command() *cobra.Command {
	return c.cmd
}

// Run executes the projects create command with interactive wizard
func (c *ProjectsCreateCommand) Run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	projectService := c.parent.Root().Container().ProjectService()

	// Step 1: Project name
	var name string
	if err := survey.AskOne(&survey.Input{
		Message: "Project name:",
	}, &name, survey.WithValidator(survey.Required)); err != nil {
		return err
	}

	// Step 2: Description (optional)
	var description string
	if err := survey.AskOne(&survey.Input{
		Message: "Description (optional, max 80 chars):",
	}, &description); err != nil {
		return err
	}

	// Truncate description if too long
	if len(description) > 80 {
		description = description[:80]
	}

	// Step 3: Plan type
	planTypes := []string{"Free", "Pro"}
	planTypeMap := map[string]string{
		"Free": "free",
		"Pro":  "pro",
	}

	var selectedPlan string
	if err := survey.AskOne(&survey.Select{
		Message: "Plan type:",
		Options: planTypes,
		Default: "Free",
	}, &selectedPlan); err != nil {
		return err
	}

	planType := planTypeMap[selectedPlan]

	// Step 4: Region
	regions := []string{"Tokyo", "Singapore"}
	regionMap := map[string]string{
		"Tokyo":     "tokyo",
		"Singapore": "singapore",
	}

	var selectedRegion string
	if err := survey.AskOne(&survey.Select{
		Message: "Region:",
		Options: regions,
		Default: "Tokyo",
	}, &selectedRegion); err != nil {
		return err
	}

	region := regionMap[selectedRegion]

	// Create the project
	fmt.Println("\nCreating project...")

	input := &iface.CreateProjectInput{
		Name:        name,
		Description: description,
		PlanType:    planType,
		Region:      region,
	}

	if err := projectService.CreateProject(ctx, input); err != nil {
		return err
	}

	fmt.Printf("\n✓ Project \"%s\" created successfully!\n", name)
	fmt.Printf("  Plan:   %s\n", planType)
	fmt.Printf("  Region: %s\n", region)
	fmt.Println("\nNext steps:")
	fmt.Printf("  kamui projects list          - View your projects\n")
	fmt.Printf("  kamui apps create            - Create an app in this project\n")

	return nil
}

// ProjectsDeleteCommand represents the projects delete command
type ProjectsDeleteCommand struct {
	parent *ProjectsCommand
	cmd    *cobra.Command
}

// NewProjectsDeleteCommand creates a new projects delete command
func NewProjectsDeleteCommand(parent *ProjectsCommand) *ProjectsDeleteCommand {
	d := &ProjectsDeleteCommand{
		parent: parent,
	}

	d.cmd = &cobra.Command{
		Use:   "delete <project-name-or-id>",
		Short: "Delete a project",
		Long: `Delete a project and all its resources.

You can specify the project by name or ID. The command will search for
a matching project and confirm before deletion.

WARNING: This action is irreversible. All apps, databases, and other resources
in the project will be permanently deleted.

Examples:
  kamui projects delete my-project
  kamui projects delete 5f809f2f-0787-40ca-9a43-a3a59edb5400`,
		Args: cobra.ExactArgs(1),
		RunE: d.Run,
	}

	d.cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	return d
}

// Command returns the underlying cobra command
func (d *ProjectsDeleteCommand) Command() *cobra.Command {
	return d.cmd
}

// Run executes the projects delete command
func (d *ProjectsDeleteCommand) Run(cmd *cobra.Command, args []string) error {
	nameOrID := args[0]
	ctx := cmd.Context()

	projectService := d.parent.Root().Container().ProjectService()

	// Fetch all projects to find by name or ID
	projects, err := projectService.ListProjects(ctx)
	if err != nil {
		return err
	}

	// Find matching project by name or ID
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

	// Check for --yes flag
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	if !skipConfirm {
		// Show warning
		fmt.Printf("\n⚠️  WARNING: You are about to delete the following project:\n\n")
		fmt.Printf("  Name:   %s\n", project.Name)
		fmt.Printf("  ID:     %s\n", project.ID)
		fmt.Printf("  Apps:   %d\n", len(project.Apps))
		fmt.Printf("  DBs:    %d\n", len(project.Databases))
		fmt.Println("\n  This action is IRREVERSIBLE. All resources will be permanently deleted.")

		// Confirmation prompt
		var confirm bool
		if err := survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("Are you sure you want to delete project \"%s\"?", project.Name),
			Default: false,
		}, &confirm); err != nil {
			return err
		}

		if !confirm {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	fmt.Println("\nDeleting project...")

	if err := projectService.DeleteProject(ctx, project.ID); err != nil {
		return err
	}

	fmt.Printf("\n✓ Project \"%s\" deleted successfully.\n", project.Name)

	return nil
}
