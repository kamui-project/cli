package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// MCP client identifiers used by `kamui mcp config <client>` and `mcp setup --client`.
const (
	mcpClientClaudeCode = "claude-code"
	mcpClientCursor     = "cursor"
	mcpClientCodex      = "codex"
	mcpClientAll        = "all"
)

const (
	tokenPlaceholder = "<YOUR_KAMUI_PAT>"
	defaultAPIURL    = "https://api.kamui-platform.com"
	envTokenKey      = "KAMUI_PAT"
)

// McpCommand groups the `mcp` subcommands for setting up AI clients.
type McpCommand struct {
	root *RootCommand
	cmd  *cobra.Command

	setupCmd  *McpSetupCommand
	configCmd *McpConfigCommand
	testCmd   *McpTestCommand
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
  kamui mcp setup --client claude-code --register   # safest: token never hits stdout
  kamui mcp setup                                   # issue a PAT and print setup instructions
  kamui mcp config cursor                           # print config snippet for an existing token
  kamui mcp test                                    # check connectivity to the MCP server`,
	}

	m.setupCmd = NewMcpSetupCommand(m)
	m.configCmd = NewMcpConfigCommand(m)
	m.testCmd = NewMcpTestCommand(m)
	m.cmd.AddCommand(m.setupCmd.Command())
	m.cmd.AddCommand(m.configCmd.Command())
	m.cmd.AddCommand(m.testCmd.Command())

	return m
}

func (m *McpCommand) Command() *cobra.Command { return m.cmd }
func (m *McpCommand) Root() *RootCommand      { return m.root }

// ── mcp setup ────────────────────────────────────────────────────────────────

type McpSetupCommand struct {
	parent *McpCommand
	cmd    *cobra.Command

	name         string
	days         int
	client       string
	register     bool
	tokenFile    string
	noPrintToken bool
	printToken   bool
}

