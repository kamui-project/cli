package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kamui-project/kamui-cli/internal/api"
	"github.com/kamui-project/kamui-cli/internal/cloudflared"
	"github.com/kamui-project/kamui-cli/internal/config"
	"github.com/kamui-project/kamui-cli/internal/daemon"
	"github.com/kamui-project/kamui-cli/internal/portcalc"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

// databaseService implements iface.DatabaseService.
type databaseService struct {
	configManager  *config.Manager
	authService    iface.AuthService
	projectService iface.ProjectService
	cf             cloudflared.Manager
}

// NewDatabaseService wires the dependencies.
func NewDatabaseService(configManager *config.Manager, authService iface.AuthService, projectService iface.ProjectService) iface.DatabaseService {
	return &databaseService{
		configManager:  configManager,
		authService:    authService,
		projectService: projectService,
		cf:             cloudflared.New(),
	}
}

func (s *databaseService) getAPIClient(ctx context.Context) (*api.Client, error) {
	if err := s.authService.EnsureAuthenticated(ctx); err != nil {
		return nil, err
	}
	token, err := s.configManager.GetAccessToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}
	apiURL, err := s.configManager.GetAPIURL()
	if err != nil {
		return nil, fmt.Errorf("failed to get API URL: %w", err)
	}
	return api.NewClient(apiURL, token), nil
}

// Connect implements `kamui db connect <project>/<db>`.
func (s *databaseService) Connect(ctx context.Context, projectAndDB string) error {
	projectName, dbName, err := parseProjectDB(projectAndDB)
	if err != nil {
		return err
	}

	// 1. Resolve project + database via API.
	project, db, err := s.resolveProjectAndDB(ctx, projectName, dbName)
	if err != nil {
		return err
	}

	// 2. Fetch external-connection (decrypted password).
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return err
	}
	conn, err := client.GetDatabaseExternalConnection(ctx, db.ID)
	if err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 409 {
			return fmt.Errorf(
				"this database has no external password yet — please open the dashboard, " +
					"go to the database detail page, and click \"Regenerate password\" once to initialize")
		}
		return fmt.Errorf("failed to get external connection info: %w", err)
	}

	// 3. Pick the local port. If the daemon already tunnels this DB, reuse
	//    its listener (no extra cloudflared, no port collision). Otherwise
	//    grab an ephemeral free port — the deterministic port is reserved
	//    for the daemon (so the dashboard's displayed port stays accurate).
	var port int
	reuseDaemon := false
	if entry := findDaemonEntryForDB(db.ID); entry != nil && daemon.IsAlive(entry.CloudflaredPID) {
		port = entry.LocalPort
		reuseDaemon = true
	} else {
		p, err := pickFreePort()
		if err != nil {
			return fmt.Errorf("failed to pick a free local port: %w", err)
		}
		port = p
	}

	// Forward Ctrl+C to psql via signal: psql handles SIGINT gracefully.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	if !reuseDaemon {
		// 4. Fetch the platform-shared service token.
		tok, err := client.GetCliServiceToken(ctx)
		if err != nil {
			return fmt.Errorf("failed to get CF service token: %w", err)
		}

		// 5. Spawn cloudflared in background (foreground mode for `connect`:
		//    we hold it for the duration of the psql session).
		listenURL := fmt.Sprintf("localhost:%d", port)
		cfCmd, err := s.cf.BuildSpawnCmd(ctx, tok.Hostname, listenURL, tok.ClientID, tok.ClientSecret)
		if err != nil {
			return err
		}
		cfCmd.Stdout = os.Stderr // surface cloudflared logs but not psql's stdout
		cfCmd.Stderr = os.Stderr
		if err := cfCmd.Start(); err != nil {
			return fmt.Errorf("failed to start cloudflared: %w", err)
		}
		defer func() {
			if cfCmd.Process != nil {
				_ = cfCmd.Process.Signal(syscall.SIGTERM)
				done := make(chan struct{})
				go func() { _, _ = cfCmd.Process.Wait(); close(done) }()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					_ = cfCmd.Process.Kill()
				}
			}
		}()

		// 6. Wait for cloudflared listener to become ready, then exec psql.
		if err := waitForListener(ctx, port, 10*time.Second); err != nil {
			return fmt.Errorf("cloudflared did not start listening on %s: %w", listenURL, err)
		}
	}

	psql, err := exec.LookPath("psql")
	if err != nil {
		return fmt.Errorf("psql not found on PATH. Install PostgreSQL client tools first " +
			"(macOS: `brew install libpq && brew link --force libpq`, Linux: `apt install postgresql-client`)")
	}
	psqlCmd := exec.CommandContext(ctx, psql,
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", port),
		"-U", conn.User,
		"-d", conn.Database,
	)
	psqlCmd.Stdin = os.Stdin
	psqlCmd.Stdout = os.Stdout
	psqlCmd.Stderr = os.Stderr
	psqlCmd.Env = append(os.Environ(), "PGPASSWORD="+conn.Password)

	fmt.Fprintf(os.Stderr, "✓ Connected to %s/%s as %s. Type \\q to exit.\n", project.Name, db.Name, conn.User)
	if err := psqlCmd.Run(); err != nil {
		// psql returns non-zero on \q'd-after-error too; fall through gracefully.
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil
		}
		return err
	}
	return nil
}

