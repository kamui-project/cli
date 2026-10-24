// Package daemon manages the long-lived `kamui db daemon` process and the
// shared state files used to coordinate between `start` / `status` / `stop`.
package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/kamui-project/kamui-cli/internal/config"
)

// Paths used by the daemon (under ~/.kamui).
const (
	Transport = "permit-local-tls-v2"
	pidFile   = "db-daemon.pid"
	stateFile = "db-daemon.state.json"
	logFile   = "db-daemon.log"
)

// Entry describes a loopback listener owned by the daemon.
// CloudflaredPID retains its JSON name for compatibility; new entries use the daemon PID.
type Entry struct {
	ProjectName    string `json:"project_name"`
	DatabaseName   string `json:"database_name"`
	DatabaseID     string `json:"database_id"`
	LocalPort      int    `json:"local_port"`
	RemoteHost     string `json:"remote_host"`
	CloudflaredPID int    `json:"cloudflared_pid"`
}

// State is what gets persisted to ~/.kamui/db-daemon.state.json. Read by
// `kamui db daemon status`, written by the daemon process itself.
type State struct {
	DaemonPID int     `json:"daemon_pid"`
	Transport string  `json:"transport,omitempty"`
	Entries   []Entry `json:"entries"`
}

// HomeDir returns ~/.kamui (creating it if missing).
func HomeDir() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, nil
}

// PIDPath returns ~/.kamui/db-daemon.pid.
func PIDPath() (string, error) {
	d, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, pidFile), nil
}

// StatePath returns ~/.kamui/db-daemon.state.json.
func StatePath() (string, error) {
	d, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, stateFile), nil
}

// LogPath returns ~/.kamui/db-daemon.log.
func LogPath() (string, error) {
	d, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, logFile), nil
}

// ReadPID returns the daemon's PID (if any). Returns 0, nil when no PID file
// exists; non-nil error only on real I/O / parse failures.
func ReadPID() (int, error) {
	path, err := PIDPath()
	if err != nil {
		return 0, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID file: %w", err)
	}
	return pid, nil
}

// WritePID atomically writes the PID file.
func WritePID(pid int) error {
	path, err := PIDPath()
	if err != nil {
		return err
	}
	return atomicWrite(path, []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

// RemovePID deletes the PID file (no error if missing).
func RemovePID() error {
	path, err := PIDPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ReadState returns the persisted daemon state or (nil, nil) if no file
// exists yet.
func ReadState() (*State, error) {
	path, err := StatePath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	body, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("invalid state file: %w", err)
	}
	return &s, nil
}

// WriteState atomically persists the daemon state.
func WriteState(s *State) error {
	path, err := StatePath()
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, body, 0o600)
}

// RemoveState deletes the state file (no error if missing).
func RemoveState() error {
	path, err := StatePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// IsAlive returns true if the given PID names a running process. Uses
// signal 0 (Unix) which checks existence without delivering a signal.
func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, Signal(0) returns nil if process exists and we have permission
	// to signal it; ESRCH if not. On Windows FindProcess always succeeds for
	// any pid, so we additionally call Signal which Windows implements as a
	// best-effort check.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}

// atomicWrite writes via tmp + rename so concurrent readers never see a
// half-written file.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
