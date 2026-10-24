package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
	"github.com/spf13/cobra"
)

// DbCommand is the parent of `kamui db ...` subcommands.
type DbCommand struct {
	root *RootCommand
	cmd  *cobra.Command

	connectCmd *DbConnectCommand
	daemonCmd  *DbDaemonCommand
}

// NewDbCommand wires the `db` group.
func NewDbCommand(root *RootCommand) *DbCommand {
	d := &DbCommand{root: root}
	d.cmd = &cobra.Command{
		Use:   "db",
		Short: "Connect to your Kamui-managed PostgreSQL databases from your laptop",
		Long: `Connect to Kamui Pro/Aurora databases via Cloudflare Tunnel + Access.

Two modes are supported:
  - kamui db connect <project>/<db>     One-shot psql session
  - kamui db daemon  start|status|stop  Persistent listener for TablePlus / pgAdmin

Both rely on the dashboard-issued external password (regenerate it from the
database detail page if it has never been set).`,
	}
	d.connectCmd = NewDbConnectCommand(d)
	d.daemonCmd = NewDbDaemonCommand(d)
	d.cmd.AddCommand(d.connectCmd.Command())
	d.cmd.AddCommand(d.daemonCmd.Command())
	return d
}

// Command returns the underlying cobra command.
func (d *DbCommand) Command() *cobra.Command { return d.cmd }

// Root returns the root command.
func (d *DbCommand) Root() *RootCommand { return d.root }

// DbConnectCommand: `kamui db connect <project>/<db>`
type DbConnectCommand struct {
	parent *DbCommand
	cmd    *cobra.Command
}

func NewDbConnectCommand(parent *DbCommand) *DbConnectCommand {
	c := &DbConnectCommand{parent: parent}
	c.cmd = &cobra.Command{
		Use:   "connect <project>/<db>",
		Short: "Open a psql session to a specific database",
		Long: `Open a local database tunnel, then run psql. The CLI verifies the database
TLS certificate and renews access permits while the SQL session is open.

Project names containing spaces are handled automatically — both forms work:
  kamui db connect "kamui platform/kamui"
  kamui db connect kamui platform/kamui

Examples:
  kamui db connect myproject/myapp
  kamui db connect myproject/analytics
  kamui db connect "kamui platform/kamui"`,
		Args: cobra.MinimumNArgs(1),
		RunE: c.Run,
	}
	return c
}

func (c *DbConnectCommand) Command() *cobra.Command { return c.cmd }

func (c *DbConnectCommand) Run(cmd *cobra.Command, args []string) error {
	// Accept project names with spaces without forcing the user to quote.
	// `kamui db connect kamui platform/kamui` and the quoted form both work
	// because we re-join args with a single space.
	target := strings.Join(args, " ")
	svc := c.parent.Root().Container().DatabaseService()
	return svc.Connect(cmd.Context(), target)
}

// DbDaemonCommand: parent for start / status / stop.
type DbDaemonCommand struct {
	parent *DbCommand
	cmd    *cobra.Command

	startCmd  *DbDaemonStartCommand
	statusCmd *DbDaemonStatusCommand
	stopCmd   *DbDaemonStopCommand
}

func NewDbDaemonCommand(parent *DbCommand) *DbDaemonCommand {
	d := &DbDaemonCommand{parent: parent}
	d.cmd = &cobra.Command{
		Use:   "daemon",
		Short: "Manage the persistent kamui db tunnel daemon",
		Long: `The daemon maintains a local listener for each accessible database so GUI
tools (TablePlus, DBeaver, pgAdmin) can connect to 127.0.0.1:<port>.
Require SSL with server certificate verification in the GUI. Use the CA file
printed by daemon start (db-local-ca.pem) with host 127.0.0.1. The CLI also
verifies upstream TLS and renews access permits automatically.`,
	}
	d.startCmd = NewDbDaemonStartCommand(d)
	d.statusCmd = NewDbDaemonStatusCommand(d)
	d.stopCmd = NewDbDaemonStopCommand(d)
	d.cmd.AddCommand(d.startCmd.Command())
	d.cmd.AddCommand(d.statusCmd.Command())
	d.cmd.AddCommand(d.stopCmd.Command())
	return d
}

