package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// LoginCommand represents the login command
type LoginCommand struct {
	root *RootCommand
	cmd  *cobra.Command
}

// NewLoginCommand creates a new login command
func NewLoginCommand(root *RootCommand) *LoginCommand {
	l := &LoginCommand{
		root: root,
	}

	l.cmd = &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Kamui Platform",
		Long: `Authenticate with the Kamui Platform using your GitHub account.

This command will open a browser window for you to authenticate with GitHub.
After successful authentication, your credentials will be stored locally.

Example:
  kamui login`,
		RunE: l.Run,
	}

	return l
}

// Command returns the underlying cobra command
func (l *LoginCommand) Command() *cobra.Command {
	return l.cmd
}

// Run executes the login command
func (l *LoginCommand) Run(cmd *cobra.Command, args []string) error {
	// Get auth service from DI container
	authService := l.root.Container().AuthService()

	// Perform login
	if err := authService.Login(cmd.Context()); err != nil {
		return err
	}

	fmt.Println("✓ Successfully logged in to Kamui Platform!")
	return nil
}
