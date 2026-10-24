package config

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// LockAuth serializes rotating OAuth refresh tokens across the daemon and CLI.
// The OS releases the lock if a process exits unexpectedly.
func (m *Manager) LockAuth(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(m.configPath), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(m.configPath+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			f.Close()
			return nil, err
		}
		locked, err := tryLock(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		if locked {
			return func() { unlock(f); f.Close() }, nil
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
