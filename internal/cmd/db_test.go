package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// TestDbConnect_HelpMentionsProjectDB exercises the cobra command without
// reaching the real service layer. Confirms the help text mentions the
// required `<project>/<db>` form so usability doesn't regress.
func TestDbConnect_HelpMentionsProjectDB(t *testing.T) {
	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.cmd.SetOut(buf)
	root.cmd.SetErr(buf)
	root.cmd.SetArgs([]string{"db", "connect", "--help"})
	if err := root.cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "<project>/<db>") {
		t.Errorf("expected help text to mention <project>/<db>, got: %s", out)
	}
}

// TestDbDaemon_HelpMentionsSubcommands verifies the daemon group exposes
// start / status / stop. Guards against accidental subcommand removal.
func TestDbDaemon_HelpMentionsSubcommands(t *testing.T) {
	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.cmd.SetOut(buf)
	root.cmd.SetErr(buf)
	root.cmd.SetArgs([]string{"db", "daemon", "--help"})
	if err := root.cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	for _, sub := range []string{"start", "status", "stop"} {
		if !strings.Contains(out, sub) {
			t.Errorf("expected daemon help to list %q subcommand; got: %s", sub, out)
		}
	}
}
