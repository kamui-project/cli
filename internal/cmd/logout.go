package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// LogoutCommand represents the logout command
type LogoutCommand struct {
	root *RootCommand
	cmd  *cobra.Command
}

// NewLogoutCommand creates a new logout command
func NewLogoutCommand(root *RootCommand) *LogoutCommand {
	l := &LogoutCommand{
		root: root,
	}

	l.cmd = &cobra.Command{
		Use:   "logout",
		Short: "Log out from Kamui Platform",
		Long: `Log out from the Kamui Platform and clear stored credentials.

This command removes your authentication tokens from local storage.

Example:
  kamui logout`,
		RunE: l.Run,
	}

	return l
}

// Command returns the underlying cobra command
func (l *LogoutCommand) Command() *cobra.Command {
	return l.cmd
}

// Run executes the logout command
func (l *LogoutCommand) Run(cmd *cobra.Command, args []string) error {
	// Get auth service from DI container
	authService := l.root.Container().AuthService()

	// Perform logout
	if err := authService.Logout(cmd.Context()); err != nil {
		return err
	}

	fmt.Println("✓ Successfully logged out from Kamui Platform!")
	return nil
}
