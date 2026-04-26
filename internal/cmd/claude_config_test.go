package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRegisterMCPServer_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")

	entry := MCPHTTPEntry{
		Type: "http",
		URL:  "https://api.kamui-platform.com/mcp",
		Headers: map[string]string{
			"Authorization": "Bearer secret-token",
		},
	}

	if err := RegisterMCPServer(path, "kamui", entry); err != nil {
		t.Fatalf("RegisterMCPServer: %v", err)
	}

	root := loadJSON(t, path)
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers missing or wrong type: %#v", root["mcpServers"])
	}
	got, ok := servers["kamui"].(map[string]any)
	if !ok {
		t.Fatalf("kamui entry missing or wrong type: %#v", servers["kamui"])
	}
	if got["url"] != entry.URL {
		t.Errorf("url = %v, want %v", got["url"], entry.URL)
	}
	headers, _ := got["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer secret-token" {
		t.Errorf("Authorization header = %v, want Bearer secret-token", headers["Authorization"])
	}
	assertMode0600(t, path)
}

func TestRegisterMCPServer_PreservesUnrelatedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")

	existing := map[string]any{
		"theme": "dark",
		"mcpServers": map[string]any{
			"other": map[string]any{
				"type":    "stdio",
				"command": "other-server",
			},
		},
	}
	writeJSON(t, path, existing)

	entry := MCPHTTPEntry{Type: "http", URL: "https://example.test/mcp"}
	if err := RegisterMCPServer(path, "kamui", entry); err != nil {
		t.Fatalf("RegisterMCPServer: %v", err)
	}

	root := loadJSON(t, path)
	if root["theme"] != "dark" {
		t.Errorf("top-level theme key was clobbered: %v", root["theme"])
	}
	servers, _ := root["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Errorf("sibling MCP server 'other' was clobbered: %#v", servers)
	}
	if _, ok := servers["kamui"]; !ok {
		t.Errorf("kamui entry was not added: %#v", servers)
	}
}

func TestRegisterMCPServer_OverwritesSameName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")

	if err := RegisterMCPServer(path, "kamui", MCPHTTPEntry{Type: "http", URL: "https://old.test/mcp"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := RegisterMCPServer(path, "kamui", MCPHTTPEntry{Type: "http", URL: "https://new.test/mcp"}); err != nil {
		t.Fatalf("second register: %v", err)
	}

	root := loadJSON(t, path)
	servers, _ := root["mcpServers"].(map[string]any)
	got, _ := servers["kamui"].(map[string]any)
	if got["url"] != "https://new.test/mcp" {
		t.Errorf("url not updated: got %v", got["url"])
	}
}

func TestRegisterMCPServer_RefusesSymlinkDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".claude.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	err := RegisterMCPServer(link, "kamui", MCPHTTPEntry{Type: "http", URL: "https://x.test/mcp"})
	if err == nil {
		t.Fatal("expected symlink destination to be refused")
	}

	// The symlink target must remain the original empty object — the
	// rename must not have followed the link.
	b, _ := os.ReadFile(target)
	if string(b) != "{}" {
		t.Errorf("symlink target was modified: %q", string(b))
	}
}

func TestRegisterMCPServer_EmptyName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	if err := RegisterMCPServer(path, "", MCPHTTPEntry{Type: "http"}); err == nil {
		t.Fatal("expected empty name to error")
	}
}

func loadJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return m
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertMode0600(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("%s mode = %#o, want 0600", path, perm)
	}
}