// DaemonStart implements `kamui db daemon start`.
func (s *databaseService) DaemonStart(ctx context.Context) error {
	// 1. Reject if a daemon is already running.
	if pid, _ := daemon.ReadPID(); pid > 0 && daemon.IsAlive(pid) {
		return fmt.Errorf("kamui db daemon is already running (pid %d). Use `kamui db daemon stop` first", pid)
	}
	// Stale PID file? Clean it up.
	_ = daemon.RemovePID()
	_ = daemon.RemoveState()

	// 2. Build the list of tunnels to spawn.
	tunnels, err := s.collectTunnels(ctx)
	if err != nil {
		return err
	}
	if len(tunnels) == 0 {
		return fmt.Errorf("no Pro databases with external-connection set up. Create a database or " +
			"press \"Regenerate password\" in the dashboard to initialize one, then try again")
	}

	// 3. Resolve service token (passed to daemon via env).
	apiCli, err := s.getAPIClient(ctx)
	if err != nil {
		return err
	}
	tok, err := apiCli.GetCliServiceToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get CF service token: %w", err)
	}

	// 4. Ensure cloudflared binary is in place (so the daemon doesn't have
	//    to download it on first start).
	if _, err := s.cf.EnsureInstalled(ctx); err != nil {
		return err
	}

	// 5. Re-exec ourselves with KAMUI_DB_DAEMON=1.
	tunnelsJSON, err := json.Marshal(tunnels)
	if err != nil {
		return err
	}
	pid, err := daemon.LaunchDetached(string(tunnelsJSON), tok.ClientID, tok.ClientSecret)
	if err != nil {
		return err
	}
	if err := daemon.WritePID(pid); err != nil {
		return err
	}
	// Give the daemon a brief moment to write its state file.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st, _ := daemon.ReadState(); st != nil && len(st.Entries) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "✓ kamui db daemon started (pid %d). Run `kamui db daemon status` to see ports.\n", pid)
	return nil
}

// DaemonStatus implements `kamui db daemon status`.
func (s *databaseService) DaemonStatus(_ context.Context) ([]iface.DaemonEntry, error) {
	st, err := daemon.ReadState()
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, nil
	}
	out := make([]iface.DaemonEntry, 0, len(st.Entries))
	for _, e := range st.Entries {
		out = append(out, iface.DaemonEntry{
			ProjectName:    e.ProjectName,
			DatabaseName:   e.DatabaseName,
			DatabaseID:     e.DatabaseID,
			LocalPort:      e.LocalPort,
			CloudflaredPID: e.CloudflaredPID,
			Alive:          daemon.IsAlive(e.CloudflaredPID),
		})
	}
	return out, nil
}

