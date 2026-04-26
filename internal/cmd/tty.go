package cmd

import (
	"os"

	"github.com/mattn/go-isatty"
)

// isStdinTTY reports whether stdin is connected to an interactive terminal.
// Used to decide whether prompting the user is safe.
func isStdinTTY() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// isStdoutTTY reports whether stdout is connected to an interactive terminal.
// When false, callers should avoid printing secrets to stdout (the output is
// likely being piped, redirected, or captured by an automation harness).
func isStdoutTTY() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
