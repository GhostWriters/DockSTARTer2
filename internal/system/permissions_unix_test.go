//go:build !windows

package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPermissionsMatch(t *testing.T) {
	uid := os.Getuid()
	gid := os.Getgid()

	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0775); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0664); err != nil {
		t.Fatal(err)
	}

	if !permissionsMatch(dir, uid, gid) {
		t.Error("expected match for freshly created tree with correct modes and current uid/gid")
	}

	if permissionsMatch(dir, uid+1, gid) {
		t.Error("expected mismatch when target uid differs from actual owner")
	}

	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if permissionsMatch(dir, uid, gid) {
		t.Error("expected mismatch when a file's mode differs from the expected 0664")
	}
}

func TestPermissionsMatchIgnoresSymlinks(t *testing.T) {
	uid := os.Getuid()
	gid := os.Getgid()

	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("x"), 0664); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported in this environment: %v", err)
	}

	if !permissionsMatch(dir, uid, gid) {
		t.Error("expected match: symlink's own (unusual) mode/ownership should not affect the result")
	}
}
