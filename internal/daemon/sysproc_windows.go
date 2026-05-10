//go:build windows

package daemon

import "syscall"

// detachSysProcAttr on Windows uses CREATE_NEW_PROCESS_GROUP so the daemon
// is isolated from the parent's console signals.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP
	}
}
