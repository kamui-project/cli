package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/kamui-project/kamui-cli/internal/dbaccess"
)

func TestRunBindsLoopbackAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KAMUI_CONFIG_DIR", dir)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, []Tunnel{{DatabaseID: "test-db", LocalPort: port}}, func(string) dbaccess.Provider {
			return func(context.Context) (*dbaccess.Permit, error) { return nil, errors.New("not authorized") }
		})
	}()
	deadline := time.Now().Add(2 * time.Second)
	var state *State
	for time.Now().Before(deadline) {
		state, _ = ReadState()
		if state != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if state == nil || state.Transport != Transport || state.DaemonPID != os.Getpid() || len(state.Entries) != 1 {
		t.Fatal("listener state not ready")
	}
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("daemon did not stop")
	}
	if s, _ := ReadState(); s != nil {
		t.Fatal("stale listener state")
	}
	if pid, _ := ReadPID(); pid != 0 {
		t.Fatal("stale pid")
	}
}
func TestRunRejectsPortCollision(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KAMUI_CONFIG_DIR", dir)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	if err := Run(context.Background(), []Tunnel{{LocalPort: port}}, nil); err == nil {
		t.Fatal("reported success for occupied port")
	}
	if s, _ := ReadState(); s != nil {
		t.Fatal("advertised a failed listener")
	}
}
