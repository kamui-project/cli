package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// codexMCPServerEntry is the TOML shape Codex expects under
// [mcp_servers.<name>]. `headers` is emitted as an inline table so the
// generated config matches the documented format exactly:
// headers = { Authorization = "Bearer ..." }.
type codexMCPServerEntry struct {
	URL     string            `toml:"url"`
	Headers map[string]string `toml:"headers,inline,omitempty"`
}

// codexConfigPath is the user-scope Codex CLI settings file. It is TOML,
// not JSON, so it needs its own merger — the existing RegisterMCPServer
// only knows how to round-trip JSON.
const codexConfigPath = ".codex/config.toml"

// LocateCodexConfig returns the absolute path to ~/.codex/config.toml.
// The file may not exist yet; RegisterCodexMCPServer will create it.
func LocateCodexConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, codexConfigPath), nil
}

// RegisterCodexMCPServer merges a single mcp_servers.<name> table into
// the Codex config at path. Top-level keys outside mcp_servers and
// sibling MCP server tables are preserved across the rewrite. The TOML
// formatting and any comments outside the kamui entry are not preserved
// — pelletier/go-toml/v2 reflows the file — but on a fresh install the
// file usually doesn't exist anyway, and Codex re-reads it on startup,
// so the user-visible outcome is identical to manual paste.
//
// Persistence goes through atomicWrite0600, so the destination ends up
// at mode 0o600 and a symlink at the destination path is refused.
func RegisterCodexMCPServer(path, name, url string, headers map[string]string) error {
	if name == "" {
		return fmt.Errorf("mcp server name is empty")
	}

	root, err := loadTOMLRoot(path)
	if err != nil {
		return err
	}

	servers, _ := root["mcp_servers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	entry := codexMCPServerEntry{URL: url}
	if len(headers) > 0 {
		h := make(map[string]string, len(headers))
		for k, v := range headers {
			h[k] = v
		}
		entry.Headers = h
	}
	servers[name] = entry
	root["mcp_servers"] = servers

	data, err := toml.Marshal(root)
	if err != nil {
		return fmt.Errorf("marshal codex config %s: %w", path, err)
	}
	return atomicWrite0600(path, data)
}

// loadTOMLRoot reads and parses the existing Codex config as a generic
// map so unknown keys round-trip safely. Missing or empty files are
// treated as an empty document — same bootstrap behavior as the JSON
// loader.
func loadTOMLRoot(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read codex config %s: %w", path, err)
	}
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	root := map[string]any{}
	if err := toml.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("parse codex config %s: %w", path, err)
	}
	return root, nil
}