func NewMcpSetupCommand(parent *McpCommand) *McpSetupCommand {
	s := &McpSetupCommand{parent: parent}
	s.cmd = &cobra.Command{
		Use:   "setup",
		Short: "Issue a PAT and print MCP setup instructions",
		Long: `Issue a Personal Access Token and print everything you need to connect
your AI client to Kamui MCP. The plaintext token is shown only once.

By default, the token is printed to stdout for piping. To avoid leaking it
into logs/transcripts, prefer one of:
  --register             call the client's CLI directly so the token never
                         touches stdout (currently: --client claude-code only)
  --token-file <path>    write the token to a file (mode 0600); stdout stays clean
  --no-print-token       suppress the stdout token entirely

When stdout is not a terminal (pipe, redirect, AI harness), the token is
withheld from stdout by default. Pass --print-token to override.

Examples:
  kamui mcp setup --client claude-code --register
  kamui mcp setup --client cursor --token-file ~/.kamui/pat
  kamui mcp setup --name "macbook-mcp" --days 365`,
		RunE: s.Run,
	}
	s.cmd.Flags().StringVar(&s.name, "name", "", "PAT identifier (default: <hostname>-mcp-<timestamp>)")
	s.cmd.Flags().IntVar(&s.days, "days", 365, "Validity in days (1-365)")
	s.cmd.Flags().StringVar(&s.client, "client", mcpClientAll, "Target client: claude-code | cursor | codex | all")
	s.cmd.Flags().BoolVar(&s.register, "register", false, "Register with the client's CLI directly (claude-code only). Token never goes to stdout.")
	s.cmd.Flags().StringVar(&s.tokenFile, "token-file", "", "Write the plaintext token to this file (mode 0600). Stdout stays clean.")
	s.cmd.Flags().BoolVar(&s.noPrintToken, "no-print-token", false, "Do not print the plaintext token to stdout.")
	s.cmd.Flags().BoolVar(&s.printToken, "print-token", false, "Force printing the token to stdout even when stdout is not a TTY.")
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
	if s.register && s.client != mcpClientClaudeCode {
		return fmt.Errorf("--register currently supports only --client claude-code (got %q)", s.client)
	}
	name := s.name
	if name == "" {
		name = defaultPATName()
	}
	if len(name) > 50 {
		return fmt.Errorf("--name must be 1-50 characters (got %d)", len(name))
	}

	outputFormat := resolveOutputFormat(cmd)

	tokens := s.parent.Root().Container().TokensService()
	plaintext, id, err := tokens.Create(cmd.Context(), name, s.days)
	if err != nil {
		return err
	}

	apiURL, _ := s.parent.Root().Container().ConfigManager().GetAPIURL()
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	// --register: hand the token to the client CLI directly. Never touches stdout.
	if s.register {
		if err := registerClaudeCode(cmd.Context(), apiURL, plaintext); err != nil {
			return fmt.Errorf("registration failed (token id %s — revoke with 'kamui tokens delete %s --yes' if unused): %w", id, id, err)
		}
		if outputFormat == "json" {
			return printSetupJSON(id, name, s.days, "", apiURL, mcpClientClaudeCode, true)
		}
		fmt.Fprintln(os.Stderr, "✓ Personal Access Token created and registered with Claude Code.")
		fmt.Fprintf(os.Stderr, "  ID:      %s\n", id)
		fmt.Fprintf(os.Stderr, "  Name:    %s\n", name)
		fmt.Fprintf(os.Stderr, "  Expires: %d days\n", s.days)
		printRevokeHint(os.Stderr, id)
		return nil
	}

	// --token-file: write to file, do not print to stdout.
	if s.tokenFile != "" {
		if err := writeTokenFile(s.tokenFile, plaintext); err != nil {
			return fmt.Errorf("failed to write token file (token id %s — revoke with 'kamui tokens delete %s --yes' if unused): %w", id, id, err)
		}
		if outputFormat == "json" {
			return printSetupJSON(id, name, s.days, "", apiURL, s.client, false)
		}
		printPATCreated(id, name, s.days)
		fmt.Fprintf(os.Stderr, "  Token written to %s (mode 0600).\n\n", s.tokenFile)
		printMCPSetupInstructions(apiURL, tokenPlaceholder, s.client)
		printRevokeHint(os.Stderr, id)
		return nil
	}

	if outputFormat == "json" {
		// JSON consumers explicitly asked for structured output — they want
		// the token in the parsed result, not embedded in instructional text.
		return printSetupJSON(id, name, s.days, plaintext, apiURL, s.client, false)
	}

	printPATCreated(id, name, s.days)
	printMCPSetupInstructions(apiURL, plaintext, s.client)
	printRevokeHint(os.Stderr, id)

	if shouldPrintTokenToStdout(s.noPrintToken, s.printToken) {
		fmt.Println(plaintext)
	} else if !isStdoutTTY() && !s.noPrintToken {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "ℹ️  stdout is not a terminal — token withheld from stdout to avoid leaking into logs.")
		fmt.Fprintln(os.Stderr, "   Use --print-token to force, --token-file to capture, or --register for the safest path.")
	}
	return nil
}

// ── mcp config <client> ─────────────────────────────────────────────────────

type McpConfigCommand struct {
	parent *McpCommand
	cmd    *cobra.Command

	tokenFromEnv string
	tokenFile    string
}

func NewMcpConfigCommand(parent *McpCommand) *McpConfigCommand {
	c := &McpConfigCommand{parent: parent}
	c.cmd = &cobra.Command{
		Use:   "config <client>",
		Short: "Print the MCP config snippet for a specific client",
		Long: `Print the MCP config snippet for a specific AI client.

By default the token is shown as a placeholder — replace it with one issued
by 'kamui mcp setup' or 'kamui tokens create'. Use --token-from-env or
--token-file to embed a real token in the printed snippet.

Supported clients: claude-code, cursor, codex, all

Examples:
  kamui mcp config claude-code
  kamui mcp config cursor --token-from-env KAMUI_PAT
  kamui mcp config codex --token-file ~/.kamui/pat`,
		Args: cobra.ExactArgs(1),
		RunE: c.Run,
	}
	c.cmd.Flags().StringVar(&c.tokenFromEnv, "token-from-env", "", "Read token from this environment variable and embed it in the snippet.")
	c.cmd.Flags().StringVar(&c.tokenFile, "token-file", "", "Read token from this file and embed it in the snippet.")
	return c
}

func (c *McpConfigCommand) Command() *cobra.Command { return c.cmd }

