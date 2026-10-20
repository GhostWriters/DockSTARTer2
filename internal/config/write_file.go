package config

import (
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to path so that a crash or a concurrent save
// never leaves it half written: data goes to a uniquely named temporary file
// in the same folder, is flushed to disk, then renamed over path in one step.
// Each call gets its own temporary file, so instances saving at once can't
// write into the same one. A symlinked path is resolved first, so the link
// stays a link. An existing file's mode, and on Linux and macOS its owner,
// carry over to the new one; a new file is 0600.
func writeFileAtomic(path string, data []byte) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	mode := os.FileMode(0600)
	existing, statErr := os.Stat(target)
	if statErr == nil {
		mode = existing.Mode().Perm()
	}

	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	if statErr == nil {
		matchOwner(tmpPath, existing)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return err
	}
	renamed = true
	syncDir(dir)
	return nil
}
