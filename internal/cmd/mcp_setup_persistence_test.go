package cmd

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPersistMCPSetupCredentials_RegisterAndTokenFile_Codex(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME override semantics differ on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	tokenFile := filepath.Join(t.TempDir(), "secrets", "mcp-pat")
	registered, err := persistMCPSetupCredentials(
		context.Background(),
		mcpClientCodex,
		"https://api.test",
		"tok-id-1",
		"secret-pat",
		tokenFile,
		true,
	)
	if err != nil {
		t.Fatalf("persistMCPSetupCredentials: %v", err)
	}
	if !registered {
		t.Fatal("registered=false, want true")
	}

	b, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if string(b) != "secret-pat\n" {
		t.Errorf("token file contents = %q, want %q", string(b), "secret-pat\n")
	}
	assertMode0600(t, tokenFile)

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

func TestPersistMCPSetupCredentials_RegisterThenTokenFileFailure_IsExplicit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := t.TempDir()
	target := filepath.Join(dir, "innocent")
	if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "pat-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	registered, err := persistMCPSetupCredentials(
		context.Background(),
		mcpClientCodex,
		"https://api.test",
		"tok-id-2",
		"secret-pat",
		link,
		true,
	)
	if err == nil {
		t.Fatal("expected error for symlink token file")
	}
	if !registered {
		t.Fatal("registered=false, want true (registration should have completed first)")
	}
	if !strings.Contains(err.Error(), "token registered with Codex but failed to write token file") {
		t.Fatalf("unexpected error: %v", err)
	}

	root := loadTOML(t, filepath.Join(home, ".codex", "config.toml"))
	servers, _ := root["mcp_servers"].(map[string]any)
	if _, ok := servers["kamui"]; !ok {
		t.Fatalf("kamui registration missing after partial failure: %#v", servers)
	}
}
