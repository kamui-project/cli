// Package main is the entry point for the Kamui CLI.
// Kamui CLI provides command-line access to the Kamui Platform,
// a PaaS service for deploying and managing applications.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/kamui-project/kamui-cli/internal/cmd"
	"github.com/kamui-project/kamui-cli/internal/daemon"
	"github.com/kamui-project/kamui-cli/internal/dbaccess"
	"github.com/kamui-project/kamui-cli/internal/di"
	"github.com/kamui-project/kamui-cli/internal/service"
)

func main() {
	// Daemon re-exec path: when `kamui db daemon start` re-launches us with
	// KAMUI_DB_DAEMON=1, hand control to the daemon Runner instead of cobra.
	if os.Getenv("KAMUI_DB_DAEMON") == "1" {
		os.Exit(runDaemon())
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// runDaemon is invoked when this binary is re-exec'd by `db daemon start`.
// It reads tunnel config from env vars and runs the long-lived loop.
func runDaemon() int {
	tunnelsJSON := os.Getenv("KAMUI_DB_DAEMON_TUNNELS")

	var tunnels []daemon.Tunnel
	if err := json.Unmarshal([]byte(tunnelsJSON), &tunnels); err != nil {
		fmt.Fprintf(os.Stderr, "[daemon] invalid KAMUI_DB_DAEMON_TUNNELS: %v\n", err)
		return 2
	}

	container, err := di.NewContainer()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	provider := func(id string) dbaccess.Provider {
		return service.NewDatabasePermitProvider(container.ConfigManager(), container.AuthService(), id)
	}
	if err := daemon.Run(context.Background(), tunnels, provider); err != nil {
		fmt.Fprintf(os.Stderr, "[daemon] exit error: %v\n", err)
		return 1
	}
	return 0
}
