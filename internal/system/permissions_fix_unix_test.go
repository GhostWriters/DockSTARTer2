//go:build !windows

package system

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestFixPermissionsChmodOnly(t *testing.T) {
	uid, gid := os.Getuid(), os.Getgid()

	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}

	if err := fixPermissions(dir, uid, gid, false, true, true); err != nil {
		t.Fatalf("fixPermissions(chmod only) failed: %v", err)
	}

	for _, tc := range []struct {
		path string
		want os.FileMode
	}{
		{dir, 0775},
		{sub, 0775},
		{file, 0664},
	} {
		info, err := os.Stat(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != tc.want {
			t.Errorf("%s mode = %o, want %o", tc.path, info.Mode().Perm(), tc.want)
		}
	}

	// doChown with the CURRENT uid:gid as target: every entry already
	// matches, so the skip-if-correct branch means os.Lchown is never
	// called and no privilege is needed.
	if err := fixPermissions(dir, uid, gid, true, false, true); err != nil {
		t.Fatalf("fixPermissions(chown to existing owner) should be a no-op, got: %v", err)
	}
}

func TestFixPermissionsNonRecursive(t *testing.T) {
	uid, gid := os.Getuid(), os.Getgid()

	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0700); err != nil {
		t.Fatal(err)
	}

	if err := fixPermissions(dir, uid, gid, false, true, false); err != nil {
		t.Fatalf("fixPermissions(non-recursive) failed: %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0775 {
		t.Errorf("dir mode = %o, want 0775", dirInfo.Mode().Perm())
	}
	subInfo, err := os.Stat(sub)
	if err != nil {
		t.Fatal(err)
	}
	if subInfo.Mode().Perm() != 0700 {
		t.Errorf("non-recursive fix must not touch children: sub mode = %o, want 0700", subInfo.Mode().Perm())
	}
}

func TestFixPermissionsSymlinkTargetUntouched(t *testing.T) {
	uid, gid := os.Getuid(), os.Getgid()

	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(outsideFile, 0600); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0775); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Skipf("symlinks not supported in this environment: %v", err)
	}

	if err := fixPermissions(dir, uid, gid, true, true, true); err != nil {
		t.Fatalf("fixPermissions failed: %v", err)
	}

	info, err := os.Stat(outsideFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("symlink target outside the tree was modified: mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestRunInternalFixPermissions(t *testing.T) {
	uid, gid := os.Getuid(), os.Getgid()

	if code := RunInternalFixPermissions([]string{"too", "few"}); code != 2 {
		t.Errorf("wrong arg count: exit = %d, want 2", code)
	}
	if code := RunInternalFixPermissions([]string{"notanint", "1000", "1", "1", "1", "/tmp/x"}); code != 2 {
		t.Errorf("bad puid: exit = %d, want 2", code)
	}
	if code := RunInternalFixPermissions([]string{"1000", "1000", "yes", "1", "1", "/tmp/x"}); code != 2 {
		t.Errorf("bad bool: exit = %d, want 2", code)
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	code := RunInternalFixPermissions([]string{strconv.Itoa(uid), strconv.Itoa(gid), "0", "1", "0", dir})
	if code != 0 {
		t.Fatalf("valid chmod-only fix: exit = %d, want 0", code)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0775 {
		t.Errorf("dir mode = %o, want 0775", info.Mode().Perm())
	}

	if code := RunInternalFixPermissions([]string{strconv.Itoa(uid), strconv.Itoa(gid), "0", "1", "0", filepath.Join(dir, "nonexistent")}); code != 1 {
		t.Errorf("nonexistent path: exit = %d, want 1", code)
	}
}
