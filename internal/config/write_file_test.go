package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"

	"DockSTARTer2/internal/strutil"
)

// noTempFiles fails t if dir holds anything besides want.
func noTempFiles(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !slices.Contains(want, e.Name()) {
			t.Errorf("left behind %q", e.Name())
		}
	}
}

func TestWriteFileAtomicNewAndExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dockstarter2.toml")

	if err := writeFileAtomic(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "one" {
		t.Errorf("new file: got %q", got)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(path); info.Mode().Perm() != 0600 {
			t.Errorf("new file mode %v, want 0600", info.Mode().Perm())
		}
		if err := os.Chmod(path, 0640); err != nil {
			t.Fatal(err)
		}
	}

	if err := writeFileAtomic(path, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "two" {
		t.Errorf("existing file: got %q", got)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(path); info.Mode().Perm() != 0640 {
			t.Errorf("existing file mode %v, want 0640 kept", info.Mode().Perm())
		}
	}
	noTempFiles(t, dir, "dockstarter2.toml")
}

func TestWriteFileAtomicKeepsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.toml")
	link := filepath.Join(dir, "dockstarter2.toml")
	if err := os.WriteFile(real, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("can't create a symlink here: %v", err)
	}

	if err := writeFileAtomic(link, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced by a file")
	}
	if got, _ := os.ReadFile(real); string(got) != "new" {
		t.Errorf("target: got %q, want new", got)
	}
	noTempFiles(t, dir, "real.toml", "dockstarter2.toml")
}

// TestWriteFileAtomicDanglingSymlink checks a link to a file that doesn't
// exist yet creates that file and stays a link.
func TestWriteFileAtomicDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "real"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "dockstarter2.toml")
	if err := os.Symlink(filepath.Join("real", "dockstarter2.toml"), link); err != nil {
		t.Skipf("can't create a symlink here: %v", err)
	}

	if err := writeFileAtomic(link, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced by a file")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "real", "dockstarter2.toml")); string(got) != "new" {
		t.Errorf("target: got %q, want new", got)
	}
	noTempFiles(t, filepath.Join(dir, "real"), "dockstarter2.toml")
}

// TestWriteFileAtomicReadOnly checks a read-only file is refused and kept.
func TestWriteFileAtomicReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dockstarter2.toml")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })

	if err := writeFileAtomic(path, []byte("new")); !errors.Is(err, os.ErrPermission) {
		t.Errorf("got %v, want a permission error", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old" {
		t.Errorf("got %q, want old kept", got)
	}
	noTempFiles(t, dir, "dockstarter2.toml")
}

// TestWriteFileAtomicConcurrent checks writers saving at once each leave the
// file whole: it ends up holding exactly one writer's data.
func TestWriteFileAtomicConcurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dockstarter2.toml")
	const writers = 20
	datas := make([]string, writers)
	for i := range datas {
		datas[i] = fmt.Sprintf("writer %02d %s", i, strutil.Repeat("x", 64<<10))
	}

	var wg sync.WaitGroup
	for _, d := range datas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A rename over a file another writer is renaming can fail on
			// Windows; the file must still be whole.
			_ = writeFileAtomic(path, []byte(d))
		}()
	}
	wg.Wait()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(datas, string(got)) {
		t.Errorf("file holds %d bytes matching no single writer", len(got))
	}
	noTempFiles(t, dir, "dockstarter2.toml")
}
