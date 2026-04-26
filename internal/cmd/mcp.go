package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// MCP client identifiers used by `kamui mcp config <client>` and `mcp setup --client`.
const (
	mcpClientClaudeCode = "claude-code"
	mcpClientCursor     = "cursor"
	mcpClientCodex      = "codex"
	mcpClientAll        = "all"
)

const tokenPlaceholder = "<YOUR_KAMUI_PAT>"

// McpCommand groups the `mcp` subcommands for setting up AI clients.
type McpCommand struct {
	root *RootCommand
	cmd  *cobra.Command

	setupCmd  *McpSetupCommand
	configCmd *McpConfigCommand
}

func NewMcpCommand(root *RootCommand) *McpCommand {
	m := &McpCommand{root: root}

	m.cmd = &cobra.Command{
		Use:   "mcp",
		Short: "Configure MCP integrations (Claude Code, Cursor, Codex)",
		Long: `Connect AI clients to the Kamui MCP server.

The Kamui MCP server lets AI clients (Claude Code, Cursor, Codex, etc.)
manage your projects, deploy apps, and read logs through tool calls.

Quickest path:
  kamui mcp setup       # issue a PAT and print setup instructions
  kamui mcp config cursor   # print config snippet for an existing token`,
	}

	m.setupCmd = NewMcpSetupCommand(m)
	m.configCmd = NewMcpConfigCommand(m)
	m.cmd.AddCommand(m.setupCmd.Command())
	m.cmd.AddCommand(m.configCmd.Command())

	return m
}

func (m *McpCommand) Command() *cobra.Command { return m.cmd }
func (m *McpCommand) Root() *RootCommand      { return m.root }

// ── mcp setup ────────────────────────────────────────────────────────────────

type McpSetupCommand struct {
	parent *McpCommand
	cmd    *cobra.Command

	name   string
	days   int
	client string
}

func NewMcpSetupCommand(parent *McpCommand) *McpSetupCommand {
	s := &McpSetupCommand{parent: parent}
	s.cmd = &cobra.Command{
		Use:   "setup",
		Short: "Issue a PAT and print MCP setup instructions",
		Long: `Issue a Personal Access Token and print everything you need to connect
your AI client to Kamui MCP. The plaintext token is shown only once.

Examples:
  kamui mcp setup
  kamui mcp setup --client claude-code
  kamui mcp setup --name "macbook-mcp" --days 365`,
		RunE: s.Run,
	}
	hostname, _ := os.Hostname()
	defaultName := strings.ToLower(strings.ReplaceAll(hostname, " ", "-")) + "-mcp"
	if hostname == "" {
		defaultName = "mcp"
	}
	s.cmd.Flags().StringVar(&s.name, "name", defaultName, "PAT identifier")
	s.cmd.Flags().IntVar(&s.days, "days", 365, "Validity in days (1-365)")
	s.cmd.Flags().StringVar(&s.client, "client", mcpClientAll, "Target client: claude-code | cursor | codex | all")
	return s
}

func (s *McpSetupCommand) Command() *cobra.Command { return s.cmd }

func (s *McpSetupCommand) Run(cmd *cobra.Command, _ []string) error {
	if s.days < 1 || s.days > 365 {
		return fmt.Errorf("--days must be between 1 and 365 (got %d)", s.days)
	}
	if !isValidMCPClient(s.client) {
		return fmt.Errorf("--client must be one of: claude-code, cursor, codex, all (got %q)", s.client)
	}

	tokens := s.parent.Root().Container().TokensService()
	plaintext, id, err := tokens.Create(cmd.Context(), s.name, s.days)
	if err != nil {
		return err
	}

	apiURL, _ := s.parent.Root().Container().ConfigManager().GetAPIURL()
	printPATCreated(id, s.name, s.days)
	printMCPSetupInstructions(apiURL, plaintext, s.client)

	// stdout: just the plaintext token (so it's still pipe-friendly).
	fmt.Println(plaintext)
	return nil
}

// ── mcp config <client> ─────────────────────────────────────────────────────

type McpConfigCommand struct {
	parent *McpCommand
	cmd    *cobra.Command
}

