// Package cloudflared resolves the cloudflared binary used by the kamui CLI
// to spawn `cloudflared access tcp` listeners. The manager prefers an
// already-installed cloudflared on PATH (typical: brew on macOS,
// apt/cloudflared package on Linux) and falls back to downloading a raw
// binary from the official GitHub release on Linux.
//
// macOS / Windows downloads are not implemented because Cloudflare ships
// those as .tgz / .exe respectively, and we don't want to maintain archive
// extraction logic. Users on those platforms get a clear error pointing to
// `brew install cloudflared` or the cloudflared website.
package cloudflared

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Version pins the cloudflared release we test against. Kept aligned with
// the EKS-side cloudflared image tag so wire-protocol behavior is consistent
// (see cluster/deployments/charts/kamui-platform/values-prod-cloud.yaml).
const Version = "2025.8.0"

// Manager exposes a single entrypoint to obtain a cloudflared binary path
// and to spawn `cloudflared access tcp` processes against it.
type Manager interface {
	// EnsureInstalled returns an absolute path to a usable cloudflared
	// binary. Tries PATH first; downloads to ~/.kamui/bin on Linux if absent.
	EnsureInstalled(ctx context.Context) (string, error)

	// Spawn starts `cloudflared access tcp` as a child process. The child is
	// configured to authenticate to Cloudflare Access via the provided
	// service token (passed as env vars so they don't leak into ps output).
	// Caller is responsible for calling Process.Kill() / Wait() on the
	// returned *exec.Cmd. Returns the started Cmd; stdout/stderr are
	// inherited from the parent so the caller can redirect to a log file by
	// setting cmd.Stdout / cmd.Stderr before invoking Spawn — actually,
	// since we already start the process here, callers wanting log capture
	// should use BuildSpawnCmd instead.
	Spawn(ctx context.Context, hostname, listenURL, clientID, clientSecret string) (*exec.Cmd, error)

	// BuildSpawnCmd is the lower-level building block used by the daemon: it
	// returns a *exec.Cmd that has not been started yet so the caller can
	// override Stdout / Stderr before calling Start().
	BuildSpawnCmd(ctx context.Context, hostname, listenURL, clientID, clientSecret string) (*exec.Cmd, error)
}

// New returns the default Manager.
func New() Manager {
	return &manager{}
}

type manager struct {
	cached string // resolved binary path, populated on first EnsureInstalled
}

func (m *manager) EnsureInstalled(ctx context.Context) (string, error) {
	if m.cached != "" {
		return m.cached, nil
	}

	// 1. Prefer cloudflared on PATH.
	if path, err := exec.LookPath("cloudflared"); err == nil {
		m.cached = path
		return path, nil
	}

	// 2. Look in our managed location.
	managed, err := managedBinaryPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(managed); err == nil {
		m.cached = managed
		return managed, nil
	}

	// 3. Auto-download (Linux only — macOS / Windows users must install
	//    cloudflared themselves via brew / official installer).
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf(
			"cloudflared not found on PATH. Install it first:\n"+
				"  macOS:   brew install cloudflared\n"+
				"  Windows: https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/\n"+
				"  Other:   https://github.com/cloudflare/cloudflared/releases (need version %s+)",
			Version)
	}
	if err := downloadLinux(ctx, managed); err != nil {
		return "", fmt.Errorf("failed to download cloudflared: %w", err)
	}
	m.cached = managed
	return managed, nil
}

func (m *manager) BuildSpawnCmd(ctx context.Context, hostname, listenURL, clientID, clientSecret string) (*exec.Cmd, error) {
	bin, err := m.EnsureInstalled(ctx)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin,
		"access", "tcp",
		"--hostname", hostname,
		"--url", listenURL,
		"--service-token-id", clientID,
		"--service-token-secret", clientSecret,
	)
	// Don't leak service-token args via ps. cloudflared also accepts these
	// values via env vars, which we use as the primary channel:
	cmd.Env = append(os.Environ(),
		"TUNNEL_SERVICE_TOKEN_ID="+clientID,
		"TUNNEL_SERVICE_TOKEN_SECRET="+clientSecret,
	)
	return cmd, nil
}

func (m *manager) Spawn(ctx context.Context, hostname, listenURL, clientID, clientSecret string) (*exec.Cmd, error) {
	cmd, err := m.BuildSpawnCmd(ctx, hostname, listenURL, clientID, clientSecret)
	if err != nil {
		return nil, err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start cloudflared: %w", err)
	}
	return cmd, nil
}

// managedBinaryPath returns ~/.kamui/bin/cloudflared (or .exe on Windows).
func managedBinaryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home dir: %w", err)
	}
	name := "cloudflared"
	if runtime.GOOS == "windows" {
		name = "cloudflared.exe"
	}
	return filepath.Join(home, ".kamui", "bin", name), nil
}

// downloadLinux fetches the raw cloudflared Linux binary from GitHub releases
// to `dest`, with the executable bit set. Used only when the user has no
// cloudflared on PATH and is on Linux.
func downloadLinux(ctx context.Context, dest string) error {
	arch := runtime.GOARCH
	switch arch {
	case "amd64", "arm64":
		// supported
	default:
		return fmt.Errorf("unsupported Linux architecture %q (only amd64, arm64 supported); install cloudflared manually", arch)
	}

	url := fmt.Sprintf(
		"https://github.com/cloudflare/cloudflared/releases/download/%s/cloudflared-linux-%s",
		Version, arch)

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d from %s", resp.StatusCode, url)
	}

	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}
