package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Tunnel describes one cloudflared listener the daemon should keep alive.
type Tunnel struct {
	ProjectName  string
	DatabaseName string
	DatabaseID   string
	LocalPort    int
	RemoteHost   string
}

// CloudflaredFactory is a thin abstraction over cloudflared.Manager so the
// daemon doesn't import the cloudflared package directly (avoids cycles
// when DatabaseService re-uses this runner).
type CloudflaredFactory func(ctx context.Context, hostname, listenURL, clientID, clientSecret string) (*exec.Cmd, error)

// Run is the daemon process body. It is invoked when the kamui binary is
// re-exec'd with KAMUI_DB_DAEMON=1; the parent process does NOT call Run
// directly. Behavior:
//
//  1. Open the log file (stdout/stderr were already redirected to it by the
//     parent before exec).
//  2. Spawn one cloudflared per tunnel, recording PIDs in state.json.
//  3. Wait for SIGTERM/SIGINT; on signal, kill all children and exit.
//  4. If any child cloudflared exits unexpectedly, log it but stay running
//     (TablePlus will reconnect when cloudflared is restarted via
//     `kamui db daemon stop && kamui db daemon start`).
//
// Return values are written to stderr of the daemon (i.e. the log file) and
// then `os.Exit` is called.
func Run(ctx context.Context, tunnels []Tunnel, clientID, clientSecret string, factory CloudflaredFactory) error {
	state := &State{
		DaemonPID: os.Getpid(),
		Entries:   make([]Entry, 0, len(tunnels)),
	}
	cmds := make([]*exec.Cmd, 0, len(tunnels))

	for _, t := range tunnels {
		listenURL := fmt.Sprintf("localhost:%d", t.LocalPort)
		cmd, err := factory(ctx, t.RemoteHost, listenURL, clientID, clientSecret)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[daemon] FAILED to build cloudflared for %s/%s: %v\n", t.ProjectName, t.DatabaseName, err)
			continue
		}
		// Children inherit our stdout/stderr (the log file).
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "[daemon] FAILED to start cloudflared for %s/%s: %v\n", t.ProjectName, t.DatabaseName, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "[daemon] started cloudflared pid=%d for %s/%s on localhost:%d (remote=%s)\n",
			cmd.Process.Pid, t.ProjectName, t.DatabaseName, t.LocalPort, t.RemoteHost)
		cmds = append(cmds, cmd)
		state.Entries = append(state.Entries, Entry{
			ProjectName:    t.ProjectName,
			DatabaseName:   t.DatabaseName,
			DatabaseID:     t.DatabaseID,
			LocalPort:      t.LocalPort,
			RemoteHost:     t.RemoteHost,
			CloudflaredPID: cmd.Process.Pid,
		})
	}

	if err := WriteState(state); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	// Block until signaled.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
	fmt.Fprintln(os.Stderr, "[daemon] shutdown signal received; killing children...")

	for _, c := range cmds {
		_ = c.Process.Signal(syscall.SIGTERM)
	}
	// Give children a moment to exit gracefully, then kill -9 stragglers.
	deadline := time.Now().Add(5 * time.Second)
	for _, c := range cmds {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			_ = c.Process.Kill()
			continue
		}
		done := make(chan error, 1)
		go func(cmd *exec.Cmd) { done <- cmd.Wait() }(c)
		select {
		case <-done:
		case <-time.After(remaining):
			_ = c.Process.Kill()
		}
	}

	_ = RemoveState()
	_ = RemovePID()
	return nil
}

// LaunchDetached starts the kamui binary again (this same executable) with
// KAMUI_DB_DAEMON=1 set, redirecting its stdio to ~/.kamui/db-daemon.log.
// Returns the daemon PID.
//
// tunnelsJSON, clientID, clientSecret are passed to the child via env vars
// so they don't leak into ps (and so the parent can keep its arg vector
// clean for cobra).
func LaunchDetached(tunnelsJSON, clientID, clientSecret string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("cannot determine kamui binary path: %w", err)
	}
	logPath, err := LogPath()
	if err != nil {
		return 0, err
	}
	logF, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("cannot open log file: %w", err)
	}
	defer logF.Close()

	cmd := exec.Command(exe)
	cmd.Stdin = nil
	cmd.Stdout = logF
	cmd.Stderr = logF
	cmd.Env = append(os.Environ(),
		"KAMUI_DB_DAEMON=1",
		"KAMUI_DB_DAEMON_TUNNELS="+tunnelsJSON,
		"KAMUI_DB_DAEMON_CF_CLIENT_ID="+clientID,
		"KAMUI_DB_DAEMON_CF_CLIENT_SECRET="+clientSecret,
	)
	cmd.SysProcAttr = detachSysProcAttr()
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("cannot launch daemon: %w", err)
	}
	// Don't Wait — we want the child to outlive us.
	return cmd.Process.Pid, nil
}

// SignalAndWaitStop sends SIGTERM to the daemon and waits up to timeout for
// it to disappear. Returns nil if the daemon exited cleanly, error
// otherwise.
func SignalAndWaitStop(pid int, timeout time.Duration) error {
	if pid <= 0 || !IsAlive(pid) {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// Best-effort: if the process is already gone, that's fine.
		if errors.Is(err, os.ErrProcessDone) || strings.Contains(err.Error(), "process already finished") {
			return nil
		}
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !IsAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Last resort: kill -9.
	_ = proc.Kill()
	if IsAlive(pid) {
		return fmt.Errorf("daemon (pid %d) did not exit after SIGTERM + SIGKILL", pid)
	}
	return nil
}

// DiscardLog returns an io.Writer that swallows everything (used as a
// safety fallback if log-file open fails inside Run).
func DiscardLog() io.Writer { return io.Discard }
