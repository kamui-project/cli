package cmd

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocateClientConfigs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME override semantics differ on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	cases := []struct {
		name string
		fn   func() (string, error)
		want string
	}{
		{"claude", LocateClaudeConfig, filepath.Join(home, ".claude.json")},
		{"cursor", LocateCursorConfig, filepath.Join(home, ".cursor", "mcp.json")},
		{"codex", LocateCodexConfig, filepath.Join(home, ".codex", "config.toml")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.fn()
			if err != nil {
				t.Fatalf("locate: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsRegisterableMCPClient(t *testing.T) {
	cases := map[string]bool{
		"claude-code": true,
		"cursor":      true,
		"codex":       true,
		"all":         false,
		"":            false,
		"unknown":     false,
	}
	for in, want := range cases {
		if got := isRegisterableMCPClient(in); got != want {
			t.Errorf("isRegisterableMCPClient(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestMcpClientDisplayName(t *testing.T) {
	cases := map[string]string{
		"claude-code": "Claude Code",
		"cursor":      "Cursor",
		"codex":       "Codex",
	}
	for in, want := range cases {
		if got := mcpClientDisplayName(in); got != want {
			t.Errorf("mcpClientDisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRegisterMCPClient_CursorEndToEnd exercises the dispatch path for
// cursor: it should route through RegisterMCPServer and land the entry
// in ~/.cursor/mcp.json under the temp HOME we set up. This is the
// integration-level guarantee that the Cursor safe path actually
// writes to disk without leaking the token through any side channel.
func TestRegisterMCPClient_CursorEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME override semantics differ on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := registerMCPClient(nil, mcpClientCursor, "https://api.test", "secret-pat"); err != nil {
		t.Fatalf("registerMCPClient: %v", err)
	}

	root := loadJSON(t, filepath.Join(home, ".cursor", "mcp.json"))
	servers, _ := root["mcpServers"].(map[string]any)
	kamui, _ := servers["kamui"].(map[string]any)
	if kamui["url"] != "https://api.test/mcp" {
		t.Errorf("url = %v, want https://api.test/mcp", kamui["url"])
	}
	headers, _ := kamui["headers"].(map[string]any)
	if !strings.Contains(headers["Authorization"].(string), "secret-pat") {
		t.Errorf("Authorization missing token: %v", headers["Authorization"])
	}
}

// TestRegisterMCPClient_CodexEndToEnd is the same guarantee for codex —
// dispatch should land the kamui entry in ~/.codex/config.toml as TOML.
func TestRegisterMCPClient_CodexEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME override semantics differ on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := registerMCPClient(nil, mcpClientCodex, "https://api.test", "secret-pat"); err != nil {
		t.Fatalf("registerMCPClient: %v", err)
	}

	root := loadTOML(t, filepath.Join(home, ".codex", "config.toml"))
	servers, _ := root["mcp_servers"].(map[string]any)
	kamui, _ := servers["kamui"].(map[string]any)
	if kamui["url"] != "https://api.test/mcp" {
		t.Errorf("url = %v, want https://api.test/mcp", kamui["url"])
	}
	headers, _ := kamui["http_headers"].(map[string]any)
	if !strings.Contains(headers["Authorization"].(string), "secret-pat") {
		t.Errorf("Authorization missing token: %v", headers["Authorization"])
	}
}
