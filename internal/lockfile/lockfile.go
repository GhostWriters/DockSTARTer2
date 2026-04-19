package lockfile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// Lock represents an advisory file lock.
type Lock struct {
	f    *flock.Flock
	path string
}

// AcquireShared acquires a shared (read) lock on the file at path.
// Multiple processes can hold a shared lock simultaneously.
// The caller must call Release when done.
func AcquireShared(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f := flock.New(path)
	locked, err := f.TryRLock() // Shared lock
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, fmt.Errorf("failed to acquire shared lock (exclusive lock held by another process)")
	}
	// Note: We don't write the PID here because multiple processes might share this lock.
	// If you need PIDs, you'd need a different mechanism (like a lock directory).
	return &Lock{f: f, path: path}, nil
}

// AcquireExclusive acquires an exclusive (write) lock on the file at path.
// Only one process can hold an exclusive lock at a time.
// The caller must call Release when done.
func AcquireExclusive(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f := flock.New(path)
	locked, err := f.TryLock() // Exclusive lock
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, fmt.Errorf("file is already locked")
	}
	// We can write the PID for exclusive locks if desired.
	_ = os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644)
	return &Lock{f: f, path: path}, nil
}

// Release releases the lock and optionally removes the file if it was exclusive.
func (l *Lock) Release() {
	if l.f != nil {
		_ = l.f.Unlock()
	}
}

// IsLocked reports whether any lock (shared or exclusive) is held on the file at path.
func IsLocked(path string) bool {
	f := flock.New(path)
	// To check if ANY lock is held, we try to take an Exclusive lock.
	// If it fails, someone else has a lock (shared or exclusive).
	locked, err := f.TryLock()
	if err != nil {
		// If the file or its parent directory doesn't exist, it's definitely not locked.
		if os.IsNotExist(err) || (err != nil && (strings.Contains(err.Error(), "no such file or directory") || strings.Contains(err.Error(), "The system cannot find the path specified"))) {
			return false
		}
		// Any other error (like Permission Denied on an active file) likely means it's locked.
		return true
	}
	if locked {
		_ = f.Unlock()
		return false
	}
	return true
}

// Deprecated logic: Compatibility wrappers that don't hold the lock properly.
// These are kept to avoid immediate compilation errors in old callers,
// but should be replaced by the Lock-returning methods.

func Acquire(path string) error {
	_, err := AcquireShared(path)
	return err
}

func Release(path string) {
	// This legacy method cannot release a flock held by a previous call to Acquire
	// because the flock object is lost. Callers MUST move to the new API.
	_ = os.Remove(path)
}
