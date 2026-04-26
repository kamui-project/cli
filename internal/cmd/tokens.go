package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
	"github.com/spf13/cobra"
)

// TokensCommand groups the `tokens` subcommands for managing PATs.
type TokensCommand struct {
	root *RootCommand
	cmd  *cobra.Command

	createCmd *TokensCreateCommand
	listCmd   *TokensListCommand
	deleteCmd *TokensDeleteCommand
}

// NewTokensCommand creates the tokens command group.
func NewTokensCommand(root *RootCommand) *TokensCommand {
	t := &TokensCommand{root: root}

	t.cmd = &cobra.Command{
		Use:   "tokens",
		Short: "Manage Personal Access Tokens (PAT)",
		Long: `Manage Personal Access Tokens (PATs) for the Kamui API.

A PAT is a long-lived bearer token you can use to authenticate API requests
from scripts, CI, or MCP clients (e.g. Claude Code).

Examples:
  kamui tokens create --name "claude-code-mcp" --days 90
  kamui tokens list
  kamui tokens delete <id>`,
	}

	t.createCmd = NewTokensCreateCommand(t)
	t.listCmd = NewTokensListCommand(t)
	t.deleteCmd = NewTokensDeleteCommand(t)

	t.cmd.AddCommand(t.createCmd.Command())
	t.cmd.AddCommand(t.listCmd.Command())
	t.cmd.AddCommand(t.deleteCmd.Command())

	return t
}

func (t *TokensCommand) Command() *cobra.Command { return t.cmd }
func (t *TokensCommand) Root() *RootCommand      { return t.root }

// ── tokens create ────────────────────────────────────────────────────────────

type TokensCreateCommand struct {
	parent *TokensCommand
	cmd    *cobra.Command

	name string
	days int
}

func NewTokensCreateCommand(parent *TokensCommand) *TokensCreateCommand {
	c := &TokensCreateCommand{parent: parent}
	c.cmd = &cobra.Command{
		Use:   "create",
		Short: "Issue a new Personal Access Token",
		Long: `Create a new PAT and print the plaintext token to stdout exactly once.

⚠️  The token is shown only here. Save it now (e.g. pipe to a clipboard tool
or redirect to a file with chmod 600). It cannot be retrieved later.

Examples:
  kamui tokens create --name "ci"
  kamui tokens create --name "claude-code" --days 90`,
		RunE: c.Run,
	}
	c.cmd.Flags().StringVar(&c.name, "name", "", "Token identifier (required, max 50 chars)")
	c.cmd.Flags().IntVar(&c.days, "days", 30, "Validity in days (1-365)")
	_ = c.cmd.MarkFlagRequired("name")
	return c
}

func (c *TokensCreateCommand) Command() *cobra.Command { return c.cmd }

func (c *TokensCreateCommand) Run(cmd *cobra.Command, _ []string) error {
	if c.days < 1 || c.days > 365 {
		return fmt.Errorf("--days must be between 1 and 365 (got %d)", c.days)
	}
	if len(c.name) == 0 || len(c.name) > 50 {
		return fmt.Errorf("--name must be 1-50 characters (got %d)", len(c.name))
	}

	tokens := c.parent.Root().Container().TokensService()
	plaintext, id, err := tokens.Create(cmd.Context(), c.name, c.days)
	if err != nil {
		return err
	}

	apiURL, _ := c.parent.Root().Container().ConfigManager().GetAPIURL()
	printPATCreated(id, c.name, c.days)
	printMCPSetupInstructions(apiURL, plaintext, mcpClientAll)

	// Plaintext goes to stdout so you can pipe / redirect cleanly.
	fmt.Println(plaintext)
	return nil
}

// ── tokens list ──────────────────────────────────────────────────────────────

type TokensListCommand struct {
	parent *TokensCommand
	cmd    *cobra.Command

	includeOAuth bool
}

func NewTokensListCommand(parent *TokensCommand) *TokensListCommand {
	l := &TokensListCommand{parent: parent}
	l.cmd = &cobra.Command{
		Use:   "list",
		Short: "List your Personal Access Tokens",
		Long: `List PATs you have issued. By default, short-lived OAuth session tokens
created automatically by 'kamui login' are hidden — pass --all to see them.

Examples:
  kamui tokens list
  kamui tokens list --all
  kamui tokens list -o json`,
		RunE: l.Run,
	}
	l.cmd.Flags().BoolVar(&l.includeOAuth, "all", false, "Also show internal OAuth session tokens")
	return l
}