func (d *DbDaemonCommand) Command() *cobra.Command { return d.cmd }

// DbDaemonStartCommand
type DbDaemonStartCommand struct {
	parent *DbDaemonCommand
	cmd    *cobra.Command
}

func NewDbDaemonStartCommand(parent *DbDaemonCommand) *DbDaemonStartCommand {
	c := &DbDaemonStartCommand{parent: parent}
	c.cmd = &cobra.Command{
		Use:   "start",
		Short: "Launch the kamui db tunnel daemon in the background",
		RunE:  c.Run,
	}
	return c
}

func (c *DbDaemonStartCommand) Command() *cobra.Command { return c.cmd }

func (c *DbDaemonStartCommand) Run(cmd *cobra.Command, args []string) error {
	svc := c.parent.parent.Root().Container().DatabaseService()
	return svc.DaemonStart(cmd.Context())
}

// DbDaemonStatusCommand
type DbDaemonStatusCommand struct {
	parent *DbDaemonCommand
	cmd    *cobra.Command
}

func NewDbDaemonStatusCommand(parent *DbDaemonCommand) *DbDaemonStatusCommand {
	c := &DbDaemonStatusCommand{parent: parent}
	c.cmd = &cobra.Command{
		Use:   "status",
		Short: "Show the daemon's tunnels and listening ports",
		RunE:  c.Run,
	}
	return c
}

func (c *DbDaemonStatusCommand) Command() *cobra.Command { return c.cmd }

func (c *DbDaemonStatusCommand) Run(cmd *cobra.Command, args []string) error {
	svc := c.parent.parent.Root().Container().DatabaseService()
	entries, err := svc.DaemonStatus(cmd.Context())
	if err != nil {
		return err
	}
	outputFormat, _ := cmd.Flags().GetString("output")
	if outputFormat == "" && cmd.Parent() != nil && cmd.Parent().Parent() != nil && cmd.Parent().Parent().Parent() != nil {
		outputFormat, _ = cmd.Parent().Parent().Parent().PersistentFlags().GetString("output")
	}
	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}
	if len(entries) == 0 {
		fmt.Println("kamui db daemon is not running.")
		fmt.Println("Start it with: kamui db daemon start")
		return nil
	}
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		alive := "yes"
		if !e.Alive {
			alive = "no"
		}
		rows = append(rows, []string{
			e.ProjectName,
			e.DatabaseName,
			fmt.Sprintf("%d", e.LocalPort),
			fmt.Sprintf("%d", e.CloudflaredPID),
			alive,
		})
	}
	printTable(os.Stdout, "", []string{"PROJECT", "DATABASE", "LOCAL_PORT", "PID", "ALIVE"}, rows)
	return nil
}

// outputJSON helper for status (kept for symmetry / tests)
func (c *DbDaemonStatusCommand) outputJSON(entries []iface.DaemonEntry) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// DbDaemonStopCommand
type DbDaemonStopCommand struct {
	parent *DbDaemonCommand
	cmd    *cobra.Command
}

func NewDbDaemonStopCommand(parent *DbDaemonCommand) *DbDaemonStopCommand {
	c := &DbDaemonStopCommand{parent: parent}
	c.cmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop the kamui db tunnel daemon",
		RunE:  c.Run,
	}
	return c
}

func (c *DbDaemonStopCommand) Command() *cobra.Command { return c.cmd }

func (c *DbDaemonStopCommand) Run(cmd *cobra.Command, args []string) error {
	svc := c.parent.parent.Root().Container().DatabaseService()
	return svc.DaemonStop(cmd.Context())
}