func NewMcpConfigCommand(parent *McpCommand) *McpConfigCommand {
	c := &McpConfigCommand{parent: parent}
	c.cmd = &cobra.Command{
		Use:   "config <client>",
		Short: "Print the MCP config snippet for a specific client",
		Long: `Print the MCP config snippet for a specific AI client. The token is
shown as a placeholder — replace it with one issued by 'kamui mcp setup' or
'kamui tokens create'.

Supported clients: claude-code, cursor, codex, all

Examples:
  kamui mcp config claude-code
  kamui mcp config cursor`,
		Args: cobra.ExactArgs(1),
		RunE: c.Run,
	}
	return c
}

func (c *McpConfigCommand) Command() *cobra.Command { return c.cmd }

func (c *McpConfigCommand) Run(cmd *cobra.Command, args []string) error {
	client := args[0]
	if !isValidMCPClient(client) {
		return fmt.Errorf("client must be one of: claude-code, cursor, codex, all (got %q)", client)
	}

	// API URL: prefer config; fall back to default if user isn't logged in.
	apiURL := "https://api.kamui-platform.com"
	if cfg := c.parent.Root().Container().ConfigManager(); cfg != nil {
		if u, err := cfg.GetAPIURL(); err == nil && u != "" {
			apiURL = u
		}
	}

	printMCPSetupInstructions(apiURL, tokenPlaceholder, client)
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func isValidMCPClient(c string) bool {
	switch c {
	case mcpClientClaudeCode, mcpClientCursor, mcpClientCodex, mcpClientAll:
		return true
	}
	return false
}

// printPATCreated prints the token-creation header to stderr.
// Shared by `kamui tokens create` and `kamui mcp setup`.
func printPATCreated(id, name string, days int) {
	fmt.Fprintln(os.Stderr, "✓ Personal Access Token created.")
	fmt.Fprintf(os.Stderr, "  ID:      %s\n", id)
	fmt.Fprintf(os.Stderr, "  Name:    %s\n", name)
	fmt.Fprintf(os.Stderr, "  Expires: %d days\n", days)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "⚠️  TOKEN (shown only once — save it now):")
	fmt.Fprintln(os.Stderr, "")
}

// printMCPSetupInstructions writes copy-pasteable MCP client config to stderr.
// `token` may be a real token or `tokenPlaceholder`. `client` selects which
// snippets are printed (use mcpClientAll for everything).
func printMCPSetupInstructions(apiURL, token, client string) {
	if apiURL == "" {
		apiURL = "https://api.kamui-platform.com"
	}
	mcpURL := apiURL + "/mcp"

	w := os.Stderr
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "────────────────────────────────────────────────────────────")
	fmt.Fprintln(w, "Connect an AI client to Kamui MCP:")

	if client == mcpClientClaudeCode || client == mcpClientAll {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "▸ Claude Code")
		fmt.Fprintf(w, "    claude mcp add --transport http kamui \\\n")
		fmt.Fprintf(w, "      %s \\\n", mcpURL)
		fmt.Fprintf(w, "      --header \"Authorization: Bearer %s\"\n", token)
	}

	if client == mcpClientCursor || client == mcpClientAll {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "▸ Cursor (~/.cursor/mcp.json)")
		fmt.Fprintln(w, "    {")
		fmt.Fprintln(w, "      \"mcpServers\": {")
		fmt.Fprintln(w, "        \"kamui\": {")
		fmt.Fprintln(w, "          \"type\": \"http\",")
		fmt.Fprintf(w, "          \"url\": \"%s\",\n", mcpURL)
		fmt.Fprintf(w, "          \"headers\": { \"Authorization\": \"Bearer %s\" }\n", token)
		fmt.Fprintln(w, "        }")
		fmt.Fprintln(w, "      }")
		fmt.Fprintln(w, "    }")
	}

	if client == mcpClientCodex || client == mcpClientAll {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "▸ Codex (~/.codex/config.toml)")
		fmt.Fprintln(w, "    [mcp_servers.kamui]")
		fmt.Fprintf(w, "    url = \"%s\"\n", mcpURL)
		fmt.Fprintf(w, "    headers = { Authorization = \"Bearer %s\" }\n", token)
	}

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "▸ Test the connection:")
	fmt.Fprintf(w, "    curl -H \"Authorization: Bearer %s\" \\\n", token)
	fmt.Fprintf(w, "      %s/api/projects | jq\n", apiURL)
	fmt.Fprintln(w, "────────────────────────────────────────────────────────────")
}
