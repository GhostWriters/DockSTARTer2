//go:build !windows

package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckPermissions(t *testing.T) {
	uid := os.Getuid()
	gid := os.Getgid()

	dir := t.TempDir()
	// os.Mkdir/os.WriteFile apply the process umask on top of the requested
	// mode, so the on-disk bits aren't guaranteed to match what was passed
	// in -- chmod explicitly afterward to pin the exact bits regardless of
	// umask (this is what caused this test to pass locally but fail in CI,
	// where the runner's default umask strips the group-write bit).
	if err := os.Chmod(dir, 0775); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0775); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0664); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0664); err != nil {
		t.Fatal(err)
	}

	if chown, chmod := checkPermissions(dir, uid, gid, true); chown || chmod {
		t.Errorf("expected no fix needed for freshly created tree with correct modes and current uid/gid, got chown=%v chmod=%v", chown, chmod)
	}

	if chown, chmod := checkPermissions(dir, uid+1, gid, true); !chown || chmod {
		t.Errorf("expected only chown needed when target uid differs from actual owner (mode still fine), got chown=%v chmod=%v", chown, chmod)
	}

	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if chown, chmod := checkPermissions(dir, uid, gid, true); chown || !chmod {
		t.Errorf("expected only chmod needed when a file's mode differs from the expected 0664 (ownership still fine), got chown=%v chmod=%v", chown, chmod)
	}

	if chown, chmod := checkPermissions(dir, uid+1, gid, true); !chown || !chmod {
		t.Errorf("expected both needed when both ownership and mode are wrong, got chown=%v chmod=%v", chown, chmod)
	}
}

func TestCheckPermissionsIgnoresSymlinks(t *testing.T) {
	uid := os.Getuid()
	gid := os.Getgid()

	dir := t.TempDir()
	if err := os.Chmod(dir, 0775); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("x"), 0664); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0664); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported in this environment: %v", err)
	}

	if chown, chmod := checkPermissions(dir, uid, gid, true); chown || chmod {
		t.Errorf("expected no fix needed: symlink's own (unusual) mode/ownership should not affect the result, got chown=%v chmod=%v", chown, chmod)
	}
}

func TestCheckPermissionsNonRecursive(t *testing.T) {
	uid := os.Getuid()
	gid := os.Getgid()

	dir := t.TempDir()
	if err := os.Chmod(dir, 0775); err != nil {
		t.Fatal(err)
	}
	// A mismatched child must NOT affect the non-recursive result -- only
	// dir's own ownership/mode matter.
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}

	if chown, chmod := checkPermissions(dir, uid, gid, false); chown || chmod {
		t.Errorf("expected no fix needed: non-recursive check should ignore mismatched children, got chown=%v chmod=%v", chown, chmod)
	}

	if chown, chmod := checkPermissions(dir, uid+1, gid, false); !chown || chmod {
		t.Errorf("expected only chown needed when target uid differs from dir's actual owner, got chown=%v chmod=%v", chown, chmod)
	}

	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if chown, chmod := checkPermissions(dir, uid, gid, false); chown || !chmod {
		t.Errorf("expected only chmod needed when dir's own mode differs from the expected 0775, got chown=%v chmod=%v", chown, chmod)
	}
}
