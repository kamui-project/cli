//go:build !windows

package daemon

import (
	"errors"
	"os"
	"syscall"
)

// detachSysProcAttr puts the daemon in its own process group so that signals
// sent to the parent shell (e.g. Ctrl+C in the terminal that invoked
// `kamui db daemon start`) don't propagate to it.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	defer proc.Release()
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func requestStop(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}
