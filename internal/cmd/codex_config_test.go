package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestRegisterCodexMCPServer_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	err := RegisterCodexMCPServer(path, "kamui",
		"https://api.kamui-platform.com/mcp",
		map[string]string{"Authorization": "Bearer secret-token"},
	)
	if err != nil {
		t.Fatalf("RegisterCodexMCPServer: %v", err)
	}

	root := loadTOML(t, path)
	servers, ok := root["mcp_servers"].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers missing or wrong type: %#v", root["mcp_servers"])
	}
	got, ok := servers["kamui"].(map[string]any)
	if !ok {
		t.Fatalf("kamui entry missing or wrong type: %#v", servers["kamui"])
	}
	if got["url"] != "https://api.kamui-platform.com/mcp" {
		t.Errorf("url = %v, want kamui mcp url", got["url"])
	}
	headers, _ := got["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer secret-token" {
		t.Errorf("Authorization = %v, want Bearer secret-token", headers["Authorization"])
	}
	assertMode0600(t, path)
}

func TestRegisterCodexMCPServer_EmitsInlineHeadersTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	err := RegisterCodexMCPServer(path, "kamui",
		"https://api.kamui-platform.com/mcp",
		map[string]string{"Authorization": "Bearer secret-token"},
	)
	if err != nil {
		t.Fatalf("RegisterCodexMCPServer: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "[mcp_servers.kamui.headers]") {
		t.Fatalf("headers emitted as sub-table, want inline table:\n%s", got)
	}
	if !strings.Contains(got, "headers = {") {
		t.Fatalf("inline headers table missing:\n%s", got)
	}
}

func TestRegisterCodexMCPServer_PreservesUnrelatedTables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	initial := []byte(`model = "o4-mini"

[history]
max_size = 1000

[mcp_servers.other]
url = "https://other.test/mcp"
`)
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RegisterCodexMCPServer(path, "kamui", "https://kamui.test/mcp", nil); err != nil {
		t.Fatalf("RegisterCodexMCPServer: %v", err)
	}

	root := loadTOML(t, path)
	if root["model"] != "o4-mini" {
		t.Errorf("top-level 'model' clobbered: %v", root["model"])
	}
	hist, _ := root["history"].(map[string]any)
	if hist["max_size"] != int64(1000) {
		t.Errorf("history.max_size clobbered: %v", hist["max_size"])
	}
	servers, _ := root["mcp_servers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Errorf("sibling mcp_servers.other clobbered: %#v", servers)
	}
	if _, ok := servers["kamui"]; !ok {
		t.Errorf("kamui entry not added: %#v", servers)
	}
}

func TestRegisterCodexMCPServer_OverwritesSameName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	if err := RegisterCodexMCPServer(path, "kamui", "https://old.test/mcp", nil); err != nil {
		t.Fatal(err)
	}
	if err := RegisterCodexMCPServer(path, "kamui", "https://new.test/mcp", nil); err != nil {
		t.Fatal(err)
	}

	root := loadTOML(t, path)
	servers, _ := root["mcp_servers"].(map[string]any)
	got, _ := servers["kamui"].(map[string]any)
	if got["url"] != "https://new.test/mcp" {
		t.Errorf("url not updated: got %v", got["url"])
	}
}

func TestRegisterCodexMCPServer_RefusesSymlinkDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.toml")
	if err := os.WriteFile(target, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	err := RegisterCodexMCPServer(link, "kamui", "https://x.test/mcp", nil)
	if err == nil {
		t.Fatal("expected symlink destination to be refused")
	}
	b, _ := os.ReadFile(target)
	if len(b) != 0 {
		t.Errorf("symlink target was modified: %q", string(b))
	}
}

func TestRegisterCodexMCPServer_EmptyName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := RegisterCodexMCPServer(path, "", "https://x.test/mcp", nil); err == nil {
		t.Fatal("expected empty name to error")
	}
}

func loadTOML(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	root := map[string]any{}
	if err := toml.Unmarshal(b, &root); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return root
}
