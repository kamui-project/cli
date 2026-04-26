package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

// atomicWrite0600 writes data to path via a same-directory tempfile and
// rename. The destination ends up with mode 0o600. The intermediate
// tempfile uses an unguessable random suffix (os.CreateTemp) so a
// symlink-hijack attack on the staging name is not feasible. We Lstat
// the destination before rename and refuse to clobber a symlink there,
// which closes the only remaining redirection vector for token writes.
//
// Callers that need to write secrets (PATs, OAuth tokens, MCP config
// containing bearer tokens) should funnel through this helper instead
// of calling os.WriteFile directly.
func atomicWrite0600(path string, data []byte) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create config dir %s: %w", dir, err)
		}
	}

	tmp, err := os.CreateTemp(dir, ".kamui-secure-*")
	if err != nil {
		return fmt.Errorf("create tempfile in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("chmod tempfile: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write tempfile: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("sync tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close tempfile: %w", err)
	}

	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			cleanup()
			return fmt.Errorf("refusing to write %s: destination is a symlink", path)
		}
	}

	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("rename tempfile to %s: %w", path, err)
	}
	return nil
}
