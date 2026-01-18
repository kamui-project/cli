// Package cmd provides the command-line interface for the Kamui CLI.
// It contains all cobra commands and their implementations.
package cmd

import (
	"fmt"
	"os"

	"github.com/kamui-project/kamui-cli/internal/di"
	"github.com/spf13/cobra"
)

var (
	// Version is set at build time via ldflags
	Version = "dev"
)

// RootCommand represents the root CLI command
type RootCommand struct {
	container *di.Container
	cmd       *cobra.Command

	// Subcommands
	loginCmd    *LoginCommand
	logoutCmd   *LogoutCommand
	projectsCmd *ProjectsCommand
	appsCmd     *AppsCommand
}

// NewRootCommand creates a new root command
func NewRootCommand() *RootCommand {
	r := &RootCommand{}

	r.cmd = &cobra.Command{
		Use:   "kamui",
		Short: "Kamui CLI - Command line interface for Kamui Platform",
		Long: `Kamui CLI is a command-line tool for interacting with the Kamui Platform.

Kamui Platform is a PaaS (Platform as a Service) that allows you to deploy
and manage applications, databases, and cron jobs with ease.

To get started, run:
  kamui login    - Authenticate with your Kamui account
  kamui projects list - View your projects`,
		Version: Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return r.initialize()
		},
	}

	// Global flags
	r.cmd.PersistentFlags().StringP("output", "o", "text", "Output format (text, json)")

	// Initialize subcommands (will be wired after container init)
	r.loginCmd = NewLoginCommand(r)
	r.logoutCmd = NewLogoutCommand(r)
	r.projectsCmd = NewProjectsCommand(r)
	r.appsCmd = NewAppsCommand(r)

	// Add subcommands
	r.cmd.AddCommand(r.loginCmd.Command())
	r.cmd.AddCommand(r.logoutCmd.Command())
	r.cmd.AddCommand(r.projectsCmd.Command())
	r.cmd.AddCommand(r.appsCmd.Command())

	return r
}

// initialize sets up the DI container
func (r *RootCommand) initialize() error {
	// Skip if container is already set (e.g., for testing)
	if r.container != nil {
		return nil
	}

	var err error
	r.container, err = di.NewContainer()
	if err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}
	return nil
}

// Execute runs the root command
func (r *RootCommand) Execute() error {
	return r.cmd.Execute()
}

// Command returns the underlying cobra command
func (r *RootCommand) Command() *cobra.Command {
	return r.cmd
}

// Container returns the DI container
func (r *RootCommand) Container() *di.Container {
	return r.container
}

// SetContainer sets a custom container (for testing)
func (r *RootCommand) SetContainer(c *di.Container) {
	r.container = c
}

// Execute is the main entry point for the CLI
func Execute() error {
	root := NewRootCommand()
	return root.Execute()
}

// ExitWithError prints an error message and exits with code 1
func ExitWithError(msg string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s: %v\n", msg, err)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
	}
	os.Exit(1)
}