// DaemonStop implements `kamui db daemon stop`.
func (s *databaseService) DaemonStop(_ context.Context) error {
	pid, err := daemon.ReadPID()
	if err != nil {
		return err
	}
	if pid == 0 {
		return fmt.Errorf("kamui db daemon is not running")
	}
	if err := daemon.SignalAndWaitStop(pid, 10*time.Second); err != nil {
		return err
	}
	_ = daemon.RemovePID()
	_ = daemon.RemoveState()
	fmt.Fprintf(os.Stderr, "✓ kamui db daemon stopped (was pid %d).\n", pid)
	return nil
}

// collectTunnels enumerates every database the user has access to (across
// all of their projects) for which an encrypted external password is set
// up. Databases without initialized external connection are silently skipped
// (dashboard returns 404/409 depending on server version).
func (s *databaseService) collectTunnels(ctx context.Context) ([]daemon.Tunnel, error) {
	projects, err := s.projectService.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	apiCli, err := s.getAPIClient(ctx)
	if err != nil {
		return nil, err
	}
	var out []daemon.Tunnel
	for _, p := range projects {
		// /api/projects (list) doesn't include databases; need /api/projects/{id}
		full, err := s.projectService.GetProject(ctx, p.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to read project %s: %v\n", p.Name, err)
			continue
		}
		for _, db := range full.Databases {
			conn, err := apiCli.GetDatabaseExternalConnection(ctx, db.ID)
			if err != nil {
				var apiErr *api.APIError
				if errors.As(err, &apiErr) && (apiErr.StatusCode == 404 || apiErr.StatusCode == 409 || apiErr.StatusCode == 400) {
					// External connection not initialized, or non-Pro plan — skip silently.
					continue
				}
				if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
					// Not the owner — skip silently.
					continue
				}
				fmt.Fprintf(os.Stderr, "warning: %s/%s skipped: %v\n", p.Name, db.Name, err)
				continue
			}
			out = append(out, daemon.Tunnel{
				ProjectName:  p.Name,
				DatabaseName: db.Name,
				DatabaseID:   db.ID,
				LocalPort:    portcalc.Port(db.ID),
				RemoteHost:   conn.RemoteHost,
			})
		}
	}
	return out, nil
}

// resolveProjectAndDB looks up a project by name and a database by name within
// it. Used by `kamui db connect`.
func (s *databaseService) resolveProjectAndDB(ctx context.Context, projectName, dbName string) (*iface.Project, *iface.Database, error) {
	projects, err := s.projectService.ListProjects(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list projects: %w", err)
	}
	var project *iface.Project
	for i := range projects {
		if projects[i].Name == projectName {
			p := projects[i]
			project = &p
			break
		}
	}
	if project == nil {
		return nil, nil, fmt.Errorf("project %q not found (or you don't have access)", projectName)
	}
	full, err := s.projectService.GetProject(ctx, project.ID)
	if err != nil {
		return nil, nil, err
	}
	for i := range full.Databases {
		if full.Databases[i].Name == dbName {
			return full, &full.Databases[i], nil
		}
	}
	return nil, nil, fmt.Errorf("database %q not found in project %q", dbName, projectName)
}

// parseProjectDB splits "project/db" into its components.
func parseProjectDB(s string) (project, db string, err error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("argument must be <project>/<db>, got %q", s)
	}
	return parts[0], parts[1], nil
}

// findDaemonEntryForDB returns the daemon's tunnel entry for this DB, or nil
// if the daemon is not running or doesn't tunnel it.
func findDaemonEntryForDB(dbID string) *daemon.Entry {
	st, err := daemon.ReadState()
	if err != nil || st == nil {
		return nil
	}
	for i := range st.Entries {
		if st.Entries[i].DatabaseID == dbID {
			return &st.Entries[i]
		}
	}
	return nil
}

// pickFreePort asks the kernel for an unused TCP port on localhost. Brief
// race window between Close and the next bind, but acceptable for a CLI.
func pickFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// waitForListener polls TCP-connect to localhost:port until success or timeout.
func waitForListener(ctx context.Context, port int, timeout time.Duration) error {
	addr := fmt.Sprintf("localhost:%d", port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout after %s", timeout)
}
