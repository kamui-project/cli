//go:build !windows

package daemon

import "syscall"

// detachSysProcAttr puts the daemon in its own process group so that signals
// sent to the parent shell (e.g. Ctrl+C in the terminal that invoked
// `kamui db daemon start`) don't propagate to it.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}
