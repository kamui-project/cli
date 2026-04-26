package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

// cursorConfigPath is the user-scope MCP config Cursor reads on startup.
// Cursor's HTTP MCP entry uses the same JSON shape as Claude Code, so
// RegisterMCPServer (the JSON merger) handles it without a dedicated
// codepath — only the file location differs.
const cursorConfigPath = ".cursor/mcp.json"

// LocateCursorConfig returns the absolute path to ~/.cursor/mcp.json.
// The file may not exist yet; RegisterMCPServer will create it.
func LocateCursorConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, cursorConfigPath), nil
}
