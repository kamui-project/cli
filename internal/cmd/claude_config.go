package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// claudeConfigFile is the documented user-scope settings file Claude Code
// reads MCP server registrations from. Writing it directly avoids handing
// the bearer token to a child process via argv.
const claudeConfigFile = ".claude.json"

// MCPHTTPEntry is the JSON shape Claude Code expects for an HTTP-transport
// MCP server registration under mcpServers.<name>. Fields mirror the
// Claude Code MCP schema; extra fields in the existing config are
// preserved by RegisterMCPServer when merging.
type MCPHTTPEntry struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// LocateClaudeConfig returns the absolute path to the user-scope Claude
// Code settings file (~/.claude.json). It does not require the file to
// exist; it just resolves the path.
func LocateClaudeConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, claudeConfigFile), nil
}

// RegisterMCPServer merges a single mcpServers.<name> entry into the
// Claude Code config at path. The rest of the file is preserved: top-
// level keys outside mcpServers and sibling MCP servers are untouched.
// Persistence goes through atomicWrite0600, so the destination ends up
// at mode 0o600 and a symlink at the destination path is refused.
func RegisterMCPServer(path, name string, entry MCPHTTPEntry) error {
	if name == "" {
		return fmt.Errorf("mcp server name is empty")
	}

	root, err := loadJSONRoot(path)
	if err != nil {
		return err
	}

	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers[name] = entry
	root["mcpServers"] = servers

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config %s: %w", path, err)
	}

	return atomicWrite0600(path, data)
}

// loadJSONRoot reads and parses an MCP-client config file as a generic
// JSON object so unknown keys round-trip safely. Missing or empty files
// are treated as an empty object — that's the documented bootstrap path
// for a fresh install.
func loadJSONRoot(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if root == nil {
		root = map[string]any{}
	}
	return root, nil
}
