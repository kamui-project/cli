package iface

import (
	"context"
)

// DatabaseExternalConnection mirrors the dashboard-backend response shape.
// Embedded directly so cmd/db.go can render it without an extra mapping step.
type DatabaseExternalConnection struct {
	// ProjectID and ProjectName are populated by the service layer (the API
	// only knows the database itself; the project context is injected so the
	// CLI can show "<project>/<db>" in tables).
	ProjectID    string `json:"project_id"`
	ProjectName  string `json:"project_name"`
	DatabaseID   string `json:"database_id"`
	DatabaseName string `json:"database_name"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Database     string `json:"database"`
	User         string `json:"user"`
	Password     string `json:"password"`
	RemoteHost   string `json:"remote_host"`
}

// DaemonEntry is one row of `kamui db daemon status`. Each entry corresponds
// to a local listener for a specific database.
type DaemonEntry struct {
	ProjectName    string `json:"project_name"`
	DatabaseName   string `json:"database_name"`
	DatabaseID     string `json:"database_id"`
	LocalPort      int    `json:"local_port"`
	CloudflaredPID int    `json:"cloudflared_pid"`
	Alive          bool   `json:"alive"`
}

// DatabaseService is the high-level CLI-side API for the `kamui db` command
// group. Implementation lives in internal/service/database.go and depends on
// AuthService and ProjectService.
type DatabaseService interface {
	// Connect opens a verified database tunnel and runs psql until it exits.
	Connect(ctx context.Context, projectAndDB string) error

	// DaemonStart launches local listeners for accessible databases. It returns
	// an error if a daemon is already running.
	DaemonStart(ctx context.Context) error

	// DaemonStatus returns the live state of the daemon (which DBs are
	// listening on which ports, whether the daemon is alive).
	DaemonStatus(ctx context.Context) ([]DaemonEntry, error)

	// DaemonStop closes the listeners and removes daemon state.
	DaemonStop(ctx context.Context) error
}
