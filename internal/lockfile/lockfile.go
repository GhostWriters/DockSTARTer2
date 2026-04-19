package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"DockSTARTer2/internal/process"
)

// Acquire writes a lock file at path containing the current PID.
// The caller must call Release when done.
func Acquire(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644)
}

// Release removes the lock file at path.
func Release(path string) {
	_ = os.Remove(path)
}

// IsLocked returns true if a valid, non-stale lock file exists at path.
func IsLocked(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid == 0 {
		return false
	}
	return process.Exists(pid)
}
