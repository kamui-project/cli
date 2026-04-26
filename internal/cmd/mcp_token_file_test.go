package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteTokenFile_CreatesParentAndSetsMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "pat")

	if err := writeTokenFile(path, "secret-pat"); err != nil {
		t.Fatalf("writeTokenFile: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "secret-pat\n" {
		t.Errorf("contents = %q, want %q", string(b), "secret-pat\n")
	}
	assertMode0600(t, path)
}

func TestWriteTokenFile_RefusesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "innocent")
	if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "pat")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := writeTokenFile(link, "secret"); err == nil {
		t.Fatal("expected writeTokenFile to refuse symlink destination")
	}
	if b, _ := os.ReadFile(target); string(b) != "untouched" {
		t.Errorf("symlink target was modified: %q", string(b))
	}
}

func TestReadTokenFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pat")
	if err := os.WriteFile(path, []byte("  secret-pat\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readTokenFile(path)
	if err != nil {
		t.Fatalf("readTokenFile: %v", err)
	}
	if got != "secret-pat" {
		t.Errorf("got %q, want %q", got, "secret-pat")
	}
}

func TestReadTokenFile_RejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pat")
	junk := strings.Repeat("x", maxTokenFileSize+1)
	if err := os.WriteFile(path, []byte(junk), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTokenFile(path); err == nil {
		t.Fatal("expected oversized file to be rejected")
	}
}

func TestReadTokenFile_RejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.WriteFile(real, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readTokenFile(link); err == nil {
		t.Fatal("expected symlink to be rejected")
	}
}

func TestReadTokenFile_EmptyFileErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pat")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTokenFile(path); err == nil {
		t.Fatal("expected empty file to be rejected")
	}
}

func TestReadTokenFile_PermissiveModeWarnsButReads(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix mode bits not meaningful on windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "pat")
	if err := os.WriteFile(path, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stderr, restore := captureStderr(t)
	defer restore()

	got, err := readTokenFile(path)
	if err != nil {
		t.Fatalf("readTokenFile: %v", err)
	}
	if got != "secret" {
		t.Errorf("got %q, want %q", got, "secret")
	}

	out := stderr()
	if !strings.Contains(out, "readable by other users") {
		t.Errorf("expected stderr warning, got %q", out)
	}
}

// captureStderr swaps os.Stderr for a pipe and returns a reader and a
// restore function. Tests that need to assert on warnings written to
// stderr by helpers like readTokenFile use this.
func captureStderr(t *testing.T) (read func() string, restore func()) {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		var got strings.Builder
		for {
			n, err := r.Read(buf)
			if n > 0 {
				got.Write(buf[:n])
			}
			if err != nil {
				done <- got.String()
				return
			}
		}
	}()

	read = func() string {
		_ = w.Close()
		s := <-done
		_ = r.Close()
		return s
	}
	restore = func() {
		os.Stderr = orig
	}
	return
}
