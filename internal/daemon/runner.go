package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kamui-project/kamui-cli/internal/dbaccess"
	"github.com/kamui-project/kamui-cli/internal/localtls"
)

// Tunnel describes one loopback listener held by the daemon.
type Tunnel struct {
	ProjectName  string
	DatabaseName string
	DatabaseID   string
	LocalPort    int
	RemoteHost   string
}

// Run hosts listeners in this process; no shared service token or child proxy.
func Run(ctx context.Context, tunnels []Tunnel, provider func(string) dbaccess.Provider) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if len(tunnels) == 0 {
		return errors.New("no database listeners configured")
	}
	identity, err := localtls.LoadOrCreate(ctx)
	if err != nil {
		return err
	}
	state := &State{DaemonPID: os.Getpid(), Transport: Transport}
	listeners := make([]net.Listener, 0, len(tunnels))
	var wg sync.WaitGroup
	defer func() {
		cancel()
		for _, listener := range listeners {
			listener.Close()
		}
		wg.Wait()
	}()
	// Bind all ports before advertising readiness. Port collisions fail visibly.
	for _, t := range tunnels {
		if t.LocalPort < 1 || t.LocalPort > 65535 {
			return errors.New("invalid database port")
		}
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", t.LocalPort))
		if err != nil {
			return fmt.Errorf("cannot listen for %s/%s: %w", t.ProjectName, t.DatabaseName, err)
		}
		listeners = append(listeners, listener)
		state.Entries = append(state.Entries, Entry{
			ProjectName: t.ProjectName, DatabaseName: t.DatabaseName, DatabaseID: t.DatabaseID,
			LocalPort: t.LocalPort, RemoteHost: t.RemoteHost, CloudflaredPID: os.Getpid(),
		})
	}
	if err := WritePID(os.Getpid()); err != nil {
		return err
	}
	defer RemovePID()
	if err := WriteState(state); err != nil {
		return err
	}
	defer RemoveState()
	errs := make(chan error, len(listeners))
	for i, listener := range listeners {
		t := tunnels[i]
		provide := provider(t.DatabaseID)
		wg.Add(1)
		go func(listener net.Listener, t Tunnel) {
			defer wg.Done()
			errs <- dbaccess.Serve(ctx, listener, provide, identity.ServerConfig, func(err error) {
				fmt.Fprintf(os.Stderr, "[daemon] %s/%s: %v\n", t.ProjectName, t.DatabaseName, err)
			})
		}(listener, t)
	}
	select {
	case <-ctx.Done():
		return nil
	case err := <-errs:
		return err
	}
}

// LaunchDetached starts the kamui binary again (this same executable) with
// KAMUI_DB_DAEMON=1 set, redirecting its stdio to ~/.kamui/db-daemon.log.
// Returns the daemon PID.
//
// Only listener metadata is passed to the child via environment variables.
// Permits are obtained by the child and remain in memory.
func LaunchDetached(tunnelsJSON string) (int, error) {
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