func (c *McpConfigCommand) Run(cmd *cobra.Command, args []string) error {
	client := args[0]
	if !isValidMCPClient(client) {
		return fmt.Errorf("client must be one of: claude-code, cursor, codex, all (got %q)", client)
	}
	if c.tokenFromEnv != "" && c.tokenFile != "" {
		return fmt.Errorf("--token-from-env and --token-file are mutually exclusive")
	}

	token := tokenPlaceholder
	switch {
	case c.tokenFromEnv != "":
		v := os.Getenv(c.tokenFromEnv)
		if v == "" {
			return fmt.Errorf("environment variable %s is empty or unset", c.tokenFromEnv)
		}
		token = v
	case c.tokenFile != "":
		v, err := readTokenFile(c.tokenFile)
		if err != nil {
			return err
		}
		token = v
	}

	apiURL := defaultAPIURL
	if cfg := c.parent.Root().Container().ConfigManager(); cfg != nil {
		if u, err := cfg.GetAPIURL(); err == nil && u != "" {
			apiURL = u
		}
	}

	printMCPSetupInstructions(apiURL, token, client)
	return nil
}

// ── mcp test ────────────────────────────────────────────────────────────────

type McpTestCommand struct {
	parent *McpCommand
	cmd    *cobra.Command

	token        string
	tokenFromEnv string
	tokenFile    string
}

func NewMcpTestCommand(parent *McpCommand) *McpTestCommand {
	t := &McpTestCommand{parent: parent}
	t.cmd = &cobra.Command{
		Use:   "test",
		Short: "Verify connectivity to the Kamui MCP server",
		Long: `Send a tools/list call to the Kamui MCP server and report whether the
connection succeeded and how many tools the server exposes.

Token resolution order:
  1. --token <value>
  2. --token-from-env <name>
  3. --token-file <path>
  4. $KAMUI_PAT environment variable
  5. Current logged-in CLI session token

Examples:
  kamui mcp test
  kamui mcp test --token-from-env KAMUI_PAT`,
		RunE: t.Run,
	}
	t.cmd.Flags().StringVar(&t.token, "token", "", "PAT to use (avoid on shared machines — leaks via process list).")
	t.cmd.Flags().StringVar(&t.tokenFromEnv, "token-from-env", "", "Read PAT from this environment variable.")
	t.cmd.Flags().StringVar(&t.tokenFile, "token-file", "", "Read PAT from this file.")
	return t
}

func (t *McpTestCommand) Command() *cobra.Command { return t.cmd }

func (t *McpTestCommand) Run(cmd *cobra.Command, _ []string) error {
	token, err := t.resolveToken()
	if err != nil {
		return err
	}

	apiURL := defaultAPIURL
	if cfg := t.parent.Root().Container().ConfigManager(); cfg != nil {
		if u, err := cfg.GetAPIURL(); err == nil && u != "" {
			apiURL = u
		}
	}
	mcpURL := apiURL + "/mcp"

	count, err := mcpToolsList(cmd.Context(), mcpURL, token)
	if err != nil {
		return fmt.Errorf("MCP test failed for %s: %w", mcpURL, err)
	}
	fmt.Printf("✓ MCP OK — %s exposes %d tools.\n", mcpURL, count)
	return nil
}