func (l *TokensListCommand) Command() *cobra.Command { return l.cmd }

func (l *TokensListCommand) Run(cmd *cobra.Command, _ []string) error {
	tokens := l.parent.Root().Container().TokensService()
	pats, err := tokens.List(cmd.Context(), l.includeOAuth)
	if err != nil {
		return err
	}

	outputFormat, _ := cmd.Flags().GetString("output")
	if outputFormat == "" {
		outputFormat, _ = cmd.Parent().Parent().PersistentFlags().GetString("output")
	}

	switch outputFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pats)
	default:
		return l.outputTable(pats)
	}
}

func (l *TokensListCommand) outputTable(pats []iface.PATInfo) error {
	if len(pats) == 0 {
		fmt.Println("No tokens found.")
		fmt.Println("\nCreate a new token with: kamui tokens create --name <name>")
		return nil
	}
	rows := make([][]string, 0, len(pats))
	for _, p := range pats {
		lastUsed := "never"
		if p.LastUsedAt != nil {
			lastUsed = *p.LastUsedAt
		}
		rows = append(rows, []string{p.ID, p.Name, p.ExpiresAt, lastUsed})
	}
	printTable(os.Stdout, "", []string{"ID", "NAME", "EXPIRES", "LAST USED"}, rows)
	return nil
}

// ── tokens delete ────────────────────────────────────────────────────────────

type TokensDeleteCommand struct {
	parent *TokensCommand
	cmd    *cobra.Command

	yes bool
}

func NewTokensDeleteCommand(parent *TokensCommand) *TokensDeleteCommand {
	d := &TokensDeleteCommand{parent: parent}
	d.cmd = &cobra.Command{
		Use:   "delete <id>",
		Short: "Revoke a Personal Access Token",
		Long: `Delete a PAT by its ID. Subsequent API calls using the deleted
token will return 401.

Examples:
  kamui tokens delete 12345678-abcd-1234-...
  kamui tokens delete 12345678-abcd-1234-... --yes`,
		Args: cobra.ExactArgs(1),
		RunE: d.Run,
	}
	d.cmd.Flags().BoolVar(&d.yes, "yes", false, "Skip confirmation prompt")
	return d
}

func (d *TokensDeleteCommand) Command() *cobra.Command { return d.cmd }

func (d *TokensDeleteCommand) Run(cmd *cobra.Command, args []string) error {
	id := args[0]
	tokens := d.parent.Root().Container().TokensService()

	if !d.yes {
		// Look up the name for the confirmation prompt.
		// Include OAuth session tokens so the user gets an explicit warning
		// if they're about to delete their own active CLI session.
		name, isOAuth := lookupPATName(cmd.Context(), tokens, id)
		fmt.Println("About to delete PAT:")
		fmt.Printf("  ID:   %s\n", id)
		if name != "" {
			fmt.Printf("  Name: %s\n", name)
		}
		if isOAuth {
			fmt.Println("  ⚠️  This is an internal OAuth session token. Deleting it will")
			fmt.Println("      log out the CLI process that owns it. Run 'kamui login' to recover.")
		}

		var confirm bool
		if err := survey.AskOne(&survey.Confirm{
			Message: "Confirm deletion?",
			Default: false,
		}, &confirm); err != nil {
			return err
		}
		if !confirm {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	if err := tokens.Delete(cmd.Context(), id); err != nil {
		return err
	}
	fmt.Println("✓ Token deleted.")
	return nil
}

// lookupPATName fetches the PAT's display name and whether it's an internal
// OAuth session token. Returns ("", false) if the token can't be found
// (e.g. wrong ID) — the delete call itself will surface the appropriate error.
func lookupPATName(ctx context.Context, svc iface.TokensService, id string) (string, bool) {
	pats, err := svc.List(ctx, true) // include OAuth tokens for accurate lookup
	if err != nil {
		return "", false
	}
	for _, p := range pats {
		if p.ID == id {
			return p.Name, len(p.Name) >= 11 && p.Name[:11] == "OAuth Token"
		}
	}
	return "", false
}
