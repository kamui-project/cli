//go:build windows

package daemon

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// detachSysProcAttr on Windows uses CREATE_NEW_PROCESS_GROUP so the daemon
// is isolated from the parent's console signals.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP
	}
}

func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		// Lack of permission does not establish that the process has exited.
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && status == uint32(windows.WAIT_TIMEOUT)
}

func requestStop(proc *os.Process) error {
	// Go does not implement SIGTERM on Windows. Terminate the process;
	// DaemonStop removes its PID and state after exit is confirmed.
	return proc.Kill()
}