func (t *McpTestCommand) resolveToken() (string, error) {
	switch {
	case t.token != "":
		return t.token, nil
	case t.tokenFromEnv != "":
		v := os.Getenv(t.tokenFromEnv)
		if v == "" {
			return "", fmt.Errorf("environment variable %s is empty or unset", t.tokenFromEnv)
		}
		return v, nil
	case t.tokenFile != "":
		return readTokenFile(t.tokenFile)
	}
	if v := os.Getenv(envTokenKey); v != "" {
		return v, nil
	}
	if cfg := t.parent.Root().Container().ConfigManager(); cfg != nil {
		if v, err := cfg.GetAccessToken(); err == nil && v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("no token available — pass --token, --token-from-env, --token-file, set $%s, or run 'kamui login'", envTokenKey)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func isValidMCPClient(c string) bool {
	switch c {
	case mcpClientClaudeCode, mcpClientCursor, mcpClientCodex, mcpClientAll:
		return true
	}
	return false
}

// defaultPATName returns "<hostname>-mcp-YYYYMMDDHHMMSS" so repeated runs
// don't collide and recently-created tokens are easy to identify in 'tokens list'.
func defaultPATName() string {
	hostname, _ := os.Hostname()
	host := strings.ToLower(strings.ReplaceAll(hostname, " ", "-"))
	if host == "" {
		host = "mcp"
	}
	return fmt.Sprintf("%s-mcp-%s", host, time.Now().UTC().Format("20060102150405"))
}

// resolveOutputFormat reads the persistent --output flag from the root command.
// Returns "" when the flag isn't reachable (e.g. tests with detached commands).
func resolveOutputFormat(cmd *cobra.Command) string {
	if v, _ := cmd.Flags().GetString("output"); v != "" && v != "text" {
		return v
	}
	for p := cmd.Parent(); p != nil; p = p.Parent() {
		if v, _ := p.PersistentFlags().GetString("output"); v != "" && v != "text" {
			return v
		}
	}
	return ""
}

// shouldPrintTokenToStdout decides whether the plaintext token belongs on
// stdout. Defaults: TTY → yes, non-TTY → no (avoid leaking into transcripts).
func shouldPrintTokenToStdout(noPrint, force bool) bool {
	if noPrint {
		return false
	}
	if force {
		return true
	}
	return isStdoutTTY()
}

// writeTokenFile writes the token to path with mode 0600. Parent directories
// are created if needed (also mode 0700).
func writeTokenFile(path, token string) error {
	if dir := dirOf(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	// Use O_EXCL? No — overwriting is the expected behavior when re-running setup.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(token + "\n"); err != nil {
		return err
	}
	return nil
}

func readTokenFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read token file %s: %w", path, err)
	}
	v := strings.TrimSpace(string(b))
	if v == "" {
		return "", fmt.Errorf("token file %s is empty", path)
	}
	return v, nil
}

func dirOf(path string) string {
	idx := strings.LastIndexAny(path, "/\\")
	if idx <= 0 {
		return ""
	}
	return path[:idx]
}

// registerClaudeCode runs `claude mcp add` to register the Kamui MCP server
// with the local Claude Code CLI. The token is passed via argv to the child
// process; it never touches the parent's stdout/stderr.
func registerClaudeCode(ctx context.Context, apiURL, token string) error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("'claude' CLI not found in PATH — install Claude Code first (https://claude.com/claude-code)")
	}
	mcpURL := apiURL + "/mcp"
	args := []string{
		"mcp", "add",
		"--transport", "http",
		"kamui",
		mcpURL,
		"--header", "Authorization: Bearer " + token,
	}
	c := exec.CommandContext(ctx, "claude", args...)
	// Forward stderr so the user can see registration warnings, but DO NOT
	// connect stdout to anything that could capture the token.
	c.Stdout = nil
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("'claude mcp add' exited with error: %w", err)
	}
	return nil
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

func printRevokeHint(w *os.File, id string) {
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "Revoke any time:  kamui tokens delete %s --yes\n", id)
}

// printMCPSetupInstructions writes copy-pasteable MCP client config to stderr.
// `token` may be a real token or `tokenPlaceholder`. `client` selects which
// snippets are printed (use mcpClientAll for everything).
func printMCPSetupInstructions(apiURL, token, client string) {
	if apiURL == "" {
		apiURL = defaultAPIURL
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
	fmt.Fprintln(w, "    kamui mcp test")
	fmt.Fprintln(w, "────────────────────────────────────────────────────────────")
}

// setupJSON is the structured output for `mcp setup -o json` and
// `tokens create -o json`. Token is omitted when --register or --token-file
// kept it off stdout.
type setupJSON struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ExpiresAt  string `json:"expires_at"`
	Token      string `json:"token,omitempty"`
	Client     string `json:"client,omitempty"`
	Registered bool   `json:"registered,omitempty"`
	APIURL     string `json:"api_url,omitempty"`
	MCPURL     string `json:"mcp_url,omitempty"`
}

func printSetupJSON(id, name string, days int, token, apiURL, client string, registered bool) error {
	expires := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
	out := setupJSON{
		ID:         id,
		Name:       name,
		ExpiresAt:  expires,
		Token:      token,
		Client:     client,
		Registered: registered,
		APIURL:     apiURL,
		MCPURL:     apiURL + "/mcp",
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
