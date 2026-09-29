//go:build !windows

package update

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteStagedFailsOnShortWrite(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "ds2"))
	if err != nil {
		t.Fatal(err)
	}
	// The archive promised 10 bytes; only 3 arrive, as when a download drops.
	if err := writeStaged(f, strings.NewReader("abc"), 10); err == nil {
		t.Fatal("writeStaged accepted a short write")
	}
}

func TestWriteStagedWritesExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ds2")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeStaged(f, strings.NewReader("binary"), 6); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 6 || info.Mode().Perm() != 0755 {
		t.Errorf("staged file: size %d mode %v, want 6 and 0755", info.Size(), info.Mode().Perm())
	}
}

func TestCheckSpace(t *testing.T) {
	dir := t.TempDir()
	if err := checkSpace(dir, 1); err != nil {
		t.Errorf("checkSpace for 1 byte: %v", err)
	}
	if err := checkSpace(dir, math.MaxInt64/2); err == nil {
		t.Error("checkSpace allowed more than any disk holds")
	}
}
